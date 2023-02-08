package policy

// MultiDevicesPolicyFactory is used to create multiDevices policy
type MultiDevicesPolicyFactory struct{}

// Create creates multiple device policy
func (m MultiDevicesPolicyFactory) Create(algorithm Algorithm) Policy {
	return &multiDevices{
		algorithm: algorithm,
	}
}

type multiDevices struct {
	algorithm Algorithm
}

// Name returns the policy name
func (m *multiDevices) Name() PolicyType {
	return MultiDevices
}

// Allocate returns the allocation for the request
func (m *multiDevices) Allocate(args PolicyArgs) ([]CandidateAllocation, error) {
	return m.algorithm.Allocate(args.TotalDevices, args.AvailableDevices, args.PodRequest, args.WantDeviceCountInAllocation)
}

// Filter is used to check the node has enough available device units for the request
func (m *multiDevices) Filter(args PolicyArgs) (bool, error) {
	totalAvailableResourceCount := 0
	for _, availableResourceCount := range args.AvailableDevices {
		totalAvailableResourceCount += availableResourceCount
	}
	return totalAvailableResourceCount >= args.PodRequest, nil
}
