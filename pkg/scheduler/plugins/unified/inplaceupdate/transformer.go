package inplaceupdate

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/node"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
)

func (p *Plugin) BeforePreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*corev1.Pod, bool, *framework.Status) {
	return pod, false, nil
}

func (p *Plugin) AfterPreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) *framework.Status {
	state, status := getPreFilterState(cycleState)
	if status != nil {
		return nil
	}
	if state.skip {
		return nil
	}
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(pod.Spec.NodeName)
	if err != nil {
		return framework.AsStatus(err)
	}
	extendedHandle, _ := p.handle.(frameworkext.FrameworkExtender)
	if extendedHandle == nil {
		return framework.NewStatus(framework.Error, "handle can't convert to FrameworkExtender")
	}
	err = extendedHandle.Scheduler().GetCache().InvalidNodeInfo(pod.Spec.NodeName)
	if err != nil {
		klog.ErrorS(err, "Failed to InvalidNodeInfo", "node", node.Name)
		return framework.AsStatus(err)
	}
	err = nodeInfo.RemovePod(state.targetPod.Pod)
	if err != nil {
		return framework.AsStatus(err)
	}
	return p.handle.RunPreFilterExtensionRemovePod(ctx, cycleState, pod, state.targetPod, nodeInfo)
}
