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
	"fmt"
	"reflect"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

type NodeStorageInfoCache struct {
	lock  sync.Mutex
	items map[string]*NodeStorageInfo
}

func NewNodeStorageCache() *NodeStorageInfoCache {
	return &NodeStorageInfoCache{
		items: map[string]*NodeStorageInfo{},
	}
}

func (c *NodeStorageInfoCache) DeleteNodeStorageInfo(nodeName string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.items, nodeName)
}

func (c *NodeStorageInfoCache) UpdateOnNode(nodeName string, fn func(info *NodeStorageInfo)) {
	c.lock.Lock()
	nodeStorageInfo := c.items[nodeName]
	if nodeStorageInfo == nil {
		nodeStorageInfo = NewNodeStorageInfo()
		c.items[nodeName] = nodeStorageInfo
	}
	c.lock.Unlock()
	if nodeStorageInfo != nil {
		nodeStorageInfo.lock.Lock()
		defer nodeStorageInfo.lock.Unlock()
		fn(nodeStorageInfo)
	}
}

func (c *NodeStorageInfoCache) GetNodeStorageInfo(nodeName string) *NodeStorageInfo {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.items[nodeName]
}

type NodeStorageInfo struct {
	lock             sync.Mutex
	GraphDiskPath    string
	LocalVolumesInfo map[string]*LocalVolume
	localPVCAllocs   map[string]*localPVCAlloc
	localPVAllocs    map[string]*localPVAlloc
	Total            *LocalVolumeResource
	Used             *LocalVolumeResource
	Free             *LocalVolumeResource

	allocSet sets.String
}

type localPVCAlloc struct {
	Namespace   string                           `json:"namespace,omitempty"`
	Name        string                           `json:"name,omitempty"`
	RequestSize int64                            `json:"requestSize,omitempty"`
	MountPoint  string                           `json:"mountPoint,omitempty"`
	IOLimit     *unified.LocalStorageIOLimitInfo `json:"iolimit,omitempty"`
}

type localPVAlloc struct {
	Name     string `json:"name,omitempty"`
	ClaimRef string `json:"claimRef,omitempty"`
	Capacity int64  `json:"capacity,omitempty"`
}

func NewNodeStorageInfo() *NodeStorageInfo {
	return &NodeStorageInfo{
		LocalVolumesInfo: make(map[string]*LocalVolume),
		localPVCAllocs:   make(map[string]*localPVCAlloc),
		localPVAllocs:    make(map[string]*localPVAlloc),
		Total:            newLocalVolumeResource(),
		Used:             newLocalVolumeResource(),
		Free:             newLocalVolumeResource(),
		allocSet:         sets.NewString(),
	}
}

func (s *NodeStorageInfo) AddLocalVolumeAlloc(pod *corev1.Pod, ephemeralStorageSize, localInlineVolumeSize int64) {
	podName := getPodName(pod)
	if s.allocSet.Has(podName) {
		return
	}
	s.allocSet.Insert(podName)
	s.Used.addLocalVolumeAlloc(s.GraphDiskPath, ephemeralStorageSize, localInlineVolumeSize)
	s.resetFreeResource()
}

func (s *NodeStorageInfo) DeleteLocalVolumeAlloc(pod *corev1.Pod, ephemeralStorageSize, localInlineVolumeSize int64) {
	podName := getPodName(pod)
	if !s.allocSet.Has(podName) {
		return
	}
	s.allocSet.Delete(podName)
	s.Used.deleteLocalVolumeAlloc(s.GraphDiskPath, ephemeralStorageSize, localInlineVolumeSize)
	s.resetFreeResource()
}

func (s *NodeStorageInfo) AddLocalPVCAlloc(pvcAlloc *localPVCAlloc) {
	s.addLocalPVCAlloc(pvcAlloc)
	s.resetFreeResource()
}

func (s *NodeStorageInfo) DeleteLocalPVCAlloc(pvcAlloc *localPVCAlloc) {
	s.deleteLocalPVCAlloc(pvcAlloc)
	s.resetFreeResource()
}

