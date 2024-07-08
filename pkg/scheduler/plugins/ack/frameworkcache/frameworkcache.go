package frameworkcache

import (
	"context"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	GPUTOPOLOGYINFO = "ack.node.gpu.schedule"
)

type FrameworkCache struct {
	// key is nodeName.
	lock       *sync.RWMutex
	nodeInfos  map[string]*ExtendNodeInfo
	handle     framework.Handle
	eventFuncs *EventFuncSet
}

var once sync.Once
var frameworkCache *FrameworkCache

var _ framework.PreFilterPlugin = &FrameworkCache{}

// Name is the name of the plugin used in Registry and configurations.
const Name = "FrameworkCache"

// Name returns name of the plugin. It is used in logs, etc.
func (fc *FrameworkCache) Name() string {
	return Name
}

// New initializes a new plugin and returns it.
func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	topologyManager := GetFrameworkCache(nil, handle)
	return topologyManager, nil
}

func GetFrameworkCache(_ runtime.Object, handle framework.Handle) *FrameworkCache {
	once.Do(func() {
		klog.Info("frameworkcache start")
		frameworkCache = &FrameworkCache{
			nodeInfos:  make(map[string]*ExtendNodeInfo),
			handle:     handle,
			lock:       new(sync.RWMutex),
			eventFuncs: NewEventFuncSet(),
		}
		stop := make(chan struct{})

		configMapInformer := handle.SharedInformerFactory().Core().V1().ConfigMaps().Informer()
		configMapInformer.AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.ConfigMap:
					_, ok := t.Labels[GPUTOPOLOGYINFO]
					if ok {
						return true
					}
					return false
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    frameworkCache.addConfigMap,
				UpdateFunc: frameworkCache.updateConfigmap,
			},
		})

		configMapInformer.AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.ConfigMap:
					value, ok := t.Labels["usage"]
					if ok && value == "scheduler-debug-request" {
						return true
					}
					return false
				case cache.DeletedFinalStateUnknown:
					if o, ok := t.Obj.(*v1.ConfigMap); ok {
						value, ok := o.Labels["usage"]
						if ok && value == "scheduler-debug-request" {
							return true
						}
					}
					return false
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    frameworkCache.addConfigMap,
				UpdateFunc: frameworkCache.updateConfigmap,
				DeleteFunc: frameworkCache.deleteConfigMap,
			},
		})

		nodeInformer := handle.SharedInformerFactory().Core().V1().Nodes().Informer()
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
					UpdateFunc: frameworkCache.updateNode,
					DeleteFunc: frameworkCache.deleteNode,
				},
			},
		)
		handle.SharedInformerFactory().Start(stop)
		handle.SharedInformerFactory().WaitForCacheSync(stop)

		podInformer := handle.SharedInformerFactory().Core().V1().Pods().Informer()
		podInformer.AddEventHandler(
			cache.FilteringResourceEventHandler{
				FilterFunc: func(obj interface{}) bool {
					switch t := obj.(type) {
					case *v1.Pod:
						return assignedPod(t)
					case cache.DeletedFinalStateUnknown:
						if pod, ok := t.Obj.(*v1.Pod); ok {
							return assignedPod(pod)
						}
						return false
					default:
						return false
					}
				},
				Handler: cache.ResourceEventHandlerFuncs{
					AddFunc:    frameworkCache.addPod,
					DeleteFunc: frameworkCache.deletePod,
					UpdateFunc: frameworkCache.updatePod,
				},
			},
		)
	})

	return frameworkCache
}

func (fc *FrameworkCache) PreFilter(ctx context.Context, state *framework.CycleState, p *v1.Pod) *framework.Status {
	return framework.NewStatus(framework.Success, "")
}

func (fc *FrameworkCache) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// AddExtendNodeInfo adds some ExtendNodeInfos
func (fc *FrameworkCache) AddExtendNodeInfo(nodeInfos ...*ExtendNodeInfo) {
	fc.lock.Lock()
	defer fc.lock.Unlock()
	for _, nodeInfo := range nodeInfos {
		name := nodeInfo.GetName()
		if _, ok := fc.nodeInfos[name]; ok {
			continue
		}
		fc.nodeInfos[name] = nodeInfo
	}
}

// GetExtendNodeInfo gets the target ExtendNodeInfo by node name
// if not found ExtendNodeInfo in cache,function will return null
func (fc *FrameworkCache) GetExtendNodeInfo(name string) *ExtendNodeInfo {
	fc.lock.Lock()
	defer fc.lock.Unlock()
	nodeInfo, ok := fc.nodeInfos[name]
	if !ok {
		nodeInfo = NewExtendNodeInfo(name)
		fc.nodeInfos[name] = nodeInfo
	}
	return nodeInfo
}

// RemoveExtendNodeInfo removes the target ExtendNodeInfo by node name
func (fc *FrameworkCache) RemoveExtendNodeInfo(name string) {
	fc.lock.Lock()
	defer fc.lock.Unlock()
	delete(fc.nodeInfos, name)
}

// GetAllExtendNodeInfos returns all ExtendNodeInfo
func (fc *FrameworkCache) GetAllExtendNodeInfos() []*ExtendNodeInfo {
	result := []*ExtendNodeInfo{}
	fc.lock.RLock()
	defer fc.lock.RUnlock()
	for _, nodeInfo := range fc.nodeInfos {
		result = append(result, nodeInfo)
	}
	return result
}

func (fc *FrameworkCache) GetHandle() framework.Handle {
	return fc.handle
}

func (fc *FrameworkCache) NodeIsInCache(nodeName string) bool {
	fc.lock.RLock()
	defer fc.lock.RUnlock()
	_, ok := fc.nodeInfos[nodeName]
	return ok
}

// assignedPod selects pods that are assigned (scheduled and running).
func assignedPod(pod *v1.Pod) bool {
	return len(pod.Spec.NodeName) != 0
}
