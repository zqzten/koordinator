package intelligentscheduler

import (
	"context"
	"fmt"
	frameworkruntime "github.com/koordinator-sh/koordinator/pkg/descheduler/framework/runtime"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/devicesharing/runtime"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/intelligentscheduler/CRDs"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"reflect"
	"time"
)

const IntelligentSchedulerName = "intelligent-scheduler"

// VirtualGpuSpecification found in annotation means a pod needed to be process by IntelligencePlugin
const (
	GPUResourceCountName       = "aliyun.com/gpu-count"
	VirtualGpuSpecificationKey = "intelligent.sofastack.io/vgpu-specification"
	VirtualGpuCountKey         = "alipay.com/virtual.gpu.count"
	VirtualGpuPodResourceKey   = "sofastack.io/intelligent-vgpu"
	MsgNoNeedToHandlePod       = "no need to handle this pod cause it does not contain specified annotation key"
	SchedulerNodeLabel         = "ack.node.gpu.schedule"
	OversellRateNodeLabel      = "ack.node.gpu.schedule.oversell"
	PhysicalGpuCountNodeLabel  = "aliyun.accelerator/nvidia_count"
	PhysicalGpuMemNodeLabel    = "aliyun.accelerator/nvidia_mem"
	PhysicalGpuTypeNodeLabel   = "aliyun.accelerator/nvidia_name"
	NameSpace                  = "Intelligent-Computing"
	IntelligentGroupName       = "intelligent.sofastack.io"
	IntelligentVersion         = "v1"
	VgsResourceName            = "VirtualGpuSpecification"
	VgiResourceName            = "VirtualGpuInstance"
	PgiResourceName            = "PhysicalGpuInstance"
)

type IntelligentScheduler struct {
	resourceNames []v1.ResourceName
	engine        *IntelligentSchedulerRuntime
	args          IntelligentSchedulerArgs
	cache         *intelligentCache
	handle        framework.Handle
	client        dynamic.Interface
}

