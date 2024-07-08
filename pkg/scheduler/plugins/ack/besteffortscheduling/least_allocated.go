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

	uniapiext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	// TODO use koordiantor api
	BatchCPU    = extension.BatchCPU
	BatchMemory = extension.BatchMemory
)

// resourceToWeightMap contains resource name and weight.
type resourceToWeightMap map[v1.ResourceName]int64

// defaultRequestedRatioResources is used to set default requestToWeight map for CPU and memory
var defaultRequestedRatioResources = resourceToWeightMap{
	BatchCPU:    1,
	BatchMemory: 1,
}

// resourceToValueMap contains resource name and score.
type resourceToValueMap map[v1.ResourceName]int64

// BELeastAllocated is a plugin that favors nodes with fewer allocation requested resources
// based on requested resources for best-effort pods.
type BELeastAllocated struct {
	handle              framework.Handle
	resourceToWeightMap resourceToWeightMap
}

var _ = framework.ScorePlugin(&BELeastAllocated{})

// BELeastAllocatedName is the name of the plugin used in Registry and configurations.
const BELeastAllocatedName = "NodeBEResourceLeastAllocated"

// Name returns name of the plugin. It is used in logs, etc.
func (la *BELeastAllocated) Name() string {
	return BELeastAllocatedName
}

// Score invoked at the score extension point.
func (la *BELeastAllocated) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	nodeInfo, err := la.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}

	// la.score favors nodes with fewer requested resources.
	// It calculates the percentage of memory and CPU requested by pods scheduled on the node, and
	// prioritizes based on the minimum of the average of the fraction of requested to capacity.
	//
	// Details:
	// (cpu((capacity-sum(requested))*MaxNodeScore/capacity) + memory((capacity-sum(requested))*MaxNodeScore/capacity))/weightSum
	return la.score(pod, nodeInfo)
}

// ScoreExtensions of the Score plugin.
func (la *BELeastAllocated) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func (la *BELeastAllocated) score(pod *v1.Pod, nodeInfo *framework.NodeInfo) (int64, *framework.Status) {
	nodeAllocatable := computeNodeBatchAllocatable(nodeInfo)
	nodeRequested := computeNodeBatchRequested(nodeInfo)
	podRequest := computePodBatchRequest(pod, true)
	klog.V(5).Infof("prepare be least allocated score for pod %v/%v, allocatable %+v, node requested %+v, "+
		"pod requestted %+v", pod.Namespace, pod.Name, nodeAllocatable, nodeRequested, podRequest)

	requested := make(resourceToValueMap, len(la.resourceToWeightMap))
	allocatable := make(resourceToValueMap, len(la.resourceToWeightMap))
	for resourceName := range la.resourceToWeightMap {
		allocatable[resourceName], requested[resourceName] = calculateBatchAllocatableRequest(nodeAllocatable, nodeRequested,
			podRequest, resourceName)
	}

	var nodeScore, weightSum int64
	for resource, weight := range la.resourceToWeightMap {
		resourceScore := leastRequestedScore(requested[resource], allocatable[resource])
		nodeScore += resourceScore * weight
		weightSum += weight
	}
	return nodeScore / weightSum, nil
}

// NewBELeastAllocated a new plugin and returns it.
func NewBELeastAllocated(laArgs runtime.Object, h framework.Handle) (framework.Plugin, error) {
	return &BELeastAllocated{
		handle:              h,
		resourceToWeightMap: defaultRequestedRatioResources,
	}, nil
}

// The more unused resource the higher the score is
func leastRequestedScore(requested, capacity int64) int64 {
	// This node does not support colocation, or the pod does not need batch resource.
	// So prefer the current node and return `MaxNodeScore`
	if requested == 0 || capacity == 0 {
		return framework.MaxNodeScore
	}
	if requested > capacity {
		return 0
	}
	return ((capacity - requested) * framework.MaxNodeScore) / capacity
}

// calculateBatchAllocatableRequest returns batch resources Allocatable and Requested values
func calculateBatchAllocatableRequest(nodeAllocatable, nodeRequested, podRequest resourceToValueMap,
	resource v1.ResourceName) (int64, int64) {
	switch resource {
	case BatchCPU, BatchMemory:
		return nodeAllocatable[resource], nodeRequested[resource] + podRequest[resource]
	}
	return 0, 0
}

