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

package deviceshare

import (
	"encoding/json"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

type VFCache struct {
	mu            sync.Mutex
	allocations   map[string]*VFAllocation
	allocatedPods map[types.UID]*VFAllocation
}

type VFAllocation struct {
	VFs map[int]sets.String
}

func newVFCache() *VFCache {
	return &VFCache{
		allocations:   map[string]*VFAllocation{},
		allocatedPods: map[types.UID]*VFAllocation{},
	}
}

func (c *VFCache) setPod(oldPod, pod *corev1.Pod) {
	if pod.Spec.NodeName == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if oldPod != nil && oldPod.Spec.NodeName != "" {
		c.removePodInternal(oldPod, nil)
	}
	vfAllocation, err := getVFAllocations(pod)
	if err != nil {
		return
	}
	c.allocatedPods[pod.UID] = vfAllocation
	c.updateNodeAllocation(pod.Spec.NodeName, vfAllocation)
}

func (c *VFCache) updateNodeAllocation(nodeName string, vfAllocation *VFAllocation) {
	nodeAllocation := c.allocations[nodeName]
	if nodeAllocation == nil {
		nodeAllocation = vfAllocation
		c.allocations[nodeName] = nodeAllocation
		return
	}
	for minor, vfs := range vfAllocation.VFs {
		r := nodeAllocation.VFs[minor]
		if r == nil {
			r = sets.NewString()
			nodeAllocation.VFs[minor] = r
		}
		r.Insert(vfs.UnsortedList()...)
	}
}

func getVFAllocations(pod *corev1.Pod) (*VFAllocation, error) {
	deviceAllocations, err := apiext.GetDeviceAllocations(pod.Annotations)
	if err != nil {
		return nil, err
	}
	vfAllocation := VFAllocation{
		VFs: map[int]sets.String{},
	}
	allocations := deviceAllocations[schedulingv1alpha1.RDMA]
	if len(allocations) != 0 {
		for _, alloc := range allocations {
			if len(alloc.Extension) == 0 {
				continue
			}
			var extension unified.DeviceAllocationExtension
			if err := json.Unmarshal(alloc.Extension, &extension); err != nil {
				return nil, err
			}
			if extension.RDMAAllocatedExtension != nil && len(extension.RDMAAllocatedExtension.VFs) > 0 {
				vfs := sets.NewString()
				for _, vf := range extension.RDMAAllocatedExtension.VFs {
					vfs.Insert(vf.BusID)
				}
				vfAllocation.VFs[int(alloc.Minor)] = vfs
			}
		}
	}
	return &vfAllocation, nil
}

func (c *VFCache) removePodInternal(pod *corev1.Pod, vfAllocation *VFAllocation) {
	if pod != nil && pod.Spec.NodeName != "" {
		if vfAllocation == nil {
			var err error
			vfAllocation, err = getVFAllocations(pod)
			if err != nil {
				return
			}
		}
		delete(c.allocatedPods, pod.UID)
		nodeAllocations := c.allocations[pod.Spec.NodeName]
		if nodeAllocations != nil {
			for minor, allocations := range vfAllocation.VFs {
				vfs := nodeAllocations.VFs[minor]
				vfs.Delete(allocations.UnsortedList()...)
				if vfs.Len() == 0 {
					delete(nodeAllocations.VFs, minor)
				}
			}
			if len(nodeAllocations.VFs) == 0 {
				delete(c.allocations, pod.Spec.NodeName)
			}
		}
	}
}

func (c *VFCache) removePod(pod *corev1.Pod) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removePodInternal(pod, nil)
}

func (c *VFCache) assume(pod *corev1.Pod, allocation *VFAllocation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.allocatedPods[pod.UID] = allocation
	c.updateNodeAllocation(pod.Spec.NodeName, allocation)
}

func (c *VFCache) unassume(pod *corev1.Pod, allocation *VFAllocation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removePodInternal(pod, allocation)
}

func (c *VFCache) getAllocatedVFs(nodeName string) map[int]sets.String {
	c.mu.Lock()
	defer c.mu.Unlock()

	allocations := c.allocations[nodeName]
	if allocations == nil || len(allocations.VFs) == 0 {
		return nil
	}
	vfs := map[int]sets.String{}
	for k, v := range allocations.VFs {
		vfs[k] = sets.NewString(v.UnsortedList()...)
	}
	return vfs
}

func (c *VFCache) onPodAdd(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	c.setPod(nil, pod)
}

func (c *VFCache) onPodUpdate(oldObj, newObj interface{}) {
	newPod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}
	if util.IsPodTerminated(newPod) {
		c.removePod(newPod)
		return
	}
	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		return
	}
	c.setPod(oldPod, newPod)
}

func (c *VFCache) onPodDelete(obj interface{}) {
	var pod *corev1.Pod
	switch t := obj.(type) {
	case *corev1.Pod:
		pod = t
	case cache.DeletedFinalStateUnknown:
		pod, _ = t.Obj.(*corev1.Pod)
	}
	if pod == nil {
		return
	}
	c.removePod(pod)
}