func New(obj apiruntime.Object, handle framework.Handle) (framework.Plugin, error) {
	klog.Infof("start to create gpuscheduler plugin")
	unknownObj := obj.(*apiruntime.Unknown)
	intelligentscheduler := &IntelligentScheduler{
		resourceNames: []v1.ResourceName{VirtualGpuPodResourceKey}, // 加入两个自定义资源类型
		args:          IntelligentSchedulerArgs{},
		handle:        handle,
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

	nodes, err := i.handle.SharedInformerFactory().Core().V1().Nodes().Lister().List(labels.Everything())
	if err != nil {
		return err
	}
	for _, node := range nodes {
		handleAddOrUpdateNode(node, i.cache)
	}

	nodeInformer := i.handle.SharedInformerFactory().Core().V1().Nodes().Informer()
	nodeInformer.AddEventHandler(cache.FilteringResourceEventHandler{
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
			AddFunc: func(obj interface{}) {
				node, ok := obj.(*v1.Node)
				if !ok {
					klog.Errorf("failed to convert %v to *v1.Node", reflect.TypeOf(obj))
					return
				}
				handleAddOrUpdateNode(node, i.cache)
			},
			UpdateFunc: func(old, new interface{}) {
				oldNode, ok := old.(*v1.Node)
				if !ok {
					klog.Errorf("failed to convert %v to *v1.Node", reflect.TypeOf(old))
					return
				}
				newNode, ok := new.(*v1.Node)
				if !ok {
					klog.Errorf("failed to convert %v to *v1.Node", reflect.TypeOf(new))
					return
				}
				updateNode(i.cache, oldNode, newNode)
			},
			DeleteFunc: func(obj interface{}) {
				node, ok := obj.(*v1.Node)
				if !ok {
					klog.Errorf("failed to convert %v to *v1.Node", reflect.TypeOf(obj))
					return
				}
				deleteNode(i.cache, node)
			},
		},
	})

	factory := dynamicinformer.NewDynamicSharedInformerFactory(i.client, 30*time.Second)

	vgsInformer := factory.ForResource(vgsGvr).Informer()
	vgsInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			switch t := obj.(type) {
			case *CRDs.VirtualGpuSpecification:
				return true
			case cache.DeletedFinalStateUnknown:
				if _, ok := t.Obj.(*CRDs.VirtualGpuSpecification); ok {
					return true
				}
				return false
			default:
				return false
			}
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				vgs, ok := obj.(*CRDs.VirtualGpuSpecification)
				if !ok {
					klog.Errorf("failed to convert %v to VirtualGpuSpecification", reflect.TypeOf(obj))
					return
				}
				i.cache.addOrUpdateVgsInfo(vgs)
			},
			UpdateFunc: func(old, new interface{}) {
				_, ok := old.(*CRDs.VirtualGpuSpecification)
				if !ok {
					klog.Errorf("failed to convert %v to VirtualGpuSpecification", reflect.TypeOf(old))
					return
				}
				newVgs, ok := new.(*CRDs.VirtualGpuSpecification)
				if !ok {
					klog.Errorf("failed to convert old vgs %v to VirtualGpuSpecification", reflect.TypeOf(new))
					return
				}
				i.cache.addOrUpdateVgsInfo(newVgs)
			},
			DeleteFunc: func(obj interface{}) {
				vgs, ok := obj.(*CRDs.VirtualGpuSpecification)
				if !ok {
					klog.Errorf("failed to convert old vgs %v to VirtualGpuSpecification", reflect.TypeOf(obj))
					return
				}
				i.cache.deleteVgsInfo(vgs)
			},
		},
	})

	vgiInformer := factory.ForResource(vgiGvr).Informer()
	vgiInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			switch t := obj.(type) {
			case *CRDs.VirtualGpuInstance:
				return true
			case cache.DeletedFinalStateUnknown:
				if _, ok := t.Obj.(*CRDs.VirtualGpuInstance); ok {
					return true
				}
				return false
			default:
				return false
			}
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				vgi, ok := obj.(*CRDs.VirtualGpuInstance)
				if !ok {
					klog.Errorf("failed to convert %v to VirtualGpuInstance", reflect.TypeOf(obj))
					return
				}
				i.cache.addOrUpdateVgiInfo(vgi)
			},
			UpdateFunc: func(old, new interface{}) {
				_, ok := old.(*CRDs.VirtualGpuInstance)
				if !ok {
					klog.Errorf("failed to convert old vgi %v to VirtualGpuInstance", reflect.TypeOf(old))
					return
				}
				newVgs, ok := new.(*CRDs.VirtualGpuInstance)
				if !ok {
					klog.Errorf("failed to convert new vgs %v to VirtualGpuInstance", reflect.TypeOf(new))
					return
				}
				i.cache.addOrUpdateVgiInfo(newVgs)
			},
			DeleteFunc: func(obj interface{}) {
				vgi, ok := obj.(*CRDs.VirtualGpuInstance)
				if !ok {
					klog.Errorf("failed to convert %v to VirtualGpuInstance", reflect.TypeOf(obj))
					return
				}
				i.cache.deleteVgiInfo(vgi)
			},
		},
	})

	return i.engine.Init()
}

