package gpuoversell

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/devicesharing/runtime"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	GPUOversellName                  = "GPUOversell"
	replicaDeviceLabelKey            = "aliyun.com/gpu-count"
	GPUResourceCountName             = "aliyun.com/gpu-count"
	GPUOversellPodAnnoAssignFlag     = "scheduler.framework.gpuoversell.assigned"
	GPUOversellPodAnnoAssumeTimeFlag = "scheduler.framework.gpuoversell.assume-time"
)

var _ framework.PreFilterPlugin = &GPUOversell{}

type GPUOversell struct {
	resourceNames   []v1.ResourceName
	frameworkHandle framework.Handle
	nodeCache       *NodeCache
}

func New(_ apiruntime.Object, handle framework.Handle) (framework.Plugin, error) {
	g := &GPUOversell{
		resourceNames:   []v1.ResourceName{"aliyun.com/gpu-mem-oversell"},
		frameworkHandle: handle,
		nodeCache:       newNodeCache(),
	}
	return g, g.Init()
}

func (g *GPUOversell) Init() error {
	nodes, err := g.frameworkHandle.SharedInformerFactory().Core().V1().Nodes().Lister().List(labels.Everything())
	if err != nil {
		return err
	}
	pods, err := g.frameworkHandle.SharedInformerFactory().Core().V1().Pods().Lister().List(labels.Everything())
	if err != nil {
		return err
	}

	for _, node := range nodes {
		AddOrUpdateNode(g.resourceNames, node, g.nodeCache)
	}
	// recover use resources from pod
	for _, pod := range pods {
		AddPod(g.resourceNames, pod, g.nodeCache)
	}
	nodeInformer := g.frameworkHandle.SharedInformerFactory().Core().V1().Nodes().Informer()
	nodeInformer.AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.Node:
					return true
				case cache.DeletedFinalStateUnknown:
					if _, ok := t.Obj.(*v1.Node); ok {
						return true
					}
					return false
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				UpdateFunc: func(oldObj, newObj interface{}) {
					oldNode, ok := oldObj.(*v1.Node)
					if !ok {
						klog.Errorf("can not convert %v to *v1.Node", reflect.TypeOf(oldObj))
						return
					}
					newNode, ok := newObj.(*v1.Node)
					if !ok {
						klog.Errorf("can not convert %v to *v1.Node", reflect.TypeOf(newObj))
						return
					}
					UpdateNode(g.resourceNames, oldNode, newNode, g.nodeCache)
				},
			},
		},
	)

	podInformer := g.frameworkHandle.SharedInformerFactory().Core().V1().Pods().Informer()
	podInformer.AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.Pod:
					return len(t.Spec.NodeName) != 0
				case cache.DeletedFinalStateUnknown:
					if pod, ok := t.Obj.(*v1.Pod); ok {
						return len(pod.Spec.NodeName) != 0
					}
					return false
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					pod, ok := obj.(*v1.Pod)
					if !ok {
						klog.Errorf("can not convert %v to *v1.Pod", reflect.TypeOf(obj))
						return
					}
					AddPod(g.resourceNames, pod, g.nodeCache)
				},
				DeleteFunc: func(obj interface{}) {
					var pod *v1.Pod
					switch t := obj.(type) {
					case *v1.Pod:
						pod = t
					case cache.DeletedFinalStateUnknown:
						var ok bool
						pod, ok = t.Obj.(*v1.Pod)
						if !ok {
							klog.Errorf("cannot convert to *v1.Pod: %v", t.Obj)
							return
						}
					default:
						klog.Errorf("cannot convert to *v1.Pod: %v", t)
						return
					}
					RemovePod(g.resourceNames, pod, g.nodeCache)
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					oldPod, ok := oldObj.(*v1.Pod)
					if !ok {
						klog.Errorf("can not convert %v to *v1.Pod", reflect.TypeOf(oldObj))
						return
					}
					newPod, ok := newObj.(*v1.Pod)
					if !ok {
						klog.Errorf("can not convert %v to *v1.Pod", reflect.TypeOf(newObj))
						return
					}
					UpdatePod(g.resourceNames, oldPod, newPod, g.nodeCache)
				},
			},
		},
	)
	return nil
}

func (g *GPUOversell) Name() string {
	return GPUOversellName
}

