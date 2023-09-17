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

package overquota

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/apis/extension"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

const (
	Name     = "UnifiedOverQuota"
	stateKey = Name

	ErrReasonNotMatch       = "node(s) didn't satisfy over-quota"
	ErrInsufficientTotalCPU = "node(s) insufficient CPU resources"
)

var (
	_ framework.FilterPlugin    = &Plugin{}
	_ framework.PreFilterPlugin = &Plugin{}
)

var (
	IsNodeEnableOverQuota = extunified.IsNodeEnableOverQuota

	IsPodDisableOverQuotaFilter = extunified.IsPodDisableOverQuotaFilter
	IsPodRequireOverQuotaNode   = extunified.IsPodRequireOverQuotaNode
	GetLocalInlineVolumeSize    = extunified.CalcLocalInlineVolumeSize
)

type Plugin struct {
	handle framework.Handle
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	plugin := &Plugin{
		handle: handle,
	}
	return plugin, nil
}

func (p *Plugin) Name() string { return Name }

type preFilterState struct {
	checkOverQuota            bool
	isPodRequireOverQuotaNode bool
	podRequestedResource      corev1.ResourceList
}

func (s *preFilterState) Clone() framework.StateData {
	return &preFilterState{
		checkOverQuota:            s.checkOverQuota,
		isPodRequireOverQuotaNode: s.isPodRequireOverQuotaNode,
	}
}

func (p *Plugin) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	podRequestedResource := util.GetPodRequest(pod, corev1.ResourceCPU, corev1.ResourceMemory, corev1.ResourceEphemeralStorage)
	cycleState.Write(stateKey, &preFilterState{
		checkOverQuota:            !IsPodDisableOverQuotaFilter(pod) && !p.isPodNotNeedResource(pod),
		isPodRequireOverQuotaNode: IsPodRequireOverQuotaNode(pod),
		podRequestedResource:      podRequestedResource,
	})
	return nil, nil
}

func (p *Plugin) isPodNotNeedResource(pod *corev1.Pod) bool {
	resourceList := util.GetPodRequest(pod, corev1.ResourceCPU, corev1.ResourceMemory, corev1.ResourceEphemeralStorage)
	inlineVolumeSize := GetLocalInlineVolumeSize(pod.Spec.Volumes, p.handle.SharedInformerFactory().Storage().V1().StorageClasses().Lister())
	return resourceList.Cpu().IsZero() &&
		resourceList.Memory().IsZero() &&
		resourceList.StorageEphemeral().IsZero() &&
		inlineVolumeSize == 0
}

func (p *Plugin) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (p *Plugin) Filter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	state, status := getPreFilterState(cycleState)
	if status != nil {
		return status
	}
	if !state.checkOverQuota {
		return nil
	}
	node := nodeInfo.Node()
	if IsNodeEnableOverQuota(node) != state.isPodRequireOverQuotaNode {
		return framework.NewStatus(framework.Unschedulable, ErrReasonNotMatch)
	}
	if state.isPodRequireOverQuotaNode {
		rawAllocatable, err := extension.GetNodeRawAllocatable(node)
		if err != nil {
			return framework.NewStatus(framework.UnschedulableAndUnresolvable, "node(s) invalid raw allocatable")
		}
		allocatableCPU := node.Status.Allocatable[corev1.ResourceCPU]
		if quantity := rawAllocatable[corev1.ResourceCPU]; !quantity.IsZero() {
			allocatableCPU = quantity
		}
		if state.podRequestedResource.Cpu().MilliValue() > allocatableCPU.MilliValue() {
			return framework.NewStatus(framework.UnschedulableAndUnresolvable, ErrInsufficientTotalCPU)
		}
	}
	return nil
}

func getPreFilterState(cycleState *framework.CycleState) (*preFilterState, *framework.Status) {
	value, err := cycleState.Read(stateKey)
	if err != nil {
		return nil, framework.AsStatus(err)
	}
	state := value.(*preFilterState)
	return state, nil
}
