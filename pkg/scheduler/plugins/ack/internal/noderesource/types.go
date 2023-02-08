package noderesource

import (
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

// specify the resource type
type ResourceType string

const (
	// CountableResource represents that resource can be countable,eg: npu core
	CountableResource ResourceType = "CountableResource"
	// NoneIDResource represents that resource without id and can not be countable,eg: gpu memory
	NoneIDResource ResourceType = "NoneIDResource"
	// UnknownResource
	UnknownResource ResourceType = "UnknownResource"
)

/*

type DeviceId string
type ContainerIndex int
type ResourceName string
*/

type CountableResourceAllocation map[types.ContainerIndex]map[types.DeviceId][]string

type NoneIDResourceAllocation map[types.ContainerIndex]map[types.DeviceId]string

type DeviceDetail struct {
	Id         types.DeviceId     `json:"id"`
	Total      string             `json:"total"`
	Unhealthy  string             `json:"unhealthy"`
	Allocated  string             `json:"allocated"`
	PodDetails map[string]float64 `json:"allocations"`
}
