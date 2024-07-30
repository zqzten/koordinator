package intelligentscheduler

import (
	//CRDs "code.alipay.com/cnstack/intelligent-operator/api/v1"
	"context"
	"fmt"
	frameworkruntime "github.com/koordinator-sh/koordinator/pkg/descheduler/framework/runtime"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/devicesharing/runtime"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/intelligentscheduler/CRDs"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"reflect"
	"sync"
)

const IntelligentSchedulerName = "intelligent-scheduler"

// VirtualGpuSpecification found in annotation means a pod needed to be process by IntelligencePlugin
const (
	VirtualGpuSpecificationKey       = "intelligent.sofastack.io/vgpu-specification"
	VirtualGpuCountKey               = "aliyun.com/gpu-count"
	VirtualGpuPodResourceKey         = "intelligent.sofastack.io/intelligent-vgpu"
	MsgNoNeedToHandlePod             = "no need to handle this pod cause it does not contain specified annotation key"
	SchedulerNodeLabel               = "ack.node.gpu.schedule"
	OversellRateNodeLabel            = "ack.node.gpu.schedule.oversell"
	PhysicalGpuCountNodeLabel        = "aliyun.accelerator/nvidia_count"
	PhysicalGpuMemNodeLabel          = "aliyun.accelerator/nvidia_mem"
	PhysicalGpuTypeNodeLabel         = "aliyun.accelerator/nvidia_name"
	NameSpace                        = "intelligent-computing"
	IntelligentGroupName             = "intelligent.sofastack.io"
	IntelligentVersion               = "v1"
	VgsResourceName                  = "virtualgpuspecifications"
	VgiResourceName                  = "virtualgpuinstances"
	VgiPodNameLabel                  = "podName"
	VgiPodNamespaceLabel             = "podNamespace"
	PodConfigMapLabel                = "vgpu-env-cm"
	IntelligentPodAnnoAssignFlag     = "scheduler.framework.intelligent.assigned"
	IntelligentPodAnnoAssumeTimeFlag = "scheduler.framework.Intelligent.assume-time"
)

type IntelligentScheduler struct {
	resourceNames []v1.ResourceName
	engine        *IntelligentSchedulerRuntime
	args          IntelligentSchedulerArgs
	cache         *intelligentCache
	handle        framework.Handle
	client        dynamic.Interface
}

var _ framework.PreFilterPlugin = &IntelligentScheduler{}
var _ framework.FilterPlugin = &IntelligentScheduler{}
var _ framework.ScorePlugin = &IntelligentScheduler{}
var _ framework.ReservePlugin = &IntelligentScheduler{}

