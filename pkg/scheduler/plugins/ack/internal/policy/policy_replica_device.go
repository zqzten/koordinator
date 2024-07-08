package policy

import "fmt"

type ReplicaDevicePolicyFactory struct{}

func (r ReplicaDevicePolicyFactory) Create(algorithm Algorithm) Policy {
	return &replicaDevice{
		algorithm: algorithm,
	}
}

type replicaDevice struct {
	algorithm Algorithm
}

func (r *replicaDevice) Name() PolicyType {
	return ReplicaDevice
}
func (r *replicaDevice) Filter(args PolicyArgs) (bool, error) {
	if len(args.TotalDevices) < args.WantDeviceCountInAllocation || args.WantDeviceCountInAllocation <= 0 {
		return false, nil
	}
	available := 0
	for devId, count := range args.AvailableDevices {
		if args.RequireWholeGPU {
			if count == args.PodRequest/args.WantDeviceCountInAllocation && count == args.TotalDevices[devId] {
				available++
			}
		} else if count >= args.PodRequest/args.WantDeviceCountInAllocation {
			available++
		}
	}
	if available < args.WantDeviceCountInAllocation {
		if args.RequireWholeGPU {
			return false, fmt.Errorf("Pod's RequireWholeGPU is true and not available devices can be fully occupied by it")
		}
		return false, nil
	}
	return available >= args.WantDeviceCountInAllocation, nil
}

// Allocate returns the allocation for the request
func (r *replicaDevice) Allocate(args PolicyArgs) ([]CandidateAllocation, error) {
	return r.algorithm.Allocate(args.TotalDevices, args.AvailableDevices, args.PodRequest, args.WantDeviceCountInAllocation)
}
