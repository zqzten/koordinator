package gpushare

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/devicesharing/runtime"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/frameworkcache"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/noderesource"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

func AddPod(resourceNames []v1.ResourceName, pod *v1.Pod, frameworkCache *frameworkcache.FrameworkCache) {
	addPod(resourceNames, pod, func(nodeName string) (nrm *noderesource.NodeResourceManager) {
		return frameworkCache.GetExtendNodeInfo(nodeName).GetNodeResourceManager()
	})
}

func addPod(resourceNames []v1.ResourceName, pod *v1.Pod, getNodeResourceManager func(nodeName string) (nrm *noderesource.NodeResourceManager)) {
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
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
	nodeName := pod.Spec.NodeName
	// if not found allocation,skip to handle it
	podResources := getPodResources(resourceNames, pod)
	if len(podResources) == 0 {
		klog.V(6).Infof("not found allocation in the pod(%v) annotation,skip to handle it", podFullName)
		return
	}
	nrm := getNodeResourceManager(nodeName)
	for resourceName, podResource := range podResources {
		cache := nrm.GetNodeResourceCache(resourceName)
		if !cache.PodResourceIsCached(podResource.PodUID) {
			cache.AddAllocatedPodResources(podResource)
		}
	}
}

func UpdatePod(resourceNames []v1.ResourceName, oldPod *v1.Pod, newPod *v1.Pod, frameworkCache *frameworkcache.FrameworkCache) {
	if !runtime.IsMyPod(oldPod, resourceNames...) {
		return
	}
	if oldPod.Status.Phase != v1.PodRunning {
		return
	}
	if runtime.IsCompletedPod(newPod) {
		RemovePod(resourceNames, newPod, frameworkCache)
		return
	}
	AddPod(resourceNames, newPod, frameworkCache)
}

// RemovePod is invoked when a pod status is changed to completed(pod status is Completed or Error)
func RemovePod(resourceNames []v1.ResourceName, pod *v1.Pod, frameworkCache *frameworkcache.FrameworkCache) {
	removePod(resourceNames, pod, func(nodeName string) (nrm *noderesource.NodeResourceManager) {
		return frameworkCache.GetExtendNodeInfo(nodeName).GetNodeResourceManager()
	})
}

// removePod is invoked when a pod status is changed to completed(pod status is Completed or Error)
func removePod(resourceNames []v1.ResourceName, pod *v1.Pod, getNodeResourceManager func(nodeName string) (nrm *noderesource.NodeResourceManager)) {
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	// if pod is not a pod that we care about,skip to handle it
	if !runtime.IsMyPod(pod, resourceNames...) {
		klog.V(6).Infof("pod %v has no resources %v request,skip to free using devices of it", podFullName, resourceNames)
		return
	}
	nodeName := pod.Spec.NodeName
	if nodeName == "" {
		klog.V(6).Infof("node name of pod %v is null,skip to free using devices", podFullName)
		return
	}
	nrm := getNodeResourceManager(nodeName)
	allocated := []v1.ResourceName{}
	removed := false
	podRequestResources := runtime.GetPodRequestResourcesByNames(pod, resourceNames...)
	for _, resourceName := range resourceNames {
		if podRequestResources[resourceName] == 0 {
			continue
		}
		if nrm.GetNodeResourceCache(resourceName).PodResourceIsCached(pod.UID) {
			allocated = append(allocated, resourceName)
			nrm.GetNodeResourceCache(resourceName).RemoveAllocatedPodResources(pod.UID)
			removed = true
		}
	}
	if removed {
		klog.Infof("Plugin=gpushare,Phase=ReleaseResource,Pod=%v,Node=%v,Message: succeed to release node device resources(%v) allocated for pod",
			podFullName,
			nodeName,
			allocated,
		)
	}
}