func (g *GPUOversell) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
	resourceNameMap := map[v1.ResourceName]bool{}
	for _, resourceName := range g.resourceNames {
		resourceNameMap[resourceName] = true
	}
	containerRequests := map[int]v1.ResourceList{}
	for index, container := range pod.Spec.Containers {
		requests := v1.ResourceList{}
		for resourceName, r := range container.Resources.Limits {
			if _, ok := resourceNameMap[resourceName]; ok && r.Value() > 0 {
				requests[resourceName] = r
			}
		}
		if len(requests) != 0 {
			containerRequests[index] = requests
		}
	}
	if len(containerRequests) == 0 {
		return nil
	}
	if !runtime.ContainersRequestResourcesAreTheSame(pod, g.resourceNames...) {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "The requested gpu resource types should be consistent for all containers")
	}
	replica, err := getReplicaDeviceCount(containerRequests, pod.Labels)
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	if replica == 0 {
		replica = 1
	}
	CreateAndSaveGPUOversellPodState(state, replica, runtime.GetPodRequestResourcesByNames(pod, g.resourceNames...))
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
	if !runtime.IsMyNode(nodeInfo.Node(), g.resourceNames...) {
		return framework.NewStatus(framework.Success, "")
	}
	if p.Spec.NodeName == "" {
		return framework.NewStatus(framework.Success, "")
	}
	state, err := getGPUOversellNodeState(cycleState, p.Spec.NodeName)
	if err != nil {
		state = g.nodeCache.GetNodeState(p.Spec.NodeName).CloneState()
		state.Reset(nodeInfo.Node(), g.resourceNames)
	}
	addPod(g.resourceNames, p, func(nodeName string) *GPUOversellNodeState {
		return state
	})
	cycleState.Write(getGPUOverSellNodeStateKey(p.Spec.NodeName), state)
	return framework.NewStatus(framework.Success, "")
}

func (g *GPUOversell) RemovePod(ctx context.Context, cycleState *framework.CycleState, podToSchedule *v1.Pod, podToRemove *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	p := podToRemove.Pod
	if !runtime.IsMyPod(p, g.resourceNames...) {
		return framework.NewStatus(framework.Success, "")
	}
	if p.Spec.NodeName == "" {
		return framework.NewStatus(framework.Success, "")
	}
	var nodeState *GPUOversellNodeState
	var err error
	nodeState, err = getGPUOversellNodeState(cycleState, p.Spec.NodeName)
	if err != nil {
		// warning: should clone the node resource
		nodeState = g.nodeCache.GetNodeState(p.Spec.NodeName).CloneState()
		nodeState.Reset(nodeInfo.Node(), g.resourceNames)
	}
	removePod(g.resourceNames, p, func(nodeName string) *GPUOversellNodeState {
		return nodeState
	})
	cycleState.Write(getGPUOverSellNodeStateKey(p.Spec.NodeName), nodeState)
	return framework.NewStatus(framework.Success, "")
}

func (g *GPUOversell) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {

	node := nodeInfo.Node()
	if node == nil {
		klog.Error("node not found,it may be deleted")
		return framework.NewStatus(framework.Error, "node not found")
	}

	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	if !runtime.IsMyPod(pod, g.resourceNames...) {
		return nil
	}

	if !runtime.IsMyNode(node, g.resourceNames...) {
		klog.Warningf("pod=%v,node=%v,message=[node is invalid for gpushare,it has none of resource in %v]", podFullName, node.Name, g.resourceNames)
		return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("none(s) should have at least one resource in %v", g.resourceNames))
	}

	ns, err := getGPUOversellNodeState(state, node.Name)
	if err != nil {
		ns = g.nodeCache.GetNodeState(node.Name)
		ns.Reset(node, g.resourceNames)
		state.Write(getGPUOverSellNodeStateKey(node.Name), ns)
	}
	podState, err := GetGPUOversellPodState(state)
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}
	if ns.GetDevices() < podState.RequestGPUs {
		return framework.NewStatus(framework.Unschedulable, runtime.ErrMsgNotEnoughDevices)
	}
	for pr, v := range podState.RequestResources {
		nr := ns.GetNodeResource(pr)
		total := nr.GetTotal()
		if total-nr.GetAllocated() < v {
			return framework.NewStatus(framework.Unschedulable, runtime.ErrMsgNotEnoughDevices)
		}
		if total*podState.RequestGPUs < v*ns.GetOversellRatio()*ns.GetDevices() {
			return framework.NewStatus(framework.Unschedulable, runtime.ErrMsgNotEnoughActualResources)
		}
	}

	return framework.NewStatus(framework.Success)
}

func getReplicaDeviceCount(containerRequests map[int]v1.ResourceList, labels map[string]string) (int, error) {
	val, ok := labels[replicaDeviceLabelKey]
	if !ok {
		return 0, nil
	}
	replica, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("failed to parse request device count from pod label %v,reason: %v", replicaDeviceLabelKey, err)
	}
	if replica < 0 {
		return 0, fmt.Errorf("request device count must be large than 0,but got %v", replica)
	}
	if replica == 0 {
		return 0, nil
	}
	for _, resourceList := range containerRequests {
		for resourceName, r := range resourceList {
			requestCount := int(r.Value())
			size := requestCount / replica
			if size*replica != requestCount {
				return 0, fmt.Errorf("invalid replica for resource %v,because %v / %v = %.1f", resourceName, requestCount, replica, float32(requestCount)/float32(replica))
			}
		}
	}
	return replica, nil
}

