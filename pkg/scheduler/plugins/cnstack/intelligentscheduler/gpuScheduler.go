package intelligentscheduler

import (
	//CRDs "code.alipay.com/cnstack/intelligent-operator/api/v1"
	"context"
	"fmt"
	frameworkruntime "github.com/koordinator-sh/koordinator/pkg/descheduler/framework/runtime"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/intelligentscheduler/CRDs"
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
	"reflect"
	"strconv"
	"sync"
)

const IntelligentSchedulerName = "intelligent-scheduler"

const (
	VirtualGpuSpecificationKey = "intelligent.sofastack.io/vgpu-specification"
	VirtualGpuCountKey         = "aliyun.com/gpu-count"
	VirtualGpuPodResourceKey   = "intelligent.sofastack.io/intelligent-vgpu"
	MsgNoNeedToHandlePod       = "no need to handle this pod cause it does not contain specified annotation key"
	SchedulerNodeLabel         = "ack.node.gpu.schedule"
	OversellRateNodeLabel      = "ack.node.gpu.schedule.oversell"

	PhysicalGpuCountNodeLabel = "aliyun.accelerator/nvidia_count"
	PhysicalGpuMemNodeLabel   = "aliyun.accelerator/nvidia_mem"
	PhysicalGpuTypeNodeLabel  = "aliyun.accelerator/nvidia_name"

	IntelligentGroupName             = "intelligent.sofastack.io"
	IntelligentVersion               = "v1"
	VgsResourceName                  = "virtualgpuspecifications"
	VgiResourceName                  = "virtualgpuinstances"
	VgiPodNameLabel                  = "podName"
	VgiPodNamespaceLabel             = "podNamespace"
	PodConfigMapLabel                = "vgpu-env-cm"
	NameSpace                        = "intelligent-computing"
	IntelligentPodAnnoAssignFlag     = "scheduler.framework.intelligent.assigned"
	IntelligentPodAnnoAssumeTimeFlag = "scheduler.framework.Intelligent.assume-time"
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
	if err != nil { // 如果为查询到CM，则设置为默认参数
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
	// informer
	var once sync.Once
	once.Do(func() {
		stop := make(chan struct{})
		nodes, err := i.handle.SharedInformerFactory().Core().V1().Nodes().Lister().List(labels.Everything())
		if err != nil {
			klog.Errorf("Failed to list nodes, err: %v", err)
			return
		}
		for _, node := range nodes {
			handleAddOrUpdateNode(node, i.cache, i.oversellRate)
		}

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
					handleAddOrUpdateNode(node, i.cache, i.oversellRate)
					//klog.Infof("succeed to add intelligent node %v to the cache", node.Name)
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
					updateNode(i.cache, oldNode, newNode, i.oversellRate)
					//klog.Infof("succeed to update intelligent node %v to the cache", newNode.Name)
				},
				DeleteFunc: func(obj interface{}) {
					node, ok := obj.(*v1.Node)
					if !ok {
						klog.Errorf("Failed to convert %v to *v1.Node", reflect.TypeOf(obj))
						return
					}
					deleteNode(i.cache, node)
					//klog.Infof("succeed to delete intelligent node %v from the cache", node.Name)
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
		vgsInformer := factory.ForResource(vgsGvr).Informer()
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

		vgiInformer := factory.ForResource(vgiGvr).Informer()
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
	// 确定是否需要处理该pod
	needToHandle := IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		klog.Infof("pod %s/%s does not need to be scheduled", pod.Namespace, pod.Name)
		return framework.NewStatus(framework.Success, MsgNoNeedToHandlePod)
	}
	klog.Infof("Start to prefilter for pod %s/%s", pod.Namespace, pod.Name)
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
	i.SaveVGpuPodState(state, vGpuCount, vGpuSpec, string(pod.UID))
	// 将该pod所对应的vgi names存入cyclestate
	i.SaveVgiState(state, vgiInfoNames, string(pod.UID))
	klog.Infof("Succeed prefilter for pod %s/%s", pod.Namespace, pod.Name)
	return nil
}

func (i *IntelligentScheduler) AddPod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod, podToAdd *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	p := podToAdd.Pod
	if !IsMyPod(p, i.resourceNames...) {
		return framework.NewStatus(framework.Success, "")
	}
	klog.Infof("Start to run intelligent AddPod function for pod %s/%s", podToAdd.Pod.Namespace, podToAdd.Pod.Name)
	if p.Spec.NodeName == "" {
		return framework.NewStatus(framework.Success, "")
	}

	var nodeInfos *NodeInfo
	nodeState, er := i.GetNodeState(state, nodeInfo.Node().Name)
	if er != nil {
		klog.Warningf("Failed to get node state, err: %v, will get it from cache", er)
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
	if len(vgiNames) == 0 {
		klog.Errorf("not find vgis for pod %s/%s in AddPod plugin", podToAdd.Pod.Namespace, podToAdd.Pod.Name)
		return framework.NewStatus(framework.Error, "not find vgis in AddPod plugin", podToAdd.Pod.Name)
	}
	er = addPod(i.cache, vgiNames, p, nodeName, nodeInfos)
	if er != nil {
		klog.Errorf("add pod %s/%s Failed, err: %v", podToAdd.Pod.Namespace, podToAdd.Pod.Name, err)
		return framework.NewStatus(framework.Error, er.Error())
	}
	return framework.NewStatus(framework.Success, "")
}

func (i *IntelligentScheduler) RemovePod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod, podToRemove *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	p := podToRemove.Pod
	if !IsMyPod(p, i.resourceNames...) {
		return framework.NewStatus(framework.Success, "")
	}
	klog.Infof("Start to run intelligent RemovePod function, pod name %s", podToRemove.Pod.Name)
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
	err = patchConfigMap(i.client, p, nil)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	return framework.NewStatus(framework.Success, "")
}

func (i *IntelligentScheduler) PreFilterExtensions() framework.PreFilterExtensions {
	return i
}

func (i *IntelligentScheduler) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	needToHandle := IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		klog.Infof("Schedulling pod %s/%s, it is not need to handle", pod.Namespace, pod.Name)
		return framework.NewStatus(framework.Success, "")
	}
	node := nodeInfo.Node()
	if node == nil {
		klog.Infof("Schedulling pod %s/%s, a node not found, it may be deleted", pod.Namespace, pod.Name)
		return framework.NewStatus(framework.Unschedulable, "node not found")
	}
	nodeName := node.Name
	// 判断node是否为智算node
	ok := i.cache.getIntelligentNode(nodeName)
	if !ok {
		klog.Infof("Schedulling pod %s/%s, node %s is not an intelligent scheduled node", pod.Namespace, pod.Name, nodeName)
		return framework.NewStatus(framework.Unschedulable, "node is not an intelligent scheduled node")
	}
	var nodeInfos *NodeInfo
	// 从cache中获得node资源信息
	ns, err := i.GetNodeState(state, nodeName) // 尝试从cycleState中取state
	if err != nil {
		klog.Warningf("Schedulling pod %s/%s, failed to get node %s info from cycleState, error is %v", pod.Namespace, pod.Name, nodeName, err)
		nodeInfos = i.cache.getNodeInfo(nodeName) // 转为从cache中取
		if nodeInfos == nil {
			klog.Errorf("Schedulling pod %s/%s, failed to get node %s info from cache", pod.Namespace, pod.Name, nodeName)
			return framework.NewStatus(framework.Error, "Failed to get node info from cache or cycleState")
		}
	} else {
		nodeInfos = ns.getNodeInfo()
	}
	// 获得pod请求资源情况
	vGpuPodState, err := i.GetVGpuPodState(state, string(pod.UID))
	if err != nil {
		klog.Errorf("Schedulling pod %s/%s, get vGPU pod state failed, error is %v", pod.Namespace, pod.Name, err)
		return framework.NewStatus(framework.Error, err.Error())
	}
	var vgiNames []string
	vgiState, err := i.GetVgiState(state, string(pod.UID))
	if err != nil {
		klog.Warningf("Schedulling pod %s/%s, get vgi failed, error is %v, it will be get from cache", pod.Namespace, pod.Name, err)
		vgiNames = i.cache.getVgiInfoNamesByPod(pod)
	} else {
		vgiNames = vgiState.getVgiNames()
	}
	// 判断node是否满足pod需求
	available := i.nodeAvailableForPod(nodeName, nodeInfos, vGpuPodState, vgiNames)
	if !available {
		klog.Infof("Schedulling pod %s/%s, node %s is not available for pod due to the GPU resources", pod.Namespace, pod.Name, nodeName)
		return framework.NewStatus(framework.Unschedulable, "node is not available for pod due to the GPU resources")
	}
	klog.Infof("Schedulling pod %s/%s, node %s is available", pod.Namespace, pod.Name, nodeName)
	return framework.NewStatus(framework.Success)
}

