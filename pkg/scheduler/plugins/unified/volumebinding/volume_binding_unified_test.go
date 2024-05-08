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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/eci"
	koordutilfeature "github.com/koordinator-sh/koordinator/pkg/util/feature"
)

var _ framework.SharedLister = &testSharedLister{}

type testSharedLister struct {
	nodes       []*v1.Node
	nodeInfos   []*framework.NodeInfo
	nodeInfoMap map[string]*framework.NodeInfo
}

func newTestSharedLister(pods []*v1.Pod, nodes []*v1.Node) *testSharedLister {
	nodeInfoMap := make(map[string]*framework.NodeInfo)
	nodeInfos := make([]*framework.NodeInfo, 0)
	for _, pod := range pods {
		nodeName := pod.Spec.NodeName
		if _, ok := nodeInfoMap[nodeName]; !ok {
			nodeInfoMap[nodeName] = framework.NewNodeInfo()
		}
		nodeInfoMap[nodeName].AddPod(pod)
	}
	for _, node := range nodes {
		if _, ok := nodeInfoMap[node.Name]; !ok {
			nodeInfoMap[node.Name] = framework.NewNodeInfo()
		}
		nodeInfoMap[node.Name].SetNode(node)
	}

	for _, v := range nodeInfoMap {
		nodeInfos = append(nodeInfos, v)
	}

	return &testSharedLister{
		nodes:       nodes,
		nodeInfos:   nodeInfos,
		nodeInfoMap: nodeInfoMap,
	}
}

func (f *testSharedLister) StorageInfos() framework.StorageInfoLister {
	return f
}

func (f *testSharedLister) IsPVCUsedByPods(key string) bool {
	return false
}

func (f *testSharedLister) NodeInfos() framework.NodeInfoLister {
	return f
}

func (f *testSharedLister) List() ([]*framework.NodeInfo, error) {
	return f.nodeInfos, nil
}

func (f *testSharedLister) HavePodsWithAffinityList() ([]*framework.NodeInfo, error) {
	return nil, nil
}

func (f *testSharedLister) HavePodsWithRequiredAntiAffinityList() ([]*framework.NodeInfo, error) {
	return nil, nil
}

func (f *testSharedLister) Get(nodeName string) (*framework.NodeInfo, error) {
	return f.nodeInfoMap[nodeName], nil
}

var (
	localPVSC = &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "local-pv-sc",
			Annotations: map[string]string{
				unified.AnnotationStorageSource: unified.LocalPVSourceIdentity,
			},
		},
		VolumeBindingMode: &waitForFirstConsumer,
		Provisioner:       "local-pv-csi-provisioner",
	}
	lvmSC = &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lvm-sc",
		},
		VolumeBindingMode: &waitForFirstConsumer,
		Provisioner:       unified.LocalPVCSIProvisioner,
		Parameters: map[string]string{
			unified.CSIVolumeType:         unified.CSILocalVolumeLVM,
			unified.CSILVMVolumeGroupName: "vg-1",
		},
	}
	quotaPathSC = &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "quotapath-sc",
		},
		VolumeBindingMode: &waitForFirstConsumer,
		Provisioner:       unified.LocalPVCSIProvisioner,
		Parameters: map[string]string{
			unified.CSIVolumeType:        unified.CSILocalVolumeQuotaPath,
			unified.CSIQuotaPathRootPath: "/path1",
		},
	}
	cloudSC1 = &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cloud-storage-sc1",
		},
		VolumeBindingMode: &waitForFirstConsumer,
		Provisioner:       unified.ACKDiskPlugin,
		Parameters: map[string]string{
			unified.ACKCloudStorageType: "cloud_ssd,cloud_essd",
		},
	}
	cloudSC2 = &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cloud-storage-sc2",
		},
		VolumeBindingMode: &waitForFirstConsumer,
		Provisioner:       unified.ACKDiskPlugin,
	}
	otherSC = &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "other-sc",
		},
		VolumeBindingMode: &waitForFirstConsumer,
		Provisioner:       "unrelated_plugin",
		Parameters: map[string]string{
			unified.ACKCloudStorageType: "some",
		},
	}
	ultronDiskSC = &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "csi-ultron-disk",
		},
		VolumeBindingMode: &waitForFirstConsumer,
		Provisioner:       "com.alibaba-inc.ack-ee.csi-ultron",
		Parameters: map[string]string{
			unified.UltronDiskCategory: "cloud_essd",
		},
	}
	lvmIOLimitSC = &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lvm-iolimit-sc",
			Annotations: map[string]string{
				unified.AnnotationStorageClassSupportLVMAndQuotaPath: "true",
			},
		},
		VolumeBindingMode: &waitForFirstConsumer,
		Provisioner:       unified.LocalPVCAntRawBlockProvisioner,
		Parameters: map[string]string{
			unified.CSIVolumeType:         unified.CSILocalVolumeLVM,
			unified.CSILVMVolumeGroupName: "/home/t4",
		},
	}
	quotaPathIOLimitSC = &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "quotapath-iolimit-sc",
			Annotations: map[string]string{
				unified.AnnotationStorageClassSupportLVMAndQuotaPath: "true",
			},
		},
		VolumeBindingMode: &waitForFirstConsumer,
		Provisioner:       unified.LocalPVCAntRawBlockProvisioner,
		Parameters: map[string]string{
			unified.CSIVolumeType:        unified.CSILocalVolumeQuotaPath,
			unified.CSIQuotaPathRootPath: "/home/t4",
		},
	}
	multiVolumeLocalPVSC = &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "multi-volume-local-pv-sc",
		},
		VolumeBindingMode: &waitForFirstConsumer,
		Provisioner:       "local-pv-csi-provisioner",
	}
)

func makePVCWithIOLimit(pvc *v1.PersistentVolumeClaim, readIOps, writeIOps, readBps, writeBps int64) *v1.PersistentVolumeClaim {
	if pvc.Annotations == nil {
		pvc.Annotations = make(map[string]string)
	}
	pvc.Annotations[unified.IOLimitAnnotationOnPVC] = fmt.Sprintf("{\"readBps\":%d, \"writeBps\":%d, \"readIops\":%d, \"writeIops\":%d}", readBps, writeBps, readIOps, writeIOps)
	return pvc
}

func makePVCWithStorageSize(name, version, boundPVName, storageClassName, storageSize string) *v1.PersistentVolumeClaim {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       v1.NamespaceDefault,
			ResourceVersion: version,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: pointer.StringPtr(storageClassName),
		},
	}
	if boundPVName != "" {
		pvc.Spec.VolumeName = boundPVName
		metav1.SetMetaDataAnnotation(&pvc.ObjectMeta, storagehelpers.AnnBindCompleted, "true")
	}
	if storageSize != "" {
		pvc.Spec.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceStorage: resource.MustParse(storageSize),
			},
		}
	}
	return pvc
}

func makePodWithInlineVolumeForBindingTest(name string, storageClassName string, size string, pvcNames ...string) *v1.Pod {
	p := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: v1.NamespaceDefault,
		},
	}
	p.Spec.Volumes = append(p.Spec.Volumes, v1.Volume{
		VolumeSource: v1.VolumeSource{
			CSI: &v1.CSIVolumeSource{
				VolumeAttributes: map[string]string{
					unified.InlineVolumeAttributeStorageClass: storageClassName,
					unified.InlineVolumeAttributeStorageSize:  size,
				},
			},
		},
	})
	for _, pvcName := range pvcNames {
		p.Spec.Volumes = append(p.Spec.Volumes, v1.Volume{
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		})
	}
	return p
}

func makePVC(name string, boundPVName string, storageClassName string) *v1.PersistentVolumeClaim {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: v1.NamespaceDefault,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: pointer.StringPtr(storageClassName),
		},
	}
	if boundPVName != "" {
		pvc.Spec.VolumeName = boundPVName
		metav1.SetMetaDataAnnotation(&pvc.ObjectMeta, storagehelpers.AnnBindCompleted, "true")
	}
	return pvc
}

func makeCSIDriverForUnified(name string, annotations map[string]string) *storagev1.CSIDriver {
	return &storagev1.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
		},
	}
}

