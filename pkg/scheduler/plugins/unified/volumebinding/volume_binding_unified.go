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
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	"k8s.io/klog/v2"
	scheduledconfigv1beta2config "k8s.io/kube-scheduler/config/v1beta2"
	"k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/kubernetes/pkg/features"
	scheduledconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/v1beta2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
)

func getDefaultVolumeBindingArgs() (*scheduledconfig.VolumeBindingArgs, error) {
	var v1beta2args scheduledconfigv1beta2config.VolumeBindingArgs
	v1beta2.SetDefaults_VolumeBindingArgs(&v1beta2args)
	var volumeBindingArgs scheduledconfig.VolumeBindingArgs
	err := v1beta2.Convert_v1beta2_VolumeBindingArgs_To_config_VolumeBindingArgs(&v1beta2args, &volumeBindingArgs, nil)
	if err != nil {
		return nil, err
	}
	return &volumeBindingArgs, nil
}

func forceSyncStorageInformers(informerFactory informers.SharedInformerFactory) {
	storageClassInformer := informerFactory.Storage().V1().StorageClasses()
	csiNodeInformer := informerFactory.Storage().V1().CSINodes()
	csiDriverInformer := informerFactory.Storage().V1().CSIDrivers()

	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), informerFactory, storageClassInformer.Informer(), &cache.ResourceEventHandlerFuncs{})
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), informerFactory, csiNodeInformer.Informer(), &cache.ResourceEventHandlerFuncs{})
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), informerFactory, csiDriverInformer.Informer(), &cache.ResourceEventHandlerFuncs{})
	if utilfeature.DefaultFeatureGate.Enabled(features.CSIStorageCapacity) {
		csiStorageCapacityInformer := informerFactory.Storage().V1beta1().CSIStorageCapacities()
		frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), informerFactory, csiStorageCapacityInformer.Informer(), &cache.ResourceEventHandlerFuncs{})
	}
}

func (pl *VolumeBinding) filterWithoutPVC(state *stateData, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	nodeStorageInfo := pl.nodeStorageCache.GetNodeStorageInfo(nodeInfo.Node().Name)
	if nodeStorageInfo != nil {
		nodeStorageInfo.lock.Lock()
		defer nodeStorageInfo.lock.Unlock()
		return HasEnoughStorageCapacity(nodeStorageInfo, pod, 0, nil, pl.classLister)
	}
	return nil
}

func (pl *VolumeBinding) assumeEphemeralStorage(pod *v1.Pod, nodeName string) {
	requests, _ := resource.PodRequestsAndLimits(pod)
	ephemeralStorageSize := requests[v1.ResourceEphemeralStorage]
	localInlineVolumeSize := unified.CalcLocalInlineVolumeSize(pod.Spec.Volumes, pl.classLister)
	if ephemeralStorageSize.IsZero() && localInlineVolumeSize == 0 {
		return
	}
	pl.nodeStorageCache.UpdateOnNode(nodeName, func(nodeStorageInfo *NodeStorageInfo) {
		nodeStorageInfo.AddLocalVolumeAlloc(pod, ephemeralStorageSize.Value(), localInlineVolumeSize)
	})
}

func (pl *VolumeBinding) revertAssumedEphemeralStorage(pod *v1.Pod, nodeName string) {
	requests, _ := resource.PodRequestsAndLimits(pod)
	ephemeralStorageSize := requests[v1.ResourceEphemeralStorage]
	localInlineVolumeSize := unified.CalcLocalInlineVolumeSize(pod.Spec.Volumes, pl.classLister)
	if ephemeralStorageSize.IsZero() && localInlineVolumeSize == 0 {
		return
	}
	pl.nodeStorageCache.UpdateOnNode(nodeName, func(nodeStorageInfo *NodeStorageInfo) {
		nodeStorageInfo.DeleteLocalVolumeAlloc(pod, ephemeralStorageSize.Value(), localInlineVolumeSize)
	})
}

func (pl *VolumeBinding) replaceStorageClassNameIfNeeded(podVolumes *PodVolumes, pod *v1.Pod, nodeName string) {
	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return
	}
	node := nodeInfo.Node()
	if node == nil {
		return
	}
	if !unified.IsVirtualKubeletNode(node) {
		return
	}

	for _, binding := range podVolumes.StaticBindings {
		binding.pvc = ReplaceStorageClassIfVirtualKubelet(nodeInfo.Node(), pod, binding.pvc, pl.classLister)
	}

	for i, claim := range podVolumes.DynamicProvisions {
		podVolumes.DynamicProvisions[i] = ReplaceStorageClassIfVirtualKubelet(nodeInfo.Node(), pod, claim, pl.classLister)
	}
	return
}

