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

package nodesorting

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

type podEventHandler struct {
	collection *baseNodeCollection
}

func registerPodEventHandler(collection *baseNodeCollection, factory informers.SharedInformerFactory) {
	eventHandler := &podEventHandler{
		collection: collection,
	}
	nodeInformer := factory.Core().V1().Pods().Informer()
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), factory, nodeInformer, eventHandler)
}

func (h *podEventHandler) OnAdd(obj interface{}) {
	pod, _ := obj.(*corev1.Pod)
	if pod == nil {
		return
	}

	if pod.Spec.NodeName == "" || util.IsPodTerminated(pod) {
		return
	}

	node := h.collection.GetNode(pod.Spec.NodeName)
	if node != nil {
		node.AddPod(pod)
		h.collection.NodeUpdated(node)
	}
}

func (h *podEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldPod, _ := oldObj.(*corev1.Pod)
	newPod, _ := newObj.(*corev1.Pod)
	if oldPod == nil || newPod == nil {
		return
	}

	if newPod.Spec.NodeName == "" {
		return
	}
	if util.IsPodTerminated(newPod) {
		h.OnDelete(newObj)
		return
	}

	if oldPod.Generation == newPod.Generation {
		return
	}

	node := h.collection.GetNode(newPod.Spec.NodeName)
	if node != nil {
		node.UpdatePod(oldPod, newPod)
		h.collection.NodeUpdated(node)
	}
}

func (h *podEventHandler) OnDelete(obj interface{}) {
	var pod *corev1.Pod
	switch t := obj.(type) {
	case *corev1.Pod:
		pod = t
	case cache.DeletedFinalStateUnknown:
		pod, _ = t.Obj.(*corev1.Pod)
	}
	if pod == nil {
		return
	}

	if pod.Spec.NodeName == "" {
		return
	}

	node := h.collection.GetNode(pod.Spec.NodeName)
	if node != nil {
		node.DeletePod(pod)
		h.collection.NodeUpdated(node)
	}
}
