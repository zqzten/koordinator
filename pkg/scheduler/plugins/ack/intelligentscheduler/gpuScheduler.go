package intelligentscheduler

import (
	"context"
	"fmt"
	frameworkruntime "github.com/koordinator-sh/koordinator/pkg/descheduler/framework/runtime"
	v1 "k8s.io/api/core/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"strconv"
)

const IntelligentSchedulerName = "intelligent-scheduler"

// VirtualGpuSpecification found in annotation means a pod needed to be process by IntelligencePlugin
const (
	VirtualGpuSpecificationAnnotationKey = "alipay.com/virtual.gpu.specification"
	VirtualGpuCountAnnotationKey         = "alipay.com/virtual.gpu.count"
	MsgNoNeedToHandlePod                 = "no need to handle this pod cause it does not contain specified annotation key"
	MsgPrefilterEndWithSuccess           = "prefilter done successfully"
)

type IntelligentScheduler struct {
	engine *IntelligentSchedulerRuntime
	args   IntelligentSchedulerArgs
	cache  *intelligentCache
	handle framework.Handle
}

func New(obj apiruntime.Object, handle framework.Handle) (framework.Plugin, error) {
	klog.Infof("start to create gpuscheduler plugin")
	unknownObj := obj.(*apiruntime.Unknown)
	intelligentscheduler := &IntelligentScheduler{
		args:   IntelligentSchedulerArgs{},
		handle: handle,
	}
	if err := frameworkruntime.DecodeInto(unknownObj, &intelligentscheduler.args); err != nil {
		return nil, err
	}
	// 校验Intelligent Scheduler的args
	if err := validateGPUSchedulerArgs(intelligentscheduler.args); err != nil {
		return nil, err
	}
	klog.Infof("succeed to validate IntelligentScheduler args")
	intelligentscheduler.engine = NewIntelligentSchedulerRuntime(IntelligentSchedulerName, intelligentscheduler.args.NodeSelectorPolicy, intelligentscheduler.args.GpuSelectorPolicy)
	intelligentscheduler.cache = newIntelligentCache()
	if err := intelligentscheduler.Init(); err != nil {
		return nil, err
	}
	return intelligentscheduler, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (i *IntelligentScheduler) Name() string {
	return IntelligentSchedulerName
}

// Init 初始化cache，engine
func (i *IntelligentScheduler) Init() error {
	//nodes, err := i.handle.SharedInformerFactory().Core().V1().Nodes().Lister().List(labels.Everything())
	//if err != nil {
	//	return err
	//}
	//pods, err := i.handle.SharedInformerFactory().Core().V1().Pods().Lister().List(labels.Everything())
	//if err != nil {
	//	return err
	//}
	//// TODO 实现下面这两个函数
	//for _, node := range nodes {
	//	AddOrUpdateNode(node, i.cache)
	//}
	//// recover use resources from pod
	//for _, pod := range pods {
	//	AddPod(g.resourceNames, pod, g.nodeCache)
	//}

	//TODO
	return i.engine.Init()
}

func (i *IntelligentScheduler) AddPod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod, podInfoToAdd *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	//TODO implement me
	panic("implement me")
}

func (i *IntelligentScheduler) RemovePod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod, podInfoToRemove *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	//TODO implement me
	panic("implement me")
}

func (i *IntelligentScheduler) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
	klog.Infof("IntelligencePlugin starts to prefilter pod with name [%v]", pod.Name)

	// Check if we need to handle this pod
	needToHandle := NeedToHandlePod(pod)
	if !needToHandle {
		return framework.NewStatus(framework.Success, MsgNoNeedToHandlePod)
	}

	// Check vgpu count
	vGpuCount, err := GetVirtualGPUCount(pod)
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}

	// Check vgpu specification
	vGpuSpecification, err := GetVirtualGpuSpecification(pod)
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}

	i.SaveVGpuPodState(state, vGpuCount, vGpuSpecification)

	return framework.NewStatus(framework.Success, MsgPrefilterEndWithSuccess)

}

func GetVGpuPodStateKey() framework.StateKey {
	return framework.StateKey(fmt.Sprintf("%v/podstate", IntelligentSchedulerName))
}

func (i *IntelligentScheduler) SaveVGpuPodState(state *framework.CycleState, VGpuCount int, VGpuSpecification string) {

	virtualGpuPodState := &VirtualGpuPodState{
		VGpuCount:         VGpuCount,
		VGpuSpecification: VGpuSpecification,
	}
	state.Write(GetVGpuPodStateKey(), virtualGpuPodState)

}

func (i *IntelligentScheduler) PreFilterExtensions() framework.PreFilterExtensions {
	return i
}

func GetVirtualGPUCount(pod *v1.Pod) (int, error) {
	annotations := pod.GetAnnotations()
	countInStr, keyFound := annotations[VirtualGpuCountAnnotationKey]
	if keyFound {
		count, err := strconv.Atoi(countInStr)
		if err != nil {
			return 0, fmt.Errorf("unable to parse %v %v into integer", VirtualGpuCountAnnotationKey, countInStr)
		} else {
			klog.Infof("Pod with name [%v] should be allocated with [%v] virtual gpu", pod.Name, count)
			return count, nil
		}
	} else {
		return 1, nil
	}

}

// NeedToHandlePod If pod annotation contains key "VirtualGpuSpecification", then it needs to be handled
func NeedToHandlePod(pod *v1.Pod) bool {
	annotations := pod.GetAnnotations()
	_, keyFound := annotations[VirtualGpuSpecificationAnnotationKey]
	if keyFound {
		klog.Infof("Pod with name [%v] needs to be handled cause annotation key %v is found", pod.Name, VirtualGpuSpecificationAnnotationKey)
	} else {
		klog.Infof("Pod with name [%v] will be ignored cause annotation key %v is not found", pod.Name, VirtualGpuSpecificationAnnotationKey)
	}
	return keyFound
}

// TODO GPU规格CR
func GetVirtualGpuSpecification(pod *v1.Pod) (string, error) {
	annotations := pod.GetAnnotations()
	specification, keyFound := annotations[VirtualGpuSpecificationAnnotationKey]
	if keyFound {
		klog.Infof("Pod with name [%v] needs to be allocated with gpu specification [%v]", pod.Name, specification)
	} else {
		klog.Infof("Pod with name [%v] has an invalid vgpu specification [%v]", pod.Name, specification)
		return "", fmt.Errorf("invalid vgpu specification [%v]", specification)
	}
	return specification, nil
}
