package intelligentscheduler

// 存放通用的工具函数

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	POD_STATE  = "/podstate"
	VGI_STATE  = "/vgistate"
	NODE_STATE = "/nodestate"
)

// IsMyPod determines the pod is a pod that we care or not
func IsMyPod(pod *v1.Pod, resourceNames ...v1.ResourceName) bool {
	return len(GetPodRequestResourcesByNames(pod, resourceNames...)) != 0
}

// GetPodRequestResourcesByNames gets the value for target resource name
func GetPodRequestResourcesByNames(pod *v1.Pod, resourceNames ...v1.ResourceName) map[v1.ResourceName]int {
	result := map[v1.ResourceName]int{}
	for _, resourceName := range resourceNames {
		total := 0
		for _, containerRequest := range GetContainerRequestResourceByName(resourceName, pod) {
			total += containerRequest
		}
		if total != 0 {
			result[resourceName] = total
		}
	}
	return result
}

// GetContainerRequestResourceByName gets the value of containers for target resource name
func GetContainerRequestResourceByName(resourceName v1.ResourceName, pod *v1.Pod) map[int]int {
	total := map[int]int{}
	containers := pod.Spec.Containers
	for index, container := range containers {
		if val, ok := container.Resources.Limits[resourceName]; ok && int(val.Value()) != 0 {
			total[int(index)] = int(val.Value())
		}
	}
	return total
}

func GetVGpuPodStateKey(podUID string) framework.StateKey {
	return framework.StateKey(podUID + POD_STATE)
}

func GetVGpuInstanceStateKey(podUID string) framework.StateKey {
	return framework.StateKey(podUID + VGI_STATE)
}

func GetNodeStateKey(nodeName string) framework.StateKey {
	return framework.StateKey(nodeName + NODE_STATE)
}

func IntelligentSchedulerCrdCondition(handle framework.Handle) bool {
	crdList, err := handle.ClientSet().Discovery().ServerResourcesForGroupVersion(IntelligentGv.String())
	if err != nil {
		klog.Warningf("resources %v not found in discovery: %v", IntelligentGv, err)
		return false
	}
	klog.V(5).Infof("group resources %v found in discovery: %v", IntelligentGv, crdList.APIResources)

	dependentGvr := []schema.GroupVersionResource{
		VgsGvr,
		VgiGvr, // 这里添加调度器依赖的所有crd
	}
	for _, gvr := range dependentGvr {
		isExist := false
		for _, r := range crdList.APIResources {
			if gvr.Resource == r.Name {
				isExist = true // 找到了匹配的资源
			}
		}
		if !isExist {
			klog.Warningf("dependentGvr %v missing crd: %s", dependentGvr, gvr.Resource)
			return false
		}

	}
	klog.Infoln("all dependent resources found in discovery crdList")
	return true
}

func GPUShareCrdCondition(handle framework.Handle) bool {
	// GPUShare不需要校验CRD
	return true
}
