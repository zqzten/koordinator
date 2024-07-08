//go:build !github
// +build !github

package util

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"

	uniext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
)

var ExtendedResourceNames = []corev1.ResourceName{
	extension.BatchCPU,
	extension.BatchMemory,
	uniext.AlibabaCloudReclaimedCPU,
	uniext.AlibabaCloudReclaimedMemory,
}

func GetBatchMilliCPUFromResourceList(r corev1.ResourceList) int64 {
	// assert r != nil
	if milliCPU, ok := r[extension.BatchCPU]; ok {
		return milliCPU.Value()
	}
	if milliCPU, ok := r[uniext.AlibabaCloudReclaimedCPU]; ok {
		return milliCPU.Value()
	}
	return -1
}

func GetBatchMemoryFromResourceList(r corev1.ResourceList) int64 {
	// assert r != nil
	if memory, ok := r[extension.BatchMemory]; ok {
		return memory.Value()
	}
	if memory, ok := r[uniext.AlibabaCloudReclaimedMemory]; ok {
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
