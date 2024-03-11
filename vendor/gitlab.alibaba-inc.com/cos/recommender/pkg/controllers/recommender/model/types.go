package model

import (
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog"
)

// ResourceName represents the name of the resource monitored by recommender.
type ResourceName string

// ResourceAmount represents quantity of a certain resource within a container.
// Note this keeps CPU in millicores (which is not a standard unit in APIs)
// and memory in bytes.
// Allowed values are in the range from 0 to MaxResourceAmount.
type ResourceAmount int64

// Resources is a map from resource name to the corresponding ResourceAmount.
type Resources map[ResourceName]ResourceAmount

const (
	// ResourceCPU represents CPU in millicores(1core = 1000 millicores).
	ResourceCPU ResourceName = "cpu"
	// ResourceMemory represents memory, in bytes.(500Gi = 500GiB = 500 * 1024 * 1024)
	ResourceMemory ResourceName = "memory"
	// MaxResourceAmount is the maximum allowed value of resource amount.
	MaxResourceAmount = ResourceAmount(1e14)
)

// CPUAmountFromCores converts CPU cores to a ResourceAmount.
func CPUAmountFromCores(cores float64) ResourceAmount {
	return resourceAmountFromFloat(cores * 1000)
}

// CoresFromCPUAmount converts ResourceAmount to number of cores expressed as float64.
func CoresFromCPUAmount(cpuAmount ResourceAmount) float64 {
	return float64(cpuAmount) / 1000.0
}

// QuantityFromCPUAmount converts CPU ResourceAmount to a resource.Quantity.
func QuantityFromCPUAmount(cpuAmount ResourceAmount) resource.Quantity {
	return *resource.NewScaledQuantity(int64(cpuAmount), -3)
}

// MemoryAmountFromBytes converts memory bytes to a ResourceAmount.
func MemoryAmountFromBytes(bytes float64) ResourceAmount {
	return resourceAmountFromFloat(bytes)
}

// BytesFromMemoryAmount converts ResourceAmount to number of bytes expressed as float64.
func BytesFromMemoryAmount(memoryAmount ResourceAmount) float64 {
	return float64(memoryAmount)
}

// ResourceAmountMax returns the larger of two resource amounts.
func ResourceAmountMax(amount1, amount2 ResourceAmount) ResourceAmount {
	if amount1 > amount2 {
		return amount1
	}
	return amount2
}

// QuantityFromMemoryAmount converts memory ResourceAmount to a resource.Quantity.
func QuantityFromMemoryAmount(memoryAmount ResourceAmount) resource.Quantity {
	return *resource.NewScaledQuantity(int64(memoryAmount), 0)
}

// ScaleResource returns the resource amount multiplied by a given factor.
func ScaleResource(amount ResourceAmount, factor float64) ResourceAmount {
	return resourceAmountFromFloat(float64(amount) * factor)
}

// ResourceAsResourceList converts internal Resources representation to ResourcesList.
func ResourceAsResourceList(resources Resources) apiv1.ResourceList {
	result := make(apiv1.ResourceList)
	for key, resourceAmount := range resources {
		var newKey apiv1.ResourceName
		var quantity resource.Quantity
		switch key {
		case ResourceCPU:
			newKey = apiv1.ResourceCPU
			quantity = QuantityFromCPUAmount(resourceAmount)
		case ResourceMemory:
			newKey = apiv1.ResourceMemory
			quantity = QuantityFromMemoryAmount(resourceAmount)
		default:
			klog.Errorf("Cannot translate %v resource name", key)
			continue
		}
		result[newKey] = quantity
	}
	return result
}

func ResourceNamesApiToModel(resources []apiv1.ResourceName) *[]ResourceName {
	result := make([]ResourceName, 0, len(resources))
	for _, resource := range resources {
		switch resource {
		case apiv1.ResourceCPU:
			result = append(result, ResourceCPU)
		case apiv1.ResourceMemory:
			result = append(result, ResourceMemory)
		default:
			klog.Errorf("Cannot translate %v resource name", resource)
			continue
		}
	}
	return &result
}

func resourceAmountFromFloat(amount float64) ResourceAmount {
	if amount < 0 {
		return ResourceAmount(0)
	} else if amount > float64(MaxResourceAmount) {
		return MaxResourceAmount
	} else {
		return ResourceAmount(amount)
	}
}

// PodID contains information needs to identity a Pod within a cluster.
type PodID struct {
	// Namespaces where the Pod is define.
	Namespace string
	// PodName is the name of the pod unique within a namespace.
	PodName string
}

// ContainerID containers information needed to identify a Container within a cluster.
type ContainerID struct {
	PodID
	// ContainerName is the name of the container,unique within a pod.
	ContainerName string
}

// RecommendationID contains information needed to identify a Recommendation API object within a cluster.
type RecommendationID struct {
	Namespace          string
	RecommendationName string
}

type WorkloadInfo struct {
	ContainerRequests map[string]apiv1.ResourceList
}

func (w *WorkloadInfo) GetRequestResources() *apiv1.ResourceList {
	totalRequests := apiv1.ResourceList{}
	for _, containerReq := range w.ContainerRequests {
		totalRequests = quotav1.Add(totalRequests, containerReq)
	}
	return &totalRequests
}
