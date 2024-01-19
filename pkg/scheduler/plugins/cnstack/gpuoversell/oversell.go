package gpuoversell

import (
	"context"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/devicesharing/runtime"
	v1 "k8s.io/api/core/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"strconv"
)

const GPUOversellName = "GPUOversell"

var _ framework.PreFilterPlugin = &GPUOversell{}

type GPUOversell struct {
	resourceNames []v1.ResourceName
}

func New(_ apiruntime.Object, handle framework.Handle) (framework.Plugin, error) {
	return &GPUOversell{
		resourceNames: []v1.ResourceName{"aliyun.com/gpu-mem-oversell"},
	}, nil
}

func (g *GPUOversell) Name() string {
	return GPUOversellName
}

func (g *GPUOversell) PreFilter(ctx context.Context, state *framework.CycleState, p *v1.Pod) *framework.Status {
	return nil
}

func (g *GPUOversell) PreFilterExtensions() framework.PreFilterExtensions {
	return g
}

func (g *GPUOversell) AddPod(ctx context.Context, cycleState *framework.CycleState, podToSchedule *v1.Pod, podToAdd *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	p := podToAdd.Pod
	if !runtime.IsMyPod(p, g.resourceNames...) {
		return framework.NewStatus(framework.Success, "")
	}
	if p.Spec.NodeName == "" {
		return framework.NewStatus(framework.Success, "")
	}
	state, err := getGPUOversellNodeState(cycleState, p.Spec.NodeName)
	if err != nil {
		state = NewGPUOversellNodeState(getOversellRatio(nodeInfo.Node()))
	}
	addPod(g.resourceNames, p, state)
	cycleState.Write(getGPUOverSellNodeStateKey(p.Spec.NodeName), state)
	return framework.NewStatus(framework.Success, "")
}

func (g *GPUOversell) RemovePod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod, podInfoToRemove *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	return framework.NewStatus(framework.Success, "")
}

func getOversellRatio(node *v1.Node) int64 {
	if node == nil {
		return 1
	}
	v, ok := node.Labels["ack.node.gpu.schedule.oversell"]
	if !ok {
		return 1
	}
	r, err := strconv.Atoi(v)
	if err != nil {
		return 2
	}
	return int64(r)
}