func (i *IntelligentScheduler) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	needToHandle := IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		return int64(0), framework.NewStatus(framework.Success, "")
	}
	var vgiNames []string
	// 获得pod拥有的vgi names
	vgiState, err := i.GetVgiState(state, string(pod.UID))
	if err != nil {
		vgiNames = i.cache.getVgiInfoNamesByPod(pod)
	} else {
		vgiNames = vgiState.getVgiNames()
	}
	// 获得node资源信息
	var nodeInfos *NodeInfo
	nodeState, err := i.GetNodeState(state, nodeName)
	if err != nil {
		nodeInfos = i.cache.getNodeInfo(nodeName)
	} else {
		nodeInfos = nodeState.getNodeInfo()
	}
	// 计算node score
	score := i.engine.calculateNodeScore(i.cache, nodeName, nodeInfos, vgiNames)
	return int64(score), framework.NewStatus(framework.Success, "")
}

func (i *IntelligentScheduler) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func (i *IntelligentScheduler) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	needToHandle := IsMyPod(pod, i.resourceNames...)
	if !needToHandle {
		return framework.NewStatus(framework.Success, "")
	}
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
	// 在CM中注入环境变量
	cmData := buildEnvVars(vgi, result, totalMem)
	err = patchConfigMap(i.client, pod, cmData)
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}
	i.SaveNodeState(state, nodeInfos, nodeName)
	klog.Infof("Scheduled pod %s/%s on node %s successfully", pod.Namespace, pod.Name, nodeName)
	return framework.NewStatus(framework.Success, "")
}