func (s *NodeStorageInfo) AddLocalPVAlloc(pvAlloc *localPVAlloc) {
	s.addLocalPVAlloc(pvAlloc)
	s.resetFreeResource()
}

func (s *NodeStorageInfo) DeleteLocalPVAlloc(pvAlloc *localPVAlloc) {
	s.deleteLocalPVAlloc(pvAlloc)
	s.resetFreeResource()
}

func (s *NodeStorageInfo) GetFree() *LocalVolumeResource {
	return s.Free
}

func (s *NodeStorageInfo) resetFreeResource() {
	free := s.Free
	free.CopyFrom(s.Total)
	free.RealSubtract(s.Used)
}

func (s *NodeStorageInfo) UpdateNodeLocalVolumeInfo(nodeLocalVolumeInfo *NodeLocalVolumeInfo) {
	if nodeLocalVolumeInfo == nil {
		return
	}
	graphDiskPath := nodeLocalVolumeInfo.SystemDisk.HostPath
	newVolumeInfos := make(map[string]*LocalVolume)
	newVolumeInfos[graphDiskPath] = nodeLocalVolumeInfo.SystemDisk
	for path, volume := range nodeLocalVolumeInfo.LocalVolumes {
		newVolumeInfos[path] = volume
	}
	if reflect.DeepEqual(s.LocalVolumesInfo, newVolumeInfos) {
		return
	}

	s.LocalVolumesInfo = newVolumeInfos
	s.GraphDiskPath = graphDiskPath
	s.Total.UpdateLocalVolumeCapacity(newVolumeInfos)
	s.resetFreeResource()
}

func (s *NodeStorageInfo) addLocalPVCAlloc(pvcAlloc *localPVCAlloc) {
	key := fmt.Sprintf("%s/%s", pvcAlloc.Namespace, pvcAlloc.Name)
	used := s.Used
	if oldPVCAlloc, ok := s.localPVCAllocs[key]; ok {
		if allocated, ok := used.VolumeSize[oldPVCAlloc.MountPoint]; ok {
			allocated -= oldPVCAlloc.RequestSize
			if allocated < 0 {
				allocated = 0
			}
			used.VolumeSize[oldPVCAlloc.MountPoint] = allocated
		} else {
			klog.Errorf("unrecognized mount point for VolumeSize: %s, PVC: %s", oldPVCAlloc.MountPoint, key)
		}
		if allocatedIoLimit, ok := used.IOLimits[oldPVCAlloc.MountPoint]; ok {
			allocatedIoLimit.Sub(oldPVCAlloc.IOLimit)
			used.IOLimits[oldPVCAlloc.MountPoint] = allocatedIoLimit
		} else {
			klog.Errorf("unrecognized mount point for IOLimit: %s, PVC: %s", oldPVCAlloc.MountPoint, key)
		}
	}
	s.localPVCAllocs[key] = pvcAlloc
	used.VolumeSize[pvcAlloc.MountPoint] += pvcAlloc.RequestSize
	if _, ok := used.IOLimits[pvcAlloc.MountPoint]; !ok {
		used.IOLimits[pvcAlloc.MountPoint] = &unified.LocalStorageIOLimitInfo{}
	}
	used.IOLimits[pvcAlloc.MountPoint].Add(pvcAlloc.IOLimit)
}