func TestVolumeBindingWithLocalPV(t *testing.T) {
	table := []struct {
		name                    string
		pod                     *v1.Pod
		node                    *v1.Node
		localInfo               *extension.LocalInfo
		pvcs                    []*v1.PersistentVolumeClaim
		pvs                     []*v1.PersistentVolume
		ephemeralStorage        string
		localInlineVolume       string
		wantPreFilterStatus     *framework.Status
		wantStateAfterPreFilter *stateData
		wantFilterStatus        *framework.Status
		wantStateAfterFilter    *stateData
	}{
		{
			name: "provisioning lvm",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					Annotations: map[string]string{
						unified.AnnotationNodeLocalStorageTopologyKey: "[{\"type\": \"lvm\", \"name\": \"vg-1\", \"capacity\": 105151127552}]",
					},
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", lvmSC.Name, "60Gi"),
			},
			wantPreFilterStatus: nil,
			wantStateAfterPreFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", lvmSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{},
			},
			wantFilterStatus: nil,
			wantStateAfterFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", lvmSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{
					"test-node-1": {
						DynamicProvisions: []*v1.PersistentVolumeClaim{
							makePVCWithStorageSize("pvc-a", "1", "", lvmSC.Name, "60Gi"),
						},
					},
				},
			},
		},
		{
			name: "provisioning QuotaPath",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					Annotations: map[string]string{
						unified.AnnotationNodeLocalStorageTopologyKey: "[{\"type\": \"lvm\", \"name\": \"vg-1\", \"capacity\": 105151127552}," +
							"{\"type\": \"quotapath\", \"name\": \"quotapath1\", \"capacity\": 105151127552, \"devicetype\": \"ssd\", \"mountPoint\": \"/path1\"}]",
					},
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", quotaPathSC.Name, "60Gi"),
			},
			wantPreFilterStatus: nil,
			wantStateAfterPreFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", quotaPathSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{},
			},
			wantFilterStatus: nil,
			wantStateAfterFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", quotaPathSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{
					"test-node-1": {
						DynamicProvisions: []*v1.PersistentVolumeClaim{
							makePVCWithStorageSize("pvc-a", "1", "", quotaPathSC.Name, "60Gi"),
						},
					},
				},
			},
		},
		{
			name: "provisioning local pv",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
			},
			wantPreFilterStatus: nil,
			wantStateAfterPreFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{},
			},
			wantFilterStatus: nil,
			wantStateAfterFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{
					"test-node-1": {
						DynamicProvisions: []*v1.PersistentVolumeClaim{
							makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
						},
					},
				},
			},
		},
		{
			name: "reuse local pv",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
			},
			pvs: []*v1.PersistentVolume{
				makeLocalPV("local-pv-a", "1", "test-node-1", localPVSC.Name, "60Gi", nil),
			},
			wantPreFilterStatus: nil,
			wantStateAfterPreFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{},
			},
			wantFilterStatus: nil,
		},
		{
			name: "provisioning new local pv on conflict pv capacity",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
			},
			pvs: []*v1.PersistentVolume{
				makeLocalPV("local-pv-a", "1", "test-node-1", localPVSC.Name, "40Gi", nil),
			},
			wantPreFilterStatus: nil,
			wantStateAfterPreFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{},
			},
			wantFilterStatus: nil,
			wantStateAfterFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{
					"test-node-1": {
						DynamicProvisions: []*v1.PersistentVolumeClaim{
							makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
						},
					},
				},
			},
		},
		{
			name: "failed to provisioning new local pv with insufficient storage",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				makePVCWithStorageSize("pvc-b", "1", "local-pv-b", localPVSC.Name, "60Gi"),
			},
			pvs: []*v1.PersistentVolume{
				makeLocalPV("local-pv-b", "1", "test-node-1", localPVSC.Name, "100Gi",
					makePVCWithStorageSize("pvc-b", "1", "local-pv-b", localPVSC.Name, "60Gi")),
			},
			wantPreFilterStatus: nil,
			wantStateAfterPreFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{},
			},
			wantFilterStatus: framework.NewStatus(framework.UnschedulableAndUnresolvable, string(ErrReasonBindConflict), string(ErrReasonNotEnoughSpace)),
		},
		{
			name: "failed to provisioning new local pv with part bound pvc and insufficient storage",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a", "pvc-b"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				makePVCWithStorageSize("pvc-b", "1", "local-pv-b", localPVSC.Name, "60Gi"),
			},
			pvs: []*v1.PersistentVolume{
				makeLocalPV("local-pv-b", "1", "test-node-1", localPVSC.Name, "100Gi",
					makePVCWithStorageSize("pvc-b", "1", "local-pv-b", localPVSC.Name, "60Gi")),
			},
			wantPreFilterStatus: nil,
			wantStateAfterPreFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-b", "1", "local-pv-b", localPVSC.Name, "60Gi"),
				},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{},
			},
			wantFilterStatus: framework.NewStatus(framework.UnschedulableAndUnresolvable, string(ErrReasonBindConflict), string(ErrReasonNotEnoughSpace)),
		},
		{
			name: "failed to provisioning new local pv with ephemeralStorage insufficient storage",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
			},
			ephemeralStorage:    "80Gi",
			wantPreFilterStatus: nil,
			wantStateAfterPreFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				},
				preemptiveUsedStorage: nil,
				podVolumesByNode:      map[string]*PodVolumes{},
			},
			wantFilterStatus: framework.NewStatus(framework.UnschedulableAndUnresolvable, string(ErrReasonBindConflict), string(ErrReasonNotEnoughSpace)),
		},
		{
			name:              "failed to provisioning new local pv with localInlineVolume insufficient storage",
			pod:               makePodWithInlineVolumeForBindingTest("pod-a", localPVSC.Name, "80Gi", "pvc-a"),
			localInlineVolume: "80Gi",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
			},
			wantPreFilterStatus: nil,
			wantStateAfterPreFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				},
				preemptiveUsedStorage: nil,
				podVolumesByNode:      map[string]*PodVolumes{},
			},
			wantFilterStatus: framework.NewStatus(framework.UnschedulableAndUnresolvable, string(ErrReasonBindConflict), string(ErrReasonNotEnoughSpace)),
		},
	}

	for _, item := range table {
		t.Run(item.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			opts := []runtime.Option{
				runtime.WithClientSet(client),
				runtime.WithInformerFactory(informerFactory),
			}
			fh, err := runtime.NewFramework(context.TODO(), nil, nil, opts...)
			if err != nil {
				t.Fatal(err)
			}

			t.Log("Feed testing data and wait for them to be synced")
			client.StorageV1().StorageClasses().Create(context.Background(), immediateSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), waitSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), localPVSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), lvmSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), quotaPathSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), multiVolumeLocalPVSC, metav1.CreateOptions{})
			if item.node != nil {
				client.CoreV1().Nodes().Create(context.Background(), item.node, metav1.CreateOptions{})
			}
			for _, pvc := range item.pvcs {
				client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
			}
			for _, pv := range item.pvs {
				client.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
			}
			if item.ephemeralStorage != "" {
				item.pod.Spec.Containers = append(item.pod.Spec.Containers, v1.Container{
					Name: "test-local-pv-ephemeral-storage",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceEphemeralStorage: resource.MustParse(item.ephemeralStorage),
						},
					},
				})
			}

			t.Log("Start informer factory after initialization")
			informerFactory.Start(ctx.Done())

			t.Log("Wait for all started informers' cache were synced")
			informerFactory.WaitForCacheSync(ctx.Done())

			args := &config.VolumeBindingArgs{
				BindTimeoutSeconds: 300,
			}

			pl, err := New(args, fh)
			if err != nil {
				t.Fatal(err)
			}

			t.Log("Verify")

			if item.localInfo != nil {
				data, err := json.Marshal(item.localInfo)
				if err != nil {
					t.Fatal(err)
				}
				if item.node.Annotations == nil {
					item.node.Annotations = make(map[string]string)
				}
				item.node.Annotations[extension.AnnotationLocalInfo] = string(data)
			}

			nodeLocalVolumeInfo, err := newNodeLocalVolumeInfo(item.node)
			assert.NoError(t, err)
			pl.(*VolumeBinding).nodeStorageCache.UpdateOnNode(item.node.Name, func(info *NodeStorageInfo) {
				info.UpdateNodeLocalVolumeInfo(nodeLocalVolumeInfo)
				for _, v := range item.pvs {
					if v.Spec.StorageClassName == localPVSC.Name {
						info.AddLocalPVAlloc(&localPVAlloc{
							Name:     v.Name,
							Capacity: v.Spec.Capacity.Storage().Value(),
						})
					}
				}
			})

			t.Logf("Verify: call PreFilter and check status")
			state := framework.NewCycleState()
			_, gotPreFilterStatus := pl.(framework.PreFilterPlugin).PreFilter(ctx, state, item.pod)
			if !reflect.DeepEqual(gotPreFilterStatus, item.wantPreFilterStatus) {
				t.Errorf("filter prefilter status does not match: %v, want: %v", gotPreFilterStatus, item.wantPreFilterStatus)
			}
			if !gotPreFilterStatus.IsSuccess() {
				// scheduler framework will skip Filter if PreFilter fails
				return
			}

			t.Logf("Verify: check state after prefilter phase")
			stateData, err := getStateData(state)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(stateData, item.wantStateAfterPreFilter) {
				t.Errorf("state got after prefilter does not match: %v, want: %v", stateData, item.wantStateAfterPreFilter)
			}

			t.Logf("Verify: call Filter and check status")
			nodeInfo := framework.NewNodeInfo()
			nodeInfo.SetNode(item.node)
			gotStatus := pl.(framework.FilterPlugin).Filter(ctx, state, item.pod, nodeInfo)
			if !reflect.DeepEqual(gotStatus, item.wantFilterStatus) {
				t.Errorf("filter status does not match: %v, want: %v", gotStatus, item.wantFilterStatus)
			}
			if item.wantStateAfterFilter != nil && !reflect.DeepEqual(stateData, item.wantStateAfterFilter) {
				t.Errorf("state got after filter does not match: %v, want: %v", stateData, item.wantStateAfterFilter)
			}
		})
	}
}

