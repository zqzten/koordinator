package types

// define DeviceId type
type DeviceId string

// define ContainerIndex Type
type ContainerIndex int

// define ContainerAllocation type
type ContainerAllocation map[DeviceId]int

const ReplicaKey = "replica"
