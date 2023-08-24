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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func TestTransformACKDeviceAllocation(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want extension.DeviceAllocations
	}{
		{
			name: "non-GPU Pod",
			pod:  &corev1.Pod{},
			want: nil,
		},
		{
			name: "koordinator GPU Pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						extension.AnnotationDeviceAllocated: `{"gpu":[{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"81251Mi","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
					},
				},
			},
			want: extension.DeviceAllocations{
				schedulingv1alpha1.GPU: {
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							extension.ResourceGPUCore:        resource.MustParse("100"),
							extension.ResourceGPUMemoryRatio: resource.MustParse("100"),
							extension.ResourceGPUMemory:      resource.MustParse("81251Mi"),
						},
					},
				},
			},
		},
		{
			name: "ACK v1 GPU Pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ack.AnnotationAliyunEnvMemPod:        "8",
						ack.AnnotationAliyunEnvResourceIndex: "1",
					},
				},
			},
			want: extension.DeviceAllocations{
				schedulingv1alpha1.GPU: {
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							extension.ResourceGPUMemory: *resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
						},
					},
				},
			},
		},
		{
			name: "ACK v2 GPU Pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"scheduler.framework.gpushare.allocation": `{"0":{"1":1},"1":{"1":7}}`,
					},
				},
			},
			want: extension.DeviceAllocations{
				schedulingv1alpha1.GPU: {
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							extension.ResourceGPUMemory: *resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
						},
					},
				},
			},
		},
		{
			name: "ACK v2 GPU Pod - GPU Memory and Core",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"scheduler.framework.gpushare.allocation": `{"0":{"1":1},"1":{"1":7}}`,
						ack.AnnotationACKGPUCoreAllocation:        `{"0":{"1":50},"1":{"1":50}}`,
					},
				},
			},
			want: extension.DeviceAllocations{
				schedulingv1alpha1.GPU: {
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							extension.ResourceGPUMemory: *resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
							extension.ResourceGPUCore:   *resource.NewQuantity(100, resource.DecimalSI),
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TransformACKDeviceAllocation(tt.pod)
			got, err := extension.GetDeviceAllocations(tt.pod.Annotations)
			assert.NoError(t, err)
			assert.True(t, equality.Semantic.DeepEqual(tt.want, got))
		})
	}
}
