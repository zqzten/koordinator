/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package volumebinding

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-helpers/scheduling/corev1"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	"k8s.io/klog/v2"
	pvutil "k8s.io/kubernetes/pkg/controller/volume/persistentvolume/util"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

// BindPoint is a local disk mount point info for pvc.
// MountPoint: the decision node mount point
// Request   : the request size
type BindPoint struct {
	MountPoint  string
	RequestSize int64
}

func (b *volumeBinder) recordExternalProvisioningEvent(pod *v1.Pod, claimsToProvision []*v1.PersistentVolumeClaim) {
	var claimNames []string
	for _, claim := range claimsToProvision {
		claimNames = append(claimNames, getPVCName(claim))
	}
	if len(claimNames) > 0 && len(claimsToProvision) > 0 {
		nodeName := claimsToProvision[0].Annotations[pvutil.AnnSelectedNode]
		b.recordEvent(pod, v1.EventTypeNormal, "ExternalProvisioning", "Binding",
			"Waiting for volumes to be created with PVC(s) %s on node %s", strings.Join(claimNames, ","), nodeName)
	}
}

func (b *volumeBinder) recordEvent(pod *v1.Pod, eventtype, reason, action, note string, args ...interface{}) {
	if b.eventRecorder == nil {
		return
	}

	if pod != nil {
		b.eventRecorder.Eventf(pod, nil, eventtype, reason, action, note, args...)
	}
}

func checkDiskAffinity(pv *v1.PersistentVolume, nodeLabels map[string]string) error {
	nodeSelector, ok := pv.Annotations[unified.AnnotationCSIVolumeTopology]
	if !ok {
		return nil
	}

	if !nodeHasCloudStorage(nodeLabels) {
		return nil
	}

	var terms v1.NodeSelector
	if err := json.Unmarshal([]byte(nodeSelector), &terms); err != nil {
		return err
	}

	klog.V(10).Infof("Match for Required node selector terms %+v", nodeSelector)
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: nodeLabels}}
	if matches, err := corev1.MatchNodeSelectorTerms(node, &terms); err != nil {
		return err
	} else if !matches {
		return errors.New(string(ErrReasonNodeConflict))
	}

	return nil
}

func (b *volumeBinder) checkNodeDeviceStorageClass(claim *v1.PersistentVolumeClaim, pod *v1.Pod, node *v1.Node, bps map[string][]*BindPoint) (match bool, shouldPassFilter bool, suggestedBP *BindPoint) {
	requestedClass := storagehelpers.GetPersistentVolumeClaimClass(claim)
	storageClass, err := b.classLister.Get(requestedClass)
	if err != nil {
		klog.Errorf("Failed to get StorageClass %v in cache, error: %v", requestedClass, err)
		return false, false, nil
	}
	if storageClass.Parameters[unified.CSIVolumeType] != unified.CSILocalVolumeDevice {
		return false, false, nil
	}

	var nodeStorageInfo *NodeStorageInfo
	b.nodeStorageCache.UpdateOnNode(node.Name, func(info *NodeStorageInfo) {
		nodeStorageInfo = info
	})
	if nodeStorageInfo == nil {
		klog.Errorf("Failed to find nodeStorageInfo, node: %s", node.Name)
		return false, false, nil
	}
	nodeStorageInfo.lock.Lock()
	defer nodeStorageInfo.lock.Unlock()

	freeLocalVolumes := make(map[string]int64)
	localVolumeFree := nodeStorageInfo.GetFree()
	for hostPath, freeSize := range localVolumeFree.VolumeSize {
		freeLocalVolumes[hostPath] = freeSize
	}
	for _, localVolume := range nodeStorageInfo.LocalVolumesInfo {
		if _, ok := bps[localVolume.HostPath]; ok {
			delete(freeLocalVolumes, localVolume.HostPath)
			continue
		}
		if localVolume.Labels[unified.LocalStorageType] != unified.NodeLocalVolumeDevice {
			delete(freeLocalVolumes, localVolume.HostPath)
			continue
		}
		if freeLocalVolumes[localVolume.HostPath] != localVolume.Capacity {
			delete(freeLocalVolumes, localVolume.HostPath)
		}
	}
	if len(freeLocalVolumes) == 0 {
		return false, true, nil
	}

	mountPoints := make([]string, 0, len(freeLocalVolumes))
	for k := range freeLocalVolumes {
		mountPoints = append(mountPoints, k)
	}

	storageResource := claim.Spec.Resources.Requests[v1.ResourceStorage]
	requestDiskSize := storageResource.Value()
	mountPoint, found := getBestFitVolumeMountPoint(freeLocalVolumes, mountPoints, requestDiskSize, bps)
	if found {
		return true, true, &BindPoint{
			MountPoint:  mountPoint,
			RequestSize: requestDiskSize,
		}
	}

	return false, true, nil
}

func getBestFitVolumeMountPoint(leftVolumes map[string]int64, paths []string, reqSize int64, bps map[string][]*BindPoint) (mountPoint string, found bool) {
	left := int64(-1)
	retPath := ""
	found = false
	// Calculate remained volume size for a given mount path
	for _, path := range paths {
		if remain, ok := leftVolumes[path]; ok {
			if vols, ok := bps[path]; ok {
				for _, vol := range vols {
					remain -= vol.RequestSize
				}
			}
			// best fit
			if remain >= reqSize {
				if left == int64(-1) {
					left = remain - reqSize
					retPath = path
					found = true
				} else if left > remain-reqSize {
					left = remain - reqSize
					retPath = path
					found = true
				}
			}
		}
	}
	return retPath, found
}

