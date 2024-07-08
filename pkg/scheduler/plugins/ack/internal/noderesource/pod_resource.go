package noderesource

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

type ContainerAllocatedResources map[types.DeviceId]Resources

func BuildContainerAllocatedResources(allocation interface{}) (ContainerAllocatedResources, error) {
	result := map[types.DeviceId]Resources{}
	switch v := allocation.(type) {
	case map[string][]string:
		for deviceId, deviceAllocatedResources := range v {
			result[types.DeviceId(deviceId)] = NewCountableResources(deviceAllocatedResources)
		}
	case map[string]int:
		for deviceId, deviceAllocatedResources := range v {
			result[types.DeviceId(deviceId)] = NewNoneIDResources(deviceAllocatedResources)
		}
	default:
		return nil, fmt.Errorf("unknown resource type %v,only support: 'map[string][]string' or 'map[string]int'", v)
	}
	return result, nil
}

func GetContainerAllocatedResourceType(c ContainerAllocatedResources) ResourceType {
	for _, resources := range c {
		return resources.Type()
	}
	return UnknownResource
}

func GetContainerCountableResourceIds(c ContainerAllocatedResources) map[types.DeviceId][]string {
	result := map[types.DeviceId][]string{}
	for deviceId, resources := range c {
		ids := strings.Split(resources.String(), ",")
		result[deviceId] = ids
	}
	return result
}

func GetContainerNoneIDResources(c ContainerAllocatedResources) map[types.DeviceId]int {
	result := map[types.DeviceId]int{}
	for deviceId, resources := range c {
		size, _ := strconv.Atoi(resources.String())
		result[deviceId] = size
	}
	return result
}

func BuildPodContainersAllocatedResources(allocation string, p *PodResource) error {
	v1 := CountableResourceAllocation{}
	err := json.Unmarshal([]byte(allocation), &v1)
	if err == nil {
		allocatedResources := map[types.ContainerIndex]ContainerAllocatedResources{}
		for containerIndex, containerResources := range v1 {
			containerAllocatedResources := map[types.DeviceId]Resources{}
			for deviceId, deviceAllocatedResources := range containerResources {
				containerAllocatedResources[deviceId] = NewCountableResources(deviceAllocatedResources)
			}
			allocatedResources[containerIndex] = containerAllocatedResources
		}
		p.AllocatedResources = allocatedResources
		return nil
	}
	v2 := NoneIDResourceAllocation{}
	err = json.Unmarshal([]byte(allocation), &v2)
	if err != nil {
		return fmt.Errorf("failed to parse allocation from pod(%v/%v) annotation: %v", p.PodNamespace, p.PodName, err)
	}
	deviceResourceCount := map[types.DeviceId]int{}
	allocatedResources := map[types.ContainerIndex]ContainerAllocatedResources{}
	for containerIndex, containerResources := range v2 {
		containerAllocatedResources := map[types.DeviceId]Resources{}
		for deviceId, deviceAllocatedResources := range containerResources {
			items := strings.Split(deviceAllocatedResources, "/")
			allocated, err := strconv.Atoi(items[0])
			if err != nil {
				klog.Errorf("failed to parse allocation (allocated resources) from pod(%v/%v) annotation: %v", err)
				continue
			}
			if len(items) > 1 {
				total, err := strconv.Atoi(items[1])
				if err != nil {
					klog.Errorf("failed to parse allocation (total resources) from pod(%v/%v) annotation: %v", err)
				} else {
					deviceResourceCount[deviceId] = total
				}
			}
			containerAllocatedResources[deviceId] = NewNoneIDResources(allocated)
		}
		allocatedResources[containerIndex] = containerAllocatedResources
	}
	p.AllocatedResources = allocatedResources
	return nil
}

type PodResource struct {
	PodUID              apitypes.UID
	PodName             string
	PodNamespace        string
	DeviceResourceCount map[types.DeviceId]int
	AllocatedResources  map[types.ContainerIndex]ContainerAllocatedResources
}

func (p *PodResource) Clone() *PodResource {
	deviceResourceCount := map[types.DeviceId]int{}
	for devId, count := range p.DeviceResourceCount {
		deviceResourceCount[devId] = count
	}
	allContainersAllocatedResources := map[types.ContainerIndex]ContainerAllocatedResources{}

	for containerIndex, allocatedResources := range p.AllocatedResources {
		containerAllocatedResources := ContainerAllocatedResources{}
		for devId, resources := range allocatedResources {
			containerAllocatedResources[devId] = resources.Clone()
		}
		allContainersAllocatedResources[containerIndex] = containerAllocatedResources
	}
	return &PodResource{
		PodUID:              p.PodUID,
		PodName:             p.PodName,
		PodNamespace:        p.PodNamespace,
		DeviceResourceCount: deviceResourceCount,
		AllocatedResources:  allContainersAllocatedResources,
	}
}

func (p *PodResource) GetResourceType() ResourceType {
	resourceType := UnknownResource
	for _, containerAllocatedResources := range p.AllocatedResources {
		resourceType = GetContainerAllocatedResourceType(containerAllocatedResources)
		break
	}
	return resourceType
}

func (p *PodResource) DeviceCountToString() string {
	d, err := json.Marshal(&p.DeviceResourceCount)
	if err != nil {
		klog.Errorf("failed to marshal device resource count: %v", err)
		return ""
	}
	return string(d)
}

func (p *PodResource) AllocatedResourcesToString() string {
	var result string
	resourceType := p.GetResourceType()
	if resourceType == CountableResource {
		allocation := map[types.ContainerIndex]map[types.DeviceId][]string{}
		for containerIndex, containerResources := range p.AllocatedResources {
			allocation[containerIndex] = GetContainerCountableResourceIds(containerResources)
		}
		d, err := json.Marshal(allocation)
		if err != nil {
			klog.Errorf("failed to marshal pod(%v/%v) allocation: %v", p.PodNamespace, p.PodName, err)
			return ""
		}
		result = string(d)
	} else if resourceType == NoneIDResource {
		allocation := map[types.ContainerIndex]map[types.DeviceId]int{}
		for containerIndex, containerResources := range p.AllocatedResources {
			containerAllocation := map[types.DeviceId]int{}
			for devId, resources := range containerResources {
				// format: allocated/total
				containerAllocation[devId] = resources.Size()
			}
			allocation[containerIndex] = containerAllocation
		}
		d, err := json.Marshal(allocation)
		if err != nil {
			klog.Errorf("failed to marshal pod(%v/%v) allocation: %v", p.PodNamespace, p.PodName, err)
			return ""
		}
		result = string(d)
	}
	return result
}
