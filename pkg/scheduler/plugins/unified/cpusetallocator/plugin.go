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
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/features"
	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config/v1beta2"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
	"github.com/koordinator-sh/koordinator/pkg/util/cpuset"
)

const (
	Name = "UnifiedCPUSetAllocator"
)

var (
	_ framework.PreFilterPlugin = &Plugin{}
	_ framework.FilterPlugin    = &Plugin{}
	_ framework.ScorePlugin     = &Plugin{}
	_ framework.ReservePlugin   = &Plugin{}
	_ framework.PreBindPlugin   = &Plugin{}

	_ frameworkext.ReservationRestorePlugin = &Plugin{}
	_ frameworkext.ReservationPreBindPlugin = &Plugin{}
)

type Plugin struct {
	handle framework.Handle
	*nodenumaresource.Plugin
	cpuSharePoolUpdater *cpuSharePoolUpdater
}

func New(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	args, err := getNodeNUMAResourceArgs(obj)
	if err != nil {
		return nil, err
	}
	defaultNUMAAllocateStrategy := nodenumaresource.GetDefaultNUMAAllocateStrategy(args)

	topologyManager := nodenumaresource.NewTopologyOptionsManager()
	cpuManager := nodenumaresource.NewResourceManager(handle, defaultNUMAAllocateStrategy, topologyManager)
	updater := newCPUSharePoolUpdater(handle, cpuManager)
	cpuManager = newCPUManagerAdapter(cpuManager, updater)
	internalPlugin, err := nodenumaresource.NewWithOptions(args, handle,
		nodenumaresource.WithTopologyOptionsManager(topologyManager),
		nodenumaresource.WithResourceManager(cpuManager),
	)
	if err != nil {
		return nil, err
	}

	p := &Plugin{
		handle:              handle,
		Plugin:              internalPlugin.(*nodenumaresource.Plugin),
		cpuSharePoolUpdater: updater,
	}
	registerNodeEventHandler(handle, p.GetTopologyOptionsManager())
	return p, nil
}

func getNodeNUMAResourceArgs(obj runtime.Object) (*schedulingconfig.NodeNUMAResourceArgs, error) {
	if obj == nil {
		return getDefaultNodeNUMAResourceArgs()
	}

	unknownObj, ok := obj.(*runtime.Unknown)
	if !ok {
		return nil, fmt.Errorf("got args of type %T, want *NodeNUMAResourceArgs", obj)
	}
	var v1beta2args v1beta2.NodeNUMAResourceArgs
	// Disable default CPUBindPolicy
	v1beta2args.DefaultCPUBindPolicy = pointer.String("")
	v1beta2.SetDefaults_NodeNUMAResourceArgs(&v1beta2args)
	if err := frameworkruntime.DecodeInto(unknownObj, &v1beta2args); err != nil {
		return nil, err
	}
	var args schedulingconfig.NodeNUMAResourceArgs
	err := v1beta2.Convert_v1beta2_NodeNUMAResourceArgs_To_config_NodeNUMAResourceArgs(&v1beta2args, &args, nil)
	if err != nil {
		return nil, err
	}
	return &args, nil
}

func getDefaultNodeNUMAResourceArgs() (*schedulingconfig.NodeNUMAResourceArgs, error) {
	var v1beta2args v1beta2.NodeNUMAResourceArgs
	// Disable default CPUBindPolicy
	v1beta2args.DefaultCPUBindPolicy = pointer.String("")
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
	status := p.Plugin.Filter(ctx, cycleState, pod, nodeInfo)
	if !status.IsSuccess() {
		return status
	}

	if k8sfeature.DefaultFeatureGate.Enabled(features.DisableCPUSetOversold) {
		return filterWithDisableCPUSetOversold(pod, nodeInfo, p.Plugin.GetResourceManager().GetAvailableCPUs)
	}
	return nil
}

type getAvailableCPUsFn func(nodeName string, preferredCPUs cpuset.CPUSet) (availableCPUs cpuset.CPUSet, allocated nodenumaresource.CPUDetails, err error)

func filterWithDisableCPUSetOversold(pod *corev1.Pod, nodeInfo *framework.NodeInfo, getAvailableCPUs getAvailableCPUsFn) *framework.Status {
	if extension.GetPodPriorityClassRaw(pod) != extension.PriorityProd {
		return nil
	}

	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "missing node")
	}

	cpuOverQuota, _, _ := extunified.GetResourceOverQuotaSpec(node)
	if cpuOverQuota <= 100 {
		return nil
	}

	availableCPUs, allocatedCPUSets, err := getAvailableCPUs(node.Name, cpuset.CPUSet{})
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	podRequests, _ := resource.PodRequestsAndLimits(pod)

	freeCPUs := nodeInfo.Allocatable.MilliCPU - nodeInfo.Requested.MilliCPU
	freeCPUs -= int64(allocatedCPUSets.CPUs().Size()*1000*(int(cpuOverQuota)-100)) / 100
	if freeCPUs < podRequests.Cpu().MilliValue() {
		return framework.NewStatus(framework.Unschedulable, "Insufficient CPUs")
	}

	cpuRequestMilli := podRequests.Cpu().MilliValue()
	numCPUsNeeded := int(cpuRequestMilli+999) / 1000
	if availableCPUs.Size() < numCPUsNeeded {
		klog.V(5).Infof("Node: %v, Pod: %v, availableCPUs: %v, numCPUsNeeded: %v", node.Name, klog.KObj(pod), availableCPUs.Size(), numCPUsNeeded)
		return framework.NewStatus(framework.Unschedulable, "Insufficient cpu cores")
	}
	return nil
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

func (p *Plugin) PreBind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	status := p.Plugin.PreBind(ctx, cycleState, pod, nodeName)
	if !status.IsSuccess() {
		return status
	}
	resourceStatus, err := extunified.GetResourceStatus(pod.Annotations)
	if err != nil {
		return framework.AsStatus(err)
	}
	err = extunified.SetUnifiedResourceStatus(pod, resourceStatus)
	if err != nil {
		return framework.AsStatus(err)
	}
	return nil
}

func (p *Plugin) PreBindReservation(ctx context.Context, cycleState *framework.CycleState, reservation *schedulingv1alpha1.Reservation, nodeName string) *framework.Status {
	return p.Plugin.PreBindReservation(ctx, cycleState, reservation, nodeName)
}
