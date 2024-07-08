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

package besteffortscheduling

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/batchresource"
)

var _ framework.PreFilterPlugin = &Fit{}
var _ framework.FilterPlugin = &Fit{}

const (
	BatchResourceFitName = batchresource.Name

	// preFilterStateKey is the key in CycleState to BatchResourceFit pre-computed data.
	// Using the name of the plugin will likely help us avoid collisions with other plugins.
	preFilterStateKey = "PreFilter" + BatchResourceFitName
)

type Fit struct {
	handle framework.Handle
}

// preFilterState computed at PreFilter and used at Filter.
type preFilterState struct {
	resourceToValueMap
}

// Clone the prefilter state.
func (s *preFilterState) Clone() framework.StateData {
	return s
}

func (f *Fit) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod) *framework.Status {
	preFilterState := &preFilterState{
		resourceToValueMap: computePodBatchRequest(pod, true),
	}
	cycleState.Write(preFilterStateKey, preFilterState)
	return nil
}

func (f *Fit) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func getPreFilterState(cycleState *framework.CycleState) (*preFilterState, error) {
	c, err := cycleState.Read(preFilterStateKey)
	if err != nil {
		// preFilterState doesn't exist, likely PreFilter wasn't invoked.
		return nil, fmt.Errorf("error reading %q from cycleState: %w", preFilterStateKey, err)
	}

	s, ok := c.(*preFilterState)
	if !ok {
		return nil, fmt.Errorf("%+v  convert to NodeBatchResourcesFit.preFilterState error", c)
	}
	return s, nil
}

func (f *Fit) Name() string {
	return BatchResourceFitName
}

func NewFit(plArgs runtime.Object, h framework.Handle) (framework.Plugin, error) {
	return &Fit{
		handle: h,
	}, nil
}

func (f *Fit) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	s, err := getPreFilterState(cycleState)
	if err != nil {
		return framework.AsStatus(err)
	}

	// pod does not request batch resource
	if s.resourceToValueMap[BatchCPU] == 0 && s.resourceToValueMap[BatchMemory] == 0 {
		return nil
	}
	klog.V(5).Infof("get pre filter state for pod %v/%v, detail %+v",
		pod.Namespace, pod.Name, s.resourceToValueMap)

	insufficientResources := fitsRequest(s, nodeInfo)

	if len(insufficientResources) != 0 {
		// We will keep all failure reasons.
		failureReasons := make([]string, 0, len(insufficientResources))
		for _, r := range insufficientResources {
			failureReasons = append(failureReasons, r.Reason)
		}
		return framework.NewStatus(framework.Unschedulable, failureReasons...)
	}
	return nil
}

// InsufficientResource describes what kind of resource limit is hit and caused the pod to not fit the node.
type InsufficientResource struct {
	ResourceName v1.ResourceName
	// We explicitly have a parameter for reason to avoid formatting a message on the fly
	// for common resources, which is expensive for cluster autoscaler simulations.
	Reason    string
	Requested int64
	Used      int64
	Capacity  int64
}

func fitsRequest(podRequest *preFilterState, nodeInfo *framework.NodeInfo) []InsufficientResource {
	insufficientResources := make([]InsufficientResource, 0, 2)
	nodeAllocatable := computeNodeBatchAllocatable(nodeInfo)
	nodeRequested := computeNodeBatchRequested(nodeInfo)

	batchCPUAllocatable, batchCPURequested := calculateBatchAllocatableRequest(nodeAllocatable, nodeRequested,
		podRequest.resourceToValueMap, BatchCPU)

	klog.V(5).Infof("fit batch resource node allocatable %+v, node requested %+v , current pod requested %+v",
		nodeAllocatable, nodeRequested, podRequest)

	if batchCPURequested > batchCPUAllocatable {
		insufficientResources = append(insufficientResources, InsufficientResource{
			ResourceName: BatchCPU,
			Reason:       fmt.Sprintf("Insufficient %v", BatchCPU),
			Requested:    podRequest.resourceToValueMap[BatchCPU],
			Used:         nodeRequested[BatchCPU],
			Capacity:     nodeAllocatable[BatchCPU],
		})
	}

	batchMemoryAllocatable, batchMemoryRequested := calculateBatchAllocatableRequest(nodeAllocatable, nodeRequested,
		podRequest.resourceToValueMap, BatchMemory)
	if batchMemoryRequested > batchMemoryAllocatable {
		insufficientResources = append(insufficientResources, InsufficientResource{
			ResourceName: BatchMemory,
			Reason:       fmt.Sprintf("Insufficient %v", BatchMemory),
			Requested:    podRequest.resourceToValueMap[BatchMemory],
			Used:         nodeRequested[BatchMemory],
			Capacity:     nodeAllocatable[BatchMemory],
		})
	}

	return insufficientResources
}
