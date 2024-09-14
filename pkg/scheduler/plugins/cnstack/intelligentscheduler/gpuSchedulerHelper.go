package intelligentscheduler

// 存放IntelligentScheduler使用的、和业务逻辑无关的工具类方法

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"strconv"
)

func (i *IntelligentScheduler) saveVirtualGpuPodToCycleState(cycleState *framework.CycleState, VGpuCount int, VGpuSpecification string, podUID string) {
	virtualGpuPodState := &VirtualGpuPodState{
		VGpuCount:         VGpuCount,
		VGpuSpecification: VGpuSpecification,
	}
	cycleState.Write(GetVGpuPodStateKey(podUID), virtualGpuPodState)
}

func (i *IntelligentScheduler) saveVgiToCycleState(cycleState *framework.CycleState, vgiNames []string, podUID string) {
	virtualGpuInstanceState := &VirtualGpuInstanceState{
		vgiNames: vgiNames,
	}
	cycleState.Write(GetVGpuInstanceStateKey(podUID), virtualGpuInstanceState)
}

func (i *IntelligentScheduler) saveNodeToCycleState(cycleState *framework.CycleState, nodeInfo *NodeInfo, nodeName string) {
	nodeState := &NodeState{
		nodeInfo: nodeInfo, // TODO
	}
	cycleState.Write(GetNodeStateKey(nodeName), nodeState)
}

func (i *IntelligentScheduler) getVirtualGpuPodStateFromCycleState(cycleState *framework.CycleState, podUID string) (*VirtualGpuPodState, error) {
	key := GetVGpuPodStateKey(podUID)
	stateData, err := cycleState.Read(key)
	if err != nil {
		return nil, fmt.Errorf("failed to read virtualGpuPodState by podUID %s from CycleState, err: %v", podUID, err)
	}
	podState, ok := stateData.(*VirtualGpuPodState)
	if !ok {
		return nil, fmt.Errorf("failed to cast virtualGpuPodState by podUID %s from CycleState", podUID)
	}
	return podState, nil
}

func (i *IntelligentScheduler) getVgiStateFromCycleState(cycleState *framework.CycleState, podUID string) (*VirtualGpuInstanceState, error) {
	key := GetVGpuInstanceStateKey(podUID)
	stateData, err := cycleState.Read(key)
	if err != nil {
		return nil, fmt.Errorf("failed to read vgiState by podUID %s, err: %v", podUID, err)
	}
	vgiState, ok := stateData.(*VirtualGpuInstanceState)
	if !ok {
		return nil, fmt.Errorf("failed to cast vgiState by podUID %s", podUID)
	}
	return vgiState, nil
}

func (i *IntelligentScheduler) getNodeStateFromCycleState(cycleState *framework.CycleState, name string) (*NodeState, error) {
	key := GetNodeStateKey(name)
	stateData, err := cycleState.Read(key)
	if err != nil {
		return nil, fmt.Errorf("failed to read nodeState for node %s, err: %v", name, err)
	}
	nodeState, ok := stateData.(*NodeState)
	if !ok {
		return nil, fmt.Errorf("failed to cast nodeState for node %s", name)
	}
	return nodeState, nil
}

func (i *IntelligentScheduler) handleAddOrUpdateCM(cm *v1.ConfigMap) error {
	data := cm.Data
	nodeSelectorPolicy, ok := data["nodeSelectorPolicy"]
	if !ok {
		return fmt.Errorf("missing nodeSelectorPolicy in configmap %s/%s", cm.Namespace, cm.Name)
	}
	gpuSelectorPolicy, ok := data["gpuSelectorPolicy"]
	if !ok {
		return fmt.Errorf("missing gpuSelectorPolicy in configmap %s/%s", cm.Namespace, cm.Name)
	}
	oversellRateStr, ok := data["oversellRate"]
	if !ok {
		return fmt.Errorf("missing oversellRate in configmap %s/%s", cm.Namespace, cm.Name)
	}
	oversellRate, err := strconv.Atoi(oversellRateStr)
	if err != nil {
		return err
	}
	gpuMemoryScoreWeightStr, ok := data["gpuMemoryScoreWeight"]
	if !ok {
		return fmt.Errorf("missing gpuMemoryScoreWeight in configmap %s/%s", cm.Namespace, cm.Name)
	}
	gpuMemoryScoreWeight, err := strconv.Atoi(gpuMemoryScoreWeightStr)
	if err != nil {
		return err
	}
	gpuUtilizationScoreWeightStr, ok := data["gpuUtilizationScoreWeight"]
	if !ok {
		return fmt.Errorf("missing gpuUtilizationScoreWeight in configmap %s/%s", cm.Namespace, cm.Name)
	}
	gpuUtilizationScoreWeight, err := strconv.Atoi(gpuUtilizationScoreWeightStr)
	if err != nil {
		return err
	}
	if !(gpuMemoryScoreWeight >= 0 && gpuMemoryScoreWeight <= 100 && gpuUtilizationScoreWeight >= 0 && gpuUtilizationScoreWeight <= 100 && gpuMemoryScoreWeight+gpuUtilizationScoreWeight == 100) {
		return fmt.Errorf("invalid GPU score weight")
	}
	if !(gpuSelectorPolicy == "spread" || gpuSelectorPolicy == "binpack") {
		return fmt.Errorf("invalid GPU selector policy. It should be 'spread' or 'binpack'")
	}
	if !(nodeSelectorPolicy == "spread" || nodeSelectorPolicy == "binpack") {
		return fmt.Errorf("invalid node selector policy. It should be 'spread' or 'binpack'")
	}
	i.engine = NewIntelligentSchedulerRuntime(IntelligentSchedulerName, nodeSelectorPolicy, gpuSelectorPolicy, gpuMemoryScoreWeight, gpuUtilizationScoreWeight)
	i.oversellRate = oversellRate
	return nil
}

func (i *IntelligentScheduler) isAvailableNodeForPod(nodeName string, nodeInfos *NodeInfo, vGpuPodState *VirtualGpuPodState, vgiNames []string) bool {
	requestVGpuCount := vGpuPodState.getCount()
	requestVGpuSpec := vGpuPodState.getSpec()
	// 判断总数量是否满足
	if requestVGpuCount > nodeInfos.getGpuCount() {
		return false
	}
	// 判断物理规格是否满足
	vgs := i.cache.getVgsInfo(requestVGpuSpec)
	requestPGpuSpecs := vgs.getPhysicalGpuSpecifications()
	ok := false
	for _, spec := range requestPGpuSpecs {
		if spec.getName() == nodeInfos.getGpuType() {
			ok = true
		}
	}
	if len(requestPGpuSpecs) == 0 || (len(requestPGpuSpecs) == 1 && requestPGpuSpecs[0].getName() == "") {
		ok = true
	}
	if !ok {
		return false
	}
	vgi := i.cache.getVgiInfo(vgiNames[0]).Clone()
	oversellRate := nodeInfos.getOversellRate()
	// 判断available数量是否满足
	nodeGpuState, totalMem := getNodeGpuState(nodeName, nodeInfos, i.cache)
	availableGpuCount := 0
	for idx := 0; idx < nodeInfos.getGpuCount(); idx++ {
		available, _, _ := isAvailableForVgi(i.cache, nodeGpuState[idx], vgi, totalMem, oversellRate)
		if available {
			availableGpuCount++
		}
	}
	if availableGpuCount >= requestVGpuCount {
		return true
	} else {
		klog.Infof("node %s has available gpus[%d] are fewer than the requested[%d]", nodeName, availableGpuCount, requestVGpuCount)
		return false
	}
}
