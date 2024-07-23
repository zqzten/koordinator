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
)

// score的计算可以放在这里
type IntelligentSchedulerRuntime struct {
	name            string
	nodeScorePolicy string
	gpuScorePolicy  string
}

func NewIntelligentSchedulerRuntime(name string, nodeScorePolicy string, gpuScorePolicy string) *IntelligentSchedulerRuntime {
	return &IntelligentSchedulerRuntime{
		name:            name,
		nodeScorePolicy: nodeScorePolicy,
		gpuScorePolicy:  gpuScorePolicy,
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

func (r *IntelligentSchedulerRuntime) findBestGpuCombination(cache *intelligentCache, combinations [][]int, vgi *VirtualGpuInstanceInfo, nodeGpuState map[int][]*VirtualGpuInstanceInfo, totalMem int) []int {
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
			_, mem, utilization := isAvailableForVgi(cache, nodeGpuState[gpuIdx], vgi, totalMem)
			totalGpuMem += totalMem
			totalUtilization += 100
			usedMem += mem
			usedUtilization += utilization
		}
		if r.getGPUScorePolicy() == "binpack" {
			score = 0.5*(float64(usedMem+requestCount*requestMem)/float64(totalGpuMem)) + 0.5*(float64(usedUtilization+requestCount*requestUtilization)/float64(totalUtilization))
		} else if r.getGPUScorePolicy() == "spread" {
			score = 0.5*(1.0-float64(usedMem+requestCount*requestMem)/float64(totalGpuMem)) + 0.5*(1.0-float64(usedUtilization+requestCount*requestUtilization)/float64(totalUtilization))
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
	requestMem := vgi.getMemAllocated()
	requestUtilization := vgi.getPercentageAllocated()
	requestCount := len(vgiNames)
	nodeGpuState, totalMem := getNodeGpuState(nodeName, nodeInfos, cache)
	totalAvailableMem := 0
	totalAvailableUtilization := 0
	totalUsedMem := 0
	totalUsedUtilization := 0
	for idx := 0; idx < nodeInfos.getGpuCount(); idx++ {
		available, mem, utilization := isAvailableForVgi(cache, nodeGpuState[idx], vgi, totalMem)
		if available {
			totalAvailableUtilization += 100
			totalAvailableMem += totalMem
			totalUsedUtilization += utilization
			totalUsedMem += mem
		}
	}
	if r.getGPUScorePolicy() == "binpack" {
		nodeScore = 0.5*(float64(totalUsedMem+requestCount*requestMem)/float64(totalAvailableMem)) + 0.5*(float64(totalUsedUtilization+requestCount*requestUtilization)/float64(totalUsedUtilization))
	} else if r.getGPUScorePolicy() == "spread" {
		nodeScore = 0.5*(1.0-float64(totalUsedMem+requestCount*requestMem)/float64(totalAvailableMem)) + 0.5*(1.0-float64(totalUsedUtilization+requestCount*requestUtilization)/float64(totalUsedUtilization))
	}
	return nodeScore
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
		if (instanceInfo.getStatus() == "Allocated" || instanceInfo.getStatus() == "Running") && instanceInfo.getNode() == nodeName {
			idx := instanceInfo.getGPUIndex()
			state[idx] = append(state[idx], instanceInfo)
		}
	}
	return state, nodeInfos.getGpuMem()
}

// 判断vgi是否还能够被分配到某张物理卡上，返回判断结果、分配前的总占用mem，utilization
func isAvailableForVgi(cache *intelligentCache, gpuState []*VirtualGpuInstanceInfo, vgi *VirtualGpuInstanceInfo, totalMem int) (bool, int, int) {
	if len(gpuState) == 0 || gpuState == nil {
		//klog.Info("in func isAvailableForVgi, return true, 0, 0")
		return true, 0, 0
	}
	tmpVgi := gpuState[0]
	vgsName := tmpVgi.getVgs()
	vgs := cache.getVgsInfo(vgsName)
	memIsolation := vgs.getGpuMemoryIsolation()
	utilizationIsolation := vgs.getGpuUtilizationIsolation()
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
	if requestMem+usedMem <= totalMem && requestUtilization+usedUtilization <= 100 {
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
	for idx := 0; idx < nodeInfos.getGpuCount(); idx++ {
		vgi := cache.getVgiInfo(vgiNames[vgiNameIdx])
		ok, _, _ := isAvailableForVgi(cache, nodeGpuState[idx], vgi, totalMem)
		if ok {
			vgi.setNode(nodeName)
			vgi.setGPUIndex(idx)
			vgi.setPhysicalGpuSpecification(nodeInfos.getGpuType())
			vgi.setStatus("Allocated")
			vgiNameIdx++
		}
	}
	if vgiNameIdx < len(vgiNames) {
		return fmt.Errorf("node %v has limit GPU for pod %v", nodeName, pod.Name)
	}
	return nil
}

func patchVgi(client dynamic.Interface, vgiName string, nodeName string, gpuIdx int, phase string, physicalGpuSpec string, podName string, podNamespace string) error {
	vgiGvr := schema.GroupVersionResource{
		Group:    IntelligentGroupName,
		Version:  IntelligentVersion,
		Resource: VgiResourceName,
	}
	patchData := map[string]interface{}{
		"status": map[string]interface{}{
			"pod":             podName + "/" + podNamespace,
			"physicalGpuSpec": physicalGpuSpec,
			"node":            nodeName,
			"phase":           phase,
			"gpuIndex":        gpuIdx,
		},
	}
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return err
	}
	_, err = client.Resource(vgiGvr).Namespace(NameSpace).Patch(
		context.TODO(),
		vgiName,
		types.MergePatchType,
		patchBytes,
		metav1.PatchOptions{})
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
