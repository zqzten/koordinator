package intelligentscheduler

import (
	//CRDs "code.alipay.com/cnstack/intelligent-operator/api/v1"
	"context"
	frameworkruntime "github.com/koordinator-sh/koordinator/pkg/descheduler/framework/runtime"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/intelligentscheduler/CRDs"
	reservationutil "github.com/koordinator-sh/koordinator/pkg/util/reservation"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	pluginhelper "k8s.io/kubernetes/pkg/scheduler/framework/plugins/helper"
	"reflect"
	"sync"
)

// 常量建议用全大写字母，下划线分隔单词
const (
	IntelligentSchedulerName = "intelligent-scheduler"

	VirtualGpuSpecificationKey = "intelligent.sofastack.io/vgpu-specification"
	VirtualGpuCountKey         = "aliyun.com/gpu-count"
	VirtualGpuPodResourceKey   = "intelligent.sofastack.io/intelligent-vgpu"
	MsgNoNeedToHandlePod       = "no need to handle this pod cause it does not contain specified annotation key"
	SchedulerNodeLabel         = "ack.node.gpu.schedule"
	OversellRateNodeLabel      = "ack.node.gpu.schedule.oversell"

	PhysicalGpuCountNodeLabel = "aliyun.accelerator/nvidia_count"
	PhysicalGpuMemNodeLabel   = "aliyun.accelerator/nvidia_mem"
	PhysicalGpuTypeNodeLabel  = "aliyun.accelerator/nvidia_name"

	IntelligentGroupName = "intelligent.sofastack.io"
	IntelligentVersion   = "v1"
	VgsResourceName      = "virtualgpuspecifications"
	VgiResourceName      = "virtualgpuinstances"
	VgiPodNameLabel      = "podName"
	VgiPodNamespaceLabel = "podNamespace"
	PodConfigMapLabel    = "vgpu-env-cm"
	NameSpace            = "intelligent-computing"
)

var (
	once sync.Once

	license *License

	VgsGvr = schema.GroupVersionResource{
		Group:    IntelligentGroupName,
		Version:  IntelligentVersion,
		Resource: VgsResourceName,
	}
	VgiGvr = schema.GroupVersionResource{
		Group:    IntelligentGroupName,
		Version:  IntelligentVersion,
		Resource: VgiResourceName,
	}

	Gv = schema.GroupVersion{
		Group:   IntelligentGroupName,
		Version: IntelligentVersion,
	}
)

type IntelligentScheduler struct {
	resourceNames []v1.ResourceName
	engine        *IntelligentSchedulerRuntime
	args          IntelligentSchedulerArgs // 智算调度器参数，里面指定CM name&namespace，用来查询调度策略、权重等信息
	cache         *intelligentCache        // 维护资源信息
	handle        framework.Handle
	client        dynamic.Interface
	oversellRate  int // 超卖比
}

var _ framework.PreFilterPlugin = &IntelligentScheduler{}
var _ framework.FilterPlugin = &IntelligentScheduler{}
var _ framework.ScorePlugin = &IntelligentScheduler{}
var _ framework.ReservePlugin = &IntelligentScheduler{}

