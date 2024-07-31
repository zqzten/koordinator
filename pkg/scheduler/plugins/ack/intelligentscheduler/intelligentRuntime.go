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

// score的计算可以放在这里
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
	//TODO
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
			score = float64(r.memoryScoreWeight)*(float64(usedMem+requestCount*requestMem)/float64(totalGpuMem)) + float64(r.utilizationScoreWeight)*(float64(usedUtilization+requestCount*requestUtilization)/float64(totalUtilization))
		} else if r.getGPUScorePolicy() == "spread" {
			score = float64(r.memoryScoreWeight)*(1.0-float64(usedMem+requestCount*requestMem)/float64(totalGpuMem)) + float64(r.utilizationScoreWeight)*(1.0-float64(usedUtilization+requestCount*requestUtilization)/float64(totalUtilization))
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
	if vgi.getIsOversell() != nodeInfos.getIsOversell() {
		return float64(0)
	}
	var oversellRate float64
	if nodeInfos.getIsOversell() {
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
	for idx := 0; idx < nodeInfos.getGpuCount(); idx++ {
		available, mem, utilization := isAvailableForVgi(cache, nodeGpuState[idx], vgi, totalMem, int(oversellRate))
		if available {
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
	return nodeScore
	// TODO normalize score to [0, 100]
}

func validateGPUSchedulerArgs(args IntelligentSchedulerArgs) error {
	if reflect.DeepEqual(args, IntelligentSchedulerArgs{}) {
		return fmt.Errorf("intelligent scheduler plugin requires at least one argument")
	}
	gpuMemoryScoreWeight := *args.GPUMemoryScoreWeight
	gpuUtilizationScoreWeight := *args.GPUUtilizationScoreWeight
	if !(gpuMemoryScoreWeight >= 0 && gpuMemoryScoreWeight <= 100 && gpuUtilizationScoreWeight >= 0 && gpuUtilizationScoreWeight <= 100 && gpuMemoryScoreWeight+gpuUtilizationScoreWeight == 100) {
		return fmt.Errorf("invalid GPU score weight")
	}
	if !(args.GpuSelectorPolicy == "spread" || args.GpuSelectorPolicy == "binpack") {
		return fmt.Errorf("invalid GPU selector policy. It should be 'spread' or 'binpack'")
	}
	if !(args.NodeSelectorPolicy == "spread" || args.NodeSelectorPolicy == "binpack") {
		return fmt.Errorf("invalid node selector policy. It should be 'spread' or 'binpack'")
	}
	return nil
}

func isIntelligentNode(node *v1.Node) bool {
	// TODO 这个label需要在设置node gpu调度方式时设置
	if node.Labels[SchedulerNodeLabel] == "intelligent" {
		return true
	}
	return false
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
	klog.Infof("Pod with name [%v] should be allocated with [%v] virtual gpu", pod.Name, vGpuCount)
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
		//klog.Info("in func isAvailableForVgi, return true, 0, 0")
		return true, 0, 0
	}
	tmpVgi := gpuState[0]
	vgsName := tmpVgi.getVgs()
	vgs := cache.getVgsInfo(vgsName)
	memIsolation := vgs.getGpuMemoryIsolation()
	utilizationIsolation := vgs.getGpuUtilizationIsolation()
	// 卡维度超卖
	isOversell := vgs.getIsOversell()
	requestOversell := vgi.getIsOversell()
	if isOversell != requestOversell {
		return false, 0, 0
	}

	requestVgsName := vgi.getVgs()
	requestVgs := cache.getVgsInfo(requestVgsName)
	if memIsolation != requestVgs.getGpuMemoryIsolation() || utilizationIsolation != requestVgs.getGpuUtilizationIsolation() {
		return false, 0, 0
	}
	usedMem := 0
	usedUtilization := 0
	for _, instanceInfo := range gpuState {
		usedMem += instanceInfo.getMemAllocated()
		usedUtilization += instanceInfo.getPercentageAllocated()
	}
	requestMem := vgi.getMemAllocated()
	requestUtilization := vgi.getPercentageAllocated()
	if requestMem+usedMem <= totalMem*oversellRate && requestUtilization+usedUtilization <= 100*oversellRate {
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

//func patchVgiOwnerReference(client dynamic.Interface, vgiName string, pod *v1.Pod) error {
//	vgiGvr := schema.GroupVersionResource{
//		Group:    IntelligentGroupName,
//		Version:  IntelligentVersion,
//		Resource: VgiResourceName,
//	}
//	// 传入的pod只包含从metadata开始的信息，从annotation中获得APIVersion和Kind，默认为v1，Pod
//	var apiVersion, kind string
//	lastAppliedConfig, ok := pod.Annotations["kubectl.kubernetes.io/last-applied-configuration"]
//	if !ok {
//		apiVersion = "v1"
//		kind = "Pod"
//	} else {
//		var lastConfig map[string]interface{}
//		err := json.Unmarshal([]byte(lastAppliedConfig), &lastConfig)
//		if err != nil {
//			apiVersion = "v1"
//			kind = "Pod"
//		}
//		apiVersion, ok = lastConfig["apiVersion"].(string)
//		if !ok {
//			apiVersion = "v1"
//		}
//		kind, ok = lastConfig["kind"].(string)
//		if !ok {
//			kind = "Pod"
//		}
//	}
//	newOwnerReferenceData := metav1.OwnerReference{
//		APIVersion: apiVersion,
//		Kind:       kind,
//		Name:       pod.Name,
//		UID:        pod.UID,
//	}
//
//	vgiCrs, err := client.Resource(vgiGvr).Namespace(NameSpace).Get(context.TODO(), vgiName, metav1.GetOptions{})
//	if err != nil {
//		return err
//	}
//	existingOwnerReferences, found, err := unstructured.NestedSlice(vgiCrs.Object, "metadata", "ownerReferences")
//	if err != nil {
//		return err
//	}
//	if !found {
//		existingOwnerReferences = []interface{}{}
//	}
//	newOwnerReferenceMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&newOwnerReferenceData)
//	if err != nil {
//		return fmt.Errorf("failed to convert ownerReference: %v", err)
//	}
//	existingOwnerReferences = []interface{}{}
//	existingOwnerReferences = append(existingOwnerReferences, newOwnerReferenceMap)
//
//	patchData := map[string]interface{}{
//		"metadata": map[string]interface{}{
//			"ownerReferences": existingOwnerReferences,
//			//"ownerReferences": newOwnerReferenceMap,
//		},
//	}
//	klog.Infof("patch ownerReferences: %v", patchData)
//	patchBytes, err := json.Marshal(patchData)
//	if err != nil {
//		return err
//	}
//	_, err = client.Resource(vgiGvr).Namespace(NameSpace).Patch(
//		context.TODO(),
//		vgiName,
//		types.MergePatchType,
//		patchBytes,
//		metav1.PatchOptions{},
//	)
//	if err != nil {
//		klog.Errorf("Failed to patch owner reference data for pod %v/%v, [%v]", pod.Name, pod.UID, err)
//	}
//	return err
//}

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
	//// patch onwerReference to vgi
	//var apiVersion, kind string
	//lastAppliedConfig, ok := pod.Annotations["kubectl.kubernetes.io/last-applied-configuration"]
	//if !ok {
	//	apiVersion = "v1"
	//	kind = "Pod"
	//} else {
	//	var lastConfig map[string]interface{}
	//	err := json.Unmarshal([]byte(lastAppliedConfig), &lastConfig)
	//	if err != nil {
	//		apiVersion = "v1"
	//		kind = "Pod"
	//	}
	//	apiVersion, ok = lastConfig["apiVersion"].(string)
	//	if !ok {
	//		apiVersion = "v1"
	//	}
	//	kind, ok = lastConfig["kind"].(string)
	//	if !ok {
	//		kind = "Pod"
	//	}
	//}
	//newOwnerReferenceData := metav1.OwnerReference{
	//	APIVersion: apiVersion,
	//	Kind:       kind,
	//	Name:       pod.Name,
	//	UID:        pod.UID,
	//}
	//
	//vgiCrs, err := client.Resource(vgiGvr).Namespace(NameSpace).Get(context.Background(), vgiName, metav1.GetOptions{})
	//if err != nil {
	//	return err
	//}
	//existingOwnerReferences, found, err := unstructured.NestedSlice(vgiCrs.Object, "metadata", "ownerReferences")
	//if err != nil {
	//	return err
	//}
	//if !found {
	//	existingOwnerReferences = []interface{}{}
	//}
	//newOwnerReferenceMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&newOwnerReferenceData)
	//if err != nil {
	//	return fmt.Errorf("failed to convert ownerReference: %v", err)
	//}
	//existingOwnerReferences = append(existingOwnerReferences, newOwnerReferenceMap)
	//
	//patchData = map[string]interface{}{
	//	"metadata": map[string]interface{}{
	//		"ownerReferences": existingOwnerReferences,
	//	},
	//}
	//klog.Infof("patch ownerReferences: %v", patchData)
	//patchBytes, err = json.Marshal(patchData)
	//if err != nil {
	//	return err
	//}
	//_, err = client.Resource(vgiGvr).Namespace(NameSpace).Patch(
	//	context.Background(),
	//	vgiName,
	//	types.MergePatchType,
	//	patchBytes,
	//	metav1.PatchOptions{},
	//)
	//if err != nil {
	//	klog.Errorf("Failed to patch owner reference data for pod %v/%v, [%v]", pod.Name, pod.UID, err)
	//} else {
	//	klog.Infof("patch vgi[%v] owner successfully", vgiName)
	//}

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
	//klog.Infof("DATA: [%v]", data)
	patchData := map[string]interface{}{
		"metadata": map[string]interface{}{
			"ownerReferences": ownerReferences,
		},
		"data": data,
	}
	//klog.Infof("PATCHDATA: [%v]", patchData)
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		klog.Errorf("failed to marshal patchData: %v", err)
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
		klog.Errorf("failed to patchConfigMap: %v", err)
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
		// warning: only care value is large than 0
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

//func buildPodAnnotations() map[string]string {
//	return map[string]string{
//		IntelligentPodAnnoAssumeTimeFlag: fmt.Sprintf("%d", time.Now().UnixNano()),
//		IntelligentPodAnnoAssignFlag:     "false",
//	}
//}
//
//func patchPodAnnotations(client dynamic.Interface, pod *v1.Pod) error {
//	podName := pod.Name
//	podNamespace := pod.Namespace
//	annotations := buildPodAnnotations()
//	patchData := map[string]interface{}{
//		"metadata": map[string]interface{}{
//			"annotations": annotations,
//		},
//	}
//	patchBytes, err := json.Marshal(patchData)
//	podGVR := schema.GroupVersionResource{
//		Group:    "",
//		Version:  pod.APIVersion,
//		Resource: "pods",
//	}
//	if err != nil {
//		klog.Errorf("Error marshaling patch data: %v", err)
//	}
//	_, er := client.Resource(podGVR).Namespace(podNamespace).Patch(
//		context.TODO(),
//		podName,
//		types.MergePatchType,
//		patchBytes,
//		metav1.PatchOptions{},
//		"metadata")
//	if er != nil {
//		klog.Errorf("Error patching pod: %v", er)
//	}
//	return er
//}
