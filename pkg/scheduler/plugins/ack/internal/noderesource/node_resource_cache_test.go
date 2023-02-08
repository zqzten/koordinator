package noderesource

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

func Test_completeAllocatedResources(t *testing.T) {
	testCases := []struct {
		Name            string
		ResourceName    string
		Total           int
		ExpectAllocated int
		Device          *Device
		AllocatedPods   []PodResource
	}{
		{
			Name:         "none id resource for total 31",
			ResourceName: "resource1",
			Total:        31,
			Device:       NewDevice("1", NewNoneIDResources(31), NewNoneIDResources(0)),
			AllocatedPods: []PodResource{
				{
					PodUID:              "pod1",
					DeviceResourceCount: map[types.DeviceId]int{"1": 31},
					AllocatedResources: map[types.ContainerIndex]ContainerAllocatedResources{
						0: {"1": NewNoneIDResources(27)},
					},
				},
			},
		},
		{
			Name:            "none id resource for total 31",
			ResourceName:    "resource1",
			Total:           31,
			ExpectAllocated: 27,
			Device:          NewDevice("1", NewNoneIDResources(31), NewNoneIDResources(0)),
			AllocatedPods: []PodResource{
				{
					PodUID:              "pod1",
					PodName:             "pod1",
					DeviceResourceCount: map[types.DeviceId]int{"1": 31},
					AllocatedResources: map[types.ContainerIndex]ContainerAllocatedResources{
						0: {"1": NewNoneIDResources(9)},
					},
				},
				{
					PodUID:              "pod2",
					PodName:             "pod2",
					DeviceResourceCount: map[types.DeviceId]int{"1": 31},
					AllocatedResources: map[types.ContainerIndex]ContainerAllocatedResources{
						0: {"1": NewNoneIDResources(9)},
					},
				},
				{
					PodUID:              "pod3",
					PodName:             "pod3",
					DeviceResourceCount: map[types.DeviceId]int{"1": 31},
					AllocatedResources: map[types.ContainerIndex]ContainerAllocatedResources{
						0: {"1": NewNoneIDResources(9)},
					},
				},
			},
		},
		{
			Name:         "none id resource for total 127",
			ResourceName: "resource1",
			Total:        127,
			Device:       NewDevice("1", NewNoneIDResources(127), NewNoneIDResources(0)),
			AllocatedPods: []PodResource{
				{
					PodUID:              "pod1",
					DeviceResourceCount: map[types.DeviceId]int{"1": 127},
					AllocatedResources: map[types.ContainerIndex]ContainerAllocatedResources{
						0: {"1": NewNoneIDResources(27)},
					},
				},
			},
		},
	}

	for _, c := range testCases {
		t.Run(c.Name, func(tt *testing.T) {
			if len(c.AllocatedPods) == 1 {
				for i := 1; i <= c.Total; i++ {
					cache := NewNodeResourceCache(v1.ResourceName(c.ResourceName))
					cache.AddDevices(c.Device)
					c.AllocatedPods[0].AllocatedResources[0]["1"] = NewNoneIDResources(i)
					cache.AddAllocatedPodResources(c.AllocatedPods...)
					cache.completeAllocatedResources(map[types.DeviceId]*Device{c.Device.id: c.Device})
					assert.Equal(tt, i, cache.deviceResources[c.Device.id].allocated.Size())
				}
				for i := 1; i <= c.Total; i++ {
					cache := NewNodeResourceCache(v1.ResourceName(c.ResourceName))
					cache.AddDevices(c.Device)
					c.AllocatedPods[0].PodName = "pod1"
					c.AllocatedPods[0].AllocatedResources[0]["1"] = NewNoneIDResources(1)
					for j := i - 1; j > 0; j-- {
						newPod := c.AllocatedPods[0].Clone()
						newPod.PodUID = apitypes.UID(fmt.Sprintf("pod%v", i-j+1))
						newPod.PodName = fmt.Sprintf("pod%v", i-j+1)
						c.AllocatedPods = append(c.AllocatedPods, *newPod)
					}
					cache.AddAllocatedPodResources(c.AllocatedPods...)
					cache.completeAllocatedResources(map[types.DeviceId]*Device{c.Device.id: c.Device})
					assert.Equal(tt, i, cache.deviceResources[c.Device.id].allocated.Size())
				}
			} else {
				cache := NewNodeResourceCache(v1.ResourceName(c.ResourceName))
				cache.AddDevices(c.Device)
				cache.AddAllocatedPodResources(c.AllocatedPods...)
				cache.completeAllocatedResources(map[types.DeviceId]*Device{c.Device.id: c.Device})
				assert.Equal(tt, c.ExpectAllocated, cache.deviceResources[c.Device.id].allocated.Size())
			}
		})
	}
}