func TestVolumeBindingWithLocalPVPreemption(t *testing.T) {
	table := []struct {
		name                                 string
		existingPod                          *v1.Pod
		ephemeralStorage                     string
		ephemeralStorageSize                 int64
		localInlineVolumeSize                int64
		pod                                  *v1.Pod
		node                                 *v1.Node
		localInfo                            *extension.LocalInfo
		wantPreFilterStatus                  *framework.Status
		wantStateAfterPreFilter              *stateData
		wantFilterStatus                     *framework.Status
		wantStateAfterFilter                 *stateData
		wantStateAfterPreFilterRemove        *stateData
		wantFilterStatusAfterPreFilterRemove *framework.Status
		wantStateAfterPreFilterAdd           *stateData
		wantFilterStatusAfterPreFilterAdd    *framework.Status
	}{
		{
			name:                 "preempt ephemeralStorage pods",
			existingPod:          makePodForBindingTest("pod-0", nil),
			pod:                  makePodForBindingTest("pod-a", nil),
			ephemeralStorage:     "80Gi",
			ephemeralStorageSize: 80 * 1024 * 1024 * 1024,
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			wantPreFilterStatus: nil,
			wantStateAfterPreFilter: &stateData{
				skip: true,
			},
			wantFilterStatus: framework.NewStatus(framework.Unschedulable, ErrInsufficientEphemeralStorage),
			wantStateAfterPreFilterRemove: &stateData{
				skip: true,
				preemptiveUsedStorage: map[string]map[string]int64{
					"test-node-1": {"default/pod-0": 80 * 1024 * 1024 * 1024},
				},
			},
			wantFilterStatusAfterPreFilterRemove: nil,
			wantStateAfterPreFilterAdd: &stateData{
				skip: true,
			},
			wantFilterStatusAfterPreFilterAdd: framework.NewStatus(framework.Unschedulable, ErrInsufficientEphemeralStorage),
		},
		{
			name:                  "preempt inline volume pods",
			existingPod:           makePodWithInlineVolumeForBindingTest("pod-0", localPVSC.Name, "80Gi"),
			pod:                   makePodWithInlineVolumeForBindingTest("pod-0", localPVSC.Name, "80Gi"),
			localInlineVolumeSize: 80 * 1024 * 1024 * 1024,
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			wantPreFilterStatus: nil,
			wantStateAfterPreFilter: &stateData{
				skip: true,
			},
			wantFilterStatus: framework.NewStatus(framework.Unschedulable, ErrInsufficientEphemeralStorage),
			wantStateAfterPreFilterRemove: &stateData{
				skip: true,
				preemptiveUsedStorage: map[string]map[string]int64{
					"test-node-1": {"default/pod-0": 80 * 1024 * 1024 * 1024},
				},
			},
			wantFilterStatusAfterPreFilterRemove: nil,
			wantStateAfterPreFilterAdd: &stateData{
				skip: true,
			},
			wantFilterStatusAfterPreFilterAdd: framework.NewStatus(framework.Unschedulable, ErrInsufficientEphemeralStorage),
		},
	}

	for _, item := range table {
		t.Run(item.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			opts := []runtime.Option{
				runtime.WithClientSet(client),
				runtime.WithInformerFactory(informerFactory),
			}
			fh, err := runtime.NewFramework(context.TODO(), nil, nil, opts...)
			if err != nil {
				t.Fatal(err)
			}

			t.Log("Feed testing data and wait for them to be synced")
			client.StorageV1().StorageClasses().Create(context.Background(), immediateSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), waitSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), localPVSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), lvmSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), quotaPathSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), multiVolumeLocalPVSC, metav1.CreateOptions{})
			if item.node != nil {
				client.CoreV1().Nodes().Create(context.Background(), item.node, metav1.CreateOptions{})
			}
			if item.ephemeralStorage != "" {
				item.existingPod.Spec.Containers = append(item.existingPod.Spec.Containers, v1.Container{
					Name: "test-local-pv-ephemeral-storage",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceEphemeralStorage: resource.MustParse(item.ephemeralStorage),
						},
					},
				})
				item.pod.Spec.Containers = append(item.pod.Spec.Containers, v1.Container{
					Name: "test-local-pv-ephemeral-storage",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceEphemeralStorage: resource.MustParse(item.ephemeralStorage),
						},
					},
				})
			}

			t.Log("Start informer factory after initialization")
			informerFactory.Start(ctx.Done())

			t.Log("Wait for all started informers' cache were synced")
			informerFactory.WaitForCacheSync(ctx.Done())

			args := &config.VolumeBindingArgs{
				BindTimeoutSeconds: 300,
			}

			pl, err := New(args, fh)
			if err != nil {
				t.Fatal(err)
			}

			t.Log("Verify")

			if item.localInfo != nil {
				data, err := json.Marshal(item.localInfo)
				if err != nil {
					t.Fatal(err)
				}
				if item.node.Annotations == nil {
					item.node.Annotations = make(map[string]string)
				}
				item.node.Annotations[extension.AnnotationLocalInfo] = string(data)
			}

			nodeLocalVolumeInfo, err := newNodeLocalVolumeInfo(item.node)
			assert.NoError(t, err)
			pl.(*VolumeBinding).nodeStorageCache.UpdateOnNode(item.node.Name, func(info *NodeStorageInfo) {
				info.UpdateNodeLocalVolumeInfo(nodeLocalVolumeInfo)
				info.AddLocalVolumeAlloc(item.existingPod, item.ephemeralStorageSize, item.localInlineVolumeSize)
			})

			t.Logf("Verify: call PreFilter and check status")
			state := framework.NewCycleState()
			_, gotPreFilterStatus := pl.(framework.PreFilterPlugin).PreFilter(ctx, state, item.pod)
			if !reflect.DeepEqual(gotPreFilterStatus, item.wantPreFilterStatus) {
				t.Errorf("filter prefilter status does not match: %v, want: %v", gotPreFilterStatus, item.wantPreFilterStatus)
			}
			if !gotPreFilterStatus.IsSuccess() {
				// scheduler framework will skip Filter if PreFilter fails
				return
			}

			t.Logf("Verify: check state after prefilter phase")
			stateData, err := getStateData(state)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(stateData, item.wantStateAfterPreFilter) {
				t.Errorf("state got after prefilter does not match: %v, want: %v", stateData, item.wantStateAfterPreFilter)
			}

			t.Logf("Verify: call Filter and check status")
			nodeInfo := framework.NewNodeInfo()
			nodeInfo.SetNode(item.node)
			gotStatus := pl.(framework.FilterPlugin).Filter(ctx, state, item.pod, nodeInfo)
			if !reflect.DeepEqual(gotStatus, item.wantFilterStatus) {
				t.Errorf("filter status does not match: %v, want: %v", gotStatus, item.wantFilterStatus)
			}
			if item.wantStateAfterFilter != nil && !reflect.DeepEqual(stateData, item.wantStateAfterFilter) {
				t.Errorf("state got after filter does not match: %v, want: %v", stateData, item.wantStateAfterFilter)
			}

			volume := v1.Volume{}
			volume.PersistentVolumeClaim = &v1.PersistentVolumeClaimVolumeSource{}
			volume.PersistentVolumeClaim.ClaimName = "fake"
			item.existingPod.Spec.Volumes = append(item.existingPod.Spec.Volumes, volume)

			podInfo, _ := framework.NewPodInfo(item.existingPod)
			pl.(*VolumeBinding).RemovePod(ctx, state, item.pod, podInfo, nodeInfo)

			t.Logf("Verify: check state after prefilterRemove phase")
			stateData, err = getStateData(state)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(stateData, item.wantStateAfterPreFilterRemove) {
				t.Errorf("state got after prefilter remove does not match: %v, want: %v", stateData, item.wantStateAfterPreFilterRemove)
			}

			t.Logf("Verify: call Filter and check status")
			nodeInfo = framework.NewNodeInfo()
			nodeInfo.SetNode(item.node)
			gotStatus = pl.(framework.FilterPlugin).Filter(ctx, state, item.pod, nodeInfo)
			if !reflect.DeepEqual(gotStatus, item.wantFilterStatusAfterPreFilterRemove) {
				t.Errorf("filter status does not match: %v, want: %v", gotStatus, item.wantFilterStatusAfterPreFilterRemove)
			}

			podInfo, _ = framework.NewPodInfo(item.existingPod)
			pl.(*VolumeBinding).AddPod(ctx, state, item.pod, podInfo, nodeInfo)

			t.Logf("Verify: check state after prefilterAdd phase")
			stateData, err = getStateData(state)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(stateData, item.wantStateAfterPreFilterAdd) {
				t.Errorf("state got after prefilter add does not match: %v, want: %v", stateData, item.wantStateAfterPreFilterAdd)
			}

			t.Logf("Verify: call Filter and check status")
			nodeInfo = framework.NewNodeInfo()
			nodeInfo.SetNode(item.node)
			gotStatus = pl.(framework.FilterPlugin).Filter(ctx, state, item.pod, nodeInfo)
			if !reflect.DeepEqual(gotStatus, item.wantFilterStatusAfterPreFilterAdd) {
				t.Errorf("filter status does not match: %v, want: %v", gotStatus, item.wantFilterStatusAfterPreFilterAdd)
			}

		})
	}
}

