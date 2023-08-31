package transformer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	koordfeatures "github.com/koordinator-sh/koordinator/pkg/features"
	utilfeature "github.com/koordinator-sh/koordinator/pkg/util/feature"
)

func TestTransformNodeAllocatableWithACKGPUMemory(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, k8sfeature.DefaultMutableFeatureGate, koordfeatures.EnableACKGPUShareScheduling, true)()

	tests := []struct {
		name string
		node *corev1.Node
		want *corev1.Node
	}{
		{
			name: "missing ack gpu share memory",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						apiext.ResourceGPUMemory: *resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
					},
				},
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"__internal_gpu-compatible__": "ack-gpu-share",
					},
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						ack.ResourceAliyunGPUMemory: *resource.NewQuantity(100, resource.DecimalSI),
						apiext.ResourceGPUMemory:    *resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
					},
				},
			},
		},
		{
			name: "has ack gpu share memory",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						apiext.ResourceGPUMemory:    *resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
						ack.ResourceAliyunGPUMemory: *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
			want: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						ack.ResourceAliyunGPUMemory: *resource.NewQuantity(100, resource.DecimalSI),
						apiext.ResourceGPUMemory:    *resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
					},
				},
			},
		},
		{
			name: "missing koordinator gpu memory",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceMemory: *resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
					},
				},
			},
			want: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceMemory: *resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TransformNodeAllocatableWithACKGPUMemory(tt.node)
			assert.Equal(t, tt.want, tt.node)
		})
	}
}

func TestTransformNodeAllocatableWithACKGPUCorePercentage(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, k8sfeature.DefaultMutableFeatureGate, koordfeatures.EnableACKGPUShareScheduling, true)()

	tests := []struct {
		name string
		node *corev1.Node
		want *corev1.Node
	}{
		{
			name: "missing ack gpu core",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						apiext.ResourceGPUCore: *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"__internal_gpu-compatible__": "ack-gpu-share",
					},
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						ack.ResourceALiyunGPUCorePercentage: *resource.NewQuantity(100, resource.DecimalSI),
						apiext.ResourceGPUCore:              *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
		},
		{
			name: "has ack gpu core",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						ack.ResourceALiyunGPUCorePercentage: *resource.NewQuantity(100, resource.DecimalSI),
						apiext.ResourceGPUCore:              *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
			want: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						ack.ResourceALiyunGPUCorePercentage: *resource.NewQuantity(100, resource.DecimalSI),
						apiext.ResourceGPUCore:              *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
		},
		{
			name: "missing koordinator gpu core",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU: *resource.NewMilliQuantity(32*1000, resource.DecimalSI),
					},
				},
			},
			want: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU: *resource.NewMilliQuantity(32*1000, resource.DecimalSI),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TransformNodeAllocatableWithACKGPUCorePercentage(tt.node)
		})
	}
}