func New(obj apiruntime.Object, handle framework.Handle) (framework.Plugin, error) {
	klog.Infoln("Start to create gpu-intelligent-scheduler plugin")
	unknownObj := obj.(*apiruntime.Unknown)
	intelligentscheduler := &IntelligentScheduler{
		resourceNames: []v1.ResourceName{VirtualGpuPodResourceKey}, // 加入两个自定义资源类型
		args:          IntelligentSchedulerArgs{},
		handle:        handle,
	}
	if err := frameworkruntime.DecodeInto(unknownObj, &intelligentscheduler.args); err != nil {
		klog.Errorf("Failed to decode intelligent-scheduler, err: %v", err)
		return nil, err
	}
	// 校验Intelligent Scheduler的args
	if err := validateGPUSchedulerArgs(intelligentscheduler.args); err != nil {
		klog.Errorf("Failed to validate intelligent-scheduler args, err: %v", err)
		return nil, err
	}
	klog.Infoln("Succeed to validate intelligent-scheduler args")
	// 初始化client
	cfg := intelligentscheduler.handle.KubeConfig()
	intelligentscheduler.client = dynamic.NewForConfigOrDie(cfg)
	klog.Infoln("Succeed to init client for intelligent-scheduler")
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}
	// 从CM中获得调度参数
	unstructuredCM, err := intelligentscheduler.client.Resource(gvr).Namespace(intelligentscheduler.args.CMNamespace).
		Get(context.Background(), intelligentscheduler.args.CMName, metav1.GetOptions{})
	if err != nil { // 如果未查询到CM，则设置为默认参数
		intelligentscheduler.engine = NewIntelligentSchedulerRuntime(IntelligentSchedulerName, "spread", "spread", 50, 50)
		intelligentscheduler.oversellRate = 1
	} else { // 成功查询到CM
		cm := &v1.ConfigMap{}
		err = apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredCM.Object, cm)
		if err != nil {
			klog.Errorf("Filed to convert unstructuredCM to configmap, err: %v", err)
			return nil, err
		}
		err = intelligentscheduler.handleAddOrUpdateCM(cm)
		if err != nil {
			klog.Errorf("Failed to add or update configmap %s, err: %v", cm.Name, err)
			return nil, err
		}
		klog.Infof("Succeed to parse intelligent-scheduler config %s", cm.Name)
	}
	// 初始化cache
	intelligentscheduler.cache = newIntelligentCache()
	if err := intelligentscheduler.Init(); err != nil {
		klog.Errorf("Failed to init intelligent-scheduler cache, err: %v", err)
		return nil, err
	}
	klog.Infoln("Succeed to create gpu-intelligent-scheduler plugin")
	return intelligentscheduler, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (i *IntelligentScheduler) Name() string {
	return IntelligentSchedulerName
}

