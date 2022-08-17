package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	uniext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
)

func Test_GetContainerReclaimedValue(t *testing.T) {
	type args struct {
		name        string
		fn          func(container *corev1.Container) int64
		container   *corev1.Container
		expectValue int64
	}

	tests := []args{
		{
			name: "test_GetContainerBatchMilliCPURequest",
			fn:   GetContainerBatchMilliCPURequest,
			container: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList(
						map[corev1.ResourceName]resource.Quantity{
							uniext.AlibabaCloudReclaimedCPU: *resource.NewMilliQuantity(1, resource.DecimalSI),
						}),
				},
			},
			expectValue: 1,
		},
		{
			name:        "test_GetContainerBEMilliCPURequest_invalid",
			fn:          GetContainerBatchMilliCPURequest,
			container:   &corev1.Container{},
			expectValue: -1,
		},
		{
			name: "test_GetContainerBatchMilliCPULimit",
			fn:   GetContainerBatchMilliCPULimit,
			container: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList(
						map[corev1.ResourceName]resource.Quantity{
							uniext.AlibabaCloudReclaimedCPU: *resource.NewMilliQuantity(1, resource.DecimalSI),
						}),
				},
			},
			expectValue: 1,
		},
		{
			name:        "test_GetContainerBatchMilliCPULimit_invalid",
			fn:          GetContainerBatchMilliCPULimit,
			container:   &corev1.Container{},
			expectValue: -1,
		},
		{
			name: "test_GetContainerBatchMemoryByteRequest",
			fn:   GetContainerBatchMemoryByteRequest,
			container: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList(
						map[corev1.ResourceName]resource.Quantity{
							uniext.AlibabaCloudReclaimedMemory: *resource.NewQuantity(1, resource.BinarySI),
						}),
				},
			},
			expectValue: 1,
		},
		{
			name:        "test_GetContainerBatchMemoryByteRequest_invalid",
			fn:          GetContainerBatchMemoryByteRequest,
			container:   &corev1.Container{},
			expectValue: -1,
		},
		{
			name: "test_GetContainerBatchMemoryByteLimit",
			fn:   GetContainerBatchMemoryByteLimit,
			container: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList(
						map[corev1.ResourceName]resource.Quantity{
							uniext.AlibabaCloudReclaimedMemory: *resource.NewQuantity(1, resource.BinarySI),
						}),
				},
			},
			expectValue: 1,
		},
		{
			name:        "test_GetContainerBatchMemoryByteLimit_invalid",
			fn:          GetContainerBatchMemoryByteLimit,
			container:   &corev1.Container{},
			expectValue: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue := tt.fn(tt.container)
			assert.Equal(t, tt.expectValue, gotValue, "checkValue")
		})
	}
}