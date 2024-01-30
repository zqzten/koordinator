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

import (
	"encoding/json"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/util/cpuset"
)

const (
	ResourceAliyunMemberENI corev1.ResourceName = "aliyun/member-eni"
	ResourceSigmaENI        corev1.ResourceName = "sigma/eni"

	DisableCPUSetOversold corev1.ResourceName = "DisableCPUSetOversold"
)

// GetResourceSpec parses ResourceSpec from annotations; first koordinator protocols, second unified, third asi-sigma
func GetResourceSpec(annotations map[string]string) (*extension.ResourceSpec, string, error) {
	// koordinator protocols
	if _, ok := annotations[extension.AnnotationResourceSpec]; ok {
		spec, err := extension.GetResourceSpec(annotations)
		if err != nil {
			return nil, extension.AnnotationResourceSpec, err
		}
		return spec, extension.AnnotationResourceSpec, nil
	}
	// unified protocols
	if _, ok := annotations[uniext.AnnotationAllocSpec]; ok {
		spec, err := convertUnifiedResourceSpec(annotations)
		if err != nil {
			return nil, uniext.AnnotationAllocSpec, err
		}
		return spec, uniext.AnnotationAllocSpec, nil
	}
	// asi protocols
	if resourceSpec, err := convertASIAllocSpec(annotations); err == nil && resourceSpec != nil {
		return resourceSpec, AnnotationPodRequestAllocSpec, nil
	}
	return &extension.ResourceSpec{PreferredCPUBindPolicy: ""}, "", nil
}

func convertUnifiedResourceSpec(annotations map[string]string) (*extension.ResourceSpec, error) {
	unifiedResourceSpec, err := uniext.GetAllocSpec(annotations)
	if err != nil {
		return nil, err
	}
	resourceSpec := &extension.ResourceSpec{
		PreferredCPUBindPolicy: "",
	}
	switch unifiedResourceSpec.CPU {
	case uniext.CPUBindStrategySpread:
		resourceSpec.PreferredCPUBindPolicy = extension.CPUBindPolicySpreadByPCPUs
	case uniext.CPUBindStrategySameCoreFirst:
		resourceSpec.PreferredCPUBindPolicy = extension.CPUBindPolicyFullPCPUs
	case uniext.CPUBindStrategyMustSpread:
		resourceSpec.RequiredCPUBindPolicy = extension.CPUBindPolicySpreadByPCPUs
	}
	return resourceSpec, nil
}

func convertASIAllocSpec(annotations map[string]string) (*extension.ResourceSpec, error) {
	allocSpec, err := GetASIAllocSpec(annotations)
	if err != nil || allocSpec == nil {
		return nil, err
	}
	resourceSpec := &extension.ResourceSpec{
		PreferredCPUBindPolicy: "",
	}
	for _, container := range allocSpec.Containers {
		if container.Resource.CPU.CPUSet != nil {
			resourceSpec.PreferredCPUBindPolicy = extension.CPUBindPolicySpreadByPCPUs
			if container.Resource.CPU.CPUSet.SpreadStrategy == SpreadStrategySameCoreFirst {
				resourceSpec.PreferredCPUBindPolicy = extension.CPUBindPolicyFullPCPUs
			} else if container.Resource.CPU.CPUSet.SpreadStrategy == SpreadStrategyMustSpread {
				resourceSpec.RequiredCPUBindPolicy = extension.CPUBindPolicySpreadByPCPUs
			}
			break
		}
	}
	return resourceSpec, nil
}

// GetResourceStatus parses ResourceStatus from annotations
func GetResourceStatus(annotations map[string]string) (*extension.ResourceStatus, error) {
	if _, ok := annotations[extension.AnnotationResourceStatus]; ok {
		return extension.GetResourceStatus(annotations)
	}
	// unified
	resourceStatus := &extension.ResourceStatus{
		CPUSet: "",
	}
	if _, ok := annotations[uniext.AnnotationAllocStatus]; ok {
		if allocStatus, err := uniext.GetAllocStatus(annotations); err != nil {
			return nil, err
		} else {
			resourceStatus.CPUSet = cpuset.NewCPUSet(allocStatus.CPU...).String()
			return resourceStatus, nil
		}
	}
	// asi
	asiAllocSpec, err := GetASIAllocSpec(annotations)
	if err != nil {
		return nil, err
	}
	if asiAllocSpec == nil || len(asiAllocSpec.Containers) == 0 {
		return resourceStatus, nil
	}
	for _, container := range asiAllocSpec.Containers {
		if container.Resource.CPU.CPUSet != nil && len(container.Resource.CPU.CPUSet.CPUIDs) != 0 {
			resourceStatus.CPUSet = cpuset.NewCPUSet(container.Resource.CPU.CPUSet.CPUIDs...).String()
			return resourceStatus, nil
		}
	}
	return resourceStatus, nil
}

func SetUnifiedResourceStatusIfHasCPUs(pod *corev1.Pod, status *extension.ResourceStatus) error {
	cpus, err := cpuset.Parse(status.CPUSet)
	if err != nil {
		return err
	}
	if cpus.IsEmpty() {
		return nil
	}

	// unified-scheduler AnnotationAllocStatus
	unifiedAllocState := &uniext.ResourceAllocState{
		CPU: cpus.ToSlice(),
	}
	unifiedAllocStateData, err := json.Marshal(unifiedAllocState)
	if err != nil {
		return err
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[uniext.AnnotationAllocStatus] = string(unifiedAllocStateData)

	// ASI AnnotationAllocSpec
	asiAllocSpec, err := GetASIAllocSpec(pod.Annotations)
	if err != nil {
		return err
	}
	if asiAllocSpec == nil {
		return nil
	}
	spec, _, err := GetResourceSpec(pod.Annotations)
	if err != nil {
		return nil
	}
	existsContainerNames := sets.NewString()
	asiCPUBindPolicy := koordCPUStrategyToASI(spec)
	for i := range asiAllocSpec.Containers {
		existsContainerNames.Insert(asiAllocSpec.Containers[i].Name)
		if asiAllocSpec.Containers[i].Resource.CPU.CPUSet == nil {
			asiAllocSpec.Containers[i].Resource.CPU.CPUSet = &CPUSetSpec{}
		}
		asiAllocSpec.Containers[i].Resource.CPU.CPUSet.SpreadStrategy = asiCPUBindPolicy
		asiAllocSpec.Containers[i].Resource.CPU.CPUSet.CPUIDs = cpus.ToSlice()
	}
	for i := range pod.Spec.Containers {
		if !existsContainerNames.Has(pod.Spec.Containers[i].Name) {
			allocSpecContainer := Container{
				Name: pod.Spec.Containers[i].Name,
				Resource: ResourceRequirements{
					CPU: CPUSpec{
						CPUSet: &CPUSetSpec{
							SpreadStrategy: asiCPUBindPolicy,
							CPUIDs:         cpus.ToSlice(),
						},
					},
				},
			}
			asiAllocSpec.Containers = append(asiAllocSpec.Containers, allocSpecContainer)
		}
	}
	asiAllocSpecData, err := json.Marshal(asiAllocSpec)
	if err != nil {
		return err
	}
	pod.Annotations[AnnotationAllocSpec] = string(asiAllocSpecData)
	return nil
}

func koordCPUStrategyToASI(spec *extension.ResourceSpec) SpreadStrategy {
	policy := spec.RequiredCPUBindPolicy
	if policy == "" {
		policy = spec.PreferredCPUBindPolicy
	}
	if policy == extension.CPUBindPolicyFullPCPUs {
		return SpreadStrategySameCoreFirst
	}
	return SpreadStrategySpread
}
