package policy

import (
	"sort"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

type binpack struct{}

func NewBinpackAlgorithm() Algorithm {
	return &binpack{}
}

func (b *binpack) Name() AlgorithmType {
	return Binpack
}

func (b *binpack) Allocate(totalDevices, availableDevices map[types.DeviceId]int, req int, wantDeviceCount int) ([]CandidateAllocation, error) {
	result := []CandidateAllocation{}
	if len(totalDevices) == 0 || len(availableDevices) == 0 || req == 0 || wantDeviceCount == 0 {
		return result, nil
	}
	Iterate(availableDevices, req, wantDeviceCount, func(item map[types.DeviceId]int) {
		score := getAllocationScore(totalDevices, availableDevices, item, b.Name())
		result = append(result, CandidateAllocation{Score: score, Hint: item})
	})
	sort.Slice(result, func(i int, j int) bool {
		return result[i].Score >= result[j].Score
	})
	return result, nil
}

type replicaBinpack struct{}

func NewReplicaBinpackAlgorithm() Algorithm {
	return &replicaBinpack{}
}

func (b *replicaBinpack) Name() AlgorithmType {
	return Binpack
}

func (b *replicaBinpack) Allocate(totalDevices, availableDevices map[types.DeviceId]int, req int, wantDeviceCount int) ([]CandidateAllocation, error) {
	result := []CandidateAllocation{}
	if len(totalDevices) == 0 || len(availableDevices) == 0 || req == 0 || wantDeviceCount == 0 {
		return result, nil
	}
	IterateWithReplica(availableDevices, req, wantDeviceCount, func(item map[types.DeviceId]int) {
		score := getAllocationScore(totalDevices, availableDevices, item, b.Name())
		result = append(result, CandidateAllocation{Score: score, Hint: item})
	})
	sort.Slice(result, func(i int, j int) bool {
		return result[i].Score >= result[j].Score
	})
	return result, nil
}
