package intelligentscheduler

// 存放IntelligentScheduler使用的、和业务逻辑无关的工具类方法

import (
	"fmt"
	"k8s.io/kubernetes/pkg/scheduler/framework"
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
