package policy

import (
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

type Algorithm interface {
	// Name returns the name of Algorithm
	Name() AlgorithmType

	// Allocate returns the allocation of the given node
	Allocate(totalDevices, availableDevices map[types.DeviceId]int, request int, wantDeviceCount int) ([]CandidateAllocation, error)
}

type PolicyArgs struct {
	TotalDevices                map[types.DeviceId]int
	AvailableDevices            map[types.DeviceId]int
	PodRequest                  int
	WantDeviceCountInAllocation int
	RequireWholeGPU             bool
}

// Policy is used to define how many devices are allocated to pod
// there is two:
// (1) SingleDevice: the allocation must includes only one device
// (2) MultiDevices: the allocation can include multiple devices
type Policy interface {
	// return the policy name
	Name() PolicyType

	// Allocate gets an allocation under the target policy
	Allocate(args PolicyArgs) ([]CandidateAllocation, error)

	// Filter is used to check the node has enough units for request or not
	Filter(args PolicyArgs) (bool, error)
}

// PolicyFactory is used to create a policy
type PolicyFactory interface {
	Create(algorithm Algorithm) Policy
}
