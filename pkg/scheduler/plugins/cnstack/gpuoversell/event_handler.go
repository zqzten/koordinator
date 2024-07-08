package gpuoversell

import (
	"fmt"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/devicesharing/runtime"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

func addPod(resourceNames []corev1.ResourceName, pod *corev1.Pod, getNodeState func(nodeName string) *GPUOversellNodeState) {
	// if pod is not requested target resources,skip it
	if !runtime.IsMyPod(pod, resourceNames...) {
		return
	}
	// if node name is null,skip it
	if pod.Spec.NodeName == "" {
		return
	}
	// if pod is completed,skip it
	if runtime.IsCompletedPod(pod) {
		return
	}
	// if not found allocation,skip to handle it
	state := getNodeState(pod.Spec.NodeName)
	if state == nil {
		return
	}
	for _, resouceName := range resourceNames {
		state.GetNodeResource(resouceName).AddPodResource(pod)
	}
}

func AddPod(resourceNames []corev1.ResourceName, pod *corev1.Pod, nc *NodeCache) {
	addPod(resourceNames, pod, func(nodeName string) *GPUOversellNodeState {
		return nc.GetNodeState(nodeName)
	})
}

func UpdatePod(resourceNames []corev1.ResourceName, oldPod *corev1.Pod, newPod *corev1.Pod, nc *NodeCache) {
	if !runtime.IsMyPod(oldPod, resourceNames...) {
		return
	}
	if oldPod.Status.Phase != corev1.PodRunning {
		return
	}
	if runtime.IsCompletedPod(newPod) {
		RemovePod(resourceNames, newPod, nc)
		return
	}
	AddPod(resourceNames, newPod, nc)
}

func RemovePod(resourceNames []corev1.ResourceName, pod *corev1.Pod, nc *NodeCache) {
	removePod(resourceNames, pod, func(nodeName string) *GPUOversellNodeState {
		return nc.GetNodeState(nodeName)
	})
}

func removePod(resourceNames []corev1.ResourceName, pod *corev1.Pod, getNodeState func(nodeName string) *GPUOversellNodeState) {
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	if !runtime.IsMyPod(pod, resourceNames...) {
		klog.V(6).Infof("pod %v has no resources %v request,skip to free using devices of it", podFullName, resourceNames)
		return
	}
	nodeName := pod.Spec.NodeName
	if nodeName == "" {
		klog.V(6).Infof("node name of pod %v is null,skip to free using devices", podFullName)
		return
	}
	ns := getNodeState(nodeName)
	allocated := []corev1.ResourceName{}
	removed := false
	podRequestResources := runtime.GetPodRequestResourcesByNames(pod, resourceNames...)
	for _, resourceName := range resourceNames {
		if podRequestResources[resourceName] == 0 {
			continue
		}
		if ns.GetNodeResource(resourceName).PodResourceIsCached(pod.UID) {
			allocated = append(allocated, resourceName)
			ns.GetNodeResource(resourceName).RemoveAllocatedPodResources(pod.UID)
			removed = true
		}
	}
	if removed {
		klog.Infof("Plugin=gpuoversell,Phase=ReleaseResource,Pod=%v,Node=%v,Message: succeed to release node device resources(%v) allocated for pod",
			podFullName,
			nodeName,
			allocated,
		)
	}
}

func AddOrUpdateNode(resourceNames []corev1.ResourceName, node *corev1.Node, nc *NodeCache) {
	// check node is a device sharing node
	if !runtime.IsMyNode(node, resourceNames...) {
		klog.V(6).Infof("node %v has no resources %v,skip to handle it", node.Name, resourceNames)
		return
	}
	devices := getNodeGPUCount(node)
	if devices == 0 {
		return
	}
	ns := nc.GetNodeState(node.Name)
	ns.Reset(node, resourceNames)
	klog.Infof("succeed to add node(%v) device to node resource cache", node.Name)
}

func UpdateNode(resourceNames []corev1.ResourceName, oldNode, newNode *corev1.Node, nc *NodeCache) {
	if !runtime.IsMyNode(oldNode, resourceNames...) && !runtime.IsMyNode(newNode, resourceNames...) {
		return
	}
	shouldUpdate := false
	oldNodeResources := runtime.GetNodeResourcesByNames(oldNode, resourceNames...)
	newNodeResources := runtime.GetNodeResourcesByNames(newNode, resourceNames...)
	for _, resourceName := range resourceNames {
		if oldNodeResources[resourceName] == newNodeResources[resourceName] {
			continue
		}
		shouldUpdate = true
	}
	if shouldUpdate {
		AddOrUpdateNode(resourceNames, newNode, nc)
	}
}