func TestVolumePreAssignFilterForLocalPV(t *testing.T) {
	table := []struct {
		name                          string
		pod                           *v1.Pod
		node                          *v1.Node
		localInfo                     *extension.LocalInfo
		pvcs                          []*v1.PersistentVolumeClaim
		pvs                           []*v1.PersistentVolume
		disableQuotaPathCapacity      bool
		ephemeralStorage              string
		assignedSystemDisk            string
		assignedLocalVolumes          map[string]string
		wantPreAssignFilterStatus     *framework.Status
		wantStateAfterPreAssignFilter *stateData
		wantFilterFailure             bool
	}{
		{
			name: "preAssignFilter lvm",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					Annotations: map[string]string{
						unified.AnnotationNodeLocalStorageTopologyKey: "[{\"type\": \"lvm\", \"name\": \"vg-1\", \"capacity\": 105151127552}]",
					},
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", lvmSC.Name, "60Gi"),
			},
			wantPreAssignFilterStatus: nil,
			wantStateAfterPreAssignFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", lvmSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{
					"test-node-1": {
						DynamicProvisions: []*v1.PersistentVolumeClaim{
							makePVCWithStorageSize("pvc-a", "1", "", lvmSC.Name, "60Gi"),
						},
					},
				},
			},
		},
		{
			name: "provisioning QuotaPath",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					Annotations: map[string]string{
						unified.AnnotationNodeLocalStorageTopologyKey: "[{\"type\": \"lvm\", \"name\": \"vg-1\", \"capacity\": 105151127552}," +
							"{\"type\": \"quotapath\", \"name\": \"quotapath1\", \"capacity\": 105151127552, \"devicetype\": \"ssd\", \"mountPoint\": \"/path1\"}]",
					},
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", quotaPathSC.Name, "60Gi"),
			},
			wantPreAssignFilterStatus: nil,
			wantStateAfterPreAssignFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", quotaPathSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{
					"test-node-1": {
						DynamicProvisions: []*v1.PersistentVolumeClaim{
							makePVCWithStorageSize("pvc-a", "1", "", quotaPathSC.Name, "60Gi"),
						},
					},
				},
			},
		},
		{
			name: "provisioning QuotaPath and disable capacity check",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					Annotations: map[string]string{
						unified.AnnotationNodeLocalStorageTopologyKey: "[{\"type\": \"lvm\", \"name\": \"vg-1\", \"capacity\": 105151127552}]",
					},
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			disableQuotaPathCapacity: true,
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", quotaPathSC.Name, "60Gi"),
			},
			wantPreAssignFilterStatus: nil,
			wantStateAfterPreAssignFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", quotaPathSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{
					"test-node-1": {
						DynamicProvisions: []*v1.PersistentVolumeClaim{
							makePVCWithStorageSize("pvc-a", "1", "", quotaPathSC.Name, "60Gi"),
						},
					},
				},
			},
		},
		{
			name: "preAssignFilter local pv",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
			},
			wantPreAssignFilterStatus: nil,
			wantStateAfterPreAssignFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{
					"test-node-1": {
						DynamicProvisions: []*v1.PersistentVolumeClaim{
							makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
						},
					},
				},
			},
		},
		{
			name: "preAssignFilter local pv with ephemeral storage",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
			},
			assignedSystemDisk:        "60Gi",
			wantPreAssignFilterStatus: nil,
			wantStateAfterPreAssignFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{
					"test-node-1": {
						DynamicProvisions: []*v1.PersistentVolumeClaim{
							makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
						},
					},
				},
			},
		},
		{
			name: "failed preAssignFilter local pv",
			pod:  makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
			},
			assignedSystemDisk:        "100Gi",
			wantPreAssignFilterStatus: framework.NewStatus(framework.UnschedulableAndUnresolvable, string(ErrReasonBindConflict), string(ErrReasonNotEnoughSpace)),
			wantStateAfterPreAssignFilter: &stateData{
				boundClaims: []*v1.PersistentVolumeClaim{},
				claimsToBind: []*v1.PersistentVolumeClaim{
					makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
				},
				podVolumesByNode: map[string]*PodVolumes{},
			},
		},
	}

	for _, item := range table {
		t.Run(item.name, func(t *testing.T) {
			defer koordutilfeature.SetFeatureGateDuringTest(t, k8sfeature.DefaultMutableFeatureGate, features.EnableQuotaPathCapacity, !item.disableQuotaPathCapacity)()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			opts := []runtime.Option{
				runtime.WithClientSet(client),
				runtime.WithInformerFactory(informerFactory),
			}
			fh, err := runtime.NewFramework(context.TODO(), nil, nil, opts...)
			if err != nil {
				t.Fatal(err)
			}

			t.Log("Feed testing data and wait for them to be synced")
			client.StorageV1().StorageClasses().Create(context.Background(), immediateSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), waitSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), localPVSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), lvmSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), quotaPathSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), multiVolumeLocalPVSC, metav1.CreateOptions{})
			if item.node != nil {
				client.CoreV1().Nodes().Create(context.Background(), item.node, metav1.CreateOptions{})
			}
			for _, pvc := range item.pvcs {
				client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
			}
			for _, pv := range item.pvs {
				client.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
			}
			if item.ephemeralStorage != "" {
				item.pod.Spec.Containers = append(item.pod.Spec.Containers, v1.Container{
					Name: "test-local-pv-ephemeral-storage",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceEphemeralStorage: resource.MustParse(item.ephemeralStorage),
						},
					},
				})
			}

			t.Log("Start informer factory after initialization")
			informerFactory.Start(ctx.Done())

			t.Log("Wait for all started informers' cache were synced")
			informerFactory.WaitForCacheSync(ctx.Done())

			args := &config.VolumeBindingArgs{
				BindTimeoutSeconds: 300,
			}

			pl, err := New(args, fh)
			if err != nil {
				t.Fatal(err)
			}

			t.Log("Verify")

			if item.localInfo != nil {
				data, err := json.Marshal(item.localInfo)
				if err != nil {
					t.Fatal(err)
				}
				metav1.SetMetaDataAnnotation(&item.node.ObjectMeta, extension.AnnotationLocalInfo, string(data))
			}

			nodeLocalVolumeInfo, err := newNodeLocalVolumeInfo(item.node)
			assert.NoError(t, err)
			var nodeStorageInfo *NodeStorageInfo
			pl.(*VolumeBinding).nodeStorageCache.UpdateOnNode(item.node.Name, func(info *NodeStorageInfo) {
				nodeStorageInfo = info
				info.UpdateNodeLocalVolumeInfo(nodeLocalVolumeInfo)
				for _, v := range item.pvs {
					info.AddLocalPVAlloc(&localPVAlloc{
						Name:     v.Name,
						Capacity: v.Spec.Capacity.Storage().Value(),
					})
				}
			})

			state := framework.NewCycleState()
			t.Logf("Verify: call PreFilter and check status")
			_, gotPreFilterStatus := pl.(framework.PreFilterPlugin).PreFilter(ctx, state, item.pod)
			if !gotPreFilterStatus.IsSuccess() {
				// scheduler framework will skip Filter if PreFilter fails
				return
			}

			t.Logf("Verify: check state after prefilter phase")
			_, err = getStateData(state)
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("Verify: call Filter and check status")
			nodeInfo := framework.NewNodeInfo()
			nodeInfo.SetNode(item.node)
			gotStatus := pl.(framework.FilterPlugin).Filter(ctx, state, item.pod, nodeInfo)
			if !gotStatus.IsSuccess() {
				t.Fatalf("failed to filter, err: %v", gotStatus)
			}

			if item.assignedSystemDisk != "" {
				quantity := resource.MustParse(item.assignedSystemDisk)
				graphDiskPath := nodeStorageInfo.GraphDiskPath
				nodeStorageInfo.Used.VolumeSize[graphDiskPath] = quantity.Value()
			}
			for volume, s := range item.assignedLocalVolumes {
				quantity := resource.MustParse(s)
				nodeStorageInfo.Used.VolumeSize[volume] = quantity.Value()
			}
			nodeStorageInfo.resetFreeResource()
			stateData, err := getStateData(state)
			if err != nil {
				t.Fatal(err)
			}
			stateData.podVolumesByNode = map[string]*PodVolumes{}
			gotStatus = pl.(framework.FilterPlugin).Filter(ctx, state, item.pod, nodeInfo)
			if !reflect.DeepEqual(gotStatus, item.wantPreAssignFilterStatus) {
				t.Errorf("PreAssignFilter status does not match: %v, want: %v", gotStatus, item.wantPreAssignFilterStatus)
			}

			if !assert.Equal(t, item.wantStateAfterPreAssignFilter, stateData) {
				t.Errorf("fail to match stateData after PreAssignFilter, got: %v, want %v", stateData, item.wantStateAfterPreAssignFilter)
			}
		})
	}
}

type allocatedVolumes struct {
	LocalVolumesInBytes map[string]int64
	LocalPVCAllocs      map[string]*localPVCAlloc
	LocalPVAllocs       map[string]*localPVAlloc
	LocalVolumeIOLimit  map[string]*unified.LocalStorageIOLimitInfo
}