func (i *IntelligentScheduler) Init() error {
	klog.Infoln("Start to init gpu-intelligent-scheduler plugin")
	// 初始化license
	once.Do(func() {
		license = InitLicense()
	})
	//后台刷新license
	go license.RefreshLicense()

	// informer
	once.Do(func() {
		stop := make(chan struct{})
		nodes, err := i.handle.SharedInformerFactory().Core().V1().Nodes().Lister().List(labels.Everything())
		if err != nil {
			klog.Errorf("Failed to list nodes, err: %v", err)
			return
		}
		klog.Infoln("Succeed to list nodes")
		for _, node := range nodes {
			handleNodeAddEvent(i.cache, node, i.oversellRate)
		}
		klog.Infoln("Success init nodeInfos to intelligent cache ")

		cmInformer := i.handle.SharedInformerFactory().Core().V1().ConfigMaps().Informer()
		cmInformer.AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.ConfigMap:
					return t.Name == i.args.CMName && t.Namespace == i.args.CMNamespace
				case cache.DeletedFinalStateUnknown:
					if cm, ok := obj.(*v1.ConfigMap); ok {
						return cm.Name == i.args.CMName && cm.Namespace == i.args.CMNamespace
					}
					return false
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					cm, ok := obj.(*v1.ConfigMap)
					if !ok {
						klog.Errorf("Failed to convert cm to configmap: %v", obj)
						return
					}
					err := i.handleAddOrUpdateCM(cm)
					if err != nil {
						klog.Errorf("Failed to add configmap %s, err: %v", cm.Name, err)
					}
					klog.Infof("Succeed to add configmap: %v", cm.Data)
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					newCM, ok := newObj.(*v1.ConfigMap)
					if !ok {
						klog.Errorf("Failed to convert newObj to configmap: %v", newObj)
					}
					err := i.handleAddOrUpdateCM(newCM)
					if err != nil {
						klog.Errorf("Failed to update configmap %s, err: %v", newCM.Name, err)
					}
					klog.Infof("Succeed to update configmap: %v", newCM.Data)
				},
			},
		})

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
						klog.Errorf("Failed to convert %v to *v1.Node", reflect.TypeOf(obj))
						return
					}
					handleNodeAddEvent(i.cache, node, i.oversellRate)
				},
				UpdateFunc: func(old, new interface{}) {
					oldNode, ok := old.(*v1.Node)
					if !ok {
						klog.Errorf("Failed to convert %v to *v1.Node", reflect.TypeOf(old))
						return
					}
					newNode, ok := new.(*v1.Node)
					if !ok {
						klog.Errorf("Failed to convert %v to *v1.Node", reflect.TypeOf(new))
						return
					}
					handleNodeUpdateEvent(i.cache, oldNode, newNode, i.oversellRate)
				},
				DeleteFunc: func(obj interface{}) {
					node, ok := obj.(*v1.Node)
					if !ok {
						klog.Errorf("Failed to convert %v to *v1.Node", reflect.TypeOf(obj))
						return
					}
					handleNodeDeleteEvent(i.cache, node)
				},
			},
		})

		podInformer := i.handle.SharedInformerFactory().Core().V1().Pods().Informer()
		podInformer.AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.Pod:
					return true
				case cache.DeletedFinalStateUnknown:
					if _, ok := t.Obj.(*v1.Pod); ok {
						return true
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
						klog.Errorf("Failed to convert %v to *v1.Pod", reflect.TypeOf(obj))
						return
					}
					if !IsMyPod(pod, i.resourceNames...) {
						return
					}
					handleAddPod(i.client, pod)
				},
			},
		})
		// TODO 更新频率
		factory := dynamicinformer.NewDynamicSharedInformerFactory(i.client, 0)
		vgsInformer := factory.ForResource(VgsGvr).Informer()
		vgsInformer.AddEventHandler(
			cache.FilteringResourceEventHandler{
				FilterFunc: func(obj interface{}) bool {
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
						var vgs *CRDs.VirtualGpuSpecification
						unstructuredObj, ok := obj.(*unstructured.Unstructured)
						if !ok {
							klog.Errorf("Failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(obj))
						}
						er := apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &vgs)
						if er != nil {
							klog.Errorf("Failed to convert unstruct to VirtualGpuSpecification, err: %v", er)
							return
						}
						i.cache.addOrUpdateVgsInfo(vgs)
						//klog.Infof("succeed to add VirtualGpuSpecification %v to the cache", vgs.Name)
					},
					UpdateFunc: func(old, new interface{}) {
						var oldVgs *CRDs.VirtualGpuSpecification
						var newVgs *CRDs.VirtualGpuSpecification
						oldUnstructuredObj, ok := old.(*unstructured.Unstructured)
						if !ok {
							klog.Errorf("Failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(old))
						}
						oldEr := apiruntime.DefaultUnstructuredConverter.FromUnstructured(oldUnstructuredObj.Object, &oldVgs)
						if oldEr != nil {
							klog.Errorf("Failed to convert oldUnstruct to VirtualGpuSpecification: %v", oldEr)
							return
						}
						newUnstructuredObj, ok := new.(*unstructured.Unstructured)
						if !ok {
							klog.Errorf("Failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(new))
						}
						newEr := apiruntime.DefaultUnstructuredConverter.FromUnstructured(newUnstructuredObj.Object, &newVgs)
						if newEr != nil {
							klog.Errorf("Failed to convert newUnstruct to VirtualGpuSpecification: %v", newEr)
							return
						}

						i.cache.addOrUpdateVgsInfo(newVgs)
						//klog.Infof("succeed to update VirtualGpuSpecification %v to the cache", newVgs.Name)
					},
					DeleteFunc: func(obj interface{}) {
						var vgs *CRDs.VirtualGpuSpecification
						unstructuredObj, ok := obj.(*unstructured.Unstructured)
						if !ok {
							klog.Errorf("Failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(obj))
						}
						er := apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &vgs)
						if er != nil {
							klog.Errorf("Failed to convert unstruct to VirtualGpuSpecification: %v", er)
							return
						}
						i.cache.deleteVgsInfo(vgs)
						//klog.Infof("succeed to delete VirtualGpuSpecification %v from the cache", vgs.Name)
					},
				},
			})

		vgiInformer := factory.ForResource(VgiGvr).Informer()
		vgiInformer.AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
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
						klog.Errorf("Failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(obj))
					}
					er := apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &vgi)
					if er != nil {
						klog.Errorf("Failed to convert unstruct to VirtualGpuSpecification: %v", er)
						return
					}
					i.cache.addOrUpdateVgiInfo(vgi)
					//klog.Infof("succeed to add VirtualGpuInstance %v to the cache", vgi.Name)
				},
				UpdateFunc: func(old, new interface{}) {
					var oldVgi *CRDs.VirtualGpuInstance
					var newVgi *CRDs.VirtualGpuInstance
					oldUnstructuredObj, ok := old.(*unstructured.Unstructured)
					if !ok {
						klog.Errorf("Failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(old))
					}
					oldEr := apiruntime.DefaultUnstructuredConverter.FromUnstructured(oldUnstructuredObj.Object, &oldVgi)
					if oldEr != nil {
						klog.Errorf("Failed to convert oldUnstruct to VirtualGpuSpecification: %v", oldEr)
						return
					}
					newUnstructuredObj, ok := new.(*unstructured.Unstructured)
					if !ok {
						klog.Errorf("Failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(new))
					}
					newEr := apiruntime.DefaultUnstructuredConverter.FromUnstructured(newUnstructuredObj.Object, &newVgi)
					if newEr != nil {
						klog.Errorf("Failed to convert newUnstruct to VirtualGpuSpecification: %v", newEr)
						return
					}
					i.cache.addOrUpdateVgiInfo(newVgi)
				},
				DeleteFunc: func(obj interface{}) {
					var vgi *CRDs.VirtualGpuInstance
					unstructuredObj, ok := obj.(*unstructured.Unstructured)
					if !ok {
						klog.Errorf("Failed to convert %v to *unstructured.Unstructured", reflect.TypeOf(obj))
					}
					er := apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &vgi)
					if er != nil {
						klog.Errorf("Failed to convert unstruct to VirtualGpuSpecification: %v", er)
						return
					}
					i.cache.deleteVgiInfo(vgi)
					//klog.Infof("succeed to delete VirtualGpuInstance %v from the cache", vgi.Name)
				},
			},
		})
		factory.Start(stop)
		factory.WaitForCacheSync(stop)
	})
	klog.Infoln("init finished")
	return i.engine.Init()
}

