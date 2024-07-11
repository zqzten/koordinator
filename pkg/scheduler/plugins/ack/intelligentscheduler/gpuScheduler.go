package intelligentscheduler

import (
	"context"
	"fmt"
	frameworkruntime "github.com/koordinator-sh/koordinator/pkg/descheduler/framework/runtime"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"strconv"
)

const IntelligentSchedulerName = "intelligent-scheduler"

// VirtualGpuSpecification found in annotation means a pod needed to be process by IntelligencePlugin
const (
	VirtualGpuSpecificationKey = "alipay.com/virtual.gpu.specification"
	VirtualGpuCountKey         = "alipay.com/virtual.gpu.count"
	MsgNoNeedToHandlePod       = "no need to handle this pod cause it does not contain specified annotation key"
	MsgPrefilterEndWithSuccess = "prefilter done successfully"
	NameSpace                  = "Intelligent-Scheduling"
	IntelligentGroupName       = "intelligent.sofastack.io"
	IntelligentVersion         = "v1"
	VgsResourceName            = "VirtualGpuSpecification"
	VgiResourceName            = "VirtualGpuInstance"
	PgiResourceName            = "PhysicalGpuInstance"
)

type IntelligentScheduler struct {
	engine *IntelligentSchedulerRuntime
	args   IntelligentSchedulerArgs
	cache  *intelligentCache
	handle framework.Handle
	client dynamic.Interface
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
	cfg := i.handle.KubeConfig()
	i.client = dynamic.NewForConfigOrDie(cfg)
	// TODO 将所有可用的vgs存入cache
	vgsGvr := schema.GroupVersionResource{
		Group:    IntelligentGroupName,
		Version:  IntelligentVersion,
		Resource: VgsResourceName,
	}
	vgsCrs, err := i.client.Resource(vgsGvr).Namespace(NameSpace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, cr := range vgsCrs.Items {
		handleAddOrUpdateVgs(cr, i.cache)
	}

	vgiGvr := schema.GroupVersionResource{
		Group:    IntelligentGroupName,
		Version:  IntelligentVersion,
		Resource: VgiResourceName,
	}
	vgiCrs, err := i.client.Resource(vgiGvr).Namespace(NameSpace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, cr := range vgiCrs.Items {
		handleAddOrUpdateVgi(cr, i.cache)
	}

	pgiGvr := schema.GroupVersionResource{
		Group:    IntelligentGroupName,
		Version:  IntelligentVersion,
		Resource: PgiResourceName,
	}
	pgiCrs, err := i.client.Resource(pgiGvr).Namespace(NameSpace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, cr := range pgiCrs.Items {
		handleAddOrUpdatePgi(cr, i.cache)
	}

	nodes, err := i.handle.SharedInformerFactory().Core().V1().Nodes().Lister().List(labels.Everything())
	if err != nil {
		return err
	}
	for _, node := range nodes {
		handleAddOrUpdateNode(node, i.cache)
	}

	//TODO 实现三个informer。vgsInformer add, delete, update; vgiInformer add, delete, update; nodeInformer
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
	klog.Infof("IntelligentSchedulePlugin starts to prefilter pod with name [%v]", pod.Name)

	// Check if we need to handle this pod
	needToHandle := isIntelligentPod(pod)
	if !needToHandle {
		return framework.NewStatus(framework.Success, MsgNoNeedToHandlePod)
	}

	// Check vgpu count and spec
	vGpuCount, vGpuSpec, err := GetVirtualGPUCountAndSpec(pod, i.cache)
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}

	// TODO 创建vgi，设置其spec部分字段，设置其ownerReference = pod.id，设置其name=（virtual-gpu-instance-{随机数}）存入cycleState中
	ownerRef := []metav1.OwnerReference{
		{
			APIVersion: pod.APIVersion,
			Kind:       pod.Kind,
			Name:       pod.Name,
			UID:        pod.UID,
		},
	}
	vgiGvr := schema.GroupVersionResource{
		Group:    IntelligentGroupName,
		Version:  IntelligentVersion,
		Resource: VgiResourceName,
	}
	var vgiNames map[int]string
	for j := 0; j < vGpuCount; j++ {
		randomSuffix := strconv.Itoa(rand.Intn(10000))
		vgiCrdName := fmt.Sprintf("VGI-%s-%s", vGpuSpec, randomSuffix)
		crd := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": fmt.Sprintf("%s/%s", "apiextensions.k8s.io", "v1"), // TODO 优化，格式化字符串
				"kind":       VgiResourceName,
				"metadata": map[string]interface{}{
					"name":            vgiCrdName,
					"ownerReferences": ownerRef,
				},
				"spec": map[string]interface{}{
					"virtualGpuSpecification": vGpuSpec,
				},
				"status": map[string]interface{}{
					"podUid": string(pod.UID),
					"status": "Pending",
				},
			},
		}
		_, err := i.client.Resource(vgiGvr).Namespace(NameSpace).Create(context.TODO(), crd, metav1.CreateOptions{})
		if err != nil {
			klog.Errorf("Failed to create VGI CRD for Pod %s. [%v]", pod.UID, err.Error())
			return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
		}
		vgiNames[j] = vgiCrdName
	}

	i.SaveVGpuPodState(state, vGpuCount, vGpuSpec, string(pod.UID))
	i.SaveVgiState(state, vgiNames, string(pod.UID))
	return nil
}

