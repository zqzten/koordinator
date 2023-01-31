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

package nodeports

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeports"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

// Plugin is a plugin that checks if a node has free ports for the requested pod ports.
type Plugin struct {
	*nodeports.NodePorts
}

var _ framework.PreFilterPlugin = &Plugin{}
var _ framework.FilterPlugin = &Plugin{}
var _ framework.EnqueueExtensions = &Plugin{}

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	Name = "UnifiedNodePorts"
)

// Name returns name of the plugin. It is used in logs, etc.
func (pl *Plugin) Name() string {
	return Name
}

// Filter invoked at the filter extension point.
func (pl *Plugin) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if extunified.IsVirtualKubeletNode(node) {
		return nil
	}
	return pl.NodePorts.Filter(ctx, cycleState, pod, nodeInfo)
}

// New initializes a new plugin and returns it.
func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	internalPlugin, err := nodeports.New(args, handle)
	if err != nil {
		return nil, err
	}
	return &Plugin{
		NodePorts: internalPlugin.(*nodeports.NodePorts),
	}, nil
}