func (i *IntelligentScheduler) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
	klog.V(2).Infoln("Verifying the legality of license")
	if !license.CheckLicenseLegality() {
		klog.Errorf("License is not legality: %+v", license)
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "License is not legal")
	}
	// 确定是否需要处理该pod
	needToHandle := IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		klog.Infof("PreFilter: pod %s/%s does not need to be scheduled", pod.Namespace, pod.Name)
		return framework.NewStatus(framework.Success, MsgNoNeedToHandlePod)
	}
	klog.Infof("Start to prefilter pod %s/%s", pod.Namespace, pod.Name)
	// 获得pod指定的VGS种类和数量
	vGpuCount, vGpuSpec, err := GetVirtualGPUCountAndSpec(pod)
	if err != nil {
		klog.Errorf("Failed to get VirtualGPUCount and Spec for pod %s/%s, err: %v", pod.Namespace, pod.Name, err)
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	// 查询pod对应的所有vgi并将这些vgi的name写入cycleState
	vgiInfoNames := i.cache.getVgiInfoNamesByPod(pod)
	if len(vgiInfoNames) == 0 {
		klog.Infof("Unable to find VGI infos for the pod %s/%s", pod.Namespace, pod.Name)
		return framework.NewStatus(framework.Error, "unable to find VGI infos for the pod")
	}
	if len(vgiInfoNames) != vGpuCount {
		klog.Errorf("The count of vgi is not equal to the requested vGPU count for pod %s/%s", pod.Namespace, pod.Name)
		return framework.NewStatus(framework.Error, "the count of vgi is not equal to the requested vGPU count")
	}
	// 将pod申请的vgs数量和种类存入cyclestate
	i.saveVirtualGpuPodToCycleState(state, vGpuCount, vGpuSpec, string(pod.UID))
	// 将该pod所对应的vgi names存入cyclestate
	i.saveVgiToCycleState(state, vgiInfoNames, string(pod.UID))
	klog.Infof("Succeed prefilter pod %s/%s", pod.Namespace, pod.Name)
	return framework.NewStatus(framework.Success)
}

