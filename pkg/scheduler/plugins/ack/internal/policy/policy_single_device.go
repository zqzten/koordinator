package policy

import "fmt"

// SingleDevicePolicyFactory is used to create SingleDevice policy
type SingleDevicePolicyFactory struct{}

func (s SingleDevicePolicyFactory) Create(algorithm Algorithm) Policy {
	return &singleDevice{
		algorithm: algorithm,
	}
}

// singleDevice implements the policy
type singleDevice struct {
	algorithm Algorithm
}

// Name returns the policy name
func (s *singleDevice) Name() PolicyType {
	return SingleDevice
}

// Allocate returns the allocation for the request
func (s *singleDevice) Allocate(args PolicyArgs) ([]CandidateAllocation, error) {
	return s.algorithm.Allocate(args.TotalDevices, args.AvailableDevices, args.PodRequest, 1)
}

// Filter is used to check the node has enough available device units for the request
func (s *singleDevice) Filter(args PolicyArgs) (bool, error) {
	for devId, availableResourceCount := range args.AvailableDevices {
		if args.RequireWholeGPU {
			if availableResourceCount == args.PodRequest && availableResourceCount == args.TotalDevices[devId] {
				return true, nil
			}
		} else if availableResourceCount >= args.PodRequest {
			return true, nil
		}
	}
	if args.RequireWholeGPU {
		return false, fmt.Errorf("Pod's RequireWholeGPU is true and not available devices can be fully occupied by it")
	}
	return false, nil
}