func (i *IntelligentScheduler) Unreserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	if !IsMyPod(pod, i.resourceNames...) {
		return
	}
	if !i.cache.getIntelligentNode(nodeName) {
		klog.Infof("Reserving pod %s/%s, node %s is not an intelligent node in cache", pod.Namespace, pod.Name, nodeName)
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

func (i *IntelligentScheduler) handleAddOrUpdateCM(cm *v1.ConfigMap) error {
	data := cm.Data
	nodeSelectorPolicy, ok := data["nodeSelectorPolicy"]
	if !ok {
		return fmt.Errorf("missing nodeSelectorPolicy in configmap")
	}
	gpuSelectorPolicy, ok := data["gpuSelectorPolicy"]
	if !ok {
		return fmt.Errorf("missing gpuSelectorPolicy in configmap")
	}
	oversellRateStr, ok := data["oversellRate"]
	if !ok {
		return fmt.Errorf("missing oversellRate in configmap")
	}
	oversellRate, err := strconv.Atoi(oversellRateStr)
	if err != nil {
		return err
	}
	gpuMemoryScoreWeightStr, ok := data["gpuMemoryScoreWeight"]
	if !ok {
		return fmt.Errorf("missing gpuMemoryScoreWeight in configmap")
	}
	gpuMemoryScoreWeight, err := strconv.Atoi(gpuMemoryScoreWeightStr)
	if err != nil {
		return err
	}
	gpuUtilizationScoreWeightStr, ok := data["gpuUtilizationScoreWeight"]
	if !ok {
		return fmt.Errorf("missing gpuUtilizationScoreWeight in configmap")
	}
	gpuUtilizationScoreWeight, err := strconv.Atoi(gpuUtilizationScoreWeightStr)
	if err != nil {
		return err
	}
	if !(gpuMemoryScoreWeight >= 0 && gpuMemoryScoreWeight <= 100 && gpuUtilizationScoreWeight >= 0 && gpuUtilizationScoreWeight <= 100 && gpuMemoryScoreWeight+gpuUtilizationScoreWeight == 100) {
		return fmt.Errorf("invalid GPU score weight")
	}
	if !(gpuSelectorPolicy == "spread" || gpuSelectorPolicy == "binpack") {
		return fmt.Errorf("invalid GPU selector policy. It should be 'spread' or 'binpack'")
	}
	if !(nodeSelectorPolicy == "spread" || nodeSelectorPolicy == "binpack") {
		return fmt.Errorf("invalid node selector policy. It should be 'spread' or 'binpack'")
	}
	i.engine = NewIntelligentSchedulerRuntime(IntelligentSchedulerName, nodeSelectorPolicy, gpuSelectorPolicy, gpuMemoryScoreWeight, gpuUtilizationScoreWeight)
	i.oversellRate = oversellRate
	return nil
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
		if spec.getName() == nodeInfos.getGpuType() {
			ok = true
		}
	}
	if len(requestPGpuSpecs) == 0 || (len(requestPGpuSpecs) == 1 && requestPGpuSpecs[0].getName() == "") {
		ok = true
	}
	if !ok {
		return false
	}
	vgi := i.cache.getVgiInfo(vgiNames[0]).Clone()
	oversellRate := nodeInfos.getOversellRate()
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
		klog.Infof("node %s has available gpus[%d] are fewer than the requested[%d]", nodeName, availableGpuCount, requestVGpuCount)
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
		return nil, fmt.Errorf("Failed to read state for pod %s: %v", podUID, err)
	}
	podState, ok := c.(*VirtualGpuPodState)
	if !ok {
		return nil, fmt.Errorf("Failed to cast state for pod %s: %v", podUID, err)
	}
	return podState, nil
}

func (i *IntelligentScheduler) GetVgiState(state *framework.CycleState, podUID string) (*VirtualGpuInstanceState, error) {
	key := GetVGpuInstanceStateKey(podUID)
	c, err := state.Read(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to read virtual gpu instance states for pod %s: %v", podUID, err)
	}
	vgiState, ok := c.(*VirtualGpuInstanceState)
	if !ok {
		return nil, fmt.Errorf("Failed to cast virtual gpu instance state for pod %s: %v", podUID, err)
	}
	return vgiState, nil
}

func (i *IntelligentScheduler) GetNodeState(state *framework.CycleState, name string) (*NodeState, error) {
	key := GetNodeStateKey(name)
	c, err := state.Read(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to read state for node %s: %v", name, err)
	}
	nodeState, ok := c.(*NodeState)
	if !ok {
		return nil, fmt.Errorf("Failed to cast state for node %s: %v", name, c)
	}
	return nodeState, nil
}
