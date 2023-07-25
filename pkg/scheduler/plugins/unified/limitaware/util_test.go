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

package limitaware

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
	schedutil "k8s.io/kubernetes/pkg/scheduler/util"
)

func Test_getPodResourceLimit(t *testing.T) {
	tests := []struct {
		name    string
		pod     *corev1.Pod
		nonZero bool
		want    *framework.Resource
	}{
		{
			name: "no resource",
			pod:  &corev1.Pod{},
			want: framework.NewResource(nil),
		},
		{
			name: "init container resource",
			pod:  newResourceInitPod(newResourcePod(framework.Resource{}), framework.Resource{ScalarResources: map[corev1.ResourceName]int64{extendedResourceA: 1}}),
			want: &framework.Resource{ScalarResources: map[corev1.ResourceName]int64{extendedResourceA: 1}},
		},
		{
			name: "limit not set, but request set",
			pod:  st.MakePod().Req(map[corev1.ResourceName]string{"cpu": "1", "memory": "2000"}).Obj(),
			want: &framework.Resource{MilliCPU: 1000, Memory: 2000},
		},
		{
			name:    "cpu,mem nonZero for not set",
			pod:     newResourceInitPod(newResourcePodForNonZeroTest(true, framework.Resource{}), framework.Resource{ScalarResources: map[corev1.ResourceName]int64{extendedResourceA: 1}}),
			nonZero: true,
			want:    &framework.Resource{MilliCPU: schedutil.DefaultMilliCPURequest, Memory: schedutil.DefaultMemoryRequest, ScalarResources: map[corev1.ResourceName]int64{extendedResourceA: 1}},
		},
		{
			name:    "cpu,mem zero if set zero",
			pod:     newResourceInitPod(newResourcePod(framework.Resource{}), framework.Resource{ScalarResources: map[corev1.ResourceName]int64{extendedResourceA: 1}}),
			nonZero: true,
			want:    &framework.Resource{ScalarResources: map[corev1.ResourceName]int64{extendedResourceA: 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getPodResourceLimit(tt.pod, tt.nonZero); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getPodResourceLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}