// AddOrUpdateNode is invoked when a node status has been updated
func AddOrUpdateNode(resourceNames []v1.ResourceName, node *v1.Node, frameworkCache *frameworkcache.FrameworkCache) {
	// check node is a device sharing node
	if !runtime.IsMyNode(node, resourceNames...) {
		klog.V(6).Infof("node %v has no resources %v,skip to handle it", node.Name, resourceNames)
		return
	}
	devices := buildNodeDevices(resourceNames, node)
	if len(devices) == 0 {
		return
	}
	nrm := frameworkCache.GetExtendNodeInfo(node.Name).GetNodeResourceManager()
	for resourceName, resourceDevices := range devices {
		nrm.GetNodeResourceCache(resourceName).ResetDevices(resourceDevices...)
	}
	klog.Infof("succeed to add node(%v) device to node resource cache", node.Name)
}

func UpdateNode(resourceNames []v1.ResourceName, oldNode, newNode *v1.Node, frameworkCache *frameworkcache.FrameworkCache) {
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
		AddOrUpdateNode(resourceNames, newNode, frameworkCache)
	}
}

func getPodResources(resourceNames []v1.ResourceName, pod *v1.Pod) map[v1.ResourceName]noderesource.PodResource {
	podResources := getPodResourceFromV2(pod)
	if len(podResources) != 0 {
		return podResources
	}
	return getPodResourceFromV1(pod)
}

func buildNodeDevices(_ []v1.ResourceName, node *v1.Node) map[v1.ResourceName][]*noderesource.Device {
	// get gpu count
	totalGPU := getGPUShareNodeGPUCount(node)
	// check totalGPU is zero or not
	// because we will get the gpu memory of single gpu using totalMem/totalGPU
	// so must make sure totalGPU is not zero
	if totalGPU == 0 {
		klog.Infof("%v is 0 of node %v,skip to handle it", GPUShareResourceCountName, node.Name)
		return nil
	}
	resourceNames := GetResourceInfoList().GetNames()
	nodeResources := runtime.GetNodeResourcesByNames(node, resourceNames...)
	// create devices
	printInfo := []string{}
	result := map[v1.ResourceName][]*noderesource.Device{}
	for _, name := range resourceNames {
		count := nodeResources[name]
		if count == 0 {
			continue
		}
		devices := []*noderesource.Device{}
		for i := 0; i < totalGPU; i++ {
			totalResources := noderesource.NewNoneIDResources(count / totalGPU)
			unhealthyUnits := noderesource.NewNoneIDResources(0)
			device := noderesource.NewDevice(types.DeviceId(fmt.Sprintf("%v", i)), totalResources, unhealthyUnits)
			devices = append(devices, device)
		}
		if len(devices) != 0 {
			result[name] = devices
		}
		printInfo = append(printInfo, fmt.Sprintf("%v: %v", name, count))
	}
	klog.V(3).Infof("node: %v,total gpu: %v,%v", node.Name, totalGPU, strings.Join(printInfo, ","))
	return result
}

// getPodResourceFromV1 is used to parse pod annotations and the pod annotations assigned by old
// version gpushare
func getPodResourceFromV1(pod *v1.Pod) map[v1.ResourceName]noderesource.PodResource {
	value, found := pod.Annotations[GPUShareV1AllocationFlag]
	if !found || value == "" {
		return nil
	}
	allocatedGPUMemory, err := strconv.Atoi(value)
	if err != nil {
		klog.Infof("failed to parse allocated gpu memory from pod annotation %v,reason: %v", GPUShareV1AllocationFlag, err)
		return nil
	}
	deviceIndex, found := pod.Annotations[GPUShareV1DeviceIndexFlag]
	if !found || deviceIndex == "" {
		return nil
	}
	value, found = pod.Annotations[GPUShareV1TotalGPUMemoryFlag]
	totalGPUMemory, err := strconv.Atoi(value)
	if err != nil {
		klog.Infof("failed to parse total gpu memory from pod annotation %v,reason: %v", GPUShareV1TotalGPUMemoryFlag, err)
		return nil
	}
	if totalGPUMemory == 0 {
		return nil
	}
	containerAllocateResource := map[types.DeviceId]noderesource.Resources{
		types.DeviceId(deviceIndex): noderesource.NewNoneIDResources(allocatedGPUMemory),
	}
	podResource := noderesource.PodResource{
		PodUID:       pod.UID,
		PodName:      pod.Name,
		PodNamespace: pod.Namespace,
		DeviceResourceCount: map[types.DeviceId]int{
			types.DeviceId(deviceIndex): totalGPUMemory,
		},
		AllocatedResources: map[types.ContainerIndex]noderesource.ContainerAllocatedResources{
			types.ContainerIndex(0): noderesource.ContainerAllocatedResources(containerAllocateResource),
		},
	}
	return map[v1.ResourceName]noderesource.PodResource{
		v1.ResourceName(GPUShareResourceMemoryName): podResource,
	}
}