func New(obj apiruntime.Object, handle framework.Handle) (framework.Plugin, error) {
	klog.Infof("start to create gpu-intelligent-scheduler plugin")
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
	intelligentscheduler.engine = NewIntelligentSchedulerRuntime(IntelligentSchedulerName, intelligentscheduler.args.NodeSelectorPolicy, intelligentscheduler.args.GpuSelectorPolicy, int(*intelligentscheduler.args.GPUMemoryScoreWeight), int(*intelligentscheduler.args.GPUUtilizationScoreWeight))
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
	klog.Infof("start to init gpu-intelligent-scheduler plugin")
	cfg := i.handle.KubeConfig()
	i.client = dynamic.NewForConfigOrDie(cfg)
	vgsGvr := schema.GroupVersionResource{
		Group:    IntelligentGroupName,
		Version:  IntelligentVersion,
		Resource: VgsResourceName,
	}
	vgiGvr := schema.GroupVersionResource{
		Group:    IntelligentGroupName,
		Version:  IntelligentVersion,
		Resource: VgiResourceName,
	}

	//vgsCrs, err := i.client.Resource(vgsGvr).Namespace(NameSpace).List(context.TODO(), metav1.ListOptions{})
	//if err != nil {
	//	klog.Errorf("failed to list gpu-intelligent-scheduler vgs resources: %v", err)
	//	return err
	//}
	//for _, cr := range vgsCrs.Items {
	//	handleAddOrUpdateVgs(cr, i.cache)
	//}
	//klog.Info("init vgs to the cache")
	//vgiCrs, err := i.client.Resource(vgiGvr).Namespace(NameSpace).List(context.TODO(), metav1.ListOptions{})
	//if err != nil {
	//	return err
	//}
	//for _, cr := range vgiCrs.Items {
	//	handleAddOrUpdateVgi(cr, i.cache)
	//}
	//klog.Info("init vgi to the cache")
	var once sync.Once
	once.Do(func() {
		stop := make(chan struct{})
		nodes, err := i.handle.SharedInformerFactory().Core().V1().Nodes().Lister().List(labels.Everything())
		if err != nil {
			klog.Errorf("failed to list nodes: %v", err)
			return
		}
		for _, node := range nodes {
			handleAddOrUpdateNode(node, i.cache)
		}
		klog.Info("init intelligent nodes to the cache")
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
					//klog.Infof("succeed to add intelligent node %v to the cache", node.Name)
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
					//klog.Infof("succeed to update intelligent node %v to the cache", newNode.Name)
				},
				DeleteFunc: func(obj interface{}) {
					node, ok := obj.(*v1.Node)
					if !ok {
						klog.Errorf("failed to convert %v to *v1.Node", reflect.TypeOf(obj))
						return
					}
					deleteNode(i.cache, node)
					//klog.Infof("succeed to delete intelligent node %v from the cache", node.Name)
				},
			},
		})

		factory := dynamicinformer.NewDynamicSharedInformerFactory(i.client, 0)
		vgsInformer := factory.ForResource(vgsGvr).Informer()
		vgsInformer.AddEventHandler(
			cache.FilteringResourceEventHandler{
				FilterFunc: func(obj interface{}) bool {
					//klog.Info("in vgs informer filter")
					switch t := obj.(type) {
					case *unstructured.Unstructured:
						return true
					case cache.DeletedFinalStateUnknown:
						if _, ok := t.Obj.(*unstructured.Unstructured); ok {
							return true
						}
						return false
					default:
						return false
					}
				},
				Handler: cache.ResourceEventHandlerFuncs{
					AddFunc: func(obj interface{}) {
						//klog.Info("in vgs AddFunc")
						var vgs *CRDs.VirtualGpuSpecification
						unstructuredObj, ok := obj.(*unstructured.Unstructured)
						if !ok {
							klog.Errorf("failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(obj))
						}
						er := apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &vgs)
						if er != nil {
							klog.Errorf("failed to convert unstruct to VirtualGpuSpecification: %v", er)
							return
						}
						i.cache.addOrUpdateVgsInfo(vgs)
						//klog.Infof("succeed to add VirtualGpuSpecification %v to the cache", vgs.Name)
					},
					UpdateFunc: func(old, new interface{}) {
						//klog.Info("in vgs UpdateFunc")
						var oldVgs *CRDs.VirtualGpuSpecification
						var newVgs *CRDs.VirtualGpuSpecification
						oldUnstructuredObj, ok := old.(*unstructured.Unstructured)
						if !ok {
							klog.Errorf("failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(old))
						}
						oldEr := apiruntime.DefaultUnstructuredConverter.FromUnstructured(oldUnstructuredObj.Object, &oldVgs)
						if oldEr != nil {
							klog.Errorf("failed to convert oldUnstruct to VirtualGpuSpecification: %v", oldEr)
							return
						}
						newUnstructuredObj, ok := old.(*unstructured.Unstructured)
						if !ok {
							klog.Errorf("failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(new))
						}
						newEr := apiruntime.DefaultUnstructuredConverter.FromUnstructured(newUnstructuredObj.Object, &newVgs)
						if newEr != nil {
							klog.Errorf("failed to convert newUnstruct to VirtualGpuSpecification: %v", newEr)
							return
						}

						i.cache.addOrUpdateVgsInfo(newVgs)
						//klog.Infof("succeed to update VirtualGpuSpecification %v to the cache", newVgs.Name)
					},
					DeleteFunc: func(obj interface{}) {
						//klog.Info("in vgs DeleteFunc")
						var vgs *CRDs.VirtualGpuSpecification
						unstructuredObj, ok := obj.(*unstructured.Unstructured)
						if !ok {
							klog.Errorf("failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(obj))
						}
						er := apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &vgs)
						if er != nil {
							klog.Errorf("failed to convert unstruct to VirtualGpuSpecification: %v", er)
							return
						}
						i.cache.deleteVgsInfo(vgs)
						//klog.Infof("succeed to delete VirtualGpuSpecification %v from the cache", vgs.Name)
					},
				},
			})

		vgiInformer := factory.ForResource(vgiGvr).Informer()
		vgiInformer.AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				//klog.Info("in vgi informer filter")
				switch t := obj.(type) {
				case *unstructured.Unstructured:
					return true
				case cache.DeletedFinalStateUnknown:
					if _, ok := t.Obj.(*unstructured.Unstructured); ok {
						return true
					}
					return false
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					var vgi *CRDs.VirtualGpuInstance
					unstructuredObj, ok := obj.(*unstructured.Unstructured)
					if !ok {
						klog.Errorf("failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(obj))
					}
					er := apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &vgi)
					if er != nil {
						klog.Errorf("failed to convert unstruct to VirtualGpuSpecification: %v", er)
						return
					}
					i.cache.addOrUpdateVgiInfo(vgi)
					klog.Infof("succeed to add VirtualGpuInstance %v to the cache", vgi.Name)
				},
				UpdateFunc: func(old, new interface{}) {
					var oldVgi *CRDs.VirtualGpuInstance
					var newVgi *CRDs.VirtualGpuInstance
					oldUnstructuredObj, ok := old.(*unstructured.Unstructured)
					if !ok {
						klog.Errorf("failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(old))
					}
					oldEr := apiruntime.DefaultUnstructuredConverter.FromUnstructured(oldUnstructuredObj.Object, &oldVgi)
					if oldEr != nil {
						klog.Errorf("failed to convert oldUnstruct to VirtualGpuSpecification: %v", oldEr)
						return
					}
					newUnstructuredObj, ok := old.(*unstructured.Unstructured)
					if !ok {
						klog.Errorf("failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(new))
					}
					newEr := apiruntime.DefaultUnstructuredConverter.FromUnstructured(newUnstructuredObj.Object, &newVgi)
					if newEr != nil {
						klog.Errorf("failed to convert newUnstruct to VirtualGpuSpecification: %v", newEr)
						return
					}
					i.cache.addOrUpdateVgiInfo(newVgi)
					klog.Infof("succeed to update VirtualGpuInstance %v to the cache", newVgi.Name)
				},
				DeleteFunc: func(obj interface{}) {
					var vgi *CRDs.VirtualGpuInstance
					unstructuredObj, ok := obj.(*unstructured.Unstructured)
					if !ok {
						klog.Errorf("failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(obj))
					}
					er := apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &vgi)
					if er != nil {
						klog.Errorf("failed to convert unstruct to VirtualGpuSpecification: %v", er)
						return
					}
					i.cache.deleteVgiInfo(vgi)
					klog.Infof("succeed to delete VirtualGpuInstance %v from the cache", vgi.Name)
				},
			},
		})
		factory.Start(stop)
		factory.WaitForCacheSync(stop)
	})
	klog.Info("init finished")
	return i.engine.Init()
}