func TestCloudStorageAdapterScheduling(t *testing.T) {
	table := []struct {
		name             string
		pod              *v1.Pod
		node             *v1.Node
		pvs              []*v1.PersistentVolume
		pvcs             []*v1.PersistentVolumeClaim
		drivers          []*storagev1.CSIDriver
		wantAssignStatus *framework.Status
	}{
		{
			name: "pvc has bounded and pv without topology",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					Labels: map[string]string{
						unified.LabelNodeDiskPrefix + "cloud_ssd":  "available",
						unified.LabelNodeDiskPrefix + "cloud_essd": "available",
					},
				},
			},
			pvs: []*v1.PersistentVolume{
				makePVWithAnnotationAndPhase("cloud-storage-pv1", "1", "cloud-storage-sc1", nil, v1.VolumeBound),
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "cloud-storage-pv1", "cloud-storage-sc1"),
			},
		},
		{
			name: "pvc has bounded and node without topology",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-2",
				},
			},
			pvs: []*v1.PersistentVolume{
				makePVWithAnnotationAndPhase("cloud-storage-pv1", "1", "cloud-storage-sc1", map[string]string{
					unified.AnnotationCSIVolumeTopology: "{\"nodeSelectorTerms\":[{\"matchExpressions\":[{\"key\":" +
						"\"node.csi.alibabacloud.com/disktype.cloud_ssd\",\"operator\":\"In\",\"values\":[\"available\"]}]}]}",
				}, v1.VolumeBound),
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "cloud-storage-pv1", "cloud-storage-sc1"),
			},
		},
		{
			name: "pvc has bounded and pv'stopology not matched with node",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-3",
					Labels: map[string]string{
						unified.LabelNodeDiskPrefix + "cloud_essd": "available",
					},
				},
			},
			pvs: []*v1.PersistentVolume{
				makePVWithAnnotationAndPhase("cloud-storage-pv1", "1", "cloud-storage-sc1", map[string]string{
					unified.AnnotationCSIVolumeTopology: "{\"nodeSelectorTerms\":[{\"matchExpressions\":[{\"key\":" +
						"\"node.csi.alibabacloud.com/disktype.cloud_ssd\",\"operator\":\"In\",\"values\":[\"available\"]}]}]}",
				}, v1.VolumeBound),
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "cloud-storage-pv1", "cloud-storage-sc1"),
			},
			wantAssignStatus: framework.NewStatus(3, string(ErrReasonNodeConflict)),
		},
		{
			name: "pvc has bounded and pv's topology matched with node",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-4",
					Labels: map[string]string{
						unified.LabelNodeDiskPrefix + "cloud_ssd": "available",
					},
				},
			},
			pvs: []*v1.PersistentVolume{
				makePVWithAnnotationAndPhase("cloud-storage-pv1", "1", "cloud-storage-sc1", map[string]string{
					unified.AnnotationCSIVolumeTopology: "{\"nodeSelectorTerms\":[{\"matchExpressions\":[{\"key\":" +
						"\"node.csi.alibabacloud.com/disktype.cloud_ssd\",\"operator\":\"In\",\"values\":[\"available\"]}]}]}",
				}, v1.VolumeBound),
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "cloud-storage-pv1", "cloud-storage-sc1"),
			},
		},
		{
			name: "pvc is pending and pv's topology matched with node",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-5",
					Labels: map[string]string{
						unified.LabelNodeDiskPrefix + "cloud_ssd": "available",
					},
				},
			},
			pvs: []*v1.PersistentVolume{
				makePVWithAnnotationAndPhase("cloud-storage-pv1", "1", "cloud-storage-sc1", map[string]string{
					unified.AnnotationCSIVolumeTopology: "{\"nodeSelectorTerms\":[{\"matchExpressions\":[{\"key\":" +
						"\"node.csi.alibabacloud.com/disktype.cloud_ssd\",\"operator\":\"In\",\"values\":[\"available\"]}]}]}",
				}, v1.VolumeAvailable),
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "", "cloud-storage-sc1"),
			},
		},
		{
			name: "pvc is pending and pv's topology mismatched with node",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-6",
					Labels: map[string]string{
						unified.LabelNodeDiskPrefix + "cloud_essd": "available",
					},
				},
			},
			pvs: []*v1.PersistentVolume{
				makePVWithAnnotationAndPhase("cloud-storage-pv1", "1", "cloud-storage-sc1", map[string]string{
					unified.AnnotationCSIVolumeTopology: "{\"nodeSelectorTerms\":[{\"matchExpressions\":[{\"key\":" +
						"\"node.csi.alibabacloud.com/disktype.cloud_ssd\",\"operator\":\"In\",\"values\":[\"available\"]}]}]}",
				}, v1.VolumeAvailable),
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "", "cloud-storage-sc1"),
			},
			wantAssignStatus: framework.AsStatus(errors.New(string(ErrReasonNodeConflict))),
		},
		{
			name: "pvc is pending and node has different disk type",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-6",
					Labels: map[string]string{
						unified.LabelNodeDiskPrefix + "cloud_efficiency": "available",
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "", "cloud-storage-sc1"),
			},
			wantAssignStatus: framework.NewStatus(3, string(ErrReasonBindConflict)),
		},
		{
			name: "pvc is pending and sc doesn't have disk type",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-7",
					Labels: map[string]string{
						unified.LabelNodeDiskPrefix + "cloud_essd": "available",
					},
				},
			},
			pvs: []*v1.PersistentVolume{
				makePVWithAnnotationAndPhase("cloud-storage-pv1", "1", "cloud-storage-sc2", map[string]string{
					unified.AnnotationCSIVolumeTopology: "{\"nodeSelectorTerms\":[{\"matchExpressions\":[{\"key\":" +
						"\"node.csi.alibabacloud.com/disktype.cloud_ssd\",\"operator\":\"In\",\"values\":[\"available\"]}]}]}",
				}, v1.VolumeAvailable),
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "", "cloud-storage-sc2"),
			},
			wantAssignStatus: framework.AsStatus(errors.New(string(ErrReasonNodeConflict))),
		},
		{
			name: "pvc is pending and node doesn't have disk type",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-8",
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "", "cloud-storage-sc1"),
			},
		},
		{
			name: "other pvc is pending and sc also has type",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-8",
					Labels: map[string]string{
						unified.LabelNodeDiskPrefix + "cloud_essd": "available",
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "", "other-sc"),
			},
		},
		{
			name: "ultron pvc is pending and driver do not use disk capabilities",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-8",
					Labels: map[string]string{
						unified.LabelNodeDiskPrefix + "cloud_ssd": "available",
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "", "csi-ultron-disk"),
			},
			drivers: []*storagev1.CSIDriver{
				makeCSIDriverForUnified("com.alibaba-inc.ack-ee.csi-ultron", nil),
			},
		},
		{
			name: "ultron pvc is pending and driver use disk capabilities",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-8",
					Labels: map[string]string{
						unified.LabelNodeDiskPrefix + "cloud_essd": "available",
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "", "csi-ultron-disk"),
			},
			drivers: []*storagev1.CSIDriver{
				makeCSIDriverForUnified("com.alibaba-inc.ack-ee.csi-ultron", map[string]string{
					unified.AnnotationUltronDiskUseCapabilities: "true",
				}),
			},
		},
		{
			name: "ultron pvc is pending and node type mismatch",
			pod:  makePodForBindingTest("pod-a", []string{"cloud-storage-pvc1"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-8",
					Labels: map[string]string{
						unified.LabelNodeDiskPrefix + "cloud_ssd": "available",
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVC("cloud-storage-pvc1", "", "csi-ultron-disk"),
			},
			drivers: []*storagev1.CSIDriver{
				makeCSIDriverForUnified("com.alibaba-inc.ack-ee.csi-ultron", map[string]string{
					unified.AnnotationUltronDiskUseCapabilities: "true",
				}),
			},
			wantAssignStatus: framework.NewStatus(3, string(ErrReasonBindConflict)),
		},
	}

	for _, item := range table {
		t.Run(item.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			opts := []runtime.Option{
				runtime.WithClientSet(client),
				runtime.WithInformerFactory(informerFactory),
			}
			fh, err := runtime.NewFramework(context.TODO(), nil, nil, opts...)
			if err != nil {
				t.Fatal(err)
			}

			client.StorageV1().StorageClasses().Create(ctx, cloudSC1, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(ctx, cloudSC2, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(ctx, otherSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(ctx, ultronDiskSC, metav1.CreateOptions{})
			if item.node != nil {
				client.CoreV1().Nodes().Create(ctx, item.node, metav1.CreateOptions{})
			}
			for _, pvc := range item.pvcs {
				client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
			}
			for _, pv := range item.pvs {
				client.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
			}
			for _, driver := range item.drivers {
				client.StorageV1().CSIDrivers().Create(ctx, driver, metav1.CreateOptions{})
			}

			t.Log("Start informer factory after initialization")
			informerFactory.Start(ctx.Done())

			t.Log("Wait for all started informers' cache were synced")
			informerFactory.WaitForCacheSync(ctx.Done())

			args := &config.VolumeBindingArgs{
				BindTimeoutSeconds: 300,
			}

			pl, err := New(args, fh)
			if err != nil {
				t.Fatal(err)
			}

			t.Log("Verify")

			state := framework.NewCycleState()
			t.Logf("Verify: call PreFilter and check status")
			_, gotPreFilterStatus := pl.(framework.PreFilterPlugin).PreFilter(ctx, state, item.pod)
			if !gotPreFilterStatus.IsSuccess() {
				// scheduler framework will skip Filter if PreFilter fails
				return
			}

			t.Logf("Verify: check state after prefilter phase")
			_, err = getStateData(state)
			if err != nil {
				t.Fatal(err)
			}
			nodeInfo := framework.NewNodeInfo()
			nodeInfo.SetNode(item.node)
			gotStatus := pl.(framework.FilterPlugin).Filter(ctx, state, item.pod, nodeInfo)
			if !reflect.DeepEqual(gotStatus, item.wantAssignStatus) {
				t.Errorf("assign status does not match: %v, want: %v", gotStatus, item.wantAssignStatus)
			}
		})
	}
}

func TestVolumeAssignAndUnAssignLocalPVAndMultiVolumeLocalPV(t *testing.T) {
	table := []struct {
		name                              string
		pod                               *v1.Pod
		node                              *v1.Node
		localInfo                         *extension.LocalInfo
		pvcs                              []*v1.PersistentVolumeClaim
		pvs                               []*v1.PersistentVolume
		ephemeralStorage                  string
		assignedSystemDisk                string
		assignedLocalVolumes              map[string]string
		wantAssignStatus                  *framework.Status
		wantAllocatedVolumes              *allocatedVolumes
		wantAssumedLocalPVCAllocs         []*localPVCAlloc
		wantAllocatedVolumesAfterUnAssign *allocatedVolumes
		enableLocalVolumeCapacity         bool
		enableLocalVolumeIOLimit          bool
	}{
		{
			name:                      "assign lvm",
			enableLocalVolumeCapacity: true,
			enableLocalVolumeIOLimit:  false,
			pod:                       makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					Annotations: map[string]string{
						unified.AnnotationNodeLocalStorageTopologyKey: "[{\"type\": \"lvm\", \"name\": \"vg-1\", \"capacity\": 105151127552}]",
					},
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", lvmSC.Name, "60Gi"),
			},
			wantAssumedLocalPVCAllocs: []*localPVCAlloc{
				{
					Namespace:   v1.NamespaceDefault,
					Name:        "pvc-a",
					RequestSize: 60 * 1024 * 1024 * 1024,
					MountPoint:  "vg-1",
					IOLimit:     &unified.LocalStorageIOLimitInfo{},
				},
			},
			wantAllocatedVolumes: &allocatedVolumes{
				LocalPVAllocs: map[string]*localPVAlloc{},
				LocalPVCAllocs: map[string]*localPVCAlloc{
					"default/pvc-a": {
						Namespace:   v1.NamespaceDefault,
						Name:        "pvc-a",
						RequestSize: 60 * 1024 * 1024 * 1024,
						MountPoint:  "vg-1",
						IOLimit:     &unified.LocalStorageIOLimitInfo{},
					},
				},
				LocalVolumesInBytes: map[string]int64{
					"vg-1": 60 * 1024 * 1024 * 1024,
				},
				LocalVolumeIOLimit: map[string]*unified.LocalStorageIOLimitInfo{
					"vg-1": {},
				},
			},
			wantAllocatedVolumesAfterUnAssign: &allocatedVolumes{
				LocalPVAllocs:  map[string]*localPVAlloc{},
				LocalPVCAllocs: map[string]*localPVCAlloc{},
				LocalVolumesInBytes: map[string]int64{
					"vg-1": 0,
				},
				LocalVolumeIOLimit: map[string]*unified.LocalStorageIOLimitInfo{
					"vg-1": {},
				},
			},
		},
		{
			name:                      "assign QuotaPath",
			enableLocalVolumeCapacity: true,
			enableLocalVolumeIOLimit:  false,
			pod:                       makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					Annotations: map[string]string{
						unified.AnnotationNodeLocalStorageTopologyKey: "[{\"type\": \"lvm\", \"name\": \"vg-1\", \"capacity\": 105151127552}," +
							"{\"type\": \"quotapath\", \"name\": \"quotapath1\", \"capacity\": 105151127552, \"devicetype\": \"ssd\", \"mountPoint\": \"/path1\"}]",
					},
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", quotaPathSC.Name, "60Gi"),
			},
			wantAssumedLocalPVCAllocs: []*localPVCAlloc{
				{
					Namespace:   v1.NamespaceDefault,
					Name:        "pvc-a",
					RequestSize: 60 * 1024 * 1024 * 1024,
					MountPoint:  "/path1",
					IOLimit:     &unified.LocalStorageIOLimitInfo{},
				},
			},
			wantAllocatedVolumes: &allocatedVolumes{
				LocalPVAllocs: map[string]*localPVAlloc{},
				LocalPVCAllocs: map[string]*localPVCAlloc{
					"default/pvc-a": {
						Namespace:   v1.NamespaceDefault,
						Name:        "pvc-a",
						RequestSize: 60 * 1024 * 1024 * 1024,
						MountPoint:  "/path1",
						IOLimit:     &unified.LocalStorageIOLimitInfo{},
					},
				},
				LocalVolumesInBytes: map[string]int64{
					"/path1": 60 * 1024 * 1024 * 1024,
				},
				LocalVolumeIOLimit: map[string]*unified.LocalStorageIOLimitInfo{
					"/path1": {},
				},
			},
			wantAllocatedVolumesAfterUnAssign: &allocatedVolumes{
				LocalPVAllocs:  map[string]*localPVAlloc{},
				LocalPVCAllocs: map[string]*localPVCAlloc{},
				LocalVolumesInBytes: map[string]int64{
					"/path1": 0,
				},
				LocalVolumeIOLimit: map[string]*unified.LocalStorageIOLimitInfo{
					"/path1": {},
				},
			},
		},
		// 节点 storage-topology 不上报 capacity，只上报 iolimit，通过 enableLocalVolumeCapacity 跳过 capacity 检查
		// 蚂蚁的 capacity 检查在 check nodediskstorage 阶段会做
		{
			name:                      "assign lvm pvc iolimit，skip capacity check without node capacity",
			enableLocalVolumeCapacity: false,
			enableLocalVolumeIOLimit:  true,
			pod:                       makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					Annotations: map[string]string{
						unified.AnnotationNodeLocalStorageTopologyKey: "[{\"type\": \"lvm\", \"name\": \"/home/t4\", \"readIops\":100000,\"readBps\":1500000000,\"writeIops\":150000,\"writeBps\":1500000000}]",
					},
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithIOLimit(makePVCWithStorageSize("pvc-a", "1", "", lvmIOLimitSC.Name, "60Gi"), 30000, 20000, 30000000, 20000000),
			},
			wantAssumedLocalPVCAllocs: []*localPVCAlloc{
				{
					Namespace:   v1.NamespaceDefault,
					Name:        "pvc-a",
					RequestSize: 60 * 1024 * 1024 * 1024,
					MountPoint:  "/home/t4",
					IOLimit: &unified.LocalStorageIOLimitInfo{
						ReadIops:  30000,
						WriteIops: 20000,
						ReadBps:   30000000,
						WriteBps:  20000000,
					},
				},
			},
			wantAllocatedVolumes: &allocatedVolumes{
				LocalPVAllocs: map[string]*localPVAlloc{},
				LocalPVCAllocs: map[string]*localPVCAlloc{
					"default/pvc-a": {
						Namespace:   v1.NamespaceDefault,
						Name:        "pvc-a",
						RequestSize: 60 * 1024 * 1024 * 1024,
						MountPoint:  "/home/t4",
						IOLimit: &unified.LocalStorageIOLimitInfo{
							ReadIops:  30000,
							WriteIops: 20000,
							ReadBps:   30000000,
							WriteBps:  20000000,
						},
					},
				},
				LocalVolumesInBytes: map[string]int64{
					"/home/t4": 60 * 1024 * 1024 * 1024,
				},
				LocalVolumeIOLimit: map[string]*unified.LocalStorageIOLimitInfo{
					"/home/t4": {
						ReadIops:  30000,
						WriteIops: 20000,
						ReadBps:   30000000,
						WriteBps:  20000000,
					},
				},
			},
			wantAllocatedVolumesAfterUnAssign: &allocatedVolumes{
				LocalPVAllocs:  map[string]*localPVAlloc{},
				LocalPVCAllocs: map[string]*localPVCAlloc{},
				LocalVolumesInBytes: map[string]int64{
					"/home/t4": 0,
				},
				LocalVolumeIOLimit: map[string]*unified.LocalStorageIOLimitInfo{
					"/home/t4": {
						ReadIops:  0,
						WriteIops: 0,
						ReadBps:   0,
						WriteBps:  0,
					},
				},
			},
		},
		{
			name:                      "assign QuotaPath pvc iolimit，skip capacity check without node capacity",
			enableLocalVolumeCapacity: false,
			enableLocalVolumeIOLimit:  true,
			pod:                       makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					Annotations: map[string]string{
						unified.AnnotationNodeLocalStorageTopologyKey: "[{\"type\": \"quotapath\", \"name\": \"quotapath1\", \"devicetype\": \"ssd\", \"mountPoint\": \"/home/t4\", \"readIops\":100000,\"readBps\":1500000000,\"writeIops\":150000,\"writeBps\":1500000000}]",
					},
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithIOLimit(makePVCWithStorageSize("pvc-a", "1", "", quotaPathIOLimitSC.Name, "60Gi"), 30000, 20000, 30000000, 20000000),
			},
			wantAssumedLocalPVCAllocs: []*localPVCAlloc{
				{
					Namespace:   v1.NamespaceDefault,
					Name:        "pvc-a",
					RequestSize: 60 * 1024 * 1024 * 1024,
					MountPoint:  "/home/t4",
					IOLimit: &unified.LocalStorageIOLimitInfo{
						ReadIops:  30000,
						WriteIops: 20000,
						ReadBps:   30000000,
						WriteBps:  20000000,
					},
				},
			},
			wantAllocatedVolumes: &allocatedVolumes{
				LocalPVAllocs: map[string]*localPVAlloc{},
				LocalPVCAllocs: map[string]*localPVCAlloc{
					"default/pvc-a": {
						Namespace:   v1.NamespaceDefault,
						Name:        "pvc-a",
						RequestSize: 60 * 1024 * 1024 * 1024,
						MountPoint:  "/home/t4",
						IOLimit: &unified.LocalStorageIOLimitInfo{
							ReadIops:  30000,
							WriteIops: 20000,
							ReadBps:   30000000,
							WriteBps:  20000000,
						},
					},
				},
				LocalVolumesInBytes: map[string]int64{
					"/home/t4": 60 * 1024 * 1024 * 1024,
				},
				LocalVolumeIOLimit: map[string]*unified.LocalStorageIOLimitInfo{
					"/home/t4": {
						ReadIops:  30000,
						WriteIops: 20000,
						ReadBps:   30000000,
						WriteBps:  20000000,
					},
				},
			},
			wantAllocatedVolumesAfterUnAssign: &allocatedVolumes{
				LocalPVAllocs:  map[string]*localPVAlloc{},
				LocalPVCAllocs: map[string]*localPVCAlloc{},
				LocalVolumesInBytes: map[string]int64{
					"/home/t4": 0,
				},
				LocalVolumeIOLimit: map[string]*unified.LocalStorageIOLimitInfo{
					"/home/t4": {
						ReadIops:  0,
						WriteIops: 0,
						ReadBps:   0,
						WriteBps:  0,
					},
				},
			},
		},
		{
			name:                      "assign local pv",
			enableLocalVolumeCapacity: true,
			enableLocalVolumeIOLimit:  false,
			pod:                       makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
			},
			wantAssumedLocalPVCAllocs: []*localPVCAlloc{
				{
					Namespace:   v1.NamespaceDefault,
					Name:        "pvc-a",
					RequestSize: 60 * 1024 * 1024 * 1024,
					MountPoint:  "/home/t4",
				},
			},
			wantAllocatedVolumes: &allocatedVolumes{
				LocalPVAllocs: map[string]*localPVAlloc{},
				LocalPVCAllocs: map[string]*localPVCAlloc{
					"default/pvc-a": {
						Namespace:   v1.NamespaceDefault,
						Name:        "pvc-a",
						RequestSize: 60 * 1024 * 1024 * 1024,
						MountPoint:  "/home/t4",
					},
				},
				LocalVolumesInBytes: map[string]int64{
					"/home/t4": 60 * 1024 * 1024 * 1024,
				},
				LocalVolumeIOLimit: map[string]*unified.LocalStorageIOLimitInfo{
					"/home/t4": {},
				},
			},
			wantAllocatedVolumesAfterUnAssign: &allocatedVolumes{
				LocalPVAllocs:  map[string]*localPVAlloc{},
				LocalPVCAllocs: map[string]*localPVCAlloc{},
				LocalVolumesInBytes: map[string]int64{
					"/home/t4": 0,
				},
				LocalVolumeIOLimit: map[string]*unified.LocalStorageIOLimitInfo{
					"/home/t4": {},
				},
			},
		},
		{
			name:                      "assign local pv with ephemeral storage",
			enableLocalVolumeCapacity: true,
			enableLocalVolumeIOLimit:  false,
			pod:                       makePodForBindingTest("pod-a", []string{"pvc-a"}),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceEphemeralStorage: resource.MustParse("128Gi"),
					},
				},
			},
			localInfo: &extension.LocalInfo{
				DiskInfos: []extension.DiskInfo{
					{
						Device:         "/dev/sda1",
						FileSystemType: "ext4",
						Size:           128 * 1024 * 1024 * 1024, // 128GB
						MountPoint:     "/home/t4",
						DiskType:       extension.DiskTypeSSD,
						IsGraphDisk:    true,
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				makePVCWithStorageSize("pvc-a", "1", "", localPVSC.Name, "60Gi"),
			},
			ephemeralStorage: "60Gi",
			wantAssumedLocalPVCAllocs: []*localPVCAlloc{
				{
					Namespace:   v1.NamespaceDefault,
					Name:        "pvc-a",
					RequestSize: 60 * 1024 * 1024 * 1024,
					MountPoint:  "/home/t4",
				},
			},
			wantAllocatedVolumes: &allocatedVolumes{
				LocalPVAllocs: map[string]*localPVAlloc{},
				LocalPVCAllocs: map[string]*localPVCAlloc{
					"default/pvc-a": {
						Namespace:   v1.NamespaceDefault,
						Name:        "pvc-a",
						RequestSize: 60 * 1024 * 1024 * 1024,
						MountPoint:  "/home/t4",
					},
				},
				LocalVolumesInBytes: map[string]int64{
					"/home/t4": 120 * 1024 * 1024 * 1024,
				},
				LocalVolumeIOLimit: map[string]*unified.LocalStorageIOLimitInfo{
					"/home/t4": {},
				},
			},
			wantAllocatedVolumesAfterUnAssign: &allocatedVolumes{
				LocalPVAllocs:  map[string]*localPVAlloc{},
				LocalPVCAllocs: map[string]*localPVCAlloc{},
				LocalVolumesInBytes: map[string]int64{
					"/home/t4": 0,
				},
				LocalVolumeIOLimit: map[string]*unified.LocalStorageIOLimitInfo{
					"/home/t4": {},
				},
			},
		},
	}

	for _, item := range table {
		t.Run(item.name, func(t *testing.T) {
			if !item.enableLocalVolumeCapacity {
				utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
					string(features.EnableLocalVolumeCapacity): false,
				})
				defer func() {
					utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
						string(features.EnableLocalVolumeCapacity): true,
					})
				}()
			}
			if item.enableLocalVolumeIOLimit {
				utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
					string(features.EnableLocalVolumeIOLimit): true,
				})
				defer func() {
					utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
						string(features.EnableLocalVolumeIOLimit): false,
					})
				}()
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			opts := []runtime.Option{
				runtime.WithClientSet(client),
				runtime.WithInformerFactory(informerFactory),
				runtime.WithSnapshotSharedLister(newTestSharedLister(nil, []*v1.Node{item.node})),
			}
			fh, err := runtime.NewFramework(context.TODO(), nil, nil, opts...)
			if err != nil {
				t.Fatal(err)
			}

			t.Log("Feed testing data and wait for them to be synced")
			client.StorageV1().StorageClasses().Create(context.Background(), immediateSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), waitSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), localPVSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), lvmSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), quotaPathSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), multiVolumeLocalPVSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), lvmIOLimitSC, metav1.CreateOptions{})
			client.StorageV1().StorageClasses().Create(context.Background(), quotaPathIOLimitSC, metav1.CreateOptions{})

			if item.node != nil {
				client.CoreV1().Nodes().Create(context.Background(), item.node, metav1.CreateOptions{})
			}
			for _, pvc := range item.pvcs {
				client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
			}
			for _, pv := range item.pvs {
				client.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
			}
			if item.ephemeralStorage != "" {
				item.pod.Spec.Containers = append(item.pod.Spec.Containers, v1.Container{
					Name: "test-local-pv-ephemeral-storage",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceEphemeralStorage: resource.MustParse(item.ephemeralStorage),
						},
					},
				})
			}

			t.Log("Start informer factory after initialization")
			informerFactory.Start(ctx.Done())

			t.Log("Wait for all started informers' cache were synced")
			informerFactory.WaitForCacheSync(ctx.Done())

			args := &config.VolumeBindingArgs{
				BindTimeoutSeconds: 300,
			}

			pl, err := New(args, fh)
			if err != nil {
				t.Fatal(err)
			}

			t.Log("Verify")

			if item.localInfo != nil {
				data, err := json.Marshal(item.localInfo)
				if err != nil {
					t.Fatal(err)
				}
				metav1.SetMetaDataAnnotation(&item.node.ObjectMeta, extension.AnnotationLocalInfo, string(data))
			}

			nodeLocalVolumeInfo, err := newNodeLocalVolumeInfo(item.node)
			assert.NoError(t, err)
			var nodeStorageInfo *NodeStorageInfo
			pl.(*VolumeBinding).nodeStorageCache.UpdateOnNode(item.node.Name, func(info *NodeStorageInfo) {
				nodeStorageInfo = info
				info.UpdateNodeLocalVolumeInfo(nodeLocalVolumeInfo)
				for _, v := range item.pvs {
					info.AddLocalPVAlloc(&localPVAlloc{
						Name:     v.Name,
						Capacity: v.Spec.Capacity.Storage().Value(),
					})
				}
			})

			state := framework.NewCycleState()
			t.Logf("Verify: call PreFilter and check status")
			_, gotPreFilterStatus := pl.(framework.PreFilterPlugin).PreFilter(ctx, state, item.pod)
			if !gotPreFilterStatus.IsSuccess() {
				// scheduler framework will skip Filter if PreFilter fails
				return
			}

			t.Logf("Verify: check state after prefilter phase")
			stateData, err := getStateData(state)
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("Verify: call Filter and check status")
			nodeInfo := framework.NewNodeInfo()
			nodeInfo.SetNode(item.node)
			gotStatus := pl.(framework.FilterPlugin).Filter(ctx, state, item.pod, nodeInfo)
			if !gotStatus.IsSuccess() {
				t.Fatal(gotStatus.Message())
			}

			if item.assignedSystemDisk != "" {
				quantity := resource.MustParse(item.assignedSystemDisk)
				graphDiskPath := nodeStorageInfo.GraphDiskPath
				nodeStorageInfo.Used.VolumeSize[graphDiskPath] = quantity.Value()
			}
			for volume, s := range item.assignedLocalVolumes {
				quantity := resource.MustParse(s)
				nodeStorageInfo.Used.VolumeSize[volume] = quantity.Value()
			}
			gotStatus = pl.(framework.FilterPlugin).Filter(ctx, state, item.pod, nodeInfo)
			if !gotStatus.IsSuccess() {
				t.Fatal(gotStatus.Message())
			}

			assumedPod := item.pod.DeepCopy()
			assumedPod.Spec.NodeName = item.node.Name
			gotStatus = pl.(framework.ReservePlugin).Reserve(ctx, state, assumedPod, item.node.Name)
			if !reflect.DeepEqual(gotStatus, item.wantAssignStatus) {
				t.Errorf("assign status does not match: %v, want: %v", gotStatus, item.wantAssignStatus)
			}
			if !reflect.DeepEqual(item.wantAssumedLocalPVCAllocs, stateData.podVolumesByNode[item.node.Name].AssumedLocalPVCAllocs) {
				t.Errorf("assumedLocalPVCAllocs got after assign does not match: %v, want: %v", stateData.podVolumesByNode[item.node.Name].AssumedLocalPVCAllocs, item.wantAssumedLocalPVCAllocs)
			}
			tmpAllocatedVolumes := &allocatedVolumes{
				LocalVolumesInBytes: nodeStorageInfo.Used.VolumeSize,
				LocalPVAllocs:       nodeStorageInfo.localPVAllocs,
				LocalPVCAllocs:      nodeStorageInfo.localPVCAllocs,
				LocalVolumeIOLimit:  nodeStorageInfo.Used.IOLimits,
			}
			if !reflect.DeepEqual(tmpAllocatedVolumes, item.wantAllocatedVolumes) {
				t.Errorf("allocatedVolumes doest not match: %v, want: %v", tmpAllocatedVolumes, item.wantAllocatedVolumes)
			}
			pl.(framework.ReservePlugin).Unreserve(ctx, state, assumedPod, item.node.Name)
			tmpAllocatedVolumes = &allocatedVolumes{
				LocalVolumesInBytes: nodeStorageInfo.Used.VolumeSize,
				LocalPVAllocs:       nodeStorageInfo.localPVAllocs,
				LocalPVCAllocs:      nodeStorageInfo.localPVCAllocs,
				LocalVolumeIOLimit:  nodeStorageInfo.Used.IOLimits,
			}
			if item.wantAllocatedVolumesAfterUnAssign != nil && !reflect.DeepEqual(tmpAllocatedVolumes, item.wantAllocatedVolumesAfterUnAssign) {
				t.Errorf("allocatedVolumes got after unassign does not match: %v, want: %v", tmpAllocatedVolumes, item.wantAllocatedVolumesAfterUnAssign)
			}
		})
	}
}

