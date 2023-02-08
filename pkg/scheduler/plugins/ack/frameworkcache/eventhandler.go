package frameworkcache

import (
	"fmt"
	"reflect"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type EventFuncSet struct {
	// handler func for adding  configmap event
	addConfigmapFuncs []func(*v1.ConfigMap)
	// handler func for updating configmap event
	updateConfigmapFuncs []func(oldConfigmap *v1.ConfigMap, newConfigmap *v1.ConfigMap)
	// handler func for deleting  configmap event
	deleteConfigmapFuncs []func(*v1.ConfigMap, bool)
	// handler func for adding pod event
	addPodFuncs []func(*v1.Pod)
	// handler func for updating pod event
	updatePodFuncs []func(oldPod *v1.Pod, newPod *v1.Pod)
	// handler func for deleting pod event
	deletePodFuncs []func(*v1.Pod, bool)
	// handler func for deleting node event
	deleteNodeFuncs []func(*v1.Node, bool)
	// handler func for updating node event
	updateNodeFuncs []func(oldNode *v1.Node, newNode *v1.Node)
}

func NewEventFuncSet() *EventFuncSet {
	return &EventFuncSet{
		addConfigmapFuncs:    []func(*v1.ConfigMap){},
		updateConfigmapFuncs: []func(oldConfigmap *v1.ConfigMap, newConfigmap *v1.ConfigMap){},
		deleteConfigmapFuncs: []func(*v1.ConfigMap, bool){},
		addPodFuncs:          []func(*v1.Pod){},
		updatePodFuncs:       []func(oldPod *v1.Pod, newPod *v1.Pod){},
		deletePodFuncs:       []func(*v1.Pod, bool){},
		deleteNodeFuncs:      []func(*v1.Node, bool){},
		updateNodeFuncs:      []func(oldNode *v1.Node, newNode *v1.Node){},
	}
}

func (fc *FrameworkCache) AddEventFunc(eventFunc interface{}) error {
	fc.lock.Lock()
	defer fc.lock.Unlock()
	switch t := eventFunc.(type) {
	case func(*v1.ConfigMap):
		fc.eventFuncs.addConfigmapFuncs = append(fc.eventFuncs.addConfigmapFuncs, t)
	case func(oldConfigmap *v1.ConfigMap, newConfigmap *v1.ConfigMap):
		fc.eventFuncs.updateConfigmapFuncs = append(fc.eventFuncs.updateConfigmapFuncs, t)
	case func(*v1.ConfigMap, bool):
		fc.eventFuncs.deleteConfigmapFuncs = append(fc.eventFuncs.deleteConfigmapFuncs, t)
	case func(*v1.Pod):
		fc.eventFuncs.addPodFuncs = append(fc.eventFuncs.addPodFuncs, t)
	case func(oldPod *v1.Pod, newPod *v1.Pod):
		fc.eventFuncs.updatePodFuncs = append(fc.eventFuncs.updatePodFuncs, t)
	case func(*v1.Pod, bool):
		fc.eventFuncs.deletePodFuncs = append(fc.eventFuncs.deletePodFuncs, t)
	case func(oldNode *v1.Node, newNode *v1.Node):
		fc.eventFuncs.updateNodeFuncs = append(fc.eventFuncs.updateNodeFuncs, t)
	case func(*v1.Node, bool):
		fc.eventFuncs.deleteNodeFuncs = append(fc.eventFuncs.deleteNodeFuncs, t)
	default:
		return fmt.Errorf("unknown event func type")
	}
	return nil
}

func (fc *FrameworkCache) addConfigMap(obj interface{}) {
	configMap, ok := obj.(*v1.ConfigMap)
	if !ok {
		klog.Errorf("cannot convert to *v1.ConfigMap: %v", obj)
		return
	}
	klog.V(3).Infof("add event for ConfigMap %q", configMap.Name)
	for _, eventFunc := range fc.eventFuncs.addConfigmapFuncs {
		eventFunc(configMap)
	}
}

func (fc *FrameworkCache) updateConfigmap(oldObj interface{}, newObj interface{}) {
	oldConfigmap, ok := oldObj.(*v1.ConfigMap)
	if !ok {
		klog.Errorf("can not convert %v to *v1.ConfigMap", reflect.TypeOf(oldObj))
		return
	}
	newConfigmap, ok := newObj.(*v1.ConfigMap)
	if !ok {
		klog.Errorf("can not convert %v to *v1.ConfigMap", reflect.TypeOf(newObj))
		return
	}
	for _, eventFunc := range fc.eventFuncs.updateConfigmapFuncs {
		eventFunc(oldConfigmap, newConfigmap)
	}
}

func (fc *FrameworkCache) deleteConfigMap(obj interface{}) {
	var configMap *v1.ConfigMap
	switch t := obj.(type) {
	case *v1.ConfigMap:
		configMap = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		configMap, ok = t.Obj.(*v1.ConfigMap)
		if !ok {
			klog.Errorf("cannot convert to *v1.Pod: %v", t.Obj)
			return
		}
	default:
		klog.Errorf("cannot convert to *v1.Pod: %v", t)
		return
	}
	isDeleteConfigMap := true
	for _, eventFunc := range fc.eventFuncs.deleteConfigmapFuncs {
		eventFunc(configMap, isDeleteConfigMap)
	}
}

func (fc *FrameworkCache) addPod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		klog.Errorf("can not convert %v to *v1.Pod", reflect.TypeOf(obj))
		return
	}
	for _, eventFunc := range fc.eventFuncs.addPodFuncs {
		eventFunc(pod)
	}
}

func (fc *FrameworkCache) updatePod(oldObj interface{}, newObj interface{}) {
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
	for _, eventFunc := range fc.eventFuncs.updatePodFuncs {
		eventFunc(oldPod, newPod)
	}
}

func (fc *FrameworkCache) deletePod(obj interface{}) {
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
	isDeletePod := true
	for _, eventFunc := range fc.eventFuncs.deletePodFuncs {
		eventFunc(pod, isDeletePod)
	}
}

func (fc *FrameworkCache) deleteNode(obj interface{}) {
	var node *v1.Node
	switch t := obj.(type) {
	case *v1.Node:
		node = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		node, ok = t.Obj.(*v1.Node)
		if !ok {
			klog.Errorf("cannot convert to *v1.Node: %v", t.Obj)
			return
		}
	default:
		klog.Errorf("cannot convert to *v1.Node: %v", t)
		return
	}
	isDeleteNode := true
	for _, eventFunc := range fc.eventFuncs.deleteNodeFuncs {
		eventFunc(node, isDeleteNode)
	}
	fc.RemoveExtendNodeInfo(node.Name)
}

func (fc *FrameworkCache) updateNode(oldObj interface{}, newObj interface{}) {
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
	for _, eventFunc := range fc.eventFuncs.updateNodeFuncs {
		eventFunc(oldNode, newNode)
	}
}
