package noderesource

import (
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

type Device struct {
	id        types.DeviceId
	total     Resources
	unhealthy Resources
	allocated Resources
	pods      map[string]float64
}

func NewDevice(id types.DeviceId, totalResources, unhealthyResources Resources) *Device {
	var allocated Resources
	if totalResources.Type() == CountableResource {
		allocated = NewCountableResources([]string{})
	} else {
		allocated = NewNoneIDResources(0)
	}
	return &Device{
		id:        id,
		total:     totalResources,
		unhealthy: unhealthyResources,
		allocated: allocated,
		pods:      map[string]float64{},
	}
}

func (d *Device) Clone() *Device {
	pods := map[string]float64{}
	for podName, count := range d.pods {
		pods[podName] = count
	}
	var allocated Resources
	if d.allocated != nil {
		allocated = d.allocated.Clone()
	}
	return &Device{
		id:        d.id,
		total:     d.total.Clone(),
		unhealthy: d.unhealthy.Clone(),
		allocated: allocated,
		pods:      d.pods,
	}
}

func (d *Device) GetId() types.DeviceId {
	return d.id
}

func (d *Device) GetTotal() Resources {
	return d.total
}

func (d *Device) SetUnhealthyResources(unhealthy Resources) {
	d.unhealthy = unhealthy
}

func (d *Device) GetAvailableResources() Resources {
	return d.total.Remove(d.unhealthy).Remove(d.allocated)
}

func (d *Device) GetPods() map[string]float64 {
	return d.pods
}

func (d *Device) setAllocatedResources(allocated Resources) {
	d.allocated = allocated
}

func (d *Device) addPods(pods map[string]float64) {
	d.pods = pods
}