func (i *IntelligentScheduler) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
	// Check if we need to handle this pod
	needToHandle := runtime.IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		return framework.NewStatus(framework.Success, MsgNoNeedToHandlePod)
	}
	klog.Infof("IntelligentSchedulePlugin starts to prefilter pod with name [%v]", pod.Name)
	// Check vgpu count and spec
	vGpuCount, vGpuSpec, err := GetVirtualGPUCountAndSpec(pod)
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	// 查询pod对应的所有vgi并将这些vgi写入cycleState
	vgiInfoNames := i.cache.getVgiInfoNamesByPod(pod)
	if len(vgiInfoNames) == 0 {
		klog.Errorf("unable to find VGI infos for pod [%s]", pod.Name)
		return framework.NewStatus(framework.Error, "unable to find VGI infos for the pod")
	}
	if len(vgiInfoNames) != vGpuCount {
		klog.Errorf("the count of vgi is not equal to the requested vGPU count for pod [%s]", pod.Name)
		return framework.NewStatus(framework.Error, "the count of vgi is not equal to the requested vGPU count for pod")
	}
	i.SaveVGpuPodState(state, vGpuCount, vGpuSpec, string(pod.UID))
	i.SaveVgiState(state, vgiInfoNames, string(pod.UID))
	klog.Infof("finished prefilting")
	return nil
}