func (i *IntelligentScheduler) PreFilterExtensions() framework.PreFilterExtensions {
	return i
}

func (i *IntelligentScheduler) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		klog.Error("node not found,it may be deleted")
		return framework.NewStatus(framework.Error, "node not found")
	}
	nodeName := node.Name
	_, ok := i.cache.nodeInfos[nodeName]
	if !ok {
		klog.Error("node not an intelligent scheduled node")
		return framework.NewStatus(framework.Error, "node not an intelligent scheduled node")
	}
	var nodeResources *NodeInfo
	ns, err := i.GetNodeState(state, nodeName)
	if err != nil {
		nodeResources = i.cache.getNodeInfo(nodeName)
	} else {
		nodeResources = &ns.nodeInfo
	}
	vGpuPodState, err := i.GetVGpuPodState(state, string(pod.UID))
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}
	available := nodeAvailableForPod(nodeResources, vGpuPodState) // TODO
	if !available {
		klog.Errorf("node %s is not available for pod %s due to the GPU resources", nodeName, pod.Namespace+"/"+pod.Name)
		return framework.NewStatus(framework.Unschedulable, "node is not available for pod due to the GPU resources")
	}
	return framework.NewStatus(framework.Success)
}

func (i *IntelligentScheduler) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	// TODO 完善VGIs信息，同步cache中的vgi信息
	panic("implement me")
}

func GetVGpuPodStateKey(podUID string) framework.StateKey {
	return framework.StateKey(fmt.Sprintf("%v/podstate", podUID))
}

func GetVGpuInstanceStateKey(podUID string) framework.StateKey {
	return framework.StateKey(fmt.Sprintf("%v/vgistate", podUID))
}

func GetNodeStateKey(nodeName string) framework.StateKey {
	return framework.StateKey(fmt.Sprintf("%v/nodestate", nodeName))
}

func (i *IntelligentScheduler) SaveVGpuPodState(state *framework.CycleState, VGpuCount int, VGpuSpecification string, podUID string) {
	virtualGpuPodState := &VirtualGpuPodState{
		VGpuCount:         VGpuCount,
		VGpuSpecification: VGpuSpecification,
	}
	state.Write(GetVGpuPodStateKey(podUID), virtualGpuPodState)
}

func (i *IntelligentScheduler) SaveVgiState(state *framework.CycleState, vgiNames map[int]string, podUID string) {
	virtualGpuInstanceState := &VirtualGpuInstanceState{
		vgiNames: vgiNames,
	}
	state.Write(GetVGpuInstanceStateKey(podUID), virtualGpuInstanceState)
}

func (i *IntelligentScheduler) SaveNodeState(state *framework.CycleState, nodeInfo NodeInfo, nodeName string) {
	nodeState := &NodeState{
		nodeInfo: nodeInfo,
	}
	state.Write(GetNodeStateKey(nodeName), nodeState)
}

func (i *IntelligentScheduler) GetVGpuPodState(state *framework.CycleState, podUID string) (*VirtualGpuPodState, error) {
	key := GetVGpuPodStateKey(podUID)
	c, err := state.Read(key)
	if err != nil {
		return nil, fmt.Errorf("failed to read state for pod %s: %v", podUID, err)
	}
	podState, ok := c.(*VirtualGpuPodState)
	if !ok {
		return nil, fmt.Errorf("failed to cast state for pod %s: %v", podUID, err)
	}
	return podState, nil
}

func (i *IntelligentScheduler) GetVgiState(state *framework.CycleState, podUID string) (*VirtualGpuInstanceState, error) {
	key := GetVGpuInstanceStateKey(podUID)
	c, err := state.Read(key)
	if err != nil {
		return nil, fmt.Errorf("failed to read virtual gpu instance states for pod %s: %v", podUID, err)
	}
	vgiState, ok := c.(*VirtualGpuInstanceState)
	if !ok {
		return nil, fmt.Errorf("failed to cast virtual gpu instance state for pod %s: %v", podUID, err)
	}
	return vgiState, nil
}

func (i *IntelligentScheduler) GetNodeState(state *framework.CycleState, name string) (*NodeState, error) {
	key := GetNodeStateKey(name)
	c, err := state.Read(key)
	if err != nil {
		return nil, fmt.Errorf("failed to read state for node %s: %v", name, err)
	}
	nodeState, ok := c.(*NodeState)
	if !ok {
		return nil, fmt.Errorf("failed to cast state for node %s: %v", name, c)
	}
	return nodeState, nil
}