// computeNodeBatchRequested returns best-effort Requested values of node
// for compatible purpose, sum reclaimed resource and batch resource as batch requested
func computeNodeBatchRequested(nodeInfo *framework.NodeInfo) resourceToValueMap {
	requested := resourceToValueMap{
		BatchCPU:    0,
		BatchMemory: 0,
	}
	if reclaimedCPU, exist := nodeInfo.Requested.ScalarResources[uniapiext.AlibabaCloudReclaimedCPU]; exist {
		requested[BatchCPU] += reclaimedCPU
	}
	if batchCPU, exist := nodeInfo.Requested.ScalarResources[BatchCPU]; exist {
		requested[BatchCPU] += batchCPU
	}
	if reclaimedMemory, exist := nodeInfo.Requested.ScalarResources[uniapiext.AlibabaCloudReclaimedMemory]; exist {
		requested[BatchMemory] += reclaimedMemory
	}
	if batchMemory, exist := nodeInfo.Requested.ScalarResources[BatchMemory]; exist {
		requested[BatchMemory] += batchMemory
	}
	return requested
}

// computeNodeBatchAllocatable returns koordinator batch resources Allocatable values of node
// for compatible purpose, use reclaimed resource if koordinator batch resource not exist
func computeNodeBatchAllocatable(nodeInfo *framework.NodeInfo) resourceToValueMap {
	allocatable := resourceToValueMap{
		BatchCPU:    0,
		BatchMemory: 0,
	}
	if reclaimedCPU, exist := nodeInfo.Allocatable.ScalarResources[uniapiext.AlibabaCloudReclaimedCPU]; exist {
		allocatable[BatchCPU] = reclaimedCPU
	}
	if batchCPU, exist := nodeInfo.Allocatable.ScalarResources[BatchCPU]; exist {
		allocatable[BatchCPU] = batchCPU
	}
	if reclaimedMemory, exist := nodeInfo.Allocatable.ScalarResources[uniapiext.AlibabaCloudReclaimedMemory]; exist {
		allocatable[BatchMemory] = reclaimedMemory
	}
	if batchMemory, exist := nodeInfo.Allocatable.ScalarResources[BatchMemory]; exist {
		allocatable[BatchMemory] = batchMemory
	}
	return allocatable
}

// computePodBatchRequest returns the total non-zero best-effort requests. If Overhead is defined for the pod and
// the PodOverhead feature is enabled, the Overhead is added to the result.
// podRequest = max(sum(podSpec.Containers), podSpec.InitContainers) + overHead
// for compatible purpose, use reclaimed resource if batch resource not exist
func computePodBatchRequest(pod *v1.Pod, enablePodOverhead bool) resourceToValueMap {
	podRequest := &framework.Resource{}
	for _, container := range pod.Spec.Containers {
		podRequest.Add(container.Resources.Requests)
	}

	// take max_resource(sum_pod, any_init_container)
	for _, container := range pod.Spec.InitContainers {
		podRequest.SetMaxResource(container.Resources.Requests)
	}

	// If Overhead is being utilized, add to the total requests for the pod
	if pod.Spec.Overhead != nil && enablePodOverhead {
		podRequest.Add(pod.Spec.Overhead)
	}

	result := resourceToValueMap{
		BatchCPU:    0,
		BatchMemory: 0,
	}
	if reclaimedCPU, exist := podRequest.ScalarResources[uniapiext.AlibabaCloudReclaimedCPU]; exist {
		result[BatchCPU] = reclaimedCPU
	}
	if batchCPU, exist := podRequest.ScalarResources[BatchCPU]; exist {
		result[BatchCPU] = batchCPU
	}
	if reclaimedMemory, exist := podRequest.ScalarResources[uniapiext.AlibabaCloudReclaimedMemory]; exist {
		result[BatchMemory] = reclaimedMemory
	}
	if batchMemory, exist := podRequest.ScalarResources[BatchMemory]; exist {
		result[BatchMemory] = batchMemory
	}
	return result
}
