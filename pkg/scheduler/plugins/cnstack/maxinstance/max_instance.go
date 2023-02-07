package maxinstance

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	listercorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	v1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/reservation"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

const (
	// Name is the name of the plugin used in Registry and configurations.
	Name = "MaxInstance"

	// MaxInstanceKey is the annotations key in the pod that defines the maximum pod number in the topology.
	ACKMaxInstanceKey = "alibabacloud.com/max-instances-per-group"
	MaxInstanceKey    = apiext.SchedulingDomainPrefix + "/max-instances-per-group"
)

// max instance config
type MaxInstanceConfig struct {
	// topologyKey is the key of node labels
	TopologyKey string `json:"topologyKey,omitempty"`
	// group to define whether to include pods
	Group string `json:"group,omitempty"`
	// max instance count
	MaxInstances int `json:"maxInstances,omitempty"`
}

var _ framework.FilterPlugin = &MaxInstance{}
var _ framework.ReservePlugin = &MaxInstance{}

// MaxInstance is a plugin that implements the pod number filter.
type MaxInstance struct {
	cache      map[string]map[string]map[string]int // topology -> key -> group -> count
	podCache   map[string]struct{}
	nodeLister listercorev1.NodeLister
	sync.RWMutex
}

// New initializes a new plugin and returns it.
func New(laArgs runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	mi := &MaxInstance{
		cache:      map[string]map[string]map[string]int{},
		podCache:   map[string]struct{}{},
		nodeLister: handle.SharedInformerFactory().Core().V1().Nodes().Lister(),
	}
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
				AddFunc:    mi.addPod,
				UpdateFunc: mi.updatePod,
				DeleteFunc: mi.deletePod,
			},
		},
	)
	extendedHandle, ok := handle.(frameworkext.ExtendedHandle)
	if !ok {
		return nil, fmt.Errorf("want handle to be of type frameworkext.ExtendedHandle, got %T", handle)
	}
	reservationInformer := extendedHandle.KoordinatorSharedInformerFactory().Scheduling().V1alpha1().Reservations().Informer()
	reservationInformer.AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1alpha1.Reservation:
					return scheduledReservation(t)
				case cache.DeletedFinalStateUnknown:
					if pod, ok := t.Obj.(*v1alpha1.Reservation); ok {
						return scheduledReservation(pod)
					}
					return false
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    mi.addReservation,
				UpdateFunc: mi.updateReservation,
				DeleteFunc: mi.deleteReservation,
			},
		},
	)
	return mi, nil
}

// when pod is scheduled, add the pod to cache.
func (mi *MaxInstance) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)
	if pod.Status.Phase != v1.PodRunning && pod.Status.Phase != v1.PodPending {
		return
	}
	klog.V(5).Infof("add pod %v/%v event", pod.Namespace, pod.Name)

	mi.addToCache(pod, pod.Spec.NodeName)
}

// when pod is succeeded or failed, delete the pod from cache.
func (mi *MaxInstance) updatePod(oldObj, newObj interface{}) {
	oldPod := oldObj.(*v1.Pod)
	if oldPod.Status.Phase == v1.PodSucceeded || oldPod.Status.Phase == v1.PodFailed {
		return
	}
	newPod := newObj.(*v1.Pod)
	if newPod.Status.Phase != v1.PodRunning && newPod.Status.Phase != v1.PodPending {
		mi.deletePod(newPod)
	}
}

// when pod is deleted, delete the pod from cache.
func (mi *MaxInstance) deletePod(obj interface{}) {
	var pod *v1.Pod
	switch t := obj.(type) {
	case *v1.Pod:
		pod = t
	case cache.DeletedFinalStateUnknown:
		obj, ok := t.Obj.(*v1.Pod)
		if ok {
			pod = obj
		}
	}
	if pod == nil {
		return
	}
	klog.V(5).Infof("delete pod %v/%v event", pod.Namespace, pod.Name)
	mi.deleteFromCache(pod, pod.Spec.NodeName)
}

