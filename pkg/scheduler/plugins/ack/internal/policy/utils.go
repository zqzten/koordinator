package policy

import (
	"fmt"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

func getAllocationScore(totalDevices, availableDevices map[types.DeviceId]int, allocations map[types.DeviceId]int, t AlgorithmType) float32 {
	deviceCount := float32(len(allocations))
	if deviceCount == 0 {
		return 0
	}
	avg := float32(0)
	for id, count := range allocations {
		total := totalDevices[id]
		available := availableDevices[id]
		if total == 0 || available == 0 {
			return 0
		}
		if t == Binpack {
			//avg += (float32(0.5) * float32(total-available+count) / float32(total)) + (float32(0.5) * float32(count) / float32(total))
			avg += float32(total-available+count) / float32(total)

		} else {
			avg += float32(1) - (float32(total-available+count) / float32(total))
		}
	}
	avg = avg / deviceCount
	score := float32(30)*avg + float32(70)/deviceCount/deviceCount
	return score
}

func AllocateForContainers(totalDevices map[types.DeviceId]int, podCandidateAllocation CandidateAllocation, containerRequests map[types.ContainerIndex]int, wantDeviceCount int) (map[types.ContainerIndex]CandidateAllocation, error) {
	algo := NewBinpackAlgorithm()
	hint := podCandidateAllocation.Hint
	result := map[types.ContainerIndex]CandidateAllocation{}
	for index, count := range containerRequests {
		containerAllocation, err := algo.Allocate(totalDevices, hint, count, -1)
		if err != nil {
			return nil, err
		}
		if len(containerAllocation) == 0 {
			return nil, fmt.Errorf("not found available allocation for container %v", index)
		}
		for devId, alloc := range containerAllocation[0].Hint {
			hint[devId] = hint[devId] - alloc
		}
		result[index] = containerAllocation[0]
	}
	return result, nil
}

func AllocateForContainersWithReplica(totalDevices map[types.DeviceId]int, podCandidateAllocation CandidateAllocation, containerRequests map[types.ContainerIndex]int, wantDeviceCount int) (map[types.ContainerIndex]CandidateAllocation, error) {
	available := podCandidateAllocation.Hint
	result := map[types.ContainerIndex]CandidateAllocation{}
	for index, req := range containerRequests {
		alloc := map[types.DeviceId]int{}
		for id := range available {
			alloc[id] = req / wantDeviceCount
		}
		result[index] = CandidateAllocation{Hint: alloc}
	}
	return result, nil
}
