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

package deviceshare

import (
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func init() {
	deviceAllocators[schedulingv1alpha1.GPU] = &GPUAllocator{}
}

var _ DeviceAllocator = &GPUAllocator{}

type GPUAllocator struct {
}

func (a *GPUAllocator) Allocate(requestCtx *requestContext, nodeDevice *nodeDevice, desiredCount int, maxDesiredCount int, preferredPCIEs sets.String) ([]*apiext.DeviceAllocation, *framework.Status) {
	if unified.MustAllocateGPUByPartition(requestCtx.node) {
		return a.allocateResourcesByPartition(requestCtx, nodeDevice, desiredCount, preferredPCIEs)
	}

	allocations, status := defaultAllocateDevices(
		nodeDevice,
		requestCtx,
		requestCtx.requestsPerInstance[schedulingv1alpha1.GPU],
		desiredCount,
		desiredCount,
		schedulingv1alpha1.GPU,
		preferredPCIEs,
	)
	if !status.IsSuccess() {
		return nil, status
	}
	return allocations, nil
}

func (a *GPUAllocator) allocateResourcesByPartition(requestCtx *requestContext, nodeDevice *nodeDevice, desiredCount int, preferredPCIes sets.String) ([]*apiext.DeviceAllocation, *framework.Status) {
	partitionTable := unified.GetGPUPartitionTable(requestCtx.node)
	partitions, ok := partitionTable[desiredCount]
	if !ok {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, "node(s) Unsupported number of GPU requests")
	}

	for _, partition := range partitions {
		minors := sets.NewInt()
		for _, moduleID := range partition.ModuleIDs {
			for _, v := range nodeDevice.deviceInfos[schedulingv1alpha1.GPU] {
				if v.ModuleID != nil && int(*v.ModuleID) == moduleID && v.Minor != nil {
					minors.Insert(int(*v.Minor))
					break
				}
			}
		}
		devices := map[schedulingv1alpha1.DeviceType][]int{
			schedulingv1alpha1.GPU: minors.UnsortedList(),
		}
		partitionDevice := nodeDevice.filter(devices, nil, nil, nil)
		freeDevices := partitionDevice.split(requestCtx.requestsPerInstance[schedulingv1alpha1.GPU], schedulingv1alpha1.GPU)
		freeCount := len(freeDevices)
		if freeCount < desiredCount {
			continue
		}

		allocations, status := defaultAllocateDevices(
			partitionDevice,
			requestCtx,
			requestCtx.requestsPerInstance[schedulingv1alpha1.GPU].DeepCopy(),
			desiredCount,
			desiredCount,
			schedulingv1alpha1.GPU,
			preferredPCIes,
		)
		if !status.IsSuccess() {
			continue
		}

		return allocations, nil
	}

	return nil, framework.NewStatus(framework.Unschedulable, "Insufficient gpu devices")
}
