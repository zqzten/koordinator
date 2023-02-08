package noderesource

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"sync"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

type NodeResourceCache struct {
	resourceName          corev1.ResourceName
	allocatedPodResources map[apitypes.UID]*PodResource
	deviceResources       map[types.DeviceId]*Device
	lock                  *sync.RWMutex
}

func NewNodeResourceCache(resourceName corev1.ResourceName) *NodeResourceCache {
	return &NodeResourceCache{
		resourceName:          resourceName,
		allocatedPodResources: map[apitypes.UID]*PodResource{},
		deviceResources:       map[types.DeviceId]*Device{},
		lock:                  new(sync.RWMutex),
	}
}

func (nr *NodeResourceCache) Clone() *NodeResourceCache {
	nr.lock.RLock()
	defer nr.lock.RUnlock()
	c := &NodeResourceCache{
		resourceName:          nr.resourceName,
		allocatedPodResources: map[apitypes.UID]*PodResource{},
		deviceResources:       map[types.DeviceId]*Device{},
		lock:                  new(sync.RWMutex),
	}
	for uid, pr := range nr.allocatedPodResources {
		c.allocatedPodResources[uid] = pr.Clone()
	}
	for devId, dev := range nr.deviceResources {
		c.deviceResources[devId] = dev.Clone()
	}
	return c
}

func (nr *NodeResourceCache) GetResourceName() corev1.ResourceName {
	return nr.resourceName
}

func (nr *NodeResourceCache) AddAllocatedPodResources(podResources ...PodResource) {
	nr.lock.Lock()
	defer nr.lock.Unlock()
	for _, podResource := range podResources {
		p := podResource
		nr.completePodResource(&p)
		nr.allocatedPodResources[p.PodUID] = &p
	}

}

func (nr *NodeResourceCache) RemoveAllocatedPodResources(podUID apitypes.UID) {
	nr.lock.Lock()
	defer nr.lock.Unlock()
	delete(nr.allocatedPodResources, podUID)
}

func (nr *NodeResourceCache) GetAllocatedPodResource(podUID apitypes.UID) *PodResource {
	nr.lock.RLock()
	defer nr.lock.RUnlock()
	p, ok := nr.allocatedPodResources[podUID]
	if !ok {
		return nil
	}
	return p.Clone()
}

func (nr *NodeResourceCache) PodResourceIsCached(podUID apitypes.UID) bool {
	nr.lock.RLock()
	defer nr.lock.RUnlock()
	_, ok := nr.allocatedPodResources[podUID]
	return ok
}

func (nr *NodeResourceCache) DeviceSize() int {
	nr.lock.RLock()
	defer nr.lock.RUnlock()
	return len(nr.deviceResources)
}

func (nr *NodeResourceCache) DeviceIsExisted(devId types.DeviceId) bool {
	nr.lock.RLock()
	defer nr.lock.RUnlock()
	_, ok := nr.deviceResources[devId]
	return ok
}

func (nr *NodeResourceCache) AddDevices(devices ...*Device) {
	nr.lock.Lock()
	defer nr.lock.Unlock()
	for _, dev := range devices {
		nr.deviceResources[dev.GetId()] = dev
	}
	for _, p := range nr.allocatedPodResources {
		nr.completePodResource(p)
	}
}

func (nr *NodeResourceCache) ResetDevices(devices ...*Device) {
	nr.lock.Lock()
	defer nr.lock.Unlock()
	nr.deviceResources = map[types.DeviceId]*Device{}
	for _, dev := range devices {
		nr.deviceResources[dev.GetId()] = dev
	}
	for _, p := range nr.allocatedPodResources {
		nr.completePodResource(p)
	}
}

func (nr *NodeResourceCache) GetAllDevices() map[types.DeviceId]*Device {
	nr.lock.RLock()
	defer nr.lock.RUnlock()
	devices := map[types.DeviceId]*Device{}
	for _, dev := range nr.deviceResources {
		devices[dev.GetId()] = dev.Clone()
	}
	nr.completeAllocatedResources(devices)
	return devices
}

func (nr *NodeResourceCache) CheckAndCleanInvaildPods(lister v1.PodLister) {
	nr.lock.Lock()
	defer nr.lock.Unlock()
	for _, podResource := range nr.allocatedPodResources {
		_, err := lister.Pods(podResource.PodNamespace).Get(podResource.PodName)
		if k8serrors.IsNotFound(err) {
			klog.Warningf("%v/%v is not found in client-go cache,we will remove it from node(%v) resource cache", podResource.PodNamespace, podResource.PodName)
			delete(nr.allocatedPodResources, podResource.PodUID)
		}
	}
}

