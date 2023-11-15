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

func TestTransformNodeAllocatableWithACKShareResources(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, k8sfeature.DefaultMutableFeatureGate, koordfeatures.EnableACKGPUShareScheduling, true)()

	tests := []struct {
		name string
		node *corev1.Node
		want *corev1.Node
	}{
		{
			name: "only has koordinator gpu resources",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						apiext.ResourceGPUMemory: *resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
						apiext.ResourceGPUCore:   *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"__internal_gpu-compatible__": "koordinator-gpu-as-ack-gpu",
					},
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						ack.ResourceAliyunGPUMemory:         *resource.NewQuantity(100, resource.DecimalSI),
						ack.ResourceALiyunGPUCorePercentage: *resource.NewQuantity(100, resource.DecimalSI),
						apiext.ResourceGPUMemory:            *resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
						apiext.ResourceGPUCore:              *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
		},
		{
			name: "only has ack gpu share memory",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						apiext.ResourceGPUMemory:    *resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
						apiext.ResourceGPUCore:      *resource.NewQuantity(100, resource.DecimalSI),
						ack.ResourceAliyunGPUMemory: *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
			want: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						apiext.ResourceGPUMemory:    *resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
						apiext.ResourceGPUCore:      *resource.NewQuantity(100, resource.DecimalSI),
						ack.ResourceAliyunGPUMemory: *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
		},
		{
			name: "only has ack gpu core -- this scenario is actually illegal",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						ack.ResourceALiyunGPUCorePercentage: *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
			want: &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						ack.ResourceALiyunGPUCorePercentage: *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
		},
		{
			name: "no koordinator gpu resources",
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
			TransformNodeAllocatableWithACKShareResources(tt.node)
			assert.Equal(t, tt.want, tt.node)
		})
	}
}
