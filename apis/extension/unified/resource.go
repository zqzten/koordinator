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
	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/util/cpuset"
)

// GetResourceSpec parses ResourceSpec from annotations; first koordinator protocols, second unified, third asi-sigma
func GetResourceSpec(annotations map[string]string) (*extension.ResourceSpec, error) {
	// koordinator protocols
	if _, ok := annotations[extension.AnnotationResourceSpec]; ok {
		return extension.GetResourceSpec(annotations)
	}
	// unified protocols
	if _, ok := annotations[uniext.AnnotationAllocSpec]; ok {
		return getResourceSpecByUnified(annotations)
	}
	// asi protocols
	if resourceSpec, err := getResourceSpecByASI(annotations); err == nil && resourceSpec != nil {
		return resourceSpec, nil
	}
	return &extension.ResourceSpec{PreferredCPUBindPolicy: schedulingconfig.CPUBindPolicyDefault}, nil
}

func getResourceSpecByUnified(annotations map[string]string) (*extension.ResourceSpec, error) {
	allocSpecOfUnified, err := uniext.GetAllocSpec(annotations)
	if err != nil {
		return nil, err
	}
	resourceSpec := &extension.ResourceSpec{
		PreferredCPUBindPolicy: schedulingconfig.CPUBindPolicyDefault,
	}
	switch allocSpecOfUnified.CPU {
	case uniext.CPUBindStrategySpread:
		resourceSpec.PreferredCPUBindPolicy = schedulingconfig.CPUBindPolicySpreadByPCPUs
	case uniext.CPUBindStrategySameCoreFirst:
		resourceSpec.PreferredCPUBindPolicy = schedulingconfig.CPUBindPolicyFullPCPUs
	}
	return resourceSpec, nil
}

func getResourceSpecByASI(annotations map[string]string) (*extension.ResourceSpec, error) {
	allocSpecOfASI, err := GetASIAllocSpec(annotations)
	if err != nil || allocSpecOfASI == nil {
		return nil, err
	}
	resourceSpec := &extension.ResourceSpec{
		PreferredCPUBindPolicy: schedulingconfig.CPUBindPolicyDefault,
	}
	for _, container := range allocSpecOfASI.Containers {
		// CPU Set
		if container.Resource.CPU.CPUSet != nil {
			resourceSpec.PreferredCPUBindPolicy = schedulingconfig.CPUBindPolicySpreadByPCPUs
			if container.Resource.CPU.CPUSet.SpreadStrategy == SpreadStrategySameCoreFirst {
				resourceSpec.PreferredCPUBindPolicy = schedulingconfig.CPUBindPolicyFullPCPUs
			}
			break
		}
	}
	return resourceSpec, nil
}

// GetResourceStatus parses ResourceStatus from annotations
func GetResourceStatus(annotations map[string]string) (*extension.ResourceStatus, error) {
	resourceStatus := &extension.ResourceStatus{
		CPUSet: "",
	}
	// koord
	if _, ok := annotations[extension.AnnotationResourceStatus]; ok {
		return extension.GetResourceStatus(annotations)
	}
	// unified
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

func SetResourceStatus(pod *corev1.Pod, status *extension.ResourceStatus) error {
	err := extension.SetResourceStatus(pod, status)
	if err != nil {
		return err
	}

	cpuset, err := cpuset.Parse(status.CPUSet)
	if err != nil {
		return err
	}
	// unified-scheduler AnnotationAllocStatus
	unifiedAllocState := &uniext.ResourceAllocState{
		CPU: cpuset.ToSlice(),
	}
	unifiedAllocStateData, err := json.Marshal(unifiedAllocState)
	if err != nil {
		return err
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[uniext.AnnotationAllocStatus] = string(unifiedAllocStateData)
	// ASI  AnnotationAllocSpec
	asiAllocSpec, err := GetASIAllocSpec(pod.Annotations)
	if err != nil {
		return err
	}
	if asiAllocSpec == nil {
		return nil
	}
	spec, err := GetResourceSpec(pod.Annotations)
	if err != nil {
		return nil
	}
	existsContainerNames := sets.NewString()
	for i := range asiAllocSpec.Containers {
		existsContainerNames.Insert(asiAllocSpec.Containers[i].Name)
		if asiAllocSpec.Containers[i].Resource.CPU.CPUSet == nil {
			asiAllocSpec.Containers[i].Resource.CPU.CPUSet = &CPUSetSpec{}
		}
		asiAllocSpec.Containers[i].Resource.CPU.CPUSet.SpreadStrategy = koordCPUStrategyToASI(spec.PreferredCPUBindPolicy)
		asiAllocSpec.Containers[i].Resource.CPU.CPUSet.CPUIDs = cpuset.ToSlice()
	}
	for i := range pod.Spec.Containers {
		if !existsContainerNames.Has(pod.Spec.Containers[i].Name) {
			allocSpecContainer := Container{
				Name: pod.Spec.Containers[i].Name,
				Resource: ResourceRequirements{
					CPU: CPUSpec{
						CPUSet: &CPUSetSpec{
							SpreadStrategy: koordCPUStrategyToASI(spec.PreferredCPUBindPolicy),
							CPUIDs:         cpuset.ToSlice(),
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

func koordCPUStrategyToASI(policy schedulingconfig.CPUBindPolicy) SpreadStrategy {
	switch policy {
	case schedulingconfig.CPUBindPolicySpreadByPCPUs:
		return SpreadStrategySpread
	case schedulingconfig.CPUBindPolicyFullPCPUs:
		return SpreadStrategySameCoreFirst
	default:
		return SpreadStrategyMustSpread
	}
}
