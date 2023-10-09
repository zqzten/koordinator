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

package transformer

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	apiresource "k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/utils/pointer"
	syncerconsts "sigs.k8s.io/cluster-api-provider-nested/virtualcluster/pkg/syncer/constants"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	koordfeatures "github.com/koordinator-sh/koordinator/pkg/features"
	utilfeature "github.com/koordinator-sh/koordinator/pkg/util/feature"
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
		{
			name: "pod with ACS  __IGNORE_RESOURCE__",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  unified.EnvACSIgnoreResource,
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
									Name:  unified.EnvACSIgnoreResource,
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
								Limits:   bothExists.DeepCopy(),
								Requests: bothExists.DeepCopy(),
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

func TestTransformPodGPUResourcesToCardRatio(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected corev1.ResourceList
	}{
		{
			name: "both exists",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
								},
							},
						},
					},
				},
			},
			expected: corev1.ResourceList{
				unifiedresourceext.GPUResourceCardRatio: resource.MustParse("100"),
				extension.ResourceGPUMemoryRatio:        resource.MustParse("100"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformPodGPUResourcesToCardRatio(tt.pod)
			for _, container := range tt.pod.Spec.Containers {
				if !quotav1.Equals(tt.expected, container.Resources.Limits) {
					t.Errorf("error")
				}
			}
		})
	}
}

func TestTransformHugePageToMemory(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, k8sfeature.DefaultMutableFeatureGate, koordfeatures.EnableHugePageAsMemory, true)()

	tests := []struct {
		name          string
		pod           *corev1.Pod
		wantResources corev1.ResourceRequirements
	}{
		{
			name: "normal pod",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
								},
							},
						},
					},
				},
			},
			wantResources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
		{
			name: "huge-page 2Mi pod",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
									corev1.ResourceName(fmt.Sprintf("%s2Mi", corev1.ResourceHugePagesPrefix)): resource.MustParse("2Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
									corev1.ResourceName(fmt.Sprintf("%s2Mi", corev1.ResourceHugePagesPrefix)): resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			},
			wantResources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				},
			},
		},
		{
			name: "huge-page 1Gi pod",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
									corev1.ResourceName(fmt.Sprintf("%s1Gi", corev1.ResourceHugePagesPrefix)): resource.MustParse("2Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
									corev1.ResourceName(fmt.Sprintf("%s1Gi", corev1.ResourceHugePagesPrefix)): resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			},
			wantResources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TransformHugePageToMemory(tt.pod)
			requests, limits := apiresource.PodRequestsAndLimits(tt.pod)
			assert.Equal(t, tt.wantResources, corev1.ResourceRequirements{Limits: limits, Requests: requests})
		})
	}
}
