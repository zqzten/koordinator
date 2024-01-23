package runtime

import (
	v1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

// GetPodRequestResourcesByNames gets the value for target resource name
func GetPodRequestResourcesByNames(pod *v1.Pod, resourceNames ...v1.ResourceName) map[v1.ResourceName]int {
	result := map[v1.ResourceName]int{}
	for _, resourceName := range resourceNames {
		total := 0
		for _, containerRequest := range GetContainerRequestResourceByName(resourceName, pod) {
			total += containerRequest
		}
		// warning: only care value is large than 0
		if total != 0 {
			result[resourceName] = total
		}
	}
	return result
}

// GetContainerRequestResourceByName gets the value of containers for target resource name
func GetContainerRequestResourceByName(resourceName v1.ResourceName, pod *v1.Pod) map[types.ContainerIndex]int {
	total := map[types.ContainerIndex]int{}
	containers := pod.Spec.Containers
	for index, container := range containers {
		if val, ok := container.Resources.Limits[resourceName]; ok && int(val.Value()) != 0 {
			total[types.ContainerIndex(index)] = int(val.Value())
		}
	}
	return total
}

func GetNodeResourcesByNames(node *v1.Node, resourceNames ...v1.ResourceName) map[v1.ResourceName]int {
	result := map[v1.ResourceName]int{}
	for _, resourceName := range resourceNames {
		val, ok := node.Status.Allocatable[resourceName]
		if !ok {
			continue
		}
		// only care value is large than 0
		if val.Value() > 0 {
			result[resourceName] = int(val.Value())
		}
	}
	return result
}

func ContainersRequestResourcesAreTheSame(pod *v1.Pod, resourceNames ...v1.ResourceName) bool {
	containers := pod.Spec.Containers
	containerRequestResources := map[int]map[v1.ResourceName]bool{}
	for index, container := range containers {
		containerRequestResource := map[v1.ResourceName]bool{}
		for _, resourceName := range resourceNames {
			if val, ok := container.Resources.Limits[resourceName]; ok && int(val.Value()) != 0 {
				containerRequestResource[resourceName] = true
			}
		}
		if len(containerRequestResource) != 0 {
			containerRequestResources[index] = containerRequestResource
		}
	}
	var t map[v1.ResourceName]bool
	for _, containerRequestResource := range containerRequestResources {
		if t == nil {
			t = containerRequestResource
		}
		if len(t) != len(containerRequestResource) {
			return false
		}
		for r := range t {
			if _, ok := containerRequestResource[r]; !ok {
				return false
			}
		}
	}
	return true
}

// IsCompletedPod detects the pod whether is completed
func IsCompletedPod(pod *v1.Pod) bool {
	if pod.DeletionTimestamp != nil {
		return true
	}
	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
		return true
	}
	return false
}

// IsMyPod determines the pod is a pod that we care or not
func IsMyPod(pod *v1.Pod, resourceNames ...v1.ResourceName) bool {
	return len(GetPodRequestResourcesByNames(pod, resourceNames...)) != 0
}

// IsMyNode detects the node is device sharing node or not
func IsMyNode(node *v1.Node, resourceNames ...v1.ResourceName) bool {
	return len(GetNodeResourcesByNames(node, resourceNames...)) != 0
}
