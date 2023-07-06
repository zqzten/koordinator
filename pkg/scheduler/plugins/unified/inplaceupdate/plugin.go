package inplaceupdate

import (
	"context"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/hijack"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

const (
	Name     = "UnifiedInplaceUpdate"
	stateKey = Name

	ErrTargetPodNotFound = "target pod not found"
)

var (
	_ framework.PreFilterPlugin  = &Plugin{}
	_ framework.PostFilterPlugin = &Plugin{}
	_ framework.ReservePlugin    = &Plugin{}
	_ framework.PreBindPlugin    = &Plugin{}

	_ frameworkext.PreFilterTransformer = &Plugin{}
)

type addPodInQueueFn func(pod *corev1.Pod) error

type Plugin struct {
	handle   framework.Handle
	addInQFn addPodInQueueFn
}

type preFilterState struct {
	skip      bool
	version   string
	targetPod *framework.PodInfo
}

func (s *preFilterState) Clone() framework.StateData {
	return s
}

func getPreFilterState(cycleState *framework.CycleState) (*preFilterState, *framework.Status) {
	value, err := cycleState.Read(stateKey)
	if err != nil {
		return nil, framework.AsStatus(err)
	}
	state := value.(*preFilterState)
	return state, nil
}

func New(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	extendHandle := handle.(frameworkext.ExtendedHandle)
	addInQFn := func(pod *corev1.Pod) error {
		scheduler := extendHandle.Scheduler()
		return scheduler.GetSchedulingQueue().Add(pod)
	}
	p := &Plugin{handle: handle, addInQFn: addInQFn}
	extendHandle.RegisterErrorHandler(p.ErrorHandler)
	return p, nil
}

func (p *Plugin) ErrorHandler(podInfo *framework.QueuedPodInfo, err error) bool {
	inplaceUpdatePod := podInfo.Pod
	if inplaceUpdatePod == nil || !extunified.IsInplaceUpdatePod(inplaceUpdatePod) {
		return false
	}
	klog.Errorf("InplaceUpdate Pod Schedule Failed, err: %s", err.Error())
	targetPodName := extunified.GetInplaceUpdateTargetPodName(inplaceUpdatePod)
	targetPod, err := p.handle.SharedInformerFactory().Core().V1().Pods().Lister().Pods(inplaceUpdatePod.Namespace).Get(targetPodName)
	if err != nil {
		klog.ErrorS(err, "InplaceUpdate Pod get target pod", "InplaceUpdatePod", klog.KObj(inplaceUpdatePod), "targetPodName", targetPodName)
		return true
	}
	targetPodUID := extunified.GetInplaceUpdateTargetPodUID(inplaceUpdatePod)
	if string(targetPod.UID) != targetPodUID {
		klog.Warning(err, "InplaceUpdate target pod not found", "InplaceUpdatePod", klog.KObj(inplaceUpdatePod), "targetPodName", targetPodName)
		return true
	}
	resourceUpdateSpec, err := uniext.GetResourceUpdateSpec(inplaceUpdatePod.Annotations)
	if err != nil {
		klog.ErrorS(err, "InplaceUpdate get resourceUpdateSpec err", "InplaceUpdatePod", klog.KObj(inplaceUpdatePod), "targetPod", klog.KObj(targetPod))
		return true
	}
	// todo 该函数在调度主流程中调用，后续需要改成异步Patch，否则会带来一定的调度开销
	err = reject(p.handle.ClientSet().CoreV1().Pods(targetPod.Namespace), targetPod, resourceUpdateSpec.Version, err)
	if err != nil {
		klog.ErrorS(err, "InplaceUpdate reject err", "InplaceUpdatePod", klog.KObj(inplaceUpdatePod), "targetPod", klog.KObj(targetPod))
	}
	return true
}

func (p *Plugin) Name() string {
	return Name
}

func (p *Plugin) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	if !extunified.IsInplaceUpdatePod(pod) {
		cycleState.Write(stateKey, &preFilterState{
			skip: true,
		})
		return nil, nil
	}
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(pod.Spec.NodeName)
	if err != nil {
		return nil, framework.AsStatus(err)
	}
	var targetPod *framework.PodInfo
	targetPodNamespacedName := util.GetNamespacedName(pod.Namespace, extunified.GetInplaceUpdateTargetPodName(pod))
	targetPodUID := extunified.GetInplaceUpdateTargetPodUID(pod)
	for _, podInfo := range nodeInfo.Pods {
		if util.GetNamespacedName(podInfo.Pod.Namespace, podInfo.Pod.Name) == targetPodNamespacedName && string(podInfo.Pod.UID) == targetPodUID {
			targetPod = podInfo
			break
		}
	}
	if targetPod == nil {
		return nil, framework.NewStatus(framework.Error, ErrTargetPodNotFound)
	}
	resourceUpdateSpec, err := uniext.GetResourceUpdateSpec(pod.Annotations)
	if err != nil {
		return nil, framework.AsStatus(err)
	}
	cycleState.Write(stateKey, &preFilterState{
		skip:      false,
		version:   resourceUpdateSpec.Version,
		targetPod: targetPod,
	})
	return &framework.PreFilterResult{
		NodeNames: sets.NewString(pod.Spec.NodeName),
	}, nil
}

func (p *Plugin) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (p *Plugin) PostFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, filteredNodeStatusMap framework.NodeToStatusMap) (*framework.PostFilterResult, *framework.Status) {
	state, status := getPreFilterState(cycleState)
	if !status.IsSuccess() {
		return nil, framework.NewStatus(framework.Unschedulable)
	}
	if state.skip {
		return nil, framework.NewStatus(framework.Unschedulable)
	}
	return nil, framework.NewStatus(framework.Error, "current pod is inplaceUpdate Pod, stop scheduling")
}

func (p *Plugin) Reserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	state, status := getPreFilterState(cycleState)
	if !status.IsSuccess() {
		return status
	}
	if state.skip {
		return nil
	}
	hijack.SetTargetPod(cycleState, state.targetPod.Pod)
	return nil
}

func (p *Plugin) Unreserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) {
}

func (p *Plugin) PreBind(ctx context.Context, cycleState *framework.CycleState, assumedPod *corev1.Pod, nodeName string) *framework.Status {
	state, status := getPreFilterState(cycleState)
	if !status.IsSuccess() {
		return status
	}
	if state.skip {
		return nil
	}
	resourceUpdateState := &uniext.ResourceUpdateSchedulerState{
		Status:  uniext.ResourceUpdateStateAccepted,
		Version: state.version,
	}
	if assumedPod.Annotations == nil {
		assumedPod.Annotations = map[string]string{}
	}
	err := uniext.SetResourceUpdateSchedulerState(assumedPod.Annotations, resourceUpdateState)
	if err != nil {
		return framework.AsStatus(err)
	}
	return nil
}
