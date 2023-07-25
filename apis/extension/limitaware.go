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

package extension

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	// AnnotationLimitToAllocatable The limit to allocatable used by limit aware scheduler plugin, represent by percentage
	AnnotationLimitToAllocatable = NodeDomainPrefix + "/limit-to-allocatable"
)

type LimitToAllocatable map[corev1.ResourceName]intstr.IntOrString

func (l LimitToAllocatable) GetLimitForResource(resourceName corev1.ResourceName, allocatable int64) (int64, error) {
	percent := 100
	if percentage, ok := l[resourceName]; ok {
		var err error
		percent, err = intstr.GetScaledValueFromIntOrPercent(&percentage, 100, false)
		if err != nil {
			return 0, err
		}
	}
	return int64(percent) * allocatable / 100, nil
}

func GetNodeLimitToAllocatable(annotations map[string]string, defaultLimitToAllocatable LimitToAllocatable) (LimitToAllocatable, error) {
	data, ok := annotations[AnnotationLimitToAllocatable]
	if !ok {
		return defaultLimitToAllocatable, nil
	}
	limitToAllocatable := LimitToAllocatable{}
	err := json.Unmarshal([]byte(data), &limitToAllocatable)
	if err != nil {
		return nil, err
	}
	return limitToAllocatable, nil
}

func GetNodeLimitAllocatable(nodeAllocatable *framework.Resource, limitToAllocatable LimitToAllocatable) (*framework.Resource, error) {
	milliCPU, err := limitToAllocatable.GetLimitForResource(corev1.ResourceCPU, nodeAllocatable.MilliCPU)
	if err != nil {
		return nil, err
	}
	memory, err := limitToAllocatable.GetLimitForResource(corev1.ResourceMemory, nodeAllocatable.Memory)
	if err != nil {
		return nil, err
	}
	ephemeralStorage, err := limitToAllocatable.GetLimitForResource(corev1.ResourceEphemeralStorage, nodeAllocatable.EphemeralStorage)
	if err != nil {
		return nil, err
	}
	nodeLimitCapacity := &framework.Resource{
		MilliCPU:         milliCPU,
		Memory:           memory,
		EphemeralStorage: ephemeralStorage,
	}
	for resourceName, quantity := range nodeAllocatable.ScalarResources {
		limit, err := limitToAllocatable.GetLimitForResource(resourceName, quantity)
		if err != nil {
			return nil, err
		}
		if nodeLimitCapacity.ScalarResources == nil {
			nodeLimitCapacity.ScalarResources = map[corev1.ResourceName]int64{}
		}
		nodeLimitCapacity.ScalarResources[resourceName] = limit
	}
	return nodeLimitCapacity, nil
}
