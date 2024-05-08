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
)

type nodeEventHandler struct {
	collection *baseNodeCollection
}

func registerNodeEventHandler(collection *baseNodeCollection, factory informers.SharedInformerFactory) {
	eventHandler := &nodeEventHandler{
		collection: collection,
	}
	nodeInformer := factory.Core().V1().Nodes().Informer()
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), factory, nodeInformer, eventHandler)
}

func (h *nodeEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	node, _ := obj.(*corev1.Node)
	if node == nil {
		return
	}
	h.collection.AddNode(NewNode(node))
}

func (h *nodeEventHandler) OnUpdate(oldObj, newObj interface{}) {
	nodeObj, _ := newObj.(*corev1.Node)
	if nodeObj == nil {
		return
	}
	node := h.collection.GetNode(nodeObj.Name)
	if node == nil {
		return
	}
}

func (h *nodeEventHandler) OnDelete(obj interface{}) {
	var node *corev1.Node
	switch t := obj.(type) {
	case *corev1.Node:
		node = t
	case cache.DeletedFinalStateUnknown:
		node, _ = t.Obj.(*corev1.Node)
	}
	if node == nil {
		return
	}
	h.collection.RemoveNode(node.Name)
}
