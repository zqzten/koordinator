package gpuoversell

import (
	"sync"

	v1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
)

type NodeResource struct {
	mu                    sync.RWMutex
	resourceName          v1.ResourceName
	allocatedPodResources map[apitypes.UID]*PodResource
	total                 int
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

func getPodResources(resourceName v1.ResourceName, pod *v1.Pod) *PodResource {
	pr := NewPodResource(pod)
	for i, c := range pod.Spec.Containers {
		if v, ok := c.Resources.Limits[resourceName]; ok && v.Value() > 0 {
			pr.AllocatedResources[i] = int(v.Value())
		}
	}
	if len(pr.AllocatedResources) > 0 {
		return pr
	}
	return nil
}

func (n *NodeResource) GetTotal() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.total
}

func (n *NodeResource) SetTotal(total int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.total = total
}

func (n *NodeResource) GetAllocated() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	allocated := 0
	for _, pr := range n.allocatedPodResources {
		for _, v := range pr.AllocatedResources {
			allocated = allocated + v
		}
	}
	return allocated
}

func (n *NodeResource) GetAllocatedPodResource(podUID apitypes.UID) *PodResource {
	n.mu.RLock()
	defer n.mu.RUnlock()
	p, ok := n.allocatedPodResources[podUID]
	if !ok {
		return nil
	}
	return p.Clone()
}

func (n *NodeResource) PodResourceIsCached(podUID apitypes.UID) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	_, ok := n.allocatedPodResources[podUID]
	return ok
}

func (n *NodeResource) RemoveAllocatedPodResources(podUID apitypes.UID) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.allocatedPodResources, podUID)
}

func getNodeGPUCount(node *v1.Node) int {
	val, ok := node.Status.Allocatable[v1.ResourceName(GPUResourceCountName)]
	if !ok {
		return int(0)
	}
	return int(val.Value())
}
