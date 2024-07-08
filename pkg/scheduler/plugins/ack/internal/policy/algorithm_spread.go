package policy

import (
	"sort"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

type spread struct{}

func NewSpreadAlgorithm() Algorithm {
	return &spread{}
}

func (s *spread) Name() AlgorithmType {
	return Spread
}

func (s *spread) Allocate(totalDevices, availableDevices map[types.DeviceId]int, request int, wantDeviceCount int) ([]CandidateAllocation, error) {
	result := []CandidateAllocation{}
	if len(totalDevices) == 0 || len(availableDevices) == 0 {
		return result, nil
	}
	Iterate(availableDevices, request, wantDeviceCount, func(item map[types.DeviceId]int) {
		score := getAllocationScore(totalDevices, availableDevices, item, s.Name())
		result = append(result, CandidateAllocation{Score: score, Hint: item})
	})
	sort.Slice(result, func(i int, j int) bool {
		return result[i].Score >= result[j].Score
	})
	return result, nil
}

type replicaSpread struct{}

func NewReplicaSpreadAlgorithm() Algorithm {
	return &replicaSpread{}
}

func (s *replicaSpread) Name() AlgorithmType {
	return Spread
}

func (s *replicaSpread) Allocate(totalDevices, availableDevices map[types.DeviceId]int, request int, wantDeviceCount int) ([]CandidateAllocation, error) {
	result := []CandidateAllocation{}
	if len(totalDevices) == 0 || len(availableDevices) == 0 {
		return result, nil
	}

	IterateWithReplica(availableDevices, request, wantDeviceCount, func(item map[types.DeviceId]int) {
		score := getAllocationScore(totalDevices, availableDevices, item, s.Name())
		result = append(result, CandidateAllocation{Score: score, Hint: item})
	})
	sort.Slice(result, func(i int, j int) bool {
		return result[i].Score >= result[j].Score
	})
	return result, nil
}
