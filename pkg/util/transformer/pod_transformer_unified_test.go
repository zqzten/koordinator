package transformer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	syncerconsts "sigs.k8s.io/cluster-api-provider-nested/virtualcluster/pkg/syncer/constants"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func TestTransformSigmaIgnoreResourceContainers(t *testing.T) {
	resources := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4000m"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
	}
	updatedResources := corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewQuantity(0, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(0, resource.BinarySI),
	}

	tests := []struct {
		name        string
		pod         *corev1.Pod
		expectedPod *corev1.Pod
	}{
		{
			name: "normal pods",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits:   resources.DeepCopy(),
								Requests: resources.DeepCopy(),
							},
						},
						{
							Resources: corev1.ResourceRequirements{
								Limits:   resources.DeepCopy(),
								Requests: resources.DeepCopy(),
							},
						},
					},
				},
			},
			expectedPod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits:   resources.DeepCopy(),
								Requests: resources.DeepCopy(),
							},
						},
						{
							Resources: corev1.ResourceRequirements{
								Limits:   resources.DeepCopy(),
								Requests: resources.DeepCopy(),
							},
						},
					},
				},
			},
		},
		{
			name: "pod with SIGMA_IGNORE_RESOURCE",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  unified.EnvSigmaIgnoreResource,
									Value: "true",
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits:   resources.DeepCopy(),
								Requests: resources.DeepCopy(),
							},
						},
						{
							Resources: corev1.ResourceRequirements{
								Limits:   resources.DeepCopy(),
								Requests: resources.DeepCopy(),
							},
						},
					},
				},
			},
			expectedPod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  unified.EnvSigmaIgnoreResource,
									Value: "true",
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits:   resources.DeepCopy(),
								Requests: updatedResources.DeepCopy(),
							},
						},
						{
							Resources: corev1.ResourceRequirements{
								Limits:   resources.DeepCopy(),
								Requests: resources.DeepCopy(),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TransformSigmaIgnoreResourceContainers(tt.pod)
			assert.Equal(t, tt.expectedPod, tt.pod)
		})
	}
}

func TestTransformTenantPod(t *testing.T) {
	tests := []struct {
		name           string
		ownerReference []metav1.OwnerReference
	}{
		{
			name: "normal tenant pod",
			ownerReference: []metav1.OwnerReference{
				{
					APIVersion:         "apps/v1",
					Kind:               "ReplicaSet",
					Name:               "test-pod",
					UID:                "225fdca4-1205-492a-9d99-a23dc79f2957",
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ownerReferenceJSON, err := json.Marshal(tt.ownerReference)
			assert.NoError(t, err)
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						syncerconsts.LabelOwnerReferences: string(ownerReferenceJSON),
					},
				},
			}
			TransformTenantPod(pod)
			assert.Equal(t, tt.ownerReference, pod.OwnerReferences)
		})
	}
}

func TestTransformNonProdPodResourceSpec(t *testing.T) {
	resources := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4000m"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
	}
	batchResources := corev1.ResourceList{
		extension.BatchCPU:    *resource.NewQuantity(4000, resource.DecimalSI),
		extension.BatchMemory: resource.MustParse("4Gi"),
	}

	tests := []struct {
		name        string
		pod         *corev1.Pod
		expectedPod *corev1.Pod
	}{
		{
			name: "prod pod",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Priority: pointer.Int32(extension.PriorityProdValueMax),
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits:   resources.DeepCopy(),
								Requests: resources.DeepCopy(),
							},
						},
					},
				},
			},
			expectedPod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Priority: pointer.Int32(extension.PriorityProdValueMax),
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits:   resources.DeepCopy(),
								Requests: resources.DeepCopy(),
							},
						},
					},
				},
			},
		},
		{
			name: "batch pod",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Priority: pointer.Int32(extension.PriorityBatchValueMin),
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits:   resources.DeepCopy(),
								Requests: resources.DeepCopy(),
							},
						},
					},
				},
			},
			expectedPod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Priority: pointer.Int32(extension.PriorityBatchValueMin),
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits:   batchResources.DeepCopy(),
								Requests: batchResources.DeepCopy(),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TransformNonProdPodResourceSpec(tt.pod)
			assert.Equal(t, tt.expectedPod, tt.pod)
		})
	}
}

func TestTransformUnifiedGPUMemoryRatio(t *testing.T) {
	tests := []struct {
		name        string
		pod         *corev1.Pod
		expectedPod *corev1.Pod
	}{
		{
			name: "pod without patch flag and gpu-mem-ratio",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
			expectedPod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "pod with patch flag and gpu-mem-ratio",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Env: []corev1.EnvVar{
								{Name: unified.EnvActivelyAddedUnifiedGPUMemoryRatio, Value: "true"},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:                     resource.MustParse("1"),
									unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:                     resource.MustParse("1"),
									unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
								},
							},
						},
					},
				},
			},
			expectedPod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Env: []corev1.EnvVar{
								{Name: unified.EnvActivelyAddedUnifiedGPUMemoryRatio, Value: "true"},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TransformUnifiedGPUMemoryRatio(tt.pod)
			assert.Equal(t, tt.expectedPod, tt.pod)
		})
	}
}

func TestTransformENIResource(t *testing.T) {
	enableENIResourceTransform = true
	sigmaENIResources := corev1.ResourceList{
		unified.ResourceSigmaENI: resource.MustParse("1"),
	}
	memberENIResources := corev1.ResourceList{
		unified.ResourceAliyunMemberENI: resource.MustParse("1"),
	}
	bothExists := corev1.ResourceList{
		unified.ResourceSigmaENI:        resource.MustParse("1"),
		unified.ResourceAliyunMemberENI: resource.MustParse("1"),
	}

	tests := []struct {
		name        string
		pod         *corev1.Pod
		expectedPod *corev1.Pod
	}{
		{
			name: "only sigmaEni",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits:   sigmaENIResources.DeepCopy(),
								Requests: sigmaENIResources.DeepCopy(),
							},
						},
					},
				},
			},
			expectedPod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits:   memberENIResources.DeepCopy(),
								Requests: memberENIResources.DeepCopy(),
							},
						},
					},
				},
			},
		},
		{
			name: "only member eni",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits:   memberENIResources.DeepCopy(),
								Requests: memberENIResources.DeepCopy(),
							},
						},
					},
				},
			},
			expectedPod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits:   memberENIResources.DeepCopy(),
								Requests: memberENIResources.DeepCopy(),
							},
						},
					},
				},
			},
		},
		{
			name: "both exists",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits:   bothExists.DeepCopy(),
								Requests: bothExists.DeepCopy(),
							},
						},
					},
				},
			},
			expectedPod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits:   memberENIResources.DeepCopy(),
								Requests: memberENIResources.DeepCopy(),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TransformENIResource(tt.pod)
			assert.Equal(t, tt.expectedPod, tt.pod)
		})
	}
}
