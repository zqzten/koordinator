package limitaware

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

type nodeLimitInfo struct {
	allocSet         sets.String
	allocated        *framework.Resource
	nonZeroAllocated *framework.Resource
}

func newNodeLimitInfo() *nodeLimitInfo {
	return &nodeLimitInfo{
		allocSet:         sets.NewString(),
		allocated:        &framework.Resource{},
		nonZeroAllocated: &framework.Resource{},
	}
}

func (n *nodeLimitInfo) GetAllocated() *framework.Resource {
	return n.allocated.Clone()
}

func (n *nodeLimitInfo) GetNonZeroAllocated() *framework.Resource {
	return n.nonZeroAllocated.Clone()
}

func (n *nodeLimitInfo) isZero() bool {
	return n.allocSet.Len() == 0
}

func (n *nodeLimitInfo) addPod(nodeName string, pod *corev1.Pod) {
	n.allocated = addPodToAllocated(n.allocated, getPodResourceLimit(pod, false))
	n.nonZeroAllocated = addPodToAllocated(n.nonZeroAllocated, getPodResourceLimit(pod, true))
}

func addPodToAllocated(allocated *framework.Resource, podResourceLimit *framework.Resource) *framework.Resource {
	if podResourceLimit.MilliCPU == 0 &&
		podResourceLimit.Memory == 0 &&
		podResourceLimit.EphemeralStorage == 0 &&
		len(podResourceLimit.ScalarResources) == 0 {
		return allocated
	}
	if allocated == nil {
		allocated = framework.NewResource(nil)
	}
	allocated.MilliCPU += podResourceLimit.MilliCPU
	allocated.Memory += podResourceLimit.Memory
	allocated.EphemeralStorage += podResourceLimit.EphemeralStorage
	for rName, rQuant := range podResourceLimit.ScalarResources {
		if allocated.ScalarResources == nil {
			allocated.ScalarResources = map[corev1.ResourceName]int64{}
		}
		allocated.ScalarResources[rName] += rQuant
	}
	return allocated
}

func (n *nodeLimitInfo) deletePod(nodeName string, pod *corev1.Pod) {
	n.allocated = deletePodFromAllocated(n.allocated, getPodResourceLimit(pod, false))
	n.nonZeroAllocated = deletePodFromAllocated(n.nonZeroAllocated, getPodResourceLimit(pod, true))
}

func deletePodFromAllocated(allocated *framework.Resource, podResourceLimit *framework.Resource) *framework.Resource {
	if allocated == nil {
		return nil
	}

	if podResourceLimit.MilliCPU == 0 &&
		podResourceLimit.Memory == 0 &&
		podResourceLimit.EphemeralStorage == 0 &&
		len(podResourceLimit.ScalarResources) == 0 {
		return allocated
	}

	allocated.MilliCPU -= podResourceLimit.MilliCPU
	allocated.Memory -= podResourceLimit.Memory
	allocated.EphemeralStorage -= podResourceLimit.EphemeralStorage
	if len(allocated.ScalarResources) != 0 {
		for rName, rQuant := range podResourceLimit.ScalarResources {
			allocated.ScalarResources[rName] -= rQuant
			if allocated.ScalarResources[rName] == 0 {
				delete(allocated.ScalarResources, rName)
			}
		}
	}
	if len(allocated.ScalarResources) == 0 {
		allocated.ScalarResources = nil
	}
	return allocated
}
