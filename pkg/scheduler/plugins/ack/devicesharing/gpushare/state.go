package gpushare

import (
	"fmt"

	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/devicesharing/runtime"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/noderesource"
)

const GPUShareStateKeyNamespace = "gpushare"

func getPodStateKey() framework.StateKey {
	return framework.StateKey(fmt.Sprintf("%v/podstate", GPUShareStateKeyNamespace))
}

func GetGPUSharePodState(cycleState *framework.CycleState) (*GPUSharePodState, error) {
	key := getPodStateKey()
	c, err := cycleState.Read(key)
	if err != nil {
		return nil, fmt.Errorf("error reading %q from cycleState: %v", key, err)
	}
	s, ok := c.(*GPUSharePodState)
	if !ok {
		return nil, fmt.Errorf("%+v  convert to GPUSharePodState error", c)
	}
	return s, nil
}

func CreateAndSaveGPUSharePodState(state *framework.CycleState, RequestGPUs int, ownWholeGPU bool) {
	key := getPodStateKey()
	s := &GPUSharePodState{
		RequestGPUs:     RequestGPUs,
		RequireWholeGPU: ownWholeGPU,
	}
	state.Write(key, s)
}

// GPUSharePodState  computed at PreFilter for Reserve.
type GPUSharePodState struct {
	RequestGPUs     int
	RequireWholeGPU bool
}

// Clone the pod state.
func (s *GPUSharePodState) Clone() framework.StateData {
	return s
}

func getGPUShareNodeStateKey(nodeName string) framework.StateKey {
	return framework.StateKey(fmt.Sprintf("%v/allocation/%v", GPUShareStateKeyNamespace, nodeName))
}

func GetGPUShareNodeState(cycleState *framework.CycleState, nodeName string) (*GPUShareNodeState, error) {
	key := getGPUShareNodeStateKey(nodeName)
	c, err := cycleState.Read(key)
	if err != nil {
		return nil, fmt.Errorf("error reading %q from cycleState: %v", key, err)
	}
	s, ok := c.(*GPUShareNodeState)
	if !ok {
		return nil, fmt.Errorf("%+v  convert to GPUShareNodeState error", c)
	}
	return s, nil
}

func CreateAndSaveGPUShareNodeState(state *framework.CycleState, nodeName string, s *GPUShareNodeState) {
	key := getGPUShareNodeStateKey(nodeName)
	state.Write(key, s)
}

type GPUShareNodeState struct {
	PolicyInstance      runtime.PolicyInstance
	NodeResourceManager *noderesource.NodeResourceManager
	MaxPodsPerDevice    int64
}

func (s *GPUShareNodeState) Clone() framework.StateData {
	return s
}

type GPUSharePreemptionNodeState struct {
}
