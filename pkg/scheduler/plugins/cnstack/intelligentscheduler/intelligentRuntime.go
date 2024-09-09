package intelligentscheduler

import (
	"context"
	"encoding/json"
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"reflect"
	"strconv"
	"strings"
)

type IntelligentSchedulerRuntime struct {
	name                   string
	nodeScorePolicy        string
	gpuScorePolicy         string
	memoryScoreWeight      int
	utilizationScoreWeight int
}

func NewIntelligentSchedulerRuntime(name string, nodeScorePolicy string, gpuScorePolicy string, memWeight int, utilizationWeight int) *IntelligentSchedulerRuntime {
	return &IntelligentSchedulerRuntime{
		name:                   name,
		nodeScorePolicy:        nodeScorePolicy,
		gpuScorePolicy:         gpuScorePolicy,
		memoryScoreWeight:      memWeight,
		utilizationScoreWeight: utilizationWeight,
	}
}

func (r *IntelligentSchedulerRuntime) Name() string {
	return r.name
}

func (r *IntelligentSchedulerRuntime) getNodeScorePolicy() string {
	return r.nodeScorePolicy
}

func (r *IntelligentSchedulerRuntime) getGPUScorePolicy() string {
	return r.gpuScorePolicy
}

func (r *IntelligentSchedulerRuntime) Init() error {
	return nil
}

func (r *IntelligentSchedulerRuntime) findBestGpuCombination(cache *intelligentCache, combinations [][]int, vgi *VirtualGpuInstanceInfo, nodeGpuState map[int][]*VirtualGpuInstanceInfo, totalMem int, oversellRate int) []int {
	scores := make(map[int]float64, len(combinations))
	requestCount := len(combinations[0])
	requestMem := vgi.getMemAllocated()
	requestUtilization := vgi.getPercentageAllocated()
	for idx, combination := range combinations {
		var score float64
		usedMem := 0
		usedUtilization := 0
		totalUtilization := 0
		totalGpuMem := 0
		for _, gpuIdx := range combination {
			_, mem, utilization := isAvailableForVgi(cache, nodeGpuState[gpuIdx], vgi, totalMem, oversellRate)
			totalGpuMem += totalMem
			totalUtilization += 100
			usedMem += mem
			usedUtilization += utilization
		}
		if r.getGPUScorePolicy() == "binpack" {
			score = float64(r.memoryScoreWeight)*(float64(usedMem+requestCount*requestMem)/(float64(totalGpuMem)*float64(oversellRate))) + float64(r.utilizationScoreWeight)*(float64(usedUtilization+requestCount*requestUtilization)/(float64(totalUtilization)*float64(oversellRate)))
		} else if r.getGPUScorePolicy() == "spread" {
			score = float64(r.memoryScoreWeight)*(1.0-float64(usedMem+requestCount*requestMem)/(float64(totalGpuMem)*float64(oversellRate))) + float64(r.utilizationScoreWeight)*(1.0-float64(usedUtilization+requestCount*requestUtilization)/(float64(totalUtilization)*float64(oversellRate)))
		}
		scores[idx] = score
	}
	var found bool
	var maxValue float64
	var maxIndex int
	for index, value := range scores {
		if !found || value > maxValue {
			maxValue = value
			maxIndex = index
			found = true
		}
	}
	return combinations[maxIndex]
}

