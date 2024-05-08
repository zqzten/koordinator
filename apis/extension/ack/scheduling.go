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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
	k8sresource "k8s.io/kubernetes/pkg/api/v1/resource"

	"github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

var NowFn = time.Now

func AppendAckAnnotationsIfHasGPUCompute(pod *corev1.Pod, device *schedulingv1alpha1.Device, allocations extension.DeviceAllocations) {
	requests := k8sresource.PodRequests(pod, k8sresource.PodResourcesOptions{})
	if quantity := requests[ResourceAliyunGPUCompute]; quantity.IsZero() {
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
			if deviceAlloc.Resources.Name(extension.ResourceGPUMemoryRatio, resource.DecimalSI).Value() < 100 {
				hasVirtualGPUCard = true
				gpuComputePod = fmt.Sprintf("%v", deviceAlloc.Resources.Name(extension.ResourceGPUMemoryRatio, resource.DecimalSI).Value())
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

		allocMinor := allocations[schedulingv1alpha1.GPU][0].Minor
		var totalMemory resource.Quantity
		for _, v := range device.Spec.Devices {
			if v.Type == schedulingv1alpha1.GPU && v.Minor != nil && *v.Minor == allocMinor {
				totalMemory = v.Resources[extension.ResourceGPUMemory]
				break
			}
		}
		pod.Annotations[AnnotationAliyunEnvMemDev] = fmt.Sprintf("%v", totalMemory.Value()/1024/1024/1024)
		gpuMemoryPod := allocations[schedulingv1alpha1.GPU][0].Resources[extension.ResourceGPUMemory]
		pod.Annotations[AnnotationAliyunEnvMemPod] = fmt.Sprintf("%v", gpuMemoryPod.Value()/1024/1024/1024)
	} else {
		pod.Annotations[AnnotationAliyunEnvAssignedFlag] = "false"
	}
}

func AppendAckAnnotationsIfHasGPUMemory(pod *corev1.Pod, allocations extension.DeviceAllocations) error {
	containerIndex := getContainerIndexWithGPUResource(pod, ResourceAliyunGPUMemory)
	if containerIndex < 0 {
		return nil
	}
	m := map[string]int64{}
	for deviceType, deviceAllocations := range allocations {
		if deviceType != schedulingv1alpha1.GPU {
			continue
		}
		for _, deviceAlloc := range deviceAllocations {
			minor := strconv.Itoa(int(deviceAlloc.Minor))
			quantity := deviceAlloc.Resources[extension.ResourceGPUMemory]
			m[minor] += quantity.Value()
		}
	}
	if len(m) == 0 {
		return nil
	}
	for minor, quantity := range m {
		m[minor] = quantity / 1024 / 1024 / 1024
	}
	ackResult := map[string]map[string]int64{
		strconv.Itoa(containerIndex): m,
	}
	data, err := json.Marshal(ackResult)
	if err != nil {
		return err
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[AnnotationACKGPUShareAllocation] = string(data)
	pod.Annotations[AnnotationACKGPUShareAssumeTime] = fmt.Sprintf("%d", NowFn().UnixNano())
	pod.Annotations[AnnotationACKGPUShareAssigned] = "false" // ACK DP will update the annotation to true
	return nil
}

func AppendAckAnnotationsIfHasGPUCore(pod *corev1.Pod, allocations extension.DeviceAllocations) error {
	containerIndex := getContainerIndexWithGPUResource(pod, ResourceALiyunGPUCorePercentage)
	if containerIndex < 0 {
		return nil
	}
	m := map[string]int64{}
	for deviceType, deviceAllocations := range allocations {
		if deviceType != schedulingv1alpha1.GPU {
			continue
		}
		for _, deviceAlloc := range deviceAllocations {
			minor := strconv.Itoa(int(deviceAlloc.Minor))
			quantity := deviceAlloc.Resources[extension.ResourceGPUCore]
			m[minor] += quantity.Value()
		}
	}
	if len(m) == 0 {
		return nil
	}
	for minor, quantity := range m {
		m[minor] = quantity
	}
	ackResult := map[string]map[string]int64{
		strconv.Itoa(containerIndex): m,
	}
	data, err := json.Marshal(ackResult)
	if err != nil {
		return err
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[AnnotationACKGPUCoreAllocation] = string(data)
	pod.Annotations[AnnotationACKGPUShareAssumeTime] = fmt.Sprintf("%d", NowFn().UnixNano())
	pod.Annotations[AnnotationACKGPUShareAssigned] = "false" // ACK DP will update the annotation to true
	return nil
}

func getContainerIndexWithGPUResource(pod *corev1.Pod, resourceName corev1.ResourceName) int {
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if quantity := container.Resources.Requests[resourceName]; !quantity.IsZero() {
			return i
		}
	}
	return -1
}

func GetPodResourceFromV1(pod *corev1.Pod) map[int]map[string]int64 {
	value, ok := pod.Annotations[AnnotationAliyunEnvMemPod]
	if !ok || value == "" {
		return nil
	}
	allocatedGPUMemory, err := strconv.Atoi(value)
	if err != nil {
		klog.Infof("failed to parse allocated gpu memory from pod annotation %v,reason: %v", AnnotationAliyunEnvMemPod, err)
		return nil
	}
	deviceIndex, ok := pod.Annotations[AnnotationAliyunEnvResourceIndex]
	if !ok || deviceIndex == "" {
		return nil
	}
	return map[int]map[string]int64{
		0: {
			deviceIndex: int64(allocatedGPUMemory),
		},
	}
}

func GetPodResourceFromV2(pod *corev1.Pod, annotationKey string) map[int]map[string]int64 {
	value := pod.Annotations[annotationKey]
	if value != "" {
		data := map[int]map[string]int64{}
		err := json.Unmarshal([]byte(value), &data)
		if err != nil {
			klog.Errorf("failed to parse allocation from annotation: %v", err)
		} else {
			return data
		}
	}
	return nil
}