func TrunFloat(f float64, prec int) float64 {
	x := math.Pow10(prec)
	return math.Trunc(f*x) / x
}

func (nr *NodeResourceCache) completeAllocatedResources(devices map[types.DeviceId]*Device) {
	result := map[types.DeviceId]map[string]float64{}
	allocatedResources := map[types.DeviceId]Resources{}
	total := map[types.DeviceId]int{}
	resourceType := NoneIDResource
	// add all devices allocated resources
	for _, p := range nr.allocatedPodResources {
		podFullName := fmt.Sprintf("%v/%v", p.PodNamespace, p.PodName)
		for _, containerAllocatedResources := range p.AllocatedResources {
			for devId, resources := range containerAllocatedResources {
				// if not found device resource count,skip this resources
				t := p.DeviceResourceCount[devId]
				if t == 0 {
					klog.Warningf("not found device id %v in pod(%v/%v) resource deviceResourceCount", devId, p.PodNamespace, p.PodName)
					continue
				}
				// get the total resources of device
				total[devId] = t
				// if not found device in result,init it
				if result[devId] == nil {
					result[devId] = map[string]float64{}
				}
				if resources.Type() == CountableResource {
					resourceType = CountableResource
					// summary all allocated resources for a device
					// allocatedResources[devId] may be is nil
					allocatedResources[devId] = resources.Add(allocatedResources[devId])
				}
				v := float64(resources.Size()) / float64(t)
				result[devId][podFullName] += v
			}
		}
	}

	if resourceType != CountableResource {
		for devId, podAllocatedRatios := range result {
			total := float64(0)
			for _, ratio := range podAllocatedRatios {
				total += ratio
			}
			dev := nr.deviceResources[devId]
			if dev == nil {
				klog.Warningf("not found device %v in current device set,may node device resources are not added", devId)
				continue
			}
			totalAllocated := int(math.Ceil(float64(dev.GetTotal().Size()) * TrunFloat(total, 10)))
			allocatedResources[devId] = NewNoneIDResources(totalAllocated)
		}

	}
	// complete all devices' allocated resources
	for devId, resources := range allocatedResources {
		if devices[devId] == nil {
			continue
		}
		devices[devId].setAllocatedResources(resources)
	}
	for deviceId, pods := range result {
		newPods := map[string]float64{}
		for podName, count := range pods {
			count, _ = strconv.ParseFloat(fmt.Sprintf("%.3f", count), 64)
			newPods[podName] = count
		}
		devices[deviceId].addPods(newPods)
	}
}

func (nr *NodeResourceCache) String() string {
	// no need to add lock,GetAllDevices has been added
	devices := nr.GetAllDevices()
	deviceInfos := []DeviceDetail{}
	for _, dev := range devices {
		deviceInfos = append(deviceInfos, DeviceDetail{
			Id:         dev.GetId(),
			Total:      dev.total.String(),
			Unhealthy:  dev.unhealthy.String(),
			Allocated:  dev.allocated.String(),
			PodDetails: dev.GetPods(),
		})
	}
	data, _ := json.Marshal(&deviceInfos)
	return string(data)
}

// make sure every pod resource has field DeviceResourceCount
func (nr *NodeResourceCache) completePodResource(p *PodResource) {
	if len(p.DeviceResourceCount) != 0 {
		return
	}
	total := map[types.DeviceId]int{}
	for _, containerAllocatedResources := range p.AllocatedResources {
		resourceType := GetContainerAllocatedResourceType(containerAllocatedResources)
		if resourceType == CountableResource {
			for devId := range GetContainerCountableResourceIds(containerAllocatedResources) {
				dev, ok := nr.deviceResources[devId]
				if !ok {
					klog.Errorf("found device id %v in pod resources(%v/%v),but not found it in current device set", devId, p.PodNamespace, p.PodName)
					continue
				}
				total[devId] = dev.GetTotal().Size()
			}
		} else if resourceType == NoneIDResource {
			for devId := range GetContainerNoneIDResources(containerAllocatedResources) {
				dev, ok := nr.deviceResources[devId]
				if !ok {
					klog.Errorf("found device id %v in pod resources(%v/%v),but not found it in current device set", devId, p.PodNamespace, p.PodName)
					continue
				}
				total[devId] = dev.GetTotal().Size()
			}
		}
	}
	p.DeviceResourceCount = total
}
