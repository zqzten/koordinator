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

package transformer

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/features"
)

func init() {
	if k8sfeature.DefaultFeatureGate.Enabled(features.EnableACKGPUMemoryScheduling) {
		podTransformers = append(podTransformers,
			TransformACKDeviceAllocation,
		)
	}
}

func TransformACKDeviceAllocation(pod *corev1.Pod) {
	if val := pod.Annotations[extension.AnnotationDeviceAllocated]; val != "" {
		return
	}

	gpuMemoryAllocation := ack.GetPodResourceFromV2(pod, ack.AnnotationACKGPUShareAllocation)
	if gpuMemoryAllocation == nil {
		gpuMemoryAllocation = ack.GetPodResourceFromV1(pod)
	}

	gpuCoreAllocation := ack.GetPodResourceFromV2(pod, ack.AnnotationACKGPUCoreAllocation)
	if gpuMemoryAllocation == nil && gpuCoreAllocation == nil {
		return
	}

	m := map[string]map[corev1.ResourceName]int64{}
	fn := func(allocation map[int]map[string]int64, resourceName corev1.ResourceName) {
		for _, deviceResources := range allocation {
			for deviceIndex, allocatedRes := range deviceResources {
				allocated := m[deviceIndex]
				if allocated == nil {
					allocated = make(map[corev1.ResourceName]int64)
					m[deviceIndex] = allocated
				}
				if resourceName == extension.ResourceGPUMemory {
					allocated[extension.ResourceGPUMemory] += allocatedRes * 1024 * 1024 * 1024
				} else {
					allocated[resourceName] += allocatedRes
				}
			}
		}
	}
	fn(gpuMemoryAllocation, extension.ResourceGPUMemory)
	fn(gpuCoreAllocation, extension.ResourceGPUCore)

	var allocations []*extension.DeviceAllocation
	for deviceIndex, allocated := range m {
		minor, err := strconv.Atoi(deviceIndex)
		if err != nil {
			klog.ErrorS(err, "Failed to convert ACK Device allocation", "pod", klog.KObj(pod))
			return
		}
		resourceList := corev1.ResourceList{}
		for k, v := range allocated {
			if k == extension.ResourceGPUMemory {
				resourceList[k] = *resource.NewQuantity(v, resource.BinarySI)
			} else {
				resourceList[k] = *resource.NewQuantity(v, resource.DecimalSI)
			}
		}
		if quotav1.IsZero(resourceList) {
			continue
		}
		allocations = append(allocations, &extension.DeviceAllocation{
			Minor:     int32(minor),
			Resources: resourceList,
		})
	}
	err := extension.SetDeviceAllocations(pod, extension.DeviceAllocations{
		schedulingv1alpha1.GPU: allocations,
	})
	if err != nil {
		klog.ErrorS(err, "Failed to SetDeviceAllocations from ACK result", "pod", klog.KObj(pod))
	}
}
