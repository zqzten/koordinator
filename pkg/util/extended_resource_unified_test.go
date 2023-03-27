//go:build !github
// +build !github

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

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func Test_GetContainerXXXValueUnified(t *testing.T) {
	type args struct {
		container *corev1.Container
	}
	tests := []struct {
		name string
		args args
		fn   func(container *corev1.Container) int64
		want int64
	}{
		{
			name: "get unified batch cpu request",
			args: args{
				container: &corev1.Container{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList(
							map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceCPU: *resource.NewQuantity(1, resource.DecimalSI),
							}),
					},
				},
			},
			fn:   GetContainerBatchMilliCPURequest,
			want: 1000,
		},
		{
			name: "get unified batch cpu limit",
			args: args{
				container: &corev1.Container{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{},
						Limits: corev1.ResourceList(
							map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceCPU: *resource.NewQuantity(1, resource.DecimalSI),
							}),
					},
				},
			},
			fn:   GetContainerBatchMilliCPULimit,
			want: 1000,
		},
		{
			name: "get unified batch memory request",
			args: args{
				container: &corev1.Container{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList(
							map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							}),
					},
				},
			},
			fn:   GetContainerBatchMemoryByteRequest,
			want: 1 << 30,
		},
		{
			name: "get unified batch memory limit",
			args: args{
				container: &corev1.Container{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList(
							map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							}),
					},
				},
			},
			fn:   GetContainerBatchMemoryByteLimit,
			want: 2 << 30,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.args.container)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetBatchXXXFromResourceListUnified(t *testing.T) {
	type args struct {
		r corev1.ResourceList
	}
	tests := []struct {
		name string
		args args
		fn   func(r corev1.ResourceList) int64
		want int64
	}{
		{
			name: "get unified batch cpu",
			args: args{
				r: corev1.ResourceList(
					map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU: *resource.NewQuantity(1, resource.DecimalSI),
					}),
			},
			fn:   GetBatchMilliCPUFromResourceList,
			want: 1000,
		},
		{
			name: "get unified batch memory",
			args: args{
				r: corev1.ResourceList(
					map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					}),
			},
			fn:   GetBatchMemoryFromResourceList,
			want: 2 << 30,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.args.r)
			assert.Equal(t, tt.want, got)
		})
	}
}