func (i *IntelligentScheduler) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
	klog.Infof("IntelligentSchedulePlugin starts to prefilter pod with name [%v]", pod.Name)

	// Check if we need to handle this pod
	needToHandle := runtime.IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		return framework.NewStatus(framework.Success, MsgNoNeedToHandlePod)
	}

	// Check vgpu count and spec
	vGpuCount, vGpuSpec, err := GetVirtualGPUCountAndSpec(pod, i.cache) // TODO pod中spec和count都在annotation里面吗？？
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	// 查询pod对应的所有vgi并将这些vgi写入cycleState
	vgiInfoNames := i.cache.getVgiInfoNamesByPod(pod)
	if len(vgiInfoNames) == 0 {
		klog.Errorf("unable to find VGI infos for pod [%s]", pod.Name)
		return framework.NewStatus(framework.Error, "unable to find VGI infos for the pod")
		//vgiGvr := schema.GroupVersionResource{
		//	Group:    IntelligentGroupName,
		//	Version:  IntelligentVersion,
		//	Resource: VgiResourceName,
		//}
		//vgiCrs, er := i.client.Resource(vgiGvr).Namespace(NameSpace).List(context.TODO(), metav1.ListOptions{})
		//if er != nil {
		//	return framework.NewStatus(framework.UnschedulableAndUnresolvable, er.Error())
		//}
		//for _, item := range vgiCrs.Items {
		//	if item.GetOwnerReferences() != nil {
		//		for _, owner := range item.GetOwnerReferences() {
		//			if owner.UID == pod.UID {
		//				vgiInfoNames = append(vgiInfoNames, item.GetName())
		//			}
		//		}
		//	}
		//}
	}
	if len(vgiInfoNames) != vGpuCount {
		klog.Errorf("the count of vgi is not equal to the requested vGPU count for pod [%s]", pod.Name)
		return framework.NewStatus(framework.Error, "the count of vgi is not equal to the requested vGPU count for pod")
	}
	i.SaveVGpuPodState(state, vGpuCount, vGpuSpec, string(pod.UID))
	i.SaveVgiState(state, vgiInfoNames, string(pod.UID))
	return nil
}

func (i *IntelligentScheduler) AddPod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod, podToAdd *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	klog.Infof("start to run intelligent AddPod function, pod name %s", podToAdd.Pod.Name)
	p := podToAdd.Pod
	if !runtime.IsMyPod(p, i.resourceNames...) {
		return framework.NewStatus(framework.Success, "")
	}
	if p.Spec.NodeName == "" {
		return framework.NewStatus(framework.Success, "")
	}

	var nodeInfos *NodeInfo
	nodeState, er := i.GetNodeState(state, nodeInfo.Node().Name)
	if er != nil {
		nodeInfos = i.cache.getNewNodeInfo(nodeInfo.Node().Name)
	} else {
		nodeInfos = nodeState.getNodeInfo()
	}
	nodeName := nodeInfo.Node().Name

	var vgiNames []string
	vgiState, err := i.GetVgiState(state, string(p.UID))
	if err != nil {
		vgiNames = i.cache.getVgiInfoNamesByPod(p)
	} else {
		vgiNames = vgiState.getVgiNames()
	}
	er = addPod(i.cache, vgiNames, p, nodeName, nodeInfos)
	if er != nil {
		klog.Errorf("failed to add pod [%s] to node [%s] in AddPod interface", p.Name, nodeName)
		return framework.NewStatus(framework.Error, er.Error())
	}
	return framework.NewStatus(framework.Success, "")
}

func (i *IntelligentScheduler) RemovePod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod, podToRemove *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	p := podToRemove.Pod
	if !runtime.IsMyPod(p, i.resourceNames...) {
		return framework.NewStatus(framework.Success, "")
	}
	if p.Spec.NodeName == "" {
		return framework.NewStatus(framework.Success, "")
	}

	var vgiNames []string
	vgiState, err := i.GetVgiState(state, string(p.UID))
	if err != nil {
		vgiNames = i.cache.getVgiInfoNamesByPod(p)
	} else {
		vgiNames = vgiState.getVgiNames()
	}
	for _, name := range vgiNames {
		vgiInfo := i.cache.getVgiInfo(name)
		if vgiInfo == nil {
			return framework.NewStatus(framework.Error, "unable to find VGI info for pod [%s] in AddPod interface", p.Name)
		}
		vgiInfo.lock.Lock()
		vgiInfo.setStatus("Pending")
		vgiInfo.setPhysicalGpuSpecification("")
		vgiInfo.setNode("")
		vgiInfo.setGPUIndex(-1)
		vgiInfo.lock.Unlock()
	}
	return framework.NewStatus(framework.Success, "")
}

func (i *IntelligentScheduler) PreFilterExtensions() framework.PreFilterExtensions {
	return i
}