func (r *IntelligentSchedulerRuntime) calculateNodeScore(cache *intelligentCache, nodeName string, nodeInfos *NodeInfo, vgiNames []string) float64 {
	var nodeScore float64
	vgi := cache.getVgiInfo(vgiNames[0]).Clone()
	if vgi.getIsOversell() && !nodeInfos.getIsOversell() {
		return float64(0)
	}
	var oversellRate float64
	if nodeInfos.getIsOversell() && vgi.getIsOversell() {
		oversellRate = float64(nodeInfos.getOversellRate())
	} else {
		oversellRate = float64(1)
	}
	requestMem := vgi.getMemAllocated()
	requestUtilization := vgi.getPercentageAllocated()
	requestCount := len(vgiNames)
	nodeGpuState, totalMem := getNodeGpuState(nodeName, nodeInfos, cache)
	totalAvailableMem := 0
	totalAvailableUtilization := 0
	totalUsedMem := 0
	totalUsedUtilization := 0
	tmp := 0
	for idx := 0; idx < nodeInfos.getGpuCount(); idx++ {
		available, mem, utilization := isAvailableForVgi(cache, nodeGpuState[idx], vgi, totalMem, int(oversellRate))
		if available {
			tmp++
			totalAvailableUtilization += 100
			totalAvailableMem += totalMem
			totalUsedUtilization += utilization
			totalUsedMem += mem
		}
	}
	if r.getGPUScorePolicy() == "binpack" {
		nodeScore = float64(r.memoryScoreWeight)*(float64(totalUsedMem+requestCount*requestMem)/(float64(totalAvailableMem)*oversellRate)) + float64(r.utilizationScoreWeight)*(float64(totalUsedUtilization+requestCount*requestUtilization)/(float64(totalAvailableUtilization)*oversellRate))
	} else if r.getGPUScorePolicy() == "spread" {
		nodeScore = float64(r.memoryScoreWeight)*(1.0-float64(totalUsedMem+requestCount*requestMem)/(float64(totalAvailableMem)*oversellRate)) + float64(r.utilizationScoreWeight)*(1.0-float64(totalUsedUtilization+requestCount*requestUtilization)/(float64(totalAvailableUtilization)*oversellRate))
	}
	klog.Infof("nodeScore policy: [%v], node [%v] score: [%v]", r.nodeScorePolicy, nodeName, nodeScore)
	return nodeScore
}

func validateGPUSchedulerArgs(args IntelligentSchedulerArgs) error {
	if reflect.DeepEqual(args, IntelligentSchedulerArgs{}) {
		return fmt.Errorf("intelligent scheduler plugin requires at least one argument")
	}
	return nil
}

func isIntelligentNode(node *v1.Node) bool {
	if node.Labels[SchedulerNodeLabel] != "intelligent" {
		return false
	}
	// 因为目前eGPU只支持nvidia卡，所有非n卡pod不会被intelligent scheduler调度。增加了判断该节点GPU是否为n卡的逻辑，否则不会被intelligent scheduler考虑
	_, ok := node.Labels["aliyun.accelerator/nvidia_name"]
	if !ok {
		return false
	}
	return true
}

func GetVirtualGPUCountAndSpec(pod *v1.Pod) (int, string, error) {
	vGpuSpecName, ok := pod.Labels[VirtualGpuSpecificationKey]
	if !ok {
		return 0, "", fmt.Errorf("unable to find %v in pod annotation", VirtualGpuSpecificationKey)
	}
	vGpuCount, err := strconv.Atoi(pod.Labels[VirtualGpuCountKey])
	if err != nil {
		return 0, "", fmt.Errorf("unable to parse %v %v into integer", VirtualGpuCountKey, pod.Annotations[VirtualGpuCountKey])
	}
	return vGpuCount, vGpuSpecName, nil
}

func getNodeGPUCount(node *v1.Node) int {
	val, ok := node.Labels[PhysicalGpuCountNodeLabel]
	if !ok {
		return int(0)
	}
	count, err := strconv.Atoi(val)
	if err != nil {
		klog.Errorf("unable to parse %v %v into integer", PhysicalGpuCountNodeLabel, val)
	}
	return count
}

// 从cache中获得node上每张物理卡对应的所有vgi，返回state和每张卡最大显存
func getNodeGpuState(nodeName string, nodeInfos *NodeInfo, cache *intelligentCache) (map[int][]*VirtualGpuInstanceInfo, int) {
	gpuCount := nodeInfos.getGpuCount()
	state := make(map[int][]*VirtualGpuInstanceInfo, gpuCount)
	virtualGpuInstances := cache.getAllVgiInfo()
	for _, instanceInfo := range virtualGpuInstances {
		if (instanceInfo.getStatus() == "PreAllocated" || instanceInfo.getStatus() == "Running" || instanceInfo.getStatus() == "Allocated") && instanceInfo.getNode() == nodeName {
			idx := instanceInfo.getGPUIndex()
			state[idx] = append(state[idx], instanceInfo)
		}
	}
	return state, nodeInfos.getGpuMem()
}

