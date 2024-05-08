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

package validating

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

func Test_validateGPUResources(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want field.ErrorList
	}{
		{
			name: "runc pod, gpu.shared = 0",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									extension.ResourceGPUShared: resource.MustParse("0"),
								},
							},
						},
					},
				},
			},
			want: field.ErrorList{},
		},
		{
			name: "runc pod, gpu.shared = 1",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									extension.ResourceGPUShared: resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
			want: field.ErrorList{},
		},
		{
			name: "runc pod, gpu.shared = 2",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									extension.ResourceGPUShared: resource.MustParse("2"),
								},
							},
						},
					},
				},
			},
			want: field.ErrorList{},
		},
		{
			name: "rund pod, gpu.shared = 0",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					RuntimeClassName: pointer.String("rund"),
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									extension.ResourceGPUShared: resource.MustParse("0"),
								},
							},
						},
					},
				},
			},
			want: field.ErrorList{},
		},
		{
			name: "rund pod, gpu.shared = 1",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					RuntimeClassName: pointer.String("rund"),
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									extension.ResourceGPUShared: resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
			want: field.ErrorList{},
		},
		{
			name: "rund pod, gpu.shared = 2",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					RuntimeClassName: pointer.String("rund"),
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									extension.ResourceGPUShared: resource.MustParse("2"),
								},
							},
						},
					},
				},
			},
			want: field.ErrorList{field.Invalid(field.NewPath("pod.spec.containers[*].resources.requests"), "2", "the requested gpu.shared of rund pod should be greater than one")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateGPUResources(tt.pod); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validateGPUResources() = %v, want %v", got, tt.want)
			}
		})
	}
}
