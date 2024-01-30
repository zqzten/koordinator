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

package unified

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

func TestGetResourceSpec(t *testing.T) {
	type args struct {
		annotations map[string]string
	}
	koordResourceSpec := extension.ResourceSpec{
		PreferredCPUBindPolicy: extension.CPUBindPolicyFullPCPUs,
	}
	koordResourceSpecData, err := json.Marshal(koordResourceSpec)
	assert.NoError(t, err)
	unifiedAllocSpec := uniext.ResourceAllocSpec{
		CPU: uniext.CPUBindStrategySameCoreFirst,
	}
	unifiedResourceSpecData, err := json.Marshal(unifiedAllocSpec)
	assert.NoError(t, err)
	asiSigmaAllocSpec := AllocSpec{
		Containers: []Container{
			{
				Name: "container-1",
				Resource: ResourceRequirements{
					CPU: CPUSpec{
						CPUSet: &CPUSetSpec{
							SpreadStrategy: SpreadStrategySameCoreFirst,
							CPUIDs:         nil,
						},
					},
				},
			},
		},
	}
	asiSigmaAllocSpecData, err := json.Marshal(asiSigmaAllocSpec)
	assert.NoError(t, err)

	tests := []struct {
		name    string
		args    args
		want    *extension.ResourceSpec
		wantErr bool
	}{
		{
			name: "koordinator resourceSpec",
			args: args{
				annotations: map[string]string{extension.AnnotationResourceSpec: string(koordResourceSpecData)},
			},
			want: &extension.ResourceSpec{
				PreferredCPUBindPolicy:      extension.CPUBindPolicyFullPCPUs,
				PreferredCPUExclusivePolicy: "",
			},
			wantErr: false,
		},
		{
			name: "unified resourceSpec",
			args: args{
				annotations: map[string]string{uniext.AnnotationAllocSpec: string(unifiedResourceSpecData)},
			},
			want: &extension.ResourceSpec{
				PreferredCPUBindPolicy:      extension.CPUBindPolicyFullPCPUs,
				PreferredCPUExclusivePolicy: "",
			},
			wantErr: false,
		},
		{
			name: "ASISigma resourceSpec",
			args: args{
				annotations: map[string]string{AnnotationAllocSpec: string(asiSigmaAllocSpecData)},
			},
			want: &extension.ResourceSpec{
				PreferredCPUBindPolicy:      extension.CPUBindPolicyFullPCPUs,
				PreferredCPUExclusivePolicy: "",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := GetResourceSpec(tt.args.annotations)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetResourceSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetResourceSpec() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetUnifiedResourceStatus(t *testing.T) {
	koordResourceStatusData := `{"cpuset":"0-3"}`
	unifiedAllocStatusData := `{"cpu":[0,1,2,3],"gpu":{}}`
	asiAllocSpec := AllocSpec{
		Containers: []Container{
			{
				Name: "test-container-1",
				Resource: ResourceRequirements{
					CPU: CPUSpec{
						CPUSet: &CPUSetSpec{
							SpreadStrategy: SpreadStrategySameCoreFirst,
							CPUIDs:         []int{0, 1, 2, 3},
						},
					},
				},
			},
		}}
	asiAllocSpecData, err := json.Marshal(asiAllocSpec)
	assert.NoError(t, err)
	type args struct {
		annotations map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    *extension.ResourceStatus
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "no annotations",
			args: args{
				annotations: nil,
			},
			want: &extension.ResourceStatus{
				CPUSet: "",
			},
			wantErr: assert.NoError,
		},
		{
			name: "koord resource status",
			args: args{
				annotations: map[string]string{extension.AnnotationResourceStatus: koordResourceStatusData},
			},
			want: &extension.ResourceStatus{
				CPUSet: "0-3",
			},
			wantErr: assert.NoError,
		},
		{
			name: "unified resource status",
			args: args{
				annotations: map[string]string{uniext.AnnotationAllocStatus: unifiedAllocStatusData},
			},
			want: &extension.ResourceStatus{
				CPUSet: "0-3",
			},
			wantErr: assert.NoError,
		},
		{
			name: "asi alloc-spec resource status",
			args: args{
				annotations: map[string]string{AnnotationAllocSpec: string(asiAllocSpecData)},
			},
			want: &extension.ResourceStatus{
				CPUSet: "0-3",
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetResourceStatus(tt.args.annotations)
			if !tt.wantErr(t, err, fmt.Sprintf("GetResourceStatus(%v)", tt.args.annotations)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetResourceStatus(%v)", tt.args.annotations)
		})
	}
}

func TestSetUnifiedResourceStatus(t *testing.T) {
	asiAllocSpec := AllocSpec{
		Containers: []Container{
			{
				Name: "test-container-1",
				Resource: ResourceRequirements{
					CPU: CPUSpec{
						CPUSet: &CPUSetSpec{
							SpreadStrategy: SpreadStrategySameCoreFirst,
							CPUIDs:         nil,
						},
					},
				},
			},
		},
	}
	asiAllocSpecData, err := json.Marshal(asiAllocSpec)
	assert.NoError(t, err)
	koordResourceSpec := &extension.ResourceSpec{
		PreferredCPUBindPolicy: extension.CPUBindPolicyFullPCPUs,
	}
	koordResourceSpecData := `{"preferredCPUBindPolicy":"FullPCPUs"}`
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod-1",
			Annotations: map[string]string{AnnotationAllocSpec: string(asiAllocSpecData),
				extension.AnnotationResourceSpec: koordResourceSpecData},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "test-container-1"},
				{Name: "test-container-2"},
			},
		},
	}
	unifiedAllocStatus := `{"cpu":[0,1,2,3],"gpu":{}}`
	asiAllocSpec.Containers[0].Resource.CPU.CPUSet.CPUIDs = []int{0, 1, 2, 3}
	asiAllocSpec.Containers = append(asiAllocSpec.Containers, Container{
		Name: pod.Spec.Containers[1].Name,
		Resource: ResourceRequirements{
			CPU: CPUSpec{
				CPUSet: &CPUSetSpec{
					SpreadStrategy: koordCPUStrategyToASI(koordResourceSpec),
					CPUIDs:         []int{0, 1, 2, 3},
				},
			},
		},
	})
	asiAllocSpecData, err = json.Marshal(asiAllocSpec)
	assert.NoError(t, err)

	err = SetUnifiedResourceStatusIfHasCPUs(pod, &extension.ResourceStatus{
		CPUSet: "0-3",
	})
	assert.NoError(t, err)
	assert.Equal(t, unifiedAllocStatus, pod.Annotations[uniext.AnnotationAllocStatus])
	assert.Equal(t, string(asiAllocSpecData), pod.Annotations[AnnotationAllocSpec])
}