func (i *IntelligentScheduler) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		klog.Error("node not found, it may be deleted")
		return framework.NewStatus(framework.Error, "node not found")
	}
	nodeName := node.Name
	ok := i.cache.getIntelligentNode(nodeName)
	if !ok {
		klog.Error("node is not an intelligent scheduled node")
		return framework.NewStatus(framework.Error, "node not an intelligent scheduled node")
	}
	var nodeInfos *NodeInfo
	ns, err := i.GetNodeState(state, nodeName) // 尝试从cycleState中取state
	if err != nil {
		nodeInfos = i.cache.getNodeInfo(nodeName) // 转为从cache中取
		if nodeInfos == nil {
			klog.Errorf("Failed to get node info for node %s from cache or cycleState.", nodeName)
			return framework.NewStatus(framework.Error, "failed to get node info from cache or cycleState")
		}
	} else {
		nodeInfos = ns.getNodeInfo()
	}
	vGpuPodState, err := i.GetVGpuPodState(state, string(pod.UID))
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	var vgiNames []string
	vgiState, err := i.GetVgiState(state, string(pod.UID))
	if err != nil {
		vgiNames = i.cache.getVgiInfoNamesByPod(pod)
	} else {
		vgiNames = vgiState.getVgiNames()
	}
	available := i.nodeAvailableForPod(nodeName, nodeInfos, vGpuPodState, vgiNames)
	if !available {
		klog.Errorf("node %s is not available for pod %s due to the GPU resource limit", nodeName, pod.Namespace+"/"+pod.Name)
		return framework.NewStatus(framework.Unschedulable, "node is not available for pod due to the GPU resources")
	}
	return framework.NewStatus(framework.Success)
}

func (i *IntelligentScheduler) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	var vgiNames []string
	vgiState, err := i.GetVgiState(state, string(pod.UID))
	if err != nil {
		vgiNames = i.cache.getVgiInfoNamesByPod(pod)
	} else {
		vgiNames = vgiState.getVgiNames()
	}
	var nodeInfos *NodeInfo
	nodeState, err := i.GetNodeState(state, nodeName)
	if err != nil {
		nodeInfos = i.cache.getNodeInfo(nodeName)
	} else {
		nodeInfos = nodeState.getNodeInfo()
	}
	score := i.engine.calculateNodeScore(i.cache, nodeName, nodeInfos, vgiNames)
	return int64(score), framework.NewStatus(framework.Success, "")
}

func (i *IntelligentScheduler) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	// TODO patch VGIs信息，同步cache中的vgi信息
	// TODO save nodeInfo to cycleState
	panic("implement me")
}

func (i *IntelligentScheduler) nodeAvailableForPod(nodeName string, nodeInfos *NodeInfo, vGpuPodState *VirtualGpuPodState, vgiNames []string) bool {
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
		if spec == nodeInfos.getGpuType() {
			ok = true
		}
	}
	if !ok {
		return false
	}
	vgi := i.cache.getVgiInfo(vgiNames[0]).Clone()
	// 判断oversell
	if nodeInfos.getIsOversell() != vgi.getIsOversell() {
		return false
	}
	// 判断available数量是否满足
	nodeGpuState, totalMem := getNodeGpuState(nodeName, nodeInfos, i.cache)
	availableGpuCount := 0
	for idx := 0; idx < len(nodeGpuState); idx++ {
		available, _, _ := isAvailableForVgi(i.cache, nodeGpuState[idx], vgi, totalMem)
		if available {
			availableGpuCount++
		}
	}
	if availableGpuCount >= requestVGpuCount {
		return true
	} else {
		return false
	}
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

func (i *IntelligentScheduler) SaveVgiState(state *framework.CycleState, vgiNames []string, podUID string) {
	virtualGpuInstanceState := &VirtualGpuInstanceState{
		vgiNames: vgiNames,
	}
	state.Write(GetVGpuInstanceStateKey(podUID), virtualGpuInstanceState)
}

func (i *IntelligentScheduler) SaveNodeState(state *framework.CycleState, nodeInfo *NodeInfo, nodeName string) {
	nodeState := &NodeState{
		nodeInfo: nodeInfo, // TODO
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
