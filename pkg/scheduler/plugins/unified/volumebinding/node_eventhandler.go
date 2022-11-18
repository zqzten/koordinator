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

package volumebinding

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
)

type nodeEventHandler struct {
	cache *NodeStorageInfoCache
}

func registerNodeEventHandler(informerFactory informers.SharedInformerFactory, cache *NodeStorageInfoCache) {
	nodeInformer := informerFactory.Core().V1().Nodes().Informer()
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), informerFactory, nodeInformer, &nodeEventHandler{cache: cache})
}

func (e *nodeEventHandler) OnAdd(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if !ok {
		return
	}
	e.setNode(node)
}

func (e *nodeEventHandler) OnUpdate(oldObj, newObj interface{}) {
	node, ok := newObj.(*v1.Node)
	if !ok {
		return
	}
	e.setNode(node)
}

func (e *nodeEventHandler) setNode(node *v1.Node) {
	nodeLocalVolumeInfo, err := newNodeLocalVolumeInfo(node)
	if err != nil {
		klog.Errorf("failed to newNodeLocalVolumeInfo, node: %s, err: %v", node.Name, err)
		return
	}
	e.cache.UpdateOnNode(node.Name, func(nodeStorageInfo *NodeStorageInfo) {
		nodeStorageInfo.UpdateNodeLocalVolumeInfo(nodeLocalVolumeInfo)
	})
}

func (e *nodeEventHandler) OnDelete(obj interface{}) {
	var node *v1.Node
	switch t := obj.(type) {
	case *v1.Node:
		node = t
	case cache.DeletedFinalStateUnknown:
		node, _ = t.Obj.(*v1.Node)
	}
	if node == nil {
		return
	}

	e.cache.DeleteNodeStorageInfo(node.Name)
}
