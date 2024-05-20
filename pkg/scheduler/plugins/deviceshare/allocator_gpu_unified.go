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
	"sort"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

const (
	ErrInsufficientPartitionedDevice = "Insufficient Partitioned GPU Devices"
	ErrNotSupportedGPURequests       = "node(s) Unsupported number of GPU requests"
)

func init() {
	deviceAllocators[schedulingv1alpha1.GPU] = &GPUAllocator{}
}

var _ DeviceAllocator = &GPUAllocator{}

type GPUAllocator struct {
}

func (a *GPUAllocator) Allocate(requestCtx *requestContext, nodeDevice *nodeDevice, desiredCount int, maxDesiredCount int, preferredPCIEs sets.String) ([]*apiext.DeviceAllocation, *framework.Status) {
	if unified.MustAllocateGPUByPartition(requestCtx.node) {
		if requestCtx.allocateByTopology {
			return nil, nil
		}
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
	partitionTable := nodeDevice.gpuPartitionTable
	if partitionTable == nil {
		partitionTable = unified.GetGPUPartitionTable(requestCtx.node)
		if partitionTable == nil {
			return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, "node(s) missing GPU Partition Table")
		}
	}

	sortedPartitions, status := sortCandidatePartition(partitionTable, desiredCount, requestCtx.nodeDevice)
	if !status.IsSuccess() {
		return nil, status
	}

	for _, partition := range sortedPartitions {
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

	return nil, framework.NewStatus(framework.Unschedulable, ErrInsufficientPartitionedDevice)
}

func sortCandidatePartition(partitionTable unified.GPUPartitionTable, desiredCount int, nodeDevice *nodeDevice) ([]unified.GPUPartition, *framework.Status) {
	partitions, ok := partitionTable[desiredCount]
	if !ok {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, ErrNotSupportedGPURequests)
	}
	if desiredCount == 8 || desiredCount == 4 {
		return partitions, nil
	}

	everAllocatedDevice := nodeDevice.deviceUsed[schedulingv1alpha1.GPU]
	if len(everAllocatedDevice) == 0 {
		return partitions, nil
	}
	var moduleIDs []int
	for minor := range everAllocatedDevice {
		if moduleID := nodeDevice.deviceInfos[schedulingv1alpha1.GPU][minor].ModuleID; moduleID != nil {
			moduleIDs = append(moduleIDs, int(*moduleID))
		}
	}
	everAllocatedModuleIDHash := hashModuleIDs(moduleIDs)

	var partitionDetails []*GPUPartitionDetail
	for numberOfGPUs, gpuPartitions := range partitionTable {
		if numberOfGPUs <= desiredCount || numberOfGPUs == 8 {
			continue
		}
		for i := range gpuPartitions {
			gpuPartition := gpuPartitions[i]
			moduleIDHash := hashModuleIDs(gpuPartition.ModuleIDs)
			if everAllocated := moduleIDHash & everAllocatedModuleIDHash; everAllocated != 0 {
				continue
			}
			partitionDetails = append(partitionDetails, &GPUPartitionDetail{
				GPUPartition: &gpuPartition,
				moduleIDHash: moduleIDHash,
			})
		}
	}

	candidatePartitions := partitionTable[desiredCount]
	var candidatePartitionScores []*GPUPartitionScore
	scoreOfNumOfGPUs := map[int]int{4: 10, 2: 1}
	for i := range candidatePartitions {
		candidatePartition := candidatePartitions[i]
		score := 0
		moduleIDHash := hashModuleIDs(candidatePartition.ModuleIDs)
		for _, partitionDetail := range partitionDetails {
			if moduleIDHash&partitionDetail.moduleIDHash == 0 {
				score += scoreOfNumOfGPUs[partitionDetail.NumberOfGPUs]
			}
		}
		candidatePartitionScores = append(candidatePartitionScores, &GPUPartitionScore{
			GPUPartition: &candidatePartition,
			score:        score,
		})
	}

	sort.Slice(candidatePartitionScores, func(i, j int) bool {
		return candidatePartitionScores[i].score > candidatePartitionScores[j].score
	})
	var resultPartitions []unified.GPUPartition
	for _, candidatePartitionScore := range candidatePartitionScores {
		resultPartitions = append(resultPartitions, *candidatePartitionScore.GPUPartition)
	}
	return resultPartitions, nil
}

type GPUPartitionDetail struct {
	*unified.GPUPartition
	moduleIDHash int
}

type GPUPartitionScore struct {
	*unified.GPUPartition
	score int
}

func hashModuleIDs(moduleIDs []int) int {
	hash := 0
	for _, moduleID := range moduleIDs {
		moduleHash := 1 << moduleID
		hash = hash | moduleHash
	}
	return hash
}