func (pl *VolumeBinding) assumeLocalPVCAllocs(podVolumes *PodVolumes, nodeName string, pod *v1.Pod) error {
	nodeStorageInfo := pl.nodeStorageCache.GetNodeStorageInfo(nodeName)
	if nodeStorageInfo == nil {
		return fmt.Errorf("cannot find nodeStorageInfo")
	}

	var localPVCAllocs []*localPVCAlloc
	for _, bindingInfo := range podVolumes.StaticBindings {
		if bindingInfo.bindPoint != nil && bindingInfo.pvc != nil {
			className := storagehelpers.GetPersistentVolumeClaimClass(bindingInfo.pvc)
			var ioLimit *unified.LocalStorageIOLimitInfo
			var err error
			if unified.SupportLVMOrQuotaPathOrDevice(pl.classLister, className, nil) {
				ioLimit, err = GetPVCIOLimitInfo(bindingInfo.pvc)
				if err != nil {
					klog.Errorf("get pvc %s iolimit failed: %v", bindingInfo.pvc.Name, err)
					return err
				}
			}

			localPVCAlloc := &localPVCAlloc{
				Namespace:   bindingInfo.pvc.Namespace,
				Name:        bindingInfo.pvc.Name,
				RequestSize: bindingInfo.bindPoint.RequestSize,
				MountPoint:  bindingInfo.bindPoint.MountPoint,
				IOLimit:     ioLimit,
			}
			localPVCAllocs = append(localPVCAllocs, localPVCAlloc)
		}
	}

	for _, claimToProvision := range podVolumes.DynamicProvisions {
		className := storagehelpers.GetPersistentVolumeClaimClass(claimToProvision)
		if unified.SupportLocalPV(pl.classLister, className, nil) {
			quantity := claimToProvision.Spec.Resources.Requests[v1.ResourceStorage]
			localPVCAlloc := &localPVCAlloc{
				Namespace:   claimToProvision.Namespace,
				Name:        claimToProvision.Name,
				RequestSize: quantity.Value(),
				MountPoint:  nodeStorageInfo.GraphDiskPath,
			}
			localPVCAllocs = append(localPVCAllocs, localPVCAlloc)
		}

		if unified.SupportLVMOrQuotaPathOrDevice(pl.classLister, className, nil) {
			class, err := pl.classLister.Get(className)
			if err != nil {
				return err
			}
			quantity := claimToProvision.Spec.Resources.Requests[v1.ResourceStorage]
			ioLimit, err := GetPVCIOLimitInfo(claimToProvision)
			if err != nil {
				klog.Errorf("get pvc %s iolimit failed: %v", claimToProvision.Name, err)
				return err
			}
			localPVCAlloc := &localPVCAlloc{
				Namespace:   claimToProvision.Namespace,
				Name:        claimToProvision.Name,
				RequestSize: quantity.Value(),
				IOLimit:     ioLimit,
			}

			csiType := class.Parameters[unified.CSIVolumeType]
			switch csiType {
			case unified.CSILocalVolumeLVM:
				localPVCAlloc.MountPoint = class.Parameters[unified.CSILVMVolumeGroupName]
			case unified.CSILocalVolumeQuotaPath:
				localPVCAlloc.MountPoint = class.Parameters[unified.CSIQuotaPathRootPath]
			case unified.CSILocalVolumeDevice:

			default:
				return fmt.Errorf("unknow type %s of local volume in StorageClass %s", csiType, className)
			}
			localPVCAllocs = append(localPVCAllocs, localPVCAlloc)
		}
	}

	for _, pvcAlloc := range localPVCAllocs {
		nodeStorageInfo.AddLocalPVCAlloc(pvcAlloc)
	}
	podVolumes.AssumedLocalPVCAllocs = localPVCAllocs
	return nil
}

func (pl *VolumeBinding) revertLocalPVCAllocs(nodeName string, podVolumes *PodVolumes, needLockNode bool) {
	if len(podVolumes.AssumedLocalPVCAllocs) > 0 && nodeName != "" {
		if needLockNode {
			pl.nodeStorageCache.UpdateOnNode(nodeName, func(nodeStorageInfo *NodeStorageInfo) {
				for _, pvcAlloc := range podVolumes.AssumedLocalPVCAllocs {
					nodeStorageInfo.DeleteLocalPVCAlloc(pvcAlloc)
				}
			})
		} else {
			nodeInfo := pl.nodeStorageCache.GetNodeStorageInfo(nodeName)
			if nodeInfo != nil {
				for _, pvcAlloc := range podVolumes.AssumedLocalPVCAllocs {
					nodeInfo.DeleteLocalPVCAlloc(pvcAlloc)
				}
			}
		}
	}
}
