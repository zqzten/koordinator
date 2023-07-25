package extension

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

var (
	kubernetesIOResourceA = corev1.ResourceName("kubernetes.io/something")
	kubernetesIOResourceB = corev1.ResourceName("subdomain.kubernetes.io/something")
)

func TestGetNodeLimitCapacity(t *testing.T) {
	type args struct {
	}
	tests := []struct {
		name            string
		ratio           LimitToAllocatable
		nodeAllocatable *framework.Resource
		want            *framework.Resource
		wantErr         bool
	}{
		{
			name: "normal flow",
			ratio: LimitToAllocatable{
				corev1.ResourceCPU:              intstr.FromInt(120),
				corev1.ResourceEphemeralStorage: intstr.FromInt(137),
				kubernetesIOResourceA:           intstr.FromInt(150),
			},
			nodeAllocatable: &framework.Resource{
				MilliCPU:         100,
				Memory:           2000,
				EphemeralStorage: 30,
				ScalarResources: map[corev1.ResourceName]int64{
					kubernetesIOResourceA: 30,
					kubernetesIOResourceB: 30,
				},
			},
			want: &framework.Resource{
				MilliCPU:         120,
				Memory:           2000,
				EphemeralStorage: 41,
				ScalarResources: map[corev1.ResourceName]int64{
					kubernetesIOResourceA: 45,
					kubernetesIOResourceB: 30,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetNodeLimitAllocatable(tt.nodeAllocatable, tt.ratio)
			if (err != nil) != tt.wantErr {
				t.Errorf("getNodeLimitCapacity() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getNodeLimitCapacity() got = %v, want %v", got, tt.want)
			}
		})
	}
}
