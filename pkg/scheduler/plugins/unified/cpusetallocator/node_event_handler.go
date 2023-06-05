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

package cpusetallocator

import (
	"context"

	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	corev1 "k8s.io/api/core/v1"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/features"
	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
)

var (
	CPUMaxRefCount = extunified.CPUMaxRefCount
)

type nodeEventHandler struct {
	topologyManager nodenumaresource.CPUTopologyManager
}

func registerNodeEventHandler(handle framework.Handle, topologyManager nodenumaresource.CPUTopologyManager) {
	nodeInformer := handle.SharedInformerFactory().Core().V1().Nodes().Informer()
	eventHandler := &nodeEventHandler{
		topologyManager: topologyManager,
	}
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), handle.SharedInformerFactory(), nodeInformer, eventHandler)
}

func (c *nodeEventHandler) OnAdd(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return
	}
	c.updateNode(nil, node)
}

func (c *nodeEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldNode, ok := oldObj.(*corev1.Node)
	if !ok {
		return
	}

	node, ok := newObj.(*corev1.Node)
	if !ok {
		return
	}
	c.updateNode(oldNode, node)
}

func (c *nodeEventHandler) updateNode(oldNode, node *corev1.Node) {
	c.topologyManager.UpdateCPUTopologyOptions(node.Name, func(options *nodenumaresource.CPUTopologyOptions) {
		var maxRefCount int
		if k8sfeature.DefaultFeatureGate.Enabled(features.DisableCPUSetOversold) {
			maxRefCount = 1
		} else {
			maxRefCount = CPUMaxRefCount(node)
		}
		options.MaxRefCount = maxRefCount
	})
}

func (c *nodeEventHandler) OnDelete(obj interface{}) {
	return
}
