package gputopology

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/gputopology"
)

var GetTopologyGroupPodsInterval = 10 * time.Millisecond

type ContainerIndex int

const (
	GPUTopologyResourceName      = "aliyun.com/gpu"
	TopologyGroupNameLabelKey    = "gputopology.alibabacloud.com/pod-group"
	TopologyGroupReplicaLabelKey = "gputopology.alibabacloud.com/replica"
	TopologyGroupTTLLabelKey     = "gputopology.alibabacloud.com/ttl"
	GangGroupNameLabelKey        = "pod-group.scheduling.sigs.k8s.io/name"
	GangGroupReplicaLabelKey     = "pod-group.scheduling.sigs.k8s.io/min-available"
	GPUTopologyEnableLabelKey    = "ack.node.gpu.schedule"
	GPUTopologyNamespaceKey      = "gputopology"

	GPUTopologyAllocationKey = "gputopology.alibabacloud.com/allocation"
)

type AllocationInfo struct {
	AllocatedGPUs map[gputopology.ContainerIndex]gputopology.TopologyGPUs `json:"allocatedGPUs"`
	VisibleGPUs   gputopology.TopologyGPUs                                `json:"visibleGPUs"`
	AssumeTime    int64                                                   `json:"assumeTime"`
	Assigned      bool                                                    `json:"assigned"`
}

type PodTopologyState struct {
	PodUID            apitypes.UID
	GroupName         string
	GroupReplica      int
	Allocation        *gputopology.PodTopologyAllocation
	TopologyGroupPods []*v1.Pod
}

func (p *PodTopologyState) Clone() framework.StateData {
	return p.Copy()
}

func (p *PodTopologyState) Copy() *PodTopologyState {
	c := &PodTopologyState{}
	if p.Allocation != nil {
		c.Allocation = p.Allocation.Clone()
	}
	return p
}

func getPodTopologyStateKey(podUID apitypes.UID) framework.StateKey {
	return framework.StateKey(fmt.Sprintf("%v/podstate/%v", GPUTopologyNamespaceKey, podUID))
}

func GetPodTopologyState(cycleState *framework.CycleState, podUID apitypes.UID) (*PodTopologyState, error) {
	key := getPodTopologyStateKey(podUID)
	c, err := cycleState.Read(key)
	if err != nil {
		return nil, fmt.Errorf("error reading %q from cycleState: %v", key, err)
	}
	s, ok := c.(*PodTopologyState)
	if !ok {
		return nil, fmt.Errorf("%+v  convert to GPUTopologyGroupState error", c)
	}
	return s, nil
}

func SetPodTopologyState(state *framework.CycleState, podState *PodTopologyState) {
	key := getPodTopologyStateKey(podState.PodUID)
	state.Write(key, podState)
}

type NodeState struct {
	NodeName string
	Cache    *gputopology.NodeGPUTopologyCache
}

func (n *NodeState) Clone() framework.StateData {
	return &NodeState{
		Cache: n.Cache.Clone(),
	}
}

/*
func (n *NodeState) AddPod(pod *v1.Pod) {
	gpus := gputopology.TopologyGPUs{}
	request := GetPodRequestResource(pod, GPUTopologyResourceName)
	for i := 0; i < request; i++ {
		gpus = append(gpus, "1")
	}
	podTopologyAllocation := gputopology.NewPodTopologyAllocation(pod.UID)
	podTopologyAllocation.AllocatedGPUs = map[gputopology.ContainerIndex]gputopology.TopologyGPUs{
		0: gpus,
	}
	podTopologyAllocation.SetPhase(gputopology.SchedulePhaseFinishBind)
	n.Cache.AddPod(podTopologyAllocation)
}

func (n *NodeState) RemovePod(pod *v1.Pod) {
	n.Cache.RemovePod(pod.UID)
}
*/

func getNodeStateKey(nodeName string) framework.StateKey {
	return framework.StateKey(fmt.Sprintf("%v/%v", GPUTopologyNamespaceKey, nodeName))
}

func GetNodeState(cycleState *framework.CycleState, nodeName string) (*NodeState, error) {
	key := getNodeStateKey(nodeName)
	c, err := cycleState.Read(key)
	if err != nil {
		return nil, fmt.Errorf("error reading %q from cycleState: %v", key, err)
	}
	s, ok := c.(*NodeState)
	if !ok {
		return nil, fmt.Errorf("%+v  convert to GPUTopologyNodeState error", c)
	}
	return s, nil
}

func SetGPUTopologyNodeState(state *framework.CycleState, nodeState *NodeState) {
	key := getNodeStateKey(nodeState.NodeName)
	state.Write(key, nodeState)
}