func (i *IntelligentScheduler) AddPod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod, podToAdd *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	p := podToAdd.Pod
	if !runtime.IsMyPod(p, i.resourceNames...) {
		return framework.NewStatus(framework.Success, "")
	}
	klog.Infof("start to run intelligent AddPod function, pod name %s", podToAdd.Pod.Name)
	if p.Spec.NodeName == "" {
		return framework.NewStatus(framework.Success, "")
	}

	var nodeInfos *NodeInfo
	nodeState, er := i.GetNodeState(state, nodeInfo.Node().Name)
	if er != nil {
		nodeInfos = i.cache.getNodeInfo(nodeInfo.Node().Name).Clone()
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
	klog.Infof("start to run intelligent RemovePod function, pod name %s", podToRemove.Pod.Name)
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
	needToHandle := runtime.IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		return framework.NewStatus(framework.Success, "")
	}
	klog.Infof("IntelligentSchedulePlugin starts to filter pod with name [%v], node with name [%v]", pod.Name, nodeInfo.Node().Name)
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
	needToHandle := runtime.IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		return int64(0), framework.NewStatus(framework.Success, "")
	}
	klog.Infof("IntelligentSchedulePlugin starts to score node [%v] for pod [%v]", nodeName, pod.Name)
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

func (i *IntelligentScheduler) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func (i *IntelligentScheduler) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	needToHandle := runtime.IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		return framework.NewStatus(framework.Success, "")
	}
	klog.Infof("IntelligentSchedulePlugin starts to reserve node [%v] for pod [%v]", nodeName, pod.Name)
	var requestGpuCount int
	podState, err := i.GetVGpuPodState(state, string(pod.UID))
	if err != nil {
		gpuCount, _, er := GetVirtualGPUCountAndSpec(pod)
		if er != nil {
			return framework.NewStatus(framework.Error, er.Error())
		}
		requestGpuCount = gpuCount
	} else {
		requestGpuCount = podState.getCount()
	}
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
	var oversellRate int
	if nodeInfos.getIsOversell() {
		oversellRate = nodeInfos.getOversellRate()
	} else {
		oversellRate = 1
	}
	vgi := i.cache.getVgiInfo(vgiNames[0])
	nodeGpuState, totalMem := getNodeGpuState(nodeName, nodeInfos, i.cache)
	var availableGpuIndexSet []int
	for idx := 0; idx < nodeInfos.getGpuCount(); idx++ {
		available, _, _ := isAvailableForVgi(i.cache, nodeGpuState[idx], vgi, totalMem, oversellRate)
		if available {
			availableGpuIndexSet = append(availableGpuIndexSet, idx)
		}
	}
	availableGpuCombines := combine(availableGpuIndexSet, requestGpuCount)
	if len(availableGpuCombines) <= 0 {
		return framework.NewStatus(framework.Unschedulable, "in Reserve step, the node does not have enough available GPU resources for pod [%v]", pod.Namespace+"/"+pod.Name)
	}
	result := i.engine.findBestGpuCombination(i.cache, availableGpuCombines, vgi, nodeGpuState, totalMem, oversellRate)
	if result == nil || len(result) != len(vgiNames) {
		return framework.NewStatus(framework.Unschedulable, "couldn't find proper gpus from node [%v] for pod [%v]", nodeName, pod.Namespace+"/"+pod.Name)
	}
	for idx, vgiName := range vgiNames {
		gpuIdx := result[idx]
		er := patchVgi(i.client, vgiName, nodeName, gpuIdx, "PreAllocated", nodeInfos.getGpuType(), pod)
		if er != nil {
			klog.Infof("in reserve: failed to patch vgi[%v]: [%v]", vgiName, er)
			return framework.NewStatus(framework.Unschedulable, er.Error())
		}
	}
	klog.Infof("in reserve, patch vgis status successfully")
	cmData := buildEnvVars(vgi, result, totalMem)
	err = patchConfigMap(i.client, pod, cmData)
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}
	klog.Infof("in reserve, patch ConfigMap successfully")
	i.SaveNodeState(state, nodeInfos, nodeName)
	return framework.NewStatus(framework.Success, "")
}