func (i *IntelligentScheduler) AddPod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod, podToAdd *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	p := podToAdd.Pod
	if !IsMyPod(p, i.resourceNames...) {
		return framework.NewStatus(framework.Success)
	}
	klog.Infof("Start to run intelligent AddPod function for pod %s/%s", podToAdd.Pod.Namespace, podToAdd.Pod.Name)
	if p.Spec.NodeName == "" {
		return framework.NewStatus(framework.Success)
	}

	var nodeInfos *NodeInfo
	nodeState, er := i.getNodeStateFromCycleState(state, nodeInfo.Node().Name)
	if er != nil {
		klog.Warningf("Failed to get node state, err: %v, will get it from cache", er)
		nodeInfos = i.cache.getNodeInfo(nodeInfo.Node().Name).Clone()
	} else {
		nodeInfos = nodeState.getNodeInfo()
	}
	nodeName := nodeInfo.Node().Name

	var vgiNames []string
	vgiState, err := i.getVgiStateFromCycleState(state, string(p.UID))
	if err != nil {
		vgiNames = i.cache.getVgiInfoNamesByPod(p)
	} else {
		vgiNames = vgiState.getVgiNames()
	}
	if len(vgiNames) == 0 {
		klog.Errorf("not find vgis for pod %s/%s in AddPod plugin", podToAdd.Pod.Namespace, podToAdd.Pod.Name)
		return framework.NewStatus(framework.Error, "not find vgis in AddPod plugin", podToAdd.Pod.Name)
	}
	er = addPod(i.cache, vgiNames, p, nodeName, nodeInfos)
	if er != nil {
		klog.Errorf("add pod %s/%s Failed, err: %v", podToAdd.Pod.Namespace, podToAdd.Pod.Name, err)
		return framework.NewStatus(framework.Error, er.Error())
	}
	return framework.NewStatus(framework.Success)
}

func (i *IntelligentScheduler) RemovePod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod, podToRemove *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	p := podToRemove.Pod
	if !IsMyPod(p, i.resourceNames...) {
		return framework.NewStatus(framework.Success)
	}
	klog.Infof("Start to run intelligent RemovePod function, pod name %s", podToRemove.Pod.Name)
	if p.Spec.NodeName == "" {
		return framework.NewStatus(framework.Success)
	}

	var vgiNames []string
	vgiState, err := i.getVgiStateFromCycleState(state, string(p.UID))
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
	err = patchConfigMap(i.client, p, nil)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	return framework.NewStatus(framework.Success)
}

func (i *IntelligentScheduler) PreFilterExtensions() framework.PreFilterExtensions {
	return i
}