func (mi *MaxInstance) addReservation(obj interface{}) {
	r := obj.(*v1alpha1.Reservation)
	if util.IsReservationActive(r) {
		pod := util.NewReservePod(r)
		mi.addToCache(pod, util.GetReservationNodeName(r))
	}
}

func (mi *MaxInstance) updateReservation(oldObj, newObj interface{}) {
	oldR := oldObj.(*v1alpha1.Reservation)
	newR := newObj.(*v1alpha1.Reservation)
	if util.IsReservationActive(oldR) && !util.IsReservationActive(newR) {
		pod := util.NewReservePod(newR)
		mi.deleteFromCache(pod, util.GetReservationNodeName(newR))
	}
}

func (mi *MaxInstance) deleteReservation(obj interface{}) {
	var r *v1alpha1.Reservation
	switch t := obj.(type) {
	case *v1alpha1.Reservation:
		r = t
	case cache.DeletedFinalStateUnknown:
		obj, ok := t.Obj.(*v1alpha1.Reservation)
		if ok {
			r = obj
		}
	}
	if r == nil {
		return
	}
	pod := util.NewReservePod(r)
	mi.deleteFromCache(pod, util.GetReservationNodeName(r))
}

func (mi *MaxInstance) addToCache(pod *v1.Pod, nodeName string) {
	if len(pod.Annotations) <= 0 {
		return
	}
	configs, ok := getMaxInstanceConfig(pod)
	if !ok {
		return
	}
	node, err := mi.nodeLister.Get(nodeName)
	if err != nil {
		return
	}

	mi.Lock()
	defer mi.Unlock()

	podKey := PodKey(pod)
	if _, exists := mi.podCache[podKey]; exists {
		//already added to the cache
		return
	}

	klog.V(4).Infof("%s/%s add to cache", pod.Namespace, pod.Name)
	for _, c := range configs {
		key, exist := getNodeTopology(c.TopologyKey, node)
		if !exist {
			continue
		}
		topologyCache := mi.cache[c.TopologyKey]
		if topologyCache == nil {
			topologyCache = map[string]map[string]int{}
			mi.cache[c.TopologyKey] = topologyCache
		}

		groupCache := topologyCache[key]
		if groupCache == nil {
			groupCache = map[string]int{}
			topologyCache[key] = groupCache
		}

		klog.V(5).Infof("%s/%s topology:%s key:%s group:%s add to cache", pod.Namespace, pod.Name, c.TopologyKey, key, c.Group)
		groupCache[c.Group] += 1
		mi.podCache[podKey] = struct{}{}
	}
}

func (mi *MaxInstance) deleteFromCache(pod *v1.Pod, nodeName string) {
	if len(pod.Annotations) <= 0 {
		return
	}
	configs, ok := getMaxInstanceConfig(pod)
	if !ok {
		return
	}
	node, err := mi.nodeLister.Get(nodeName)
	if err != nil {
		return
	}

	mi.Lock()
	defer mi.Unlock()

	podKey := PodKey(pod)
	if _, exists := mi.podCache[podKey]; !exists {
		//already deleted from the cache
		return
	}

	klog.V(4).Infof("%s/%s delete from cache", pod.Namespace, pod.Name)
	for _, c := range configs {
		key, exist := getNodeTopology(c.TopologyKey, node)
		if !exist {
			continue
		}
		topologyCache := mi.cache[c.TopologyKey]
		if topologyCache == nil {
			continue
		}

		groupCache := topologyCache[key]
		if groupCache == nil {
			continue
		}

		klog.V(5).Infof("%s/%s topology:%s key:%s group:%s delete from cache", pod.Namespace, pod.Name, c.TopologyKey, key, c.Group)
		groupCache[c.Group] -= 1
		delete(mi.podCache, podKey)
		if groupCache[c.Group] < 0 {
			groupCache[c.Group] = 0
			klog.Errorf("groupCache unexpect delete, %s/%s topology:%s key:%s group:%s max:%d", pod.Namespace, pod.Name, c.TopologyKey, key, c.Group, c.MaxInstances)
		}
	}

}

// Name returns name of the plugin. It is used in logs, etc.
func (mi *MaxInstance) Name() string {
	return Name
}