func (s *NodeStorageInfo) deleteLocalPVCAlloc(pvcAlloc *localPVCAlloc) {
	key := fmt.Sprintf("%s/%s", pvcAlloc.Namespace, pvcAlloc.Name)
	if oldPVCAlloc, ok := s.localPVCAllocs[key]; ok {
		if pvcAlloc.MountPoint != "" && pvcAlloc.MountPoint != oldPVCAlloc.MountPoint {
			klog.Errorf("conflict localPVCAlloc on mountPath, old: %s, current: %s", oldPVCAlloc.MountPoint, pvcAlloc.MountPoint)
			return
		}
		if pvcAlloc.RequestSize != oldPVCAlloc.RequestSize {
			klog.Errorf("conflict localPVCAlloc on requestSize, old: %d, current: %d", oldPVCAlloc.RequestSize, pvcAlloc.RequestSize)
			return
		}
		if !pvcAlloc.IOLimit.Equal(oldPVCAlloc.IOLimit) {
			klog.Errorf("conflict localPVCAlloc on request IOLimit, old: %v, current: %v", oldPVCAlloc.IOLimit, pvcAlloc.IOLimit)
			return
		}
		used := s.Used
		if allocated, ok := used.VolumeSize[pvcAlloc.MountPoint]; ok {
			allocated -= pvcAlloc.RequestSize
			if allocated < 0 {
				allocated = 0
			}
			used.VolumeSize[pvcAlloc.MountPoint] = allocated
		}
		if allocatedIoLimit, ok := used.IOLimits[pvcAlloc.MountPoint]; ok {
			allocatedIoLimit.Sub(oldPVCAlloc.IOLimit)
			used.IOLimits[oldPVCAlloc.MountPoint] = allocatedIoLimit
		}
		delete(s.localPVCAllocs, key)
	} else {
		klog.Errorf("failed to found localPVCAlloc: %s", key)
	}
}

func (s *NodeStorageInfo) addLocalPVAlloc(pvAlloc *localPVAlloc) {
	//
	// Volume Binding Plugin运行时，会可能存在两种情况：
	//   1. 复用PV。PVC与Available PV匹配成功就可以复用。在PreBind时更新PV对象，触发绑定PVC。
	//   2. 动态创建PV。在PreBind时更新PVC，设置AnnSelectedNode指定挂载节点，CSI感知后创建PV。
	// 单盘LocalPV与多盘LocalPV在动态创建PV场景下，都是在Assign账本通过PVC预扣账本。
	// 但在复用PV场景下，单盘LocalPV允许复用，而多盘不允许复用（目前代码强制绕过了复用逻辑）
	// 复用PV场景需要通过PV Informer跟踪PV状态，并通过PV更新账本。
	// FailOver时，多盘LocalPV要求PVC存在的情况下才可以更新账本；
	// 而单盘LocalPV要求PV处于Available或者Bound状态才可以更新账本。
	//
	graphDiskPath := s.GraphDiskPath
	used := s.Used
	if oldPVAlloc, ok := s.localPVAllocs[pvAlloc.Name]; ok {
		alloc := used.VolumeSize[graphDiskPath]
		alloc -= oldPVAlloc.Capacity
		if alloc < 0 {
			alloc = 0
		}
		used.VolumeSize[graphDiskPath] = alloc
	}
	s.localPVAllocs[pvAlloc.Name] = pvAlloc
	used.VolumeSize[graphDiskPath] += pvAlloc.Capacity
	if pvAlloc.ClaimRef != "" {
		if pvcAlloc, ok := s.localPVCAllocs[pvAlloc.ClaimRef]; ok {
			s.deleteLocalPVCAlloc(pvcAlloc)
		}
	}
}

func (s *NodeStorageInfo) deleteLocalPVAlloc(pvAlloc *localPVAlloc) {
	if oldPVAlloc, ok := s.localPVAllocs[pvAlloc.Name]; ok {
		if oldPVAlloc.Capacity != pvAlloc.Capacity {
			klog.Errorf("conflict localPVAlloc on capacity, old: %d, current: %d",
				oldPVAlloc.Capacity, pvAlloc.Capacity)
			return
		}
		graphDiskPath := s.GraphDiskPath
		used := s.Used
		alloc := used.VolumeSize[graphDiskPath]
		alloc -= pvAlloc.Capacity
		if alloc < 0 {
			alloc = 0
		}
		used.VolumeSize[graphDiskPath] = alloc
		delete(s.localPVAllocs, pvAlloc.Name)
	}
}
