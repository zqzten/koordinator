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
	"fmt"
	"sync/atomic"

	uniapiext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"

	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config/v1beta2"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
)

// Name is the name of the plugin used in Registry and configurations.
// Use nodenumaresource.Name as plugin name to force replace origin implements
const Name = nodenumaresource.Name

var _ framework.FilterPlugin = &Plugin{}
var _ framework.ScorePlugin = &Plugin{}
var _ framework.ReservePlugin = &Plugin{}
var _ framework.PreBindPlugin = &Plugin{}

type Plugin struct {
	*nodenumaresource.Plugin
}

var globalPlugin atomic.Value

// New initializes a new plugin and returns it.
func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	klog.V(1).Info("numa start")

	nodeNUMAResourceArgs, err := getDefaultNodeNUMAResourceArgs()
	if err != nil {
		return nil, err
	}
	if args != nil {
		switch t := args.(type) {
		case *schedulingconfig.NodeNUMAResourceArgs:
			nodeNUMAResourceArgs = t
		case *runtime.Unknown:
			if err := frameworkruntime.DecodeInto(t, nodeNUMAResourceArgs); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("got args of type %T, want *NodeNumaResourceArgs", args)
		}
	}
	args = nodeNUMAResourceArgs

	topologyManager := nodenumaresource.NewCPUTopologyManager()
	defaultNUMAAllocateStrategy := nodenumaresource.GetDefaultNUMAAllocateStrategy(nodeNUMAResourceArgs)
	cpuManager := nodenumaresource.NewCPUManager(handle, defaultNUMAAllocateStrategy, topologyManager)
	syncCPUTopologyConfigMap(handle, topologyManager)

	plugin, err := nodenumaresource.NewWithOptions(args, handle,
		nodenumaresource.WithCPUTopologyManager(topologyManager),
		nodenumaresource.WithCPUManager(cpuManager),
	)
	if err != nil {
		return nil, err
	}
	internalPlugin := plugin.(*nodenumaresource.Plugin)
	pl := &Plugin{
		Plugin: internalPlugin,
	}
	globalPlugin.Store(pl)
	return pl, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (n *Plugin) Name() string {
	return Name
}

func needCPUSetScheduling(annotations map[string]string) bool {
	return getCPUSetSchedulerFromAnnotation(annotations) == uniapiext.CPUSetSchedulerTrue &&
		getCPUPolicyFromAnnotation(annotations) != uniapiext.CPUPolicySharePool
}

func (n *Plugin) Filter(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	if !needCPUSetScheduling(p.Annotations) {
		return framework.NewStatus(framework.Success)
	}
	if nodeInfo == nil || nodeInfo.Node() == nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable)
	}

	return n.Plugin.Filter(ctx, state, p, nodeInfo)
}

func (n *Plugin) Score(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) (int64, *framework.Status) {
	if !needCPUSetScheduling(p.Annotations) {
		return 0, framework.NewStatus(framework.Success)
	}

	return n.Plugin.Score(ctx, state, p, nodeName)
}

func (n *Plugin) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func (n *Plugin) Reserve(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) *framework.Status {
	if !needCPUSetScheduling(p.Annotations) {
		return framework.NewStatus(framework.Success)
	}

	return n.Plugin.Reserve(ctx, state, p, nodeName)
}

func (n *Plugin) Unreserve(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) {
	if !needCPUSetScheduling(p.Annotations) {
		return
	}
	n.Plugin.Unreserve(ctx, state, p, nodeName)
}

func (n *Plugin) PreBind(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) *framework.Status {
	if !needCPUSetScheduling(p.Annotations) {
		return framework.NewStatus(framework.Success)
	}
	return n.Plugin.PreBind(ctx, state, p, nodeName)
}

func getDefaultNodeNUMAResourceArgs() (*schedulingconfig.NodeNUMAResourceArgs, error) {
	var v1beta1args v1beta2.NodeNUMAResourceArgs
	v1beta2.SetDefaults_NodeNUMAResourceArgs(&v1beta1args)
	var defaultNodeNUMAResourceArgs schedulingconfig.NodeNUMAResourceArgs
	err := v1beta2.Convert_v1beta2_NodeNUMAResourceArgs_To_config_NodeNUMAResourceArgs(&v1beta1args, &defaultNodeNUMAResourceArgs, nil)
	if err != nil {
		return nil, err
	}
	return &defaultNodeNUMAResourceArgs, nil
}