func (i *IntelligentScheduler) Unreserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	if !runtime.IsMyPod(pod, i.resourceNames...) {
		return
	}
	klog.Infof("IntelligentSchedulePlugin starts to unreserve node [%v] for pod [%v]", nodeName, pod.Name)
	if !i.cache.getIntelligentNode(nodeName) {
		klog.Infof("node[%v] is not an intelligent node in cache", nodeName)
		return
	}
	var vgiNames []string
	vgiState, err := i.GetVgiState(state, string(pod.UID))
	if err != nil {
		vgiNames = i.cache.getVgiInfoNamesByPod(pod)
	} else {
		vgiNames = vgiState.getVgiNames()
	}
	for _, vgiName := range vgiNames {
		vgiInfo := i.cache.getVgiInfo(vgiName)
		if vgiInfo == nil {
			klog.Warningf("Failed to find vgs info of [%v] for pod [%v] in unreserve plugin", vgiName, pod.Namespace+"/"+pod.Name)
			continue
		}
		vgiInfo.setStatus("Pending")
		vgiInfo.setNode("")
		vgiInfo.setGPUIndex(-1)
		vgiInfo.setPhysicalGpuSpecification("")
	}
	err = patchConfigMap(i.client, pod, nil)
	if err != nil {
		klog.Errorf("Failed to patch pod [%v] in Unreserve with err: %v", pod.Name, err)
	}
	klog.Warningf("Plugin=%v, Phase=UnReserve, Pod=%v, Node=%v, Message: succeed to rollback assumed podresource",
		i.Name(),
		pod.Namespace+"/"+pod.Name,
		nodeName)
}

func (i *IntelligentScheduler) nodeAvailableForPod(nodeName string, nodeInfos *NodeInfo, vGpuPodState *VirtualGpuPodState, vgiNames []string) bool {
	requestVGpuCount := vGpuPodState.getCount()
	requestVGpuSpec := vGpuPodState.getSpec()
	// 判断总数量是否满足
	if requestVGpuCount > nodeInfos.getGpuCount() {
		klog.Infof("gpu count of the node is less than the number of GPU resources for pod")
		return false
	}
	// 判断物理规格是否满足
	vgs := i.cache.getVgsInfo(requestVGpuSpec)
	requestPGpuSpecs := vgs.getPhysicalGpuSpecifications()
	//klog.Infof("vgs physical gpu specs: %v, len: [%v]", requestPGpuSpecs, len(requestPGpuSpecs))
	ok := false
	for _, spec := range requestPGpuSpecs {
		if spec == nodeInfos.getGpuType() {
			ok = true
		}
	}
	if len(requestPGpuSpecs) == 0 || (len(requestPGpuSpecs) == 1 && requestPGpuSpecs[0] == "") {
		//klog.Info("len(requestPGpuSpecs) == 0")
		ok = true
	}
	if !ok {
		klog.Infof("no vgs for pod")
		return false
	}
	vgi := i.cache.getVgiInfo(vgiNames[0]).Clone()
	// 判断oversell
	if !nodeInfos.getIsOversell() && vgi.getIsOversell() {
		return false
	}
	//if nodeInfos.getIsOversell() != vgi.getIsOversell() {
	//	//klog.Infof("vgi oversell[%v], node oversell[%v]", vgi.getIsOversell(), nodeInfos.getIsOversell())
	//	return false
	//}
	var oversellRate int
	if nodeInfos.getIsOversell() {
		oversellRate = nodeInfos.getOversellRate()
	} else {
		oversellRate = 1
	}
	// 判断available数量是否满足
	nodeGpuState, totalMem := getNodeGpuState(nodeName, nodeInfos, i.cache)
	availableGpuCount := 0
	for idx := 0; idx < nodeInfos.getGpuCount(); idx++ {
		available, _, _ := isAvailableForVgi(i.cache, nodeGpuState[idx], vgi, totalMem, oversellRate)
		if available {
			availableGpuCount++
		}
	}
	if availableGpuCount >= requestVGpuCount {
		return true
	} else {
		klog.Infof("available gpus[%v] are fewer than the requested[%v]", availableGpuCount, requestVGpuCount)
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
