package resourcesummary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func TestGetReservationPriorityResource(t *testing.T) {
	tests := []struct {
		name         string
		reservation  *schedulingv1alpha1.Reservation
		node         *corev1.Node
		gpuCapacity  corev1.ResourceList
		wantUsed     v1beta1.PodPriorityUsed
		wantCapacity v1beta1.PodPriorityUsed
		wantFree     v1beta1.PodPriorityUsed
	}{
		{
			name: "prod reservation; not allocated",
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{
					UID:  uuid.NewUUID(),
					Name: "test-reservation",
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:                     resource.MustParse("4000m"),
											corev1.ResourceMemory:                  resource.MustParse("4Gi"),
											unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
										},
									},
								},
							},
							Priority: pointer.Int32(0),
						},
					},
				},
				Status: schedulingv1alpha1.ReservationStatus{
					Phase:    schedulingv1alpha1.ReservationAvailable,
					NodeName: "test-node-1",
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU:                     resource.MustParse("4000m"),
						corev1.ResourceMemory:                  resource.MustParse("4Gi"),
						unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-0",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "sigma.ali/resource-pool",
							Value:  "sigma_public",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("110"),
						corev1.ResourceMemory:           resource.MustParse("100Gi"),
						corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
						corev1.ResourcePods:             resource.MustParse("50"),
						extension.BatchCPU:              resource.MustParse("50000"),
						extension.BatchMemory:           resource.MustParse("10Gi"),
					},
					Conditions: []corev1.NodeCondition{{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
			gpuCapacity: corev1.ResourceList{
				unifiedresourceext.GPUResourceCore:     resource.MustParse("800"),
				unifiedresourceext.GPUResourceMem:      resource.MustParse("800Gi"),
				unifiedresourceext.GPUResourceMemRatio: resource.MustParse("800"),
			},
			wantUsed: v1beta1.PodPriorityUsed{
				PriorityClass: uniext.PriorityProd,
				Allocated:     nil,
			},
			wantCapacity: v1beta1.PodPriorityUsed{
				PriorityClass: uniext.PriorityProd,
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("4000m"),
					corev1.ResourceMemory:                  resource.MustParse("4Gi"),
					extension.ResourceGPU:                  resource.MustParse("100"),
					extension.ResourceGPUCore:              resource.MustParse("100"),
					extension.ResourceGPUMemory:            resource.MustParse("100Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("100"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("100"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("100"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("100Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
				},
			},
			wantFree: v1beta1.PodPriorityUsed{
				PriorityClass: uniext.PriorityProd,
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("4000m"),
					corev1.ResourceMemory:                  resource.MustParse("4Gi"),
					extension.ResourceGPU:                  resource.MustParse("100"),
					extension.ResourceGPUCore:              resource.MustParse("100"),
					extension.ResourceGPUMemory:            resource.MustParse("100Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("100"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("100"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("100"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("100Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
				},
			},
		},
		{
			name: "batch reservation; not allocated",
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{
					UID:  uuid.NewUUID(),
					Name: "test-reservation",
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											extension.BatchCPU:                     resource.MustParse("4000"),
											extension.BatchMemory:                  resource.MustParse("4Gi"),
											unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
										},
									},
								},
							},
							Priority: pointer.Int32(uniext.PriorityBatchValueMax),
						},
					},
				},
				Status: schedulingv1alpha1.ReservationStatus{
					Phase:    schedulingv1alpha1.ReservationAvailable,
					NodeName: "test-node-1",
					Allocatable: corev1.ResourceList{
						extension.BatchCPU:                     resource.MustParse("4000"),
						extension.BatchMemory:                  resource.MustParse("4Gi"),
						unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-0",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "sigma.ali/resource-pool",
							Value:  "sigma_public",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("110"),
						corev1.ResourceMemory:           resource.MustParse("100Gi"),
						corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
						corev1.ResourcePods:             resource.MustParse("50"),
						extension.BatchCPU:              resource.MustParse("50000"),
						extension.BatchMemory:           resource.MustParse("10Gi"),
					},
					Conditions: []corev1.NodeCondition{{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
			gpuCapacity: corev1.ResourceList{
				unifiedresourceext.GPUResourceCore:     resource.MustParse("800"),
				unifiedresourceext.GPUResourceMem:      resource.MustParse("800Gi"),
				unifiedresourceext.GPUResourceMemRatio: resource.MustParse("800"),
			},
			wantUsed: v1beta1.PodPriorityUsed{
				PriorityClass: uniext.PriorityBatch,
				Allocated:     nil,
			},
			wantCapacity: v1beta1.PodPriorityUsed{
				PriorityClass: uniext.PriorityBatch,
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("4000m"),
					corev1.ResourceMemory:                  resource.MustParse("4Gi"),
					extension.ResourceGPU:                  resource.MustParse("100"),
					extension.ResourceGPUCore:              resource.MustParse("100"),
					extension.ResourceGPUMemory:            resource.MustParse("100Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("100"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("100"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("100"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("100Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
				},
			},
			wantFree: v1beta1.PodPriorityUsed{
				PriorityClass: uniext.PriorityBatch,
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("4000m"),
					corev1.ResourceMemory:                  resource.MustParse("4Gi"),
					extension.ResourceGPU:                  resource.MustParse("100"),
					extension.ResourceGPUCore:              resource.MustParse("100"),
					extension.ResourceGPUMemory:            resource.MustParse("100Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("100"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("100"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("100"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("100Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
				},
			},
		},
		{
			name: "batch reservation; allocated",
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{
					UID:  uuid.NewUUID(),
					Name: "test-reservation",
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											extension.BatchCPU:                     resource.MustParse("4000"),
											extension.BatchMemory:                  resource.MustParse("4Gi"),
											unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
										},
									},
								},
							},
							Priority: pointer.Int32(uniext.PriorityBatchValueMax),
						},
					},
				},
				Status: schedulingv1alpha1.ReservationStatus{
					Phase:    schedulingv1alpha1.ReservationAvailable,
					NodeName: "test-node-1",
					Allocatable: corev1.ResourceList{
						extension.BatchCPU:                     resource.MustParse("4000"),
						extension.BatchMemory:                  resource.MustParse("4Gi"),
						unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
					},
					Allocated: corev1.ResourceList{
						extension.BatchCPU:                     resource.MustParse("3000"),
						extension.BatchMemory:                  resource.MustParse("2Gi"),
						unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-0",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "sigma.ali/resource-pool",
							Value:  "sigma_public",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("110"),
						corev1.ResourceMemory:           resource.MustParse("100Gi"),
						corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
						corev1.ResourcePods:             resource.MustParse("50"),
						extension.BatchCPU:              resource.MustParse("50000"),
						extension.BatchMemory:           resource.MustParse("10Gi"),
					},
					Conditions: []corev1.NodeCondition{{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
			gpuCapacity: corev1.ResourceList{
				unifiedresourceext.GPUResourceCore:     resource.MustParse("800"),
				unifiedresourceext.GPUResourceMem:      resource.MustParse("800Gi"),
				unifiedresourceext.GPUResourceMemRatio: resource.MustParse("800"),
			},
			wantUsed: v1beta1.PodPriorityUsed{
				PriorityClass: uniext.PriorityBatch,
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("3000m"),
					corev1.ResourceMemory:                  resource.MustParse("2Gi"),
					extension.ResourceGPU:                  resource.MustParse("100"),
					extension.ResourceGPUCore:              resource.MustParse("100"),
					extension.ResourceGPUMemory:            resource.MustParse("100Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("100"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("100"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("100"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("100Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
				},
			},
			wantCapacity: v1beta1.PodPriorityUsed{
				PriorityClass: uniext.PriorityBatch,
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("4000m"),
					corev1.ResourceMemory:                  resource.MustParse("4Gi"),
					extension.ResourceGPU:                  resource.MustParse("200"),
					extension.ResourceGPUCore:              resource.MustParse("200"),
					extension.ResourceGPUMemory:            resource.MustParse("200Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("200"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("200"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("200"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("200Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
				},
			},
			wantFree: v1beta1.PodPriorityUsed{
				PriorityClass: uniext.PriorityBatch,
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("1000m"),
					corev1.ResourceMemory:                  resource.MustParse("2Gi"),
					extension.ResourceGPU:                  resource.MustParse("100"),
					extension.ResourceGPUCore:              resource.MustParse("100"),
					extension.ResourceGPUMemory:            resource.MustParse("100Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("100"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("100"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("100"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("100Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
				},
			},
		},
		{
			name: "batch reservation; allocated > allocatable",
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{
					UID:  uuid.NewUUID(),
					Name: "test-reservation",
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											extension.BatchCPU:                     resource.MustParse("4000"),
											extension.BatchMemory:                  resource.MustParse("4Gi"),
											unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
										},
									},
								},
							},
							Priority: pointer.Int32(uniext.PriorityBatchValueMax),
						},
					},
				},
				Status: schedulingv1alpha1.ReservationStatus{
					Phase:    schedulingv1alpha1.ReservationAvailable,
					NodeName: "test-node-1",
					Allocatable: corev1.ResourceList{
						extension.BatchCPU:                     resource.MustParse("4000"),
						extension.BatchMemory:                  resource.MustParse("4Gi"),
						unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
					},
					Allocated: corev1.ResourceList{
						extension.BatchCPU:                     resource.MustParse("5000"),
						extension.BatchMemory:                  resource.MustParse("2Gi"),
						unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-0",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "sigma.ali/resource-pool",
							Value:  "sigma_public",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("110"),
						corev1.ResourceMemory:           resource.MustParse("100Gi"),
						corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
						corev1.ResourcePods:             resource.MustParse("50"),
						extension.BatchCPU:              resource.MustParse("50000"),
						extension.BatchMemory:           resource.MustParse("10Gi"),
					},
					Conditions: []corev1.NodeCondition{{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
			gpuCapacity: corev1.ResourceList{
				unifiedresourceext.GPUResourceCore:     resource.MustParse("800"),
				unifiedresourceext.GPUResourceMem:      resource.MustParse("800Gi"),
				unifiedresourceext.GPUResourceMemRatio: resource.MustParse("800"),
			},
			wantUsed: v1beta1.PodPriorityUsed{
				PriorityClass: uniext.PriorityBatch,
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("5000m"),
					corev1.ResourceMemory:                  resource.MustParse("2Gi"),
					extension.ResourceGPU:                  resource.MustParse("100"),
					extension.ResourceGPUCore:              resource.MustParse("100"),
					extension.ResourceGPUMemory:            resource.MustParse("100Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("100"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("100"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("100"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("100Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
				},
			},
			wantCapacity: v1beta1.PodPriorityUsed{
				PriorityClass: uniext.PriorityBatch,
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("4000m"),
					corev1.ResourceMemory:                  resource.MustParse("4Gi"),
					extension.ResourceGPU:                  resource.MustParse("200"),
					extension.ResourceGPUCore:              resource.MustParse("200"),
					extension.ResourceGPUMemory:            resource.MustParse("200Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("200"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("200"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("200"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("200Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
				},
			},
			wantFree: v1beta1.PodPriorityUsed{
				PriorityClass: uniext.PriorityBatch,
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("0m"),
					corev1.ResourceMemory:                  resource.MustParse("2Gi"),
					extension.ResourceGPU:                  resource.MustParse("100"),
					extension.ResourceGPUCore:              resource.MustParse("100"),
					extension.ResourceGPUMemory:            resource.MustParse("100Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("100"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("100"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("100"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("100Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUsed, gotCapacity, gotFree := GetReservationPriorityResource(tt.reservation, tt.node, tt.gpuCapacity)
			assert.True(t, quotav1.Equals(tt.wantUsed.Allocated, gotUsed.Allocated))
			assert.True(t, quotav1.Equals(tt.wantCapacity.Allocated, gotCapacity.Allocated))
			assert.True(t, quotav1.Equals(tt.wantFree.Allocated, gotFree.Allocated))
		})
	}
}
