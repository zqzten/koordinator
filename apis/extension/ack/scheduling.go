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

package ack

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	k8sresource "k8s.io/kubernetes/pkg/api/v1/resource"

	"github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func AppendAckAnnotations(pod *corev1.Pod, allocations extension.DeviceAllocations) {
	requests, _ := k8sresource.PodRequestsAndLimits(pod)
	if quantity := requests[AliyunGPUCompute]; quantity.IsZero() {
		return
	}
	hasVirtualGPUCard, hasGPUCard := false, false
	var deviceMinors []string
	var gpuComputePod string
	for deviceType, deviceAllocations := range allocations {
		if deviceType != schedulingv1alpha1.GPU {
			continue
		}
		hasGPUCard = true
		for _, deviceAlloc := range deviceAllocations {
			deviceMinors = append(deviceMinors, fmt.Sprintf("%v", deviceAlloc.Minor))
			if deviceAlloc.Resources.Name(extension.ResourceGPUCore, resource.DecimalSI).Value() < 100 {
				hasVirtualGPUCard = true
				gpuComputePod = fmt.Sprintf("%v", deviceAlloc.Resources.Name(extension.ResourceGPUCore, resource.DecimalSI).Value())
			}
		}
	}
	if !hasGPUCard {
		return
	}
	envResourceIndex := strings.Join(deviceMinors, ",")
	pod.Annotations[AnnotationAliyunEnvResourceIndex] = envResourceIndex
	pod.Annotations[AnnotationAliyunEnvResourceAssumeTime] = fmt.Sprintf("%v", time.Now().UnixNano())

	if hasVirtualGPUCard {
		pod.Annotations[AnnotationAliyunEnvComputeAssignedFlag] = "false"
		pod.Annotations[AnnotationAliyunEnvMemAssignedFlag] = "false"
		pod.Annotations[AnnotationAliyunEnvComputeDev] = "100"
		pod.Annotations[AnnotationAliyunEnvComputePod] = gpuComputePod
	} else {
		pod.Annotations[AnnotationAliyunEnvAssignedFlag] = "false"
	}
}
