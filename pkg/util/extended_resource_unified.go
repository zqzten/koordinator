//go:build !github
// +build !github

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

package util

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

// NOTE: functions in this file can be overwritten for extension

var ExtendedResourceNames = []corev1.ResourceName{
	extension.BatchCPU,
	extension.BatchMemory,
	corev1.ResourceCPU,
	corev1.ResourceMemory,
}

// GetBatchMilliCPUFromResourceList gets the batch milli-cpu request for Batch pods.
// NOTE: DO NOT use it for Prod pods.
// TODO: add a GetPodCPURequest for pods of all priorities.
func GetBatchMilliCPUFromResourceList(r corev1.ResourceList) int64 {
	// assert r != nil
	if milliCPU, ok := r[extension.BatchCPU]; ok {
		return milliCPU.Value()
	}
	if cpu, ok := r[corev1.ResourceCPU]; ok {
		return cpu.MilliValue()
	}
	return -1
}

// GetBatchMemoryFromResourceList gets the batch memory request for Batch pods.
// NOTE: DO NOT use it for Prod pods.
// TODO: add a GetPodMemoryRequest for pods of all priorities.
func GetBatchMemoryFromResourceList(r corev1.ResourceList) int64 {
	// assert r != nil
	if memory, ok := r[extension.BatchMemory]; ok {
		return memory.Value()
	}
	if memory, ok := r[corev1.ResourceMemory]; ok {
		return memory.Value()
	}
	return -1
}

func GetContainerBatchMilliCPURequest(c *corev1.Container) int64 {
	return GetBatchMilliCPUFromResourceList(c.Resources.Requests)
}

func GetContainerBatchMilliCPULimit(c *corev1.Container) int64 {
	return GetBatchMilliCPUFromResourceList(c.Resources.Limits)
}

func GetContainerBatchMemoryByteRequest(c *corev1.Container) int64 {
	return GetBatchMemoryFromResourceList(c.Resources.Requests)
}

func GetContainerBatchMemoryByteLimit(c *corev1.Container) int64 {
	return GetBatchMemoryFromResourceList(c.Resources.Limits)
}
