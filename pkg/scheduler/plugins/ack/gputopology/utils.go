package gputopology

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/gputopology"
)

func GetPodRequestResource(pod *v1.Pod, resourceName v1.ResourceName) int {
	count := 0
	GetPodRequestResourcesByFilter(pod, func(index ContainerIndex, resourceList v1.ResourceList) {
		if val, ok := resourceList[resourceName]; ok {
			count += int(val.Value())
		}
	})
	return count
}

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

// GetPodRequestResourcesByFilter gets the resources by filter function
func GetPodRequestResourcesByFilter(pod *v1.Pod, filter func(index ContainerIndex, resourceList v1.ResourceList)) {
	for index, container := range pod.Spec.Containers {
		filter(ContainerIndex(index), container.Resources.Limits)
	}
}

// GetContainerRequestResourceByName gets the value of containers for target resource name
func GetContainerRequestResourceByName(resourceName v1.ResourceName, pod *v1.Pod) map[gputopology.ContainerIndex]int {
	total := map[gputopology.ContainerIndex]int{}
	containers := pod.Spec.Containers
	for index, container := range containers {
		if val, ok := container.Resources.Limits[v1.ResourceName(resourceName)]; ok && int(val.Value()) != 0 {
			total[gputopology.ContainerIndex(index)] = int(val.Value())
		}
	}
	return total
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
func IsMyPod(pod *v1.Pod) bool {
	return len(GetPodRequestResourcesByNames(pod, GPUTopologyResourceName)) != 0
}

func GetPodTopologyAllocation(podUID apitypes.UID, annotations map[string]string) (*gputopology.PodTopologyAllocation, error) {
	if len(annotations) == 0 {
		return nil, nil
	}
	allocationInfo := &AllocationInfo{}
	val, ok := annotations[GPUTopologyAllocationKey]
	if !ok {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(val), allocationInfo); err != nil {
		return nil, err
	}
	allocation := gputopology.NewPodTopologyAllocation(podUID)
	allocation.AllocatedGPUs = allocationInfo.AllocatedGPUs
	allocation.VisibleGPUs = allocationInfo.VisibleGPUs
	allocation.SetAssumeTime(allocationInfo.AssumeTime)
	return allocation, nil
}

func GetResourceCapacity(resourceName v1.ResourceName, node *v1.Node) int {
	val, ok := node.Status.Capacity[resourceName]
	if !ok {
		return 0
	}
	return int(val.Value())
}

func getPodGroupAndReplica(pod *v1.Pod) (string, string, int, error) {
	gangGroupName, gangOk := pod.Labels[GangGroupNameLabelKey]
	topoGroupName, topoOk := pod.Labels[TopologyGroupNameLabelKey]
	var getReplica = func(key string) (int, error) {
		replicaStr, exist := pod.Labels[key]
		if !exist || replicaStr == "" {
			return 1, nil
		}
		replica, err := strconv.Atoi(replicaStr)
		if err != nil {
			return -1, fmt.Errorf("can't convert replica from string to int: %v", err)
		}
		if replica <= 0 {
			return -1, fmt.Errorf("invalid replica %v,it should great than 0", replica)
		}
		return replica, nil
	}
	switch {
	case gangOk && topoOk:
		return "", "", 0, fmt.Errorf("gang group label and gpu topology group label cannot appear together")
	case gangOk && !topoOk:
		replica, err := getReplica(GangGroupReplicaLabelKey)
		return GangGroupNameLabelKey, gangGroupName, replica, err
	case !gangOk && topoOk:
		replica, err := getReplica(TopologyGroupReplicaLabelKey)
		return TopologyGroupNameLabelKey, topoGroupName, replica, err

	}
	return "", string(pod.UID), 1, nil
}

func getTTLFromLabel(pod *v1.Pod) (int, error) {
	val, ok := pod.Labels[TopologyGroupTTLLabelKey]
	if !ok {
		return 0, nil
	}
	if _, ok := pod.Labels[GangGroupNameLabelKey]; ok {
		return 0, fmt.Errorf("should not set label %v when gang group label is enabled", TopologyGroupTTLLabelKey)
	}
	ttl, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("failed to convert TTL string to int: %v", err)
	}
	return ttl, nil
}

// update pod annotations
func UpdatePodAnnotations(client clientset.Interface, pod *v1.Pod, annotations map[string]string) (*v1.Pod, error) {
	newPod := updateAnnotations(pod, annotations)
	var updatedPod *v1.Pod
	var err error
	updatedPod, err = client.CoreV1().Pods(newPod.ObjectMeta.Namespace).Update(context.TODO(), newPod, metav1.UpdateOptions{})
	if err != nil {
		// the object has been modified; please apply your changes to the latest version and try again
		if strings.Contains(err.Error(), "the object has been modified") {
			// retry
			oldPod, err := client.CoreV1().Pods(pod.ObjectMeta.Namespace).Get(context.TODO(), pod.ObjectMeta.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			newPod = updateAnnotations(oldPod, annotations)
			return client.CoreV1().Pods(newPod.ObjectMeta.Namespace).Update(context.TODO(), newPod, metav1.UpdateOptions{})
		}
		return nil, err
	}
	return updatedPod, nil
}

func updateAnnotations(oldPod *v1.Pod, annotations map[string]string) *v1.Pod {
	newPod := oldPod.DeepCopy()
	if len(newPod.ObjectMeta.Annotations) == 0 {
		newPod.ObjectMeta.Annotations = map[string]string{}
	}
	for k, v := range annotations {
		newPod.ObjectMeta.Annotations[k] = v
	}
	return newPod
}

func PatchPodAnnotations(client clientset.Interface, annotations map[string]string, podNamespace, podName string) error {
	patchAnnotations := map[string]interface{}{
		"metadata": map[string]map[string]string{
			"annotations": annotations,
		}}
	patchBytes, err := json.Marshal(patchAnnotations)
	if err != nil {
		return fmt.Errorf("failed to generate annotation patch string: %v", err)
	}
	return RetryOnConflictOrTooManyRequests(func() error {
		_, err := client.CoreV1().Pods(podNamespace).Patch(context.TODO(), podName, apitypes.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
		return err
	})
}

func RetryOnConflictOrTooManyRequests(fn func() error) error {
	return retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return errors.IsConflict(err) || errors.IsTooManyRequests(err)
	}, fn)
}
