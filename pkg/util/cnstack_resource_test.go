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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestMultiplyResourceList(t *testing.T) {
	type args struct {
		a      corev1.ResourceList
		factor float64
	}
	tests := []struct {
		name string
		args args
		want corev1.ResourceList
	}{
		{
			name: "common node resource",
			args: args{
				a: corev1.ResourceList{
					corev1.ResourceCPU:    *resource.NewMilliQuantity(10000, resource.DecimalSI),
					corev1.ResourceMemory: *resource.NewQuantity(1000, resource.BinarySI),
				},
				factor: 0.5,
			},
			want: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(5000, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(500, resource.BinarySI),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MultiplyResourceList(tt.args.a, tt.args.factor); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MultiplyResourceList() = %v, want %v", got, tt.want)
			}
		})
	}
}
