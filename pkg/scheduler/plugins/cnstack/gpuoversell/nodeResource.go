package gpuoversell

import (
	v1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sync"
)

type NodeResource struct {
	mu                    sync.RWMutex
	resourceName          v1.ResourceName
	allocatedPodResources map[apitypes.UID]*PodResource
	total                 int64
}

func (n *NodeResource) Clone() *NodeResource {
	n.mu.RLock()
	defer n.mu.RUnlock()
	clone := &NodeResource{
		resourceName:          n.resourceName,
		allocatedPodResources: make(map[apitypes.UID]*PodResource, len(n.allocatedPodResources)),
		total:                 n.total,
	}
	for _, pr := range n.allocatedPodResources {
		clone.allocatedPodResources[pr.PodUID] = pr.Clone()
	}
	return clone
}

func NewNodeResourceCache(resourceName v1.ResourceName) *NodeResource {
	return &NodeResource{
		resourceName:          resourceName,
		allocatedPodResources: make(map[apitypes.UID]*PodResource),
	}
}

func (n *NodeResource) AddPodResource(pod *v1.Pod) {
	pr := getPodResources(n.resourceName, pod)
	if pr == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.allocatedPodResources[pr.PodUID] = pr
}

func (n *NodeResource) RemovePodResource(pod *v1.Pod) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.allocatedPodResources, pod.UID)
}

func getPodResources(resourceName v1.ResourceName, pod *v1.Pod) *PodResource {
	pr := NewPodResource(pod)
	for i, c := range pod.Spec.Containers {
		if v, ok := c.Resources.Limits[resourceName]; ok && v.Value() > 0 {
			pr.AllocatedResources[i] = v.Value()
		}
	}
	if len(pr.AllocatedResources) > 0 {
		return pr
	}
	return nil
}

func (n *NodeResource) SetTotal(total int64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.total = total
}

func (n *NodeResource) GetAllocated() int64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	allocated := int64(0)
	for _, pr := range n.allocatedPodResources {
		for _, v := range pr.AllocatedResources {
			allocated = allocated + v
		}
	}
	return allocated
}
