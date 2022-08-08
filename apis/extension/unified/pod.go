/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package unified

const (
	AnnotationAllocSpec           = "pod.beta1.sigma.ali/alloc-spec"
	AnnotationPodRequestAllocSpec = "pod.beta1.sigma.ali/request-alloc-spec"
)

// AllocSpec contains specification of the desired allocation behavior of the pod.
// More info: https://lark.alipay.com/sigma.pouch/sigma3.x/sghayi
type AllocSpec struct {
	// List of containers belonging to the pod.
	// +optional
	Containers []Container `json:"containers,omitempty"`
}

// A single application container that you want to run within a pod.
type Container struct {
	// Name of the container.
	// Must corresponds to one container in pod spec containers fields
	Name string `json:"name,omitempty"`
	// Extra attributes of resources required by this container.
	Resource ResourceRequirements `json:"resource,omitempty"`
}

// ResourceRequirements describes extra attributes of the compute resource requirements.
type ResourceRequirements struct {
	// If specified, extra cpu attributes such as cpuset modes
	// +optional
	CPU CPUSpec `json:"cpu,omitempty"`
}

// CPUSpec contains the extra attributes of CPU resource
type CPUSpec struct {
	// BindStrategy indicate kubelet how to bind cpus
	// If BindStrategy is "", that means using default binding logic as usual
	BindingStrategy CPUBindingStrategy `json:"bindingStrategy,omitempty"`
	// If specified, cpu resource of container is allocated as cpusets, and the CPUSet fields contains information
	// about CPUSet. The cpu resource request and limit value must be integer.
	// If not specified, cpu resource of container is allocated as cpu shares.
	// +optional
	CPUSet *CPUSetSpec `json:"cpuSet,omitempty"`
}

type CPUBindingStrategy string

const (
	// CPUBindStrategyDefault is a BindingStrategy that indicates kubelet binds this container according to CPUIDs
	CPUBindStrategyDefault CPUBindingStrategy = ""
	// CPUBindStrategyAllCPUs is a BindingStrategy that indicates kubelet binds this container to all cpus
	CPUBindStrategyAllCPUs CPUBindingStrategy = "BindAllCPUs"
)

// CPUSetSpec contains extra attributes of cpuset allocation
type CPUSetSpec struct {
	// If specified, cpuset allocation strategy
	// +optional
	SpreadStrategy SpreadStrategy `json:"spreadStrategy,omitempty"`
	// the logic cpu IDs allocated to the container
	// if not specified, scheduler wil fill with allocated cpu IDs.
	// +optional
	CPUIDs []int `json:"cpuIDs,omitempty"`
}

// SpreadStrategy means how to allocate cpuset of container in the CPU topology
type SpreadStrategy string

const (
	// SpreadStrategySpread is the default strategy that favor cpuset allocation that spread
	// across physical cores
	SpreadStrategySpread SpreadStrategy = "spread"
	// SpreadStrategyMustSpread is the strategy that favor cpuset allocation that spread
	// across physical cores and give priority to cross socket
	SpreadStrategyMustSpread SpreadStrategy = "mustSpread"
	// SpreadStrategySameCoreFirst is the strategy that favor cpuset allocation that pack
	// in few physical cores
	SpreadStrategySameCoreFirst SpreadStrategy = "sameCoreFirst"
)