// 判断vgi是否还能够被分配到某张物理卡上，返回判断结果、分配前的总占用mem，utilization
func isAvailableForVgi(cache *intelligentCache, gpuState []*VirtualGpuInstanceInfo, vgi *VirtualGpuInstanceInfo, totalMem int, oversellRate int) (bool, int, int) {
	if len(gpuState) == 0 || gpuState == nil {
		return true, 0, 0
	}

	ortherVgi := gpuState[0]
	ortherVgs := cache.getVgsInfo(ortherVgi.getVgsName())
	requestVgs := cache.getVgsInfo(vgi.getVgsName())

	// 判断其他vgi和本次vgi的超卖类型是否一致
	if vgi.getIsOversell() != ortherVgi.getIsOversell() {
		klog.Infof("vgi %s's IsOversell is not equal other vgis", vgi.getName())
		return false, 0, 0
	}

	cardOversellRate := oversellRate
	// 当这张卡上的vgi类型是不超卖，则将本卡的超卖值置为1
	if !vgi.getIsOversell() {
		cardOversellRate = 1
	}

	// 判断其他vgi和本次vgi的显存&算力隔离类型是否一致
	if requestVgs.getGpuMemoryIsolation() == ortherVgs.getGpuMemoryIsolation() ||
		requestVgs.getGpuUtilizationIsolation() == ortherVgs.getGpuUtilizationIsolation() {
		klog.Infof("vgi %s's GpuMemoryIsolation or GpuUtilizationIsolation is not equal other vgis", vgi.getName())
		return false, 0, 0
	}

	// 获取已占用的显存和算力
	usedMem := 0
	usedUtilization := 0
	for _, instanceInfo := range gpuState {
		usedMem += instanceInfo.getMemAllocated()
		usedUtilization += instanceInfo.getPercentageAllocated()
	}

	// 判断显存和算力能否满足本次vgi请求
	requestMem := vgi.getMemAllocated()
	requestUtilization := vgi.getPercentageAllocated()
	if requestMem+usedMem <= totalMem*cardOversellRate &&
		requestUtilization+usedUtilization <= 100*cardOversellRate {
		return true, usedMem, usedUtilization
	} else {
		return false, 0, 0
	}
}

// 被AddPod接口调用，
func addPod(cache *intelligentCache, vgiNames []string, pod *v1.Pod, nodeName string, nodeInfos *NodeInfo) error {
	if len(vgiNames) > nodeInfos.getGpuCount() {
		return fmt.Errorf("node %v has limit GPU for pod %v", nodeName, pod.Name)
	}
	vgiNameIdx := 0
	nodeGpuState, totalMem := getNodeGpuState(nodeName, nodeInfos, cache)
	var oversellRate int
	if nodeInfos.getIsOversell() {
		oversellRate = nodeInfos.getOversellRate()
	} else {
		oversellRate = 1
	}
	for idx := 0; idx < nodeInfos.getGpuCount(); idx++ {
		if vgiNameIdx >= len(vgiNames) {
			break
		}
		vgi := cache.getVgiInfo(vgiNames[vgiNameIdx])
		ok, _, _ := isAvailableForVgi(cache, nodeGpuState[idx], vgi, totalMem, oversellRate)
		if ok {
			vgi.setNode(nodeName)
			vgi.setGPUIndex(idx)
			vgi.setPhysicalGpuSpecification(nodeInfos.getGpuType())
			vgi.setStatus("PreAllocated")
			vgiNameIdx++
		}
	}
	if vgiNameIdx < len(vgiNames) {
		return fmt.Errorf("node %v has limit GPU for pod %v", nodeName, pod.Name)
	}
	return nil
}

func patchVgi(client dynamic.Interface, vgiName string, nodeName string, gpuIdx int, phase string, physicalGpuSpec string, pod *v1.Pod) error {
	podName := pod.Name
	podNamespace := pod.Namespace
	vgiGvr := schema.GroupVersionResource{
		Group:    IntelligentGroupName,
		Version:  IntelligentVersion,
		Resource: VgiResourceName,
	}
	statusData := map[string]interface{}{
		"pod":                      fmt.Sprintf("%s/%s", podName, podNamespace),
		"physicalGpuSpecification": physicalGpuSpec,
		"node":                     nodeName,
		"phase":                    phase,
		"gpuIndex":                 gpuIdx,
		"containerIndex":           0,
	}
	patchData := map[string]interface{}{
		"status": statusData,
	}
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return err
	}
	_, err = client.Resource(vgiGvr).Namespace(NameSpace).Patch(
		context.Background(),
		vgiName,
		types.MergePatchType,
		patchBytes,
		metav1.PatchOptions{},
		"status",
	)
	return err
}

func combine(nums []int, m int) [][]int {
	var result [][]int
	comb := make([]int, m)
	var helper func(int, int)
	helper = func(start, depth int) {
		if depth == m {
			combination := make([]int, m)
			copy(combination, comb)
			result = append(result, combination)
			return
		}
		for i := start; i < len(nums); i++ {
			comb[depth] = nums[i]
			helper(i+1, depth+1)
		}
	}
	helper(0, 0)
	return result
}