func (i *IntelligentScheduler) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	needToHandle := IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		klog.Infof("Filter: pod %s/%s does not need to be scheduled", pod.Namespace, pod.Name)
		return framework.NewStatus(framework.Success)
	}
	klog.Infof("Start to filter pod %s/%s", pod.Namespace, pod.Name)
	node := nodeInfo.Node()
	if node == nil {
		klog.Infof("Filter pod %s/%s, a node not found, it may be deleted", pod.Namespace, pod.Name)
		return framework.NewStatus(framework.Unschedulable, "node(s) not found")
	}
	nodeName := node.Name
	// 判断node是否为智算node
	ok := i.cache.getIntelligentNode(nodeName)
	if !ok {
		klog.Infof("Filter pod %s/%s, node %s is not an intelligent scheduled node", pod.Namespace, pod.Name, nodeName)
		return framework.NewStatus(framework.Unschedulable, "node(s) is not intelligent scheduler node")
	}
	var nodeInfos *NodeInfo
	// 从cache中获得node资源信息
	ns, err := i.getNodeStateFromCycleState(state, nodeName) // 尝试从cycleState中取state
	if err != nil {
		klog.Warningf("Filter pod %s/%s, cannt to get nodeInfo %s from cycleState, error is %v, it will be get from cache", pod.Namespace, pod.Name, nodeName, err)
		nodeInfos = i.cache.getNodeInfo(nodeName) // 转为从cache中取
		if nodeInfos == nil {
			klog.Errorf("Filter pod %s/%s, cannt to get nodeInfo %s from intelligentCache", pod.Namespace, pod.Name, nodeName)
			return framework.NewStatus(framework.Error, "Failed to get node info from cache or cycleState")
		}
	} else {
		nodeInfos = ns.getNodeInfo()
	}
	// 获得pod请求资源情况
	vGpuPodState, err := i.getVirtualGpuPodStateFromCycleState(state, string(pod.UID))
	if err != nil {
		klog.Errorf("Filter pod %s/%s, get vGPU pod state failed, error is %v", pod.Namespace, pod.Name, err)
		return framework.NewStatus(framework.Error, err.Error())
	}
	var vgiNames []string
	vgiState, err := i.getVgiStateFromCycleState(state, string(pod.UID))
	if err != nil {
		klog.Warningf("Filter pod %s/%s, get vgi failed, error is %v, it will be get from cache", pod.Namespace, pod.Name, err)
		vgiNames = i.cache.getVgiInfoNamesByPod(pod)
	} else {
		vgiNames = vgiState.getVgiNames()
	}
	// 判断node是否满足pod需求
	available := i.isAvailableNodeForPod(nodeName, nodeInfos, vGpuPodState, vgiNames)
	if !available {
		klog.Infof("Filter pod %s/%s, node %s is not available for pod due to the GPU resources", pod.Namespace, pod.Name, nodeName)
		return framework.NewStatus(framework.Unschedulable, "node(s) is not available for pod due to the GPU resources")
	}
	klog.Infof("Succeed filter pod %s/%s, node %s is available", pod.Namespace, pod.Name, nodeName)
	return framework.NewStatus(framework.Success)
}

func (i *IntelligentScheduler) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	needToHandle := IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		klog.Infof("Score: pod %s/%s does not need to be scheduled", pod.Namespace, pod.Name)
		return int64(0), framework.NewStatus(framework.Success)
	}
	klog.Infof("Start score node %s for pod %s/%s", nodeName, pod.Namespace, pod.Name)
	var vgiNames []string
	// 获得pod拥有的vgi names
	vgiState, err := i.getVgiStateFromCycleState(state, string(pod.UID))
	if err != nil {
		vgiNames = i.cache.getVgiInfoNamesByPod(pod)
	} else {
		vgiNames = vgiState.getVgiNames()
	}
	// 获得node资源信息
	var nodeInfos *NodeInfo
	nodeState, err := i.getNodeStateFromCycleState(state, nodeName)
	if err != nil {
		nodeInfos = i.cache.getNodeInfo(nodeName)
	} else {
		nodeInfos = nodeState.getNodeInfo()
	}
	// 计算node score
	score := i.engine.calculateNodeScore(i.cache, nodeName, nodeInfos, vgiNames)
	// score = math.Max(math.Min(score, float64(framework.MaxNodeScore)), float64(framework.MinNodeScore))
	klog.Infof("Success score node %s for pod %s/%s", nodeName, pod.Namespace, pod.Name)
	return int64(score), framework.NewStatus(framework.Success)
}

func (i *IntelligentScheduler) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func (i *IntelligentScheduler) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	if reservationutil.IsReservePod(pod) {
		return nil
	}
	return pluginhelper.DefaultNormalizeScore(framework.MaxNodeScore, false, scores)
}