func (b *volumeBinder) addRequestedIoLimitFromPVC(
	claim *v1.PersistentVolumeClaim,
	class *storagev1.StorageClass,
	requestLocalVolumeIOLimit map[string]*unified.LocalStorageIOLimitInfo,
) (map[string]*unified.LocalStorageIOLimitInfo, error) {
	className := class.Name
	if unified.SupportLVMOrQuotaPathOrDevice(b.classLister, className, class) {
		ioLimit, err := GetPVCIOLimitInfo(claim)
		if err != nil {
			klog.Errorf("get pvc %s iolimit failed: %v", claim.Name, err)
			return nil, err
		}
		csiType, ok := class.Parameters[unified.CSIVolumeType]
		if !ok {
			klog.V(5).Infof("There isn't a key named %s in StorageClass %s", unified.CSIVolumeType, className)
			return nil, fmt.Errorf("storagelass %s doesn't have key %s", className, unified.CSIVolumeType)
		}
		switch csiType {
		case unified.CSILocalVolumeLVM:
			vgName := class.Parameters[unified.CSILVMVolumeGroupName]
			if _, ok := requestLocalVolumeIOLimit[vgName]; !ok {
				requestLocalVolumeIOLimit[vgName] = &unified.LocalStorageIOLimitInfo{}
			}
			requestLocalVolumeIOLimit[vgName].Add(ioLimit)
		case unified.CSILocalVolumeQuotaPath:
			rootPath := class.Parameters[unified.CSIQuotaPathRootPath]
			if _, ok := requestLocalVolumeIOLimit[rootPath]; !ok {
				requestLocalVolumeIOLimit[rootPath] = &unified.LocalStorageIOLimitInfo{}
			}
			requestLocalVolumeIOLimit[rootPath].Add(ioLimit)
		case unified.CSILocalVolumeDevice:
		case unified.CSILocalLoopDevice:
			rootPath := class.Parameters[unified.CSILoopDeviceRootPath]
			if _, ok := requestLocalVolumeIOLimit[rootPath]; !ok {
				requestLocalVolumeIOLimit[rootPath] = &unified.LocalStorageIOLimitInfo{}
			}
			requestLocalVolumeIOLimit[rootPath].Add(ioLimit)
		default:
			klog.V(5).Infof("Unknown type %s of local volume in StorageClass %s", csiType, className)
			return nil, fmt.Errorf("unknown type %s of local volume in StorageClass %s", csiType, className)
		}
	}
	return requestLocalVolumeIOLimit, nil
}

func (b *volumeBinder) matchTopologyStorageClassNode(class *storagev1.StorageClass, claim *v1.PersistentVolumeClaim, nodeLabels map[string]string) (bool, error) {
	// https://yuque.antfin.com/docs/share/17619f80-7bf2-466a-ab21-1fd6b7f13884?#

	// Skip if there isn't any disk label on node.
	if !nodeHasCloudStorage(nodeLabels) {
		return true, nil
	}

	var diskTypes []string
	if class.Provisioner == unified.ACKDiskPlugin {
		// 这里需要显式区分一下，之前设计的协议不好，如果多个CSI插件混用，其中有一个往node上写了下面的label，
		// 而另一个插件的StorageClass里也用了名叫type的parameter，就会莫名其妙的调度失败
		// Skip if there isn't any disk label on StorageClass
		if class.Parameters[unified.ACKCloudStorageType] == "" {
			return true, nil
		}
		// check the topology between node and StorageClass.
		diskTypes = strings.Split(class.Parameters[unified.ACKCloudStorageType], ",")
	} else {
		driver, err := b.csiDriverLister.Get(class.Provisioner)
		if err != nil {
			return false, fmt.Errorf("get CSIDriver %s: %w", class.Provisioner, err)
		}

		if t, _ := strconv.ParseBool(driver.Annotations[unified.AnnotationUltronDiskUseCapabilities]); !t {
			return true, nil
		}
		// https://yuque.antfin.com/docs/share/cabe7723-e1f0-4326-9493-87f3ab52b116?#
		category := claim.Annotations[unified.UltronDiskCategory]
		if category == "" {
			category = class.Parameters[unified.UltronDiskCategory]
		}
		diskTypes = []string{category}
	}
	for _, diskType := range diskTypes {
		key := unified.LabelNodeDiskPrefix + diskType
		if _, ok := nodeLabels[key]; ok {
			return true, nil
		}
	}
	return false, nil

}

func nodeHasCloudStorage(nodeLabels map[string]string) bool {
	has := false
	for k := range nodeLabels {
		if strings.HasPrefix(k, unified.LabelNodeDiskPrefix) {
			has = true
			break
		}
	}
	return has
}

func (b *volumeBinder) checkLocalVolumeStorageAndIOCapacity(nodeName string, pod *v1.Pod, requestSystemDiskInBytes int64, localVolumesInBytes map[string]int64, requestLocalVolumeIOLimit map[string]*unified.LocalStorageIOLimitInfo) (sufficientStorage, sufficientIOLimit bool, err error) {
	// TODO(joseph.lt): lock the node when the Pod requests local volumes PV
	nodeStorageInfo := b.nodeStorageCache.GetNodeStorageInfo(nodeName)
	if nodeStorageInfo == nil {
		return false, false, fmt.Errorf("nodeSotrageInfo not found")
	}
	nodeStorageInfo.lock.Lock()
	defer nodeStorageInfo.lock.Unlock()

	status := HasEnoughStorageCapacity(nodeStorageInfo, pod, requestSystemDiskInBytes, localVolumesInBytes, b.classLister)
	if !status.IsSuccess() {
		return false, true, nil
	}
	status = HasEnoughIOCapacity(nodeStorageInfo, pod, requestLocalVolumeIOLimit)
	if !status.IsSuccess() {
		return true, false, nil
	}
	return true, true, nil
}