func TestVolumeBinding_replaceStorageClassNameIfNeeded(t *testing.T) {
	tests := []struct {
		name                string
		node                *v1.Node
		affinityECI         bool
		pvc                 *v1.PersistentVolumeClaim
		defaultStorageClass string
		want                string
	}{
		{
			name:                "Pod doesn't affinity ECI, storageClass is the same as origin",
			node:                &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{extension.LabelNodeType: extension.VKType}}},
			affinityECI:         false,
			pvc:                 makePVC("test-pvc", "", lvmSC.Name),
			defaultStorageClass: otherSC.Name,
			want:                lvmSC.Name,
		},
		{
			name:                "Node is not virtual kubelet, storageClass is the same as origin",
			node:                &v1.Node{},
			affinityECI:         true,
			pvc:                 makePVC("test-pvc", "", lvmSC.Name),
			defaultStorageClass: otherSC.Name,
			want:                lvmSC.Name,
		},
		{
			name:                "storageClass doesn't SupportLocalPV and SupportLVMOrQuotaPathOrDevice, storageClass is the same as origin",
			node:                &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{extension.LabelNodeType: extension.VKType}}},
			affinityECI:         true,
			pvc:                 makePVC("test-pvc", "", cloudSC1.Name),
			defaultStorageClass: otherSC.Name,
			want:                cloudSC1.Name,
		},
		{
			name:                "defaultStorageClass is nil, storageClass is the same as origin",
			node:                &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{extension.LabelNodeType: extension.VKType}}},
			affinityECI:         true,
			pvc:                 makePVC("test-pvc", "", lvmSC.Name),
			defaultStorageClass: "",
			want:                lvmSC.Name,
		},
		{
			name:                "storageClass SupportLocalPV, storageClass is the same as default",
			node:                &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{extension.LabelNodeType: extension.VKType}}},
			affinityECI:         true,
			pvc:                 makePVC("test-pvc", "", localPVSC.Name),
			defaultStorageClass: otherSC.Name,
			want:                otherSC.Name,
		},
		{
			name:                "storageClass is '', storageClass is the same as default",
			node:                &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{extension.LabelNodeType: extension.VKType}}},
			affinityECI:         true,
			pvc:                 makePVC("test-pvc", "", ""),
			defaultStorageClass: otherSC.Name,
			want:                "",
		},
		{
			name:                "storageClass SupportLVMOrQuotaPathOrDevice, storageClass is the same as default",
			node:                &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{extension.LabelNodeType: extension.VKType}}},
			affinityECI:         true,
			pvc:                 makePVC("test-pvc", "", lvmSC.Name),
			defaultStorageClass: otherSC.Name,
			want:                otherSC.Name,
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {

			var pod *v1.Pod
			if tt.affinityECI {
				pod = makeAffinityECIPod([]*v1.PersistentVolumeClaim{tt.pvc})
			} else {
				pod = makePod([]*v1.PersistentVolumeClaim{tt.pvc})
			}
			eci.DefaultECIProfile.DefaultStorageClass = tt.defaultStorageClass
			client := &fake.Clientset{}
			informerFactory := informers.NewSharedInformerFactory(client, controller.NoResyncPeriodFunc())
			classInformer := informerFactory.Storage().V1().StorageClasses()
			classes := []*storagev1.StorageClass{localPVSC, lvmSC, quotaPathIOLimitSC, otherSC}
			for _, class := range classes {
				if err := classInformer.Informer().GetIndexer().Add(class); err != nil {
					t.Fatalf("Failed to add storage class to internal cache: %v", err)
				}
			}
			opts := []runtime.Option{
				runtime.WithClientSet(client),
				runtime.WithInformerFactory(informerFactory),
				runtime.WithSnapshotSharedLister(newTestSharedLister(nil, []*v1.Node{tt.node})),
			}
			fh, err := runtime.NewFramework(context.TODO(), nil, nil, opts...)
			if err != nil {
				t.Fatal(err)
			}
			pl := &VolumeBinding{
				handle:      fh,
				classLister: classInformer.Lister(),
			}

			podVolumes := &PodVolumes{
				StaticBindings:    []*BindingInfo{makeBinding(tt.pvc, nil)},
				DynamicProvisions: []*v1.PersistentVolumeClaim{tt.pvc},
			}

			pl.replaceStorageClassNameIfNeeded(podVolumes, pod, tt.node.Name)
			for _, bindingInfo := range podVolumes.StaticBindings {
				assert.Equal(t, tt.want, storagehelpers.GetPersistentVolumeClaimClass(bindingInfo.pvc))
			}
			for _, pvc := range podVolumes.DynamicProvisions {
				assert.Equal(t, tt.want, storagehelpers.GetPersistentVolumeClaimClass(pvc))
			}
		})
	}
}

