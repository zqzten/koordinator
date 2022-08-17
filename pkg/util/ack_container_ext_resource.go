//go:build !github
// +build !github

package util

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"

	uniext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
)

func GetContainerBatchMilliCPURequest(c *corev1.Container) int64 {
	if cpuRequest, ok := c.Resources.Requests[extension.BatchCPU]; ok {
		return cpuRequest.Value()
	}
	if cpuRequest, ok := c.Resources.Requests[uniext.AlibabaCloudReclaimedCPU]; ok {
		return cpuRequest.Value()
	}
	return -1
}

func GetContainerBatchMilliCPULimit(c *corev1.Container) int64 {
	if cpuLimit, ok := c.Resources.Limits[extension.BatchCPU]; ok {
		return cpuLimit.Value()
	}
	if cpuLimit, ok := c.Resources.Limits[uniext.AlibabaCloudReclaimedCPU]; ok {
		return cpuLimit.Value()
	}
	return -1
}

func GetContainerBatchMemoryByteRequest(c *corev1.Container) int64 {
	if memRequest, ok := c.Resources.Requests[extension.BatchMemory]; ok {
		return memRequest.Value()
	}
	if memRequest, ok := c.Resources.Requests[uniext.AlibabaCloudReclaimedMemory]; ok {
		return memRequest.Value()
	}
	return -1
}

func GetContainerBatchMemoryByteLimit(c *corev1.Container) int64 {
	if memLimit, ok := c.Resources.Limits[extension.BatchMemory]; ok {
		return memLimit.Value()
	}
	if memLimit, ok := c.Resources.Limits[uniext.AlibabaCloudReclaimedMemory]; ok {
		return memLimit.Value()
	}
	return -1
}