func (i *IntelligentScheduler) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	klog.Infof("Start reserve pod %s/%s to node %s", pod.Namespace, pod.Name, nodeName)
	needToHandle := IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		return framework.NewStatus(framework.Success)
	}
	var requestGpuCount int
	podState, err := i.getVirtualGpuPodStateFromCycleState(state, string(pod.UID))
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
	vgiState, err := i.getVgiStateFromCycleState(state, string(pod.UID))
	if err != nil {
		vgiNames = i.cache.getVgiInfoNamesByPod(pod)
	} else {
		vgiNames = vgiState.getVgiNames()
	}

	var nodeInfos *NodeInfo
	nodeState, err := i.getNodeStateFromCycleState(state, nodeName)
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
	// 获得node上的GPU占用情况
	nodeGpuState, totalMem := getNodeGpuState(nodeName, nodeInfos, i.cache)
	// 获得所有符合vgi要求的GPU的index
	var availableGpuIndexSet []int
	for idx := 0; idx < nodeInfos.getGpuCount(); idx++ {
		available, _, _ := isAvailableForVgi(i.cache, nodeGpuState[idx], vgi, totalMem, oversellRate)
		if available {
			availableGpuIndexSet = append(availableGpuIndexSet, idx)
		}
	}

	// 从所有的符合要求的GPU idx中获得数量为申请数量的所有组合的集合
	availableGpuCombines := combine(availableGpuIndexSet, requestGpuCount)
	if len(availableGpuCombines) <= 0 {
		klog.Infof("Reserving pod %s/%s, node %s does not have enough available GPU resources", pod.Namespace, pod.Name, nodeName)
		return framework.NewStatus(framework.Unschedulable, "node does not have enough available GPU resources")
	}

	// 通过计算每个组合的分数，选出分数最高的组合
	result := i.engine.findBestGpuCombination(i.cache, availableGpuCombines, vgi, nodeGpuState, totalMem, oversellRate)
	if result == nil || len(result) != len(vgiNames) {
		klog.Infof("Couldn't find proper gpus from node %s for pod %s/%s]", nodeName, pod.Namespace, pod.Name)
		return framework.NewStatus(framework.Unschedulable, "couldn't find proper gpus from node")
	}

	// 更新vgi状态信息
	for idx, vgiName := range vgiNames {
		gpuIdx := result[idx]
		er := patchVgi(i.client, vgiName, nodeName, gpuIdx, "PreAllocated", nodeInfos.getGpuType(), pod)
		if er != nil {
			klog.Infof("Reserving pod %s/%s, failed to patch vgi %v, err: %v", pod.Namespace, pod.Name, vgiName, er)
			return framework.NewStatus(framework.Unschedulable, er.Error())
		}
	}

	vgs := i.cache.getVgsInfo(vgi.getVgsName())
	// 在CM中注入环境变量
	cmData := buildEnvVars(vgi, vgs, result, totalMem)
	err = patchConfigMap(i.client, pod, cmData)
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}

	i.saveNodeToCycleState(state, nodeInfos, nodeName)
	klog.Infof("Success reserve pod %s/%s to node %s", pod.Namespace, pod.Name, nodeName)
	return framework.NewStatus(framework.Success)
}

func (i *IntelligentScheduler) Unreserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	if !IsMyPod(pod, i.resourceNames...) {
		return
	}
	klog.Infof("Start unreserve pod %s/%s to node %s", pod.Namespace, pod.Name, nodeName)
	if !i.cache.getIntelligentNode(nodeName) {
		klog.Infof("Reserving pod %s/%s, node %s is not an intelligent node in cache", pod.Namespace, pod.Name, nodeName)
		return
	}
	var vgiNames []string
	vgiState, err := i.getVgiStateFromCycleState(state, string(pod.UID))
	if err != nil {
		vgiNames = i.cache.getVgiInfoNamesByPod(pod)
	} else {
		vgiNames = vgiState.getVgiNames()
	}
	for _, vgiName := range vgiNames {
		vgiInfo := i.cache.getVgiInfo(vgiName)
		if vgiInfo == nil {
			klog.Warningf("Failed to find vgs %s info for pod %s/%s in unreserve plugin", vgiName, pod.Namespace, pod.Name)
			continue
		}
		vgiInfo.setStatus("Pending")
		vgiInfo.setNode("")
		vgiInfo.setGPUIndex(-1)
		vgiInfo.setPhysicalGpuSpecification("")
	}
	err = patchConfigMap(i.client, pod, nil)
	if err != nil {
		klog.Errorf("Failed to patch pod %s/%s in Unreserve, err: %v", pod.Namespace, pod.Name, err)
	}
	klog.Warningf("Plugin=%s, Phase=UnReserve, Pod=%s/%s, Node=%s, Message: Succeed to rollback assumed podresource",
		i.Name(),
		pod.Namespace,
		pod.Name,
		nodeName)
}