// getPodResourceFromV2 is used to parse the allocation from pod annotations
// if the scheduler reboot,it should detect which gpus has been allocated by gpushare pods.
func getPodResourceFromV2(pod *v1.Pod) map[v1.ResourceName]noderesource.PodResource {
	podResources := map[v1.ResourceName]noderesource.PodResource{}
	resourceInfos := []ResourceInfo{}
	for _, resourceInfo := range GetResourceInfoList() {
		resourceInfos = append(resourceInfos, resourceInfo)
	}
	deviceResourceCount := map[types.DeviceId]int{}
	v, ok := pod.Annotations[GPUShareV3DeviceCountFlag]
	if ok {
		if err := json.Unmarshal([]byte(v), &deviceResourceCount); err != nil {
			klog.Errorf("failed to parse device resource count from pod(%v) annotation: %v", pod.Name, err)
		}
	}
	for _, resourceInfo := range resourceInfos {
		// map[int]map[string]int{} represents: map[ContainerIndex]map[DeviceIndex]AllocatedGPUMemory
		data := map[int]map[string]int{}
		annotationKey := resourceInfo.PodAnnotationKey
		if annotationKey == "" {
			continue
		}
		value := pod.Annotations[annotationKey]
		if value == "" {
			continue
		}
		err := json.Unmarshal([]byte(value), &data)
		if err != nil {
			klog.Errorf("failed to parse allocation from annotation: %v", err)
			continue
		}
		allContainersAllocatedResources := map[types.ContainerIndex]noderesource.ContainerAllocatedResources{}
		for containerIndex, allocatedResources := range data {
			containerAllocatedResources := map[types.DeviceId]noderesource.Resources{}
			for devId, count := range allocatedResources {
				containerAllocatedResources[types.DeviceId(devId)] = noderesource.NewNoneIDResources(count)
			}
			allContainersAllocatedResources[types.ContainerIndex(containerIndex)] = containerAllocatedResources
		}
		podResource := noderesource.PodResource{
			PodUID:              pod.UID,
			PodName:             pod.Name,
			PodNamespace:        pod.Namespace,
			DeviceResourceCount: deviceResourceCount,
			AllocatedResources:  allContainersAllocatedResources,
		}
		podResources[v1.ResourceName(resourceInfo.Name)] = podResource
	}
	return podResources
}

func getPodResourceFromV3(pod *v1.Pod) map[v1.ResourceName]noderesource.PodResource {
	podResources := map[v1.ResourceName]noderesource.PodResource{}
	resourceNames := GetResourceInfoList().GetNames()
	for _, resourceName := range resourceNames {
		// map[int]map[string]int{} represents: map[ContainerIndex]map[DeviceIndex]AllocatedGPUMemory
		annotationKey := GetResourceInfoList().GetPodAnnotationKey(resourceName)
		if annotationKey == "" {
			continue
		}
		value := pod.Annotations[annotationKey]
		if value == "" {
			continue
		}
		podResource := noderesource.PodResource{
			PodUID:       pod.UID,
			PodName:      pod.Name,
			PodNamespace: pod.Namespace,
		}
		if err := noderesource.BuildPodContainersAllocatedResources(value, &podResource); err != nil {
			klog.Errorf("failed to parse allocation from annotation%v=%v: %v", annotationKey, value, err)
			continue
		}
		podResources[resourceName] = podResource
	}
	return podResources
}

func getGPUShareNodeGPUCount(node *v1.Node) int {
	val, ok := node.Status.Allocatable[v1.ResourceName(GPUShareResourceCountName)]
	if !ok {
		return int(0)
	}
	return int(val.Value())
}
