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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config/v1beta2"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
)

func init() {
	nodenumaresource.GetResourceSpec = extunified.GetResourceSpec
	nodenumaresource.GetResourceStatus = extunified.GetResourceStatus
	nodenumaresource.SetResourceStatus = extunified.SetResourceStatus
	nodenumaresource.GetPodQoSClass = extunified.GetPodQoSClass
}

const (
	Name = "UnifiedCPUSetAllocator"
)

var (
	_ framework.PreFilterPlugin = &Plugin{}
	_ framework.FilterPlugin    = &Plugin{}
	_ framework.ScorePlugin     = &Plugin{}
	_ framework.ReservePlugin   = &Plugin{}
	_ framework.PreBindPlugin   = &Plugin{}
)

type Plugin struct {
	handle framework.Handle
	*nodenumaresource.Plugin
	cpuSharePoolUpdater *cpuSharePoolUpdater
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	if args == nil {
		defaultNodeNUMAResourceArgs, err := getDefaultNodeNUMAResourceArgs()
		if err != nil {
			return nil, err
		}
		args = defaultNodeNUMAResourceArgs
	} else {
		unknownObj, ok := args.(*runtime.Unknown)
		if !ok {
			return nil, fmt.Errorf("got args of type %T, want *NodeNumaResourceArgs", args)
		}

		nodeNUMAResourceArgs, err := getDefaultNodeNUMAResourceArgs()
		if err != nil {
			return nil, err
		}
		if err := frameworkruntime.DecodeInto(unknownObj, nodeNUMAResourceArgs); err != nil {
			return nil, err
		}
		args = nodeNUMAResourceArgs
	}
	internalPlugin, err := nodenumaresource.New(args, handle)
	if err != nil {
		return nil, err
	}
	updater := &cpuSharePoolUpdater{
		queue:                workqueue.NewRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter()),
		getAvailableCPUsFunc: nil,
		handle:               handle,
	}
	p := &Plugin{
		handle:              handle,
		Plugin:              internalPlugin.(*nodenumaresource.Plugin),
		cpuSharePoolUpdater: updater,
	}
	updater.getAvailableCPUsFunc = func(nodeName string) (nodenumaresource.CPUSet, error) {
		availableCPUs, _, err := p.GetCPUManager().GetAvailableCPUs(nodeName)
		return availableCPUs, err
	}
	updater.start()

	podInformer := handle.SharedInformerFactory().Core().V1().Pods().Informer()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: p.onPodDelete,
		UpdateFunc: p.onPodUpdate,
	})
	return p, nil
}

func getDefaultNodeNUMAResourceArgs() (*schedulingconfig.NodeNUMAResourceArgs, error) {
	var v1beta2args v1beta2.NodeNUMAResourceArgs
	v1beta2.SetDefaults_NodeNUMAResourceArgs(&v1beta2args)
	var defaultNodeNUMAResourceArgs schedulingconfig.NodeNUMAResourceArgs
	err := v1beta2.Convert_v1beta2_NodeNUMAResourceArgs_To_config_NodeNUMAResourceArgs(&v1beta2args, &defaultNodeNUMAResourceArgs, nil)
	if err != nil {
		return nil, err
	}
	return &defaultNodeNUMAResourceArgs, nil
}

func (p *Plugin) Name() string { return Name }

func (p *Plugin) Filter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if extunified.IsVirtualKubeletNode(node) {
		return nil
	}
	return p.Plugin.Filter(ctx, cycleState, pod, nodeInfo)
}

func (p *Plugin) Score(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()
	if node == nil {
		return 0, framework.NewStatus(framework.Error, "node not found")
	}
	if extunified.IsVirtualKubeletNode(node) {
		return 0, nil
	}

	return p.Plugin.Score(ctx, cycleState, pod, nodeName)
}

func (p *Plugin) Reserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if extunified.IsVirtualKubeletNode(node) {
		return nil
	}

	return p.Plugin.Reserve(ctx, cycleState, pod, nodeName)
}

func (p *Plugin) Unreserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) {
	p.Plugin.Unreserve(ctx, cycleState, pod, nodeName)
	p.cpuSharePoolUpdater.asyncUpdate(pod.Spec.NodeName)
}

func (p *Plugin) PreBind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if extunified.IsVirtualKubeletNode(node) {
		return nil
	}
	p.cpuSharePoolUpdater.asyncUpdate(node.Name)
	return p.Plugin.PreBind(ctx, cycleState, pod, nodeName)
}