// Filter performs the following validations.
// 1. Validate if the total number of pods in the node topology is less than max instance count.
func (mi *MaxInstance) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if len(pod.Annotations) <= 0 {
		return framework.NewStatus(framework.Success, "")
	}
	reserved := 0
	if state != nil && reservation.CycleStateMatchReservation(state, node.Name) {
		reserved++
	}

	if configs, ok := getMaxInstanceConfig(pod); ok {
		mi.RLock()
		defer mi.RUnlock()
		for _, c := range configs {
			key, exist := getNodeTopology(c.TopologyKey, node)
			if !exist {
				continue
			}
			topologyCache := mi.cache[c.TopologyKey]
			if topologyCache == nil {
				continue
			}
			groupCache := topologyCache[key]
			if groupCache == nil {
				continue
			}
			if count, ok := groupCache[c.Group]; ok {
				if count >= c.MaxInstances+reserved {
					klog.V(5).Infof("%s/%s topology:%s key:%s group:%s max:%d, count:%d Unschedulable", pod.Namespace, pod.Name, c.TopologyKey, key, c.Group, c.MaxInstances, count)
					return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Topology: %s, Max Instances Per Group Not Fit", c.TopologyKey))
				} else {
					klog.V(5).Infof("%s/%s topology:%s key:%s group:%s max:%d, count:%d schedulable", pod.Namespace, pod.Name, c.TopologyKey, key, c.Group, c.MaxInstances, count)
				}
			}
		}
	}
	return framework.NewStatus(framework.Success, "")
}

func (mi *MaxInstance) Reserve(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) *framework.Status {
	klog.V(5).Infof("%s/%s Reserve", p.Namespace, p.Name)
	mi.addToCache(p, nodeName)
	if state != nil && reservation.CycleStateMatchReservation(state, nodeName) {
		reservePod := p.DeepCopy()
		reservePod.UID = reservation.CycleStateMatchReservationUID(state)
		mi.deleteFromCache(reservePod, nodeName)
	}
	return framework.NewStatus(framework.Success, "")
}

func (mi *MaxInstance) Unreserve(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) {
	klog.V(5).Infof("%s/%s Unreserve", p.Namespace, p.Name)
	mi.deleteFromCache(p, nodeName)
	if state != nil && reservation.CycleStateMatchReservation(state, nodeName) {
		reservePod := p.DeepCopy()
		reservePod.UID = reservation.CycleStateMatchReservationUID(state)
		mi.addToCache(reservePod, nodeName)
	}
}

// getNodeTopology return the node topology value
func getNodeTopology(key string, node *v1.Node) (val string, exist bool) {
	if len(node.Labels) > 0 {
		val, exist = node.Labels[key]
		return
	}
	return
}

// getMaxInstanceConfig return the max instance config of the pod
func getMaxInstanceConfig(pod *v1.Pod) ([]*MaxInstanceConfig, bool) {
	if str, ok := pod.Annotations[MaxInstanceKey]; ok && str != "" {
		configs := []*MaxInstanceConfig{}
		if err := json.Unmarshal([]byte(str), &configs); err == nil {
			return configs, true
		}
	}
	if str, ok := pod.Annotations[ACKMaxInstanceKey]; ok && str != "" {
		configs := []*MaxInstanceConfig{}
		if err := json.Unmarshal([]byte(str), &configs); err == nil {
			return configs, true
		}
	}
	return nil, false
}

// assignedPod selects pods that are assigned (scheduled and running).
func assignedPod(pod *v1.Pod) bool {
	return len(pod.Spec.NodeName) != 0
}

func scheduledReservation(r *v1alpha1.Reservation) bool {
	return r != nil && len(r.Status.NodeName) > 0
}

// PodKey returns a key unique to the given pod within a cluster.
// It's used so we consistently use the same key scheme in this module.
// It does exactly what cache.MetaNamespaceKeyFunc would have done
// except there's not possibility for error since we know the exact type.
func PodKey(pod *v1.Pod) string {
	return fmt.Sprintf("%v", pod.UID)
}
