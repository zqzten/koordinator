package policy

import "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"

type PolicyType string

const (
	MultiDevices  PolicyType = "MultiDevices"
	SingleDevice  PolicyType = "SingleDevice"
	ReplicaDevice PolicyType = "ReplicaDevice"
)

type AlgorithmType string

const (
	Binpack AlgorithmType = "binpack"
	Spread  AlgorithmType = "spread"
)

type UnitType string

const (
	Indexable UnitType = "indexable"
	Countable UnitType = "countable"
)

// CandidateAllocation is used to store candidate allocation
// it includes a score for the allocation and allocation hint
type CandidateAllocation struct {
	Score float32
	Hint  map[types.DeviceId]int
}
