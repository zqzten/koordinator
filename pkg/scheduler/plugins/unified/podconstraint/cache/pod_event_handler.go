/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cache

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

type podEventHandler struct {
	handle             framework.Handle
	podConstraintCache *PodConstraintCache
}

func RegisterPodEventHandler(handle framework.Handle, podConstraintCache *PodConstraintCache) {
	podInformer := handle.SharedInformerFactory().Core().V1().Pods().Informer()
	eventHandler := cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			switch t := obj.(type) {
			case *corev1.Pod:
				return assignedPod(t)
			case cache.DeletedFinalStateUnknown:
				if pod, ok := t.Obj.(*corev1.Pod); ok {
					return assignedPod(pod)
				}
				return false
			default:
				return false
			}
		},
		Handler: &podEventHandler{
			handle:             handle,
			podConstraintCache: podConstraintCache,
		},
	}
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), handle.SharedInformerFactory(), podInformer, eventHandler)
}

func assignedPod(pod *corev1.Pod) bool {
	return pod.Spec.NodeName != ""
}

func (p *podEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	node, err := p.handle.SharedInformerFactory().Core().V1().Nodes().Lister().Get(pod.Spec.NodeName)
	if err != nil {
		klog.Errorf("[PodConstraint] pod.spec.nodeName == %s but get node error: %v", pod.Spec.NodeName, err)
		return
	}
	p.podConstraintCache.AddPod(node, pod)
}

func (p *podEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		return
	}
	newPod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}
	// nodeName api-server不允许更改，所以只选一个Node就okay了
	node, err := p.handle.SharedInformerFactory().Core().V1().Nodes().Lister().Get(newPod.Spec.NodeName)
	if err != nil {
		klog.Errorf("[PodConstraint] pod.spec.nodeName == %s but get node error: %v", newPod.Spec.NodeName, err)
		return
	}
	if util.IsPodTerminated(newPod) {
		p.podConstraintCache.DeletePod(node, newPod)
	} else {
		p.podConstraintCache.UpdatePod(node, oldPod, newPod)
	}

}

func (p *podEventHandler) OnDelete(obj interface{}) {
	var pod *corev1.Pod
	switch t := obj.(type) {
	case *corev1.Pod:
		pod = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		pod, ok = t.Obj.(*corev1.Pod)
		if !ok {
			return
		}
	default:
		break
	}
	if pod == nil {
		return
	}
	node, err := p.handle.SharedInformerFactory().Core().V1().Nodes().Lister().Get(pod.Spec.NodeName)
	if err != nil {
		klog.Errorf("[PodConstraint] pod.spec.nodeName == %s but get node error: %v", pod.Spec.NodeName, err)
		return
	}
	p.podConstraintCache.DeletePod(node, pod)
}