func patchOwnerRefToConfigMap(client dynamic.Interface, pod *v1.Pod) error {
	cmName, ok := pod.Labels[PodConfigMapLabel]
	if !ok {
		return fmt.Errorf("pod %v has no label %v", pod.Name, PodConfigMapLabel)
	}
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}
	var apiVersion, kind string
	lastAppliedConfig, ok := pod.Annotations["kubectl.kubernetes.io/last-applied-configuration"]
	if !ok {
		apiVersion = "v1"
		kind = "Pod"
	} else {
		var lastConfig map[string]interface{}
		err := json.Unmarshal([]byte(lastAppliedConfig), &lastConfig)
		if err != nil {
			apiVersion = "v1"
			kind = "Pod"
		}
		apiVersion, ok = lastConfig["apiVersion"].(string)
		if !ok {
			apiVersion = "v1"
		}
		kind, ok = lastConfig["kind"].(string)
		if !ok {
			kind = "Pod"
		}
	}
	ownerReferences := []metav1.OwnerReference{
		{
			APIVersion: apiVersion,
			Kind:       kind,
			Name:       pod.Name,
			UID:        pod.UID,
		},
	}
	patchData := map[string]interface{}{
		"metadata": map[string]interface{}{
			"ownerReferences": ownerReferences,
		},
	}
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		klog.Errorf("Failed to marshal patchData: %v", err)
		return err
	}
	_, err = client.Resource(gvr).Namespace(pod.Namespace).Patch(
		context.Background(),
		cmName,
		types.MergePatchType,
		patchBytes,
		metav1.PatchOptions{},
	)
	if err != nil {
		klog.Errorf("Failed to patchConfigMap: %v", err)
	}
	return err
}

func patchConfigMap(client dynamic.Interface, pod *v1.Pod, data map[string]interface{}) error {
	cmName, ok := pod.Labels[PodConfigMapLabel]
	if !ok {
		return fmt.Errorf("pod %v has no label %v", pod.Name, PodConfigMapLabel)
	}
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}
	patchData := map[string]interface{}{
		"data": data,
	}
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		klog.Errorf("Failed to marshal patchData: %v", err)
		return err
	}
	_, err = client.Resource(gvr).Namespace(pod.Namespace).Patch(
		context.Background(),
		cmName,
		types.MergePatchType,
		patchBytes,
		metav1.PatchOptions{},
	)
	if err != nil {
		klog.Errorf("Failed to patchConfigMap: %v", err)
	}
	return err
}

func buildEnvVars(vgi *VirtualGpuInstanceInfo, gpuIdx []int, totalMem int) map[string]interface{} {
	result := make(map[string]interface{})
	var gpuIdxStr []string
	weight := vgi.getPercentageAllocated() / 5
	for _, idx := range gpuIdx {
		gpuIdxStr = append(gpuIdxStr, strconv.Itoa(idx))
	}
	if vgi.getIsOversell() {
		result["AMP_VGPU_DEV_COUNT"] = strings.Join(gpuIdxStr, ",")
		result["NVIDIA_VISIBLE_DEVICES"] = strings.Join(gpuIdxStr, ",")
		result["ALIYUN_COM_GPU_MEM_DEV"] = fmt.Sprintf("%d", totalMem)
		result["ALIYUN_COM_GPU_MEM_CONTAINER"] = fmt.Sprintf("%d", vgi.getMemAllocated())
		result["ALIYUN_COM_GPU_MEM_UNIT"] = "GB"
		result["ALIYUN_COM_GPU_SCHD_WEIGHT"] = fmt.Sprintf("%d", weight)
		result["GPU_UTIL_PER_DEVICE"] = fmt.Sprintf("%d", vgi.getPercentageAllocated())
	} else {
		result["NVIDIA_VISIBLE_DEVICES"] = strings.Join(gpuIdxStr, ",")
		result["ALIYUN_COM_GPU_MEM_DEV"] = fmt.Sprintf("%d", totalMem)
		result["ALIYUN_COM_GPU_MEM_CONTAINER"] = fmt.Sprintf("%d", vgi.getMemAllocated())
		result["ALIYUN_COM_GPU_MEM_UNIT"] = "GB"
		result["ALIYUN_COM_GPU_SCHD_WEIGHT"] = fmt.Sprintf("%d", weight)
		result["GPU_UTIL_PER_DEVICE"] = fmt.Sprintf("%d", vgi.getPercentageAllocated())
	}
	return result
}

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