func (g *GPUOversell) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	if !runtime.IsMyPod(pod, g.resourceNames...) {
		return 0, nil
	}
	// get the node
	node, err := g.frameworkHandle.ClientSet().CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return 0, framework.NewStatus(framework.UnschedulableAndUnresolvable, fmt.Sprintf("failed to get node from client-go: %v", err))
	}
	// if node has no resources,skip it
	if !runtime.IsMyNode(node, g.resourceNames...) {
		klog.Warningf("pod=%v,node=%v,message=[node is invalid for gpuoversell,it has none of resource in %v]", podFullName, node.Name, g.resourceNames)
		return 0, framework.NewStatus(framework.Unschedulable, fmt.Sprintf("none(s) should have at least one resource in %v", g.resourceNames))
	}
	ns, err := getGPUOversellNodeState(state, nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Unschedulable, err.Error())
	}
	ratio := ns.GetOversellRatio()
	podState, err := GetGPUOversellPodState(state)
	if err != nil {
		return 0, framework.NewStatus(framework.Unschedulable, err.Error())
	}
	score := 0
	for pr, v := range podState.RequestResources {
		nr := ns.GetNodeResource(pr)
		if nr.GetTotal()/ratio > nr.GetAllocated() {
			score = score + 100 - 30*(nr.GetAllocated()+v)*ratio/nr.GetTotal()
		} else {
			score = score + 70 - 70*(nr.GetAllocated()+v-nr.GetTotal()/ratio)/(nr.GetTotal()-nr.GetTotal()/ratio)
		}
		klog.Infof("pod %s gpuoversell node %s score %d, allocated %d, ratio %d, total %d, request %d",
			podFullName, nodeName, score, nr.GetAllocated(), ratio, nr.GetTotal(), v)
	}
	return int64(score), nil
}

func (g *GPUOversell) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func (g *GPUOversell) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	if !runtime.IsMyPod(pod, g.resourceNames...) {
		return nil
	}
	node, err := g.frameworkHandle.ClientSet().CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, fmt.Sprintf("failed to get node from client-go: %v", err))
	}
	if !runtime.IsMyNode(node, g.resourceNames...) {
		klog.Warningf("pod=%v,node=%v,message=[node is invalid for gpuoversell,it has none of resource in %v]", podFullName, node.Name, g.resourceNames)
		return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("none(s) should have at least one resource in %v", g.resourceNames))
	}

	podRequest := runtime.GetPodRequestResourcesByNames(pod, g.resourceNames...)
	ns := g.nodeCache.GetNodeState(nodeName)

	for r := range podRequest {
		ns.GetNodeResource(r).AddPodResource(pod)
	}

	newPod, err := g.frameworkHandle.SharedInformerFactory().Core().V1().Pods().Lister().Pods(pod.Namespace).Get(pod.Name)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("failed to get pod from client-go cache,reason: %v", err))
	}

	_, err = UpdatePodAnnotations(g.frameworkHandle.ClientSet(), newPod, buildPodAnnotations())
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	klog.V(5).Infof("Plugin=%v,Phase=Reserve,Pod=%v,Node=%v,Message: succeed to update pod annotations",
		g.Name(),
		podFullName,
		nodeName,
	)
	return nil
}

func (g *GPUOversell) Unreserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	if !runtime.IsMyPod(pod, g.resourceNames...) {
		return
	}
	if !g.nodeCache.NodeIsInCache(nodeName) {
		return
	}
	podRequest := runtime.GetPodRequestResourcesByNames(pod, g.resourceNames...)
	ns := g.nodeCache.GetNodeState(nodeName)

	for r, v := range podRequest {
		if v >= 0 {
			ns.GetNodeResource(r).RemoveAllocatedPodResources(pod.UID)
		}
	}

	klog.Warningf("Plugin=%v,Phase=UnReserve,Pod=%v,Node=%v,Message: succeed to rollback assumed podresource",
		g.Name(),
		podFullName,
		nodeName,
	)
}

func buildPodAnnotations() map[string]string {
	return map[string]string{
		GPUOversellPodAnnoAssumeTimeFlag: fmt.Sprintf("%d", time.Now().UnixNano()),
		GPUOversellPodAnnoAssignFlag:     "false",
	}
}

func UpdatePodAnnotations(client clientset.Interface, pod *v1.Pod, annotations map[string]string) (*v1.Pod, error) {
	newPod := updateAnnotations(pod, annotations)
	var updatedPod *v1.Pod
	var err error
	updatedPod, err = client.CoreV1().Pods(newPod.ObjectMeta.Namespace).Update(context.TODO(), newPod, metav1.UpdateOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "the object has been modified") {
			oldPod, err := client.CoreV1().Pods(pod.ObjectMeta.Namespace).Get(context.TODO(), pod.ObjectMeta.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			newPod = updateAnnotations(oldPod, annotations)
			return client.CoreV1().Pods(newPod.ObjectMeta.Namespace).Update(context.TODO(), newPod, metav1.UpdateOptions{})
		}
		return nil, err
	}
	return updatedPod, nil
}

func updateAnnotations(oldPod *v1.Pod, annotations map[string]string) *v1.Pod {
	newPod := oldPod.DeepCopy()
	if len(newPod.ObjectMeta.Annotations) == 0 {
		newPod.ObjectMeta.Annotations = map[string]string{}
	}
	for k, v := range annotations {
		newPod.ObjectMeta.Annotations[k] = v
	}
	return newPod
}
