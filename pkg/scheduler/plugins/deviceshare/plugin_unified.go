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

package deviceshare

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func (g *Plugin) appendAckAnnotations(pod *corev1.Pod, allocResult apiext.DeviceAllocations, nodeName string) {
	ack.AppendAckAnnotations(pod, allocResult)
	if isVirtualGPUCard(allocResult) {
		allocMinor := allocResult[schedulingv1alpha1.GPU][0].Minor
		gpuMemory := g.nodeDeviceCache.getNodeDevice(nodeName).deviceTotal[schedulingv1alpha1.GPU][int(allocMinor)][apiext.GPUMemory]
		pod.Annotations[ack.AnnotationAliyunEnvMemDev] = fmt.Sprintf("%v", gpuMemory.Value()/1024/1024/1024)
		gpuMemoryPod := allocResult[schedulingv1alpha1.GPU][0].Resources[apiext.GPUMemory]
		pod.Annotations[ack.AnnotationAliyunEnvMemPod] = fmt.Sprintf("%v", gpuMemoryPod.Value()/1024/1024/1024)
	}
}

func isVirtualGPUCard(alloc apiext.DeviceAllocations) bool {
	for deviceType, deviceAllocations := range alloc {
		if deviceType != schedulingv1alpha1.GPU {
			continue
		}
		for _, deviceAlloc := range deviceAllocations {
			if deviceAlloc.Resources.Name(apiext.GPUCore, apiresource.DecimalSI).Value() < 100 {
				return true
			}
		}
	}
	return false
}