func TestVolumeBinding_setProvisionDeniedIfNeed(t *testing.T) {
	tests := []struct {
		name string
		node *v1.Node
		pvc  *v1.PersistentVolumeClaim
		want map[string]string
	}{
		{
			name: "Node is not ACS virtual node, pvc is the same as origin",
			node: &v1.Node{},
			pvc:  makePVC("test-pvc", "", lvmSC.Name),
			want: nil,
		},
		{
			name: "Node is ACS virtual, pvc changed",
			node: &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"type": unified.ACSType}}},
			pvc:  makePVC("test-pvc", "", lvmSC.Name),
			want: map[string]string{
				unified.AnnotationProvisionDenied: "true",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fake.Clientset{}
			informerFactory := informers.NewSharedInformerFactory(client, controller.NoResyncPeriodFunc())
			classInformer := informerFactory.Storage().V1().StorageClasses()
			opts := []runtime.Option{
				runtime.WithClientSet(client),
				runtime.WithInformerFactory(informerFactory),
				runtime.WithSnapshotSharedLister(newTestSharedLister(nil, []*v1.Node{tt.node})),
			}
			fh, err := runtime.NewFramework(context.TODO(), nil, nil, opts...)
			if err != nil {
				t.Fatal(err)
			}
			pl := &VolumeBinding{
				handle:      fh,
				classLister: classInformer.Lister(),
			}

			podVolumes := &PodVolumes{
				DynamicProvisions: []*v1.PersistentVolumeClaim{tt.pvc},
			}

			pl.setProvisionDeniedIfNeed(podVolumes, tt.node.Name)
			for _, pvc := range podVolumes.DynamicProvisions {
				assert.Equal(t, tt.want, pvc.Annotations)
			}
		})
	}
}
