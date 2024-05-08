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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetAllocatableByOverQuota(t *testing.T) {
	tests := []struct {
		name            string
		nodeLabels      map[string]string
		nodeAllocatable corev1.ResourceList
		want            corev1.ResourceList
	}{
		{
			name: "cpu 1.5, and memory 2",
			nodeLabels: map[string]string{
				LabelCPUOverQuota:    "1.5",
				LabelMemoryOverQuota: "2",
			},
			nodeAllocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
			want: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(3000, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(4*1024*1024*1024, resource.BinarySI),
			},
		},
		{
			name: "cpu 1.5, and memory 2 with new api",
			nodeLabels: map[string]string{
				LabelAlibabaCPUOverQuota:    "1.5",
				LabelAlibabaMemoryOverQuota: "2",
			},
			nodeAllocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
			want: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(3000, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(4*1024*1024*1024, resource.BinarySI),
			},
		},
		{
			name: "old and new api mixed but different values, use new api first",
			nodeLabels: map[string]string{
				LabelAlibabaCPUOverQuota:    "2",
				LabelAlibabaMemoryOverQuota: "1.5",
				LabelCPUOverQuota:           "1.5",
				LabelMemoryOverQuota:        "2",
			},
			nodeAllocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
			want: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(4000, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(3*1024*1024*1024, resource.BinarySI),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeAllocatable := tt.nodeAllocatable.DeepCopy()
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node",
					Labels: tt.nodeLabels,
				},
				Status: corev1.NodeStatus{
					Allocatable: nodeAllocatable,
				},
			}
			allocatable := GetAllocatableByOverQuota(node)
			assert.Equal(t, tt.want, allocatable)
		})
	}
}

func TestGetResourceOverQuotaSpec(t *testing.T) {
	tests := []struct {
		name            string
		labels          map[string]string
		wantCPURatio    int64
		wantMemoryRatio int64
		wantDiskRatio   int64
	}{
		{
			name:            "no labels",
			wantCPURatio:    100,
			wantMemoryRatio: 100,
			wantDiskRatio:   100,
		},
		{
			name: "old sigma apis",
			labels: map[string]string{
				LabelCPUOverQuota:    "1.5",
				LabelMemoryOverQuota: "2",
				LabelDiskOverQuota:   "3",
			},
			wantCPURatio:    150,
			wantMemoryRatio: 200,
			wantDiskRatio:   300,
		},
		{
			name: "new ACS apis",
			labels: map[string]string{
				LabelAlibabaCPUOverQuota:    "1.5",
				LabelAlibabaMemoryOverQuota: "2",
				LabelAlibabaDiskOverQuota:   "3",
			},
			wantCPURatio:    150,
			wantMemoryRatio: 200,
			wantDiskRatio:   300,
		},
		{
			name: "mix old and new apis",
			labels: map[string]string{
				LabelAlibabaCPUOverQuota:    "1.5",
				LabelAlibabaMemoryOverQuota: "2",
				LabelAlibabaDiskOverQuota:   "3",
				LabelCPUOverQuota:           "1.2",
				LabelMemoryOverQuota:        "1.3",
				LabelDiskOverQuota:          "1.4",
			},
			wantCPURatio:    150,
			wantMemoryRatio: 200,
			wantDiskRatio:   300,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCPURatio, gotMemoryRatio, gotDiskRatio := GetResourceOverQuotaSpec(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: tt.labels,
				},
			})
			assert.Equal(t, tt.wantCPURatio, gotCPURatio)
			assert.Equal(t, tt.wantMemoryRatio, gotMemoryRatio)
			assert.Equal(t, tt.wantDiskRatio, gotDiskRatio)
		})
	}
}

func TestGetSigmaResourceOverQuotaSpec(t *testing.T) {
	tests := []struct {
		name            string
		labels          map[string]string
		wantCPURatio    int64
		wantMemoryRatio int64
		wantDiskRatio   int64
	}{
		{
			name:            "no labels",
			wantCPURatio:    100,
			wantMemoryRatio: 100,
			wantDiskRatio:   100,
		},
		{
			name: "old sigma apis",
			labels: map[string]string{
				LabelCPUOverQuota:    "1.5",
				LabelMemoryOverQuota: "2",
				LabelDiskOverQuota:   "3",
			},
			wantCPURatio:    150,
			wantMemoryRatio: 200,
			wantDiskRatio:   300,
		},
		{
			name: "new ACS apis",
			labels: map[string]string{
				LabelAlibabaCPUOverQuota:    "1.5",
				LabelAlibabaMemoryOverQuota: "2",
				LabelAlibabaDiskOverQuota:   "3",
			},
			wantCPURatio:    100,
			wantMemoryRatio: 100,
			wantDiskRatio:   100,
		},
		{
			name: "mix old and new apis",
			labels: map[string]string{
				LabelAlibabaCPUOverQuota:    "1.5",
				LabelAlibabaMemoryOverQuota: "2",
				LabelAlibabaDiskOverQuota:   "3",
				LabelCPUOverQuota:           "1.2",
				LabelMemoryOverQuota:        "1.3",
				LabelDiskOverQuota:          "1.4",
			},
			wantCPURatio:    120,
			wantMemoryRatio: 130,
			wantDiskRatio:   140,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCPURatio, gotMemoryRatio, gotDiskRatio := GetSigmaResourceOverQuotaSpec(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: tt.labels,
				},
			})
			assert.Equal(t, tt.wantCPURatio, gotCPURatio)
			assert.Equal(t, tt.wantMemoryRatio, gotMemoryRatio)
			assert.Equal(t, tt.wantDiskRatio, gotDiskRatio)
		})
	}
}

func TestGetAlibabaResourceOverQuotaSpec(t *testing.T) {
	tests := []struct {
		name            string
		labels          map[string]string
		wantCPURatio    int64
		wantMemoryRatio int64
		wantDiskRatio   int64
	}{
		{
			name:            "no labels",
			wantCPURatio:    100,
			wantMemoryRatio: 100,
			wantDiskRatio:   100,
		},
		{
			name: "old sigma apis",
			labels: map[string]string{
				LabelCPUOverQuota:    "1.5",
				LabelMemoryOverQuota: "2",
				LabelDiskOverQuota:   "3",
			},
			wantCPURatio:    100,
			wantMemoryRatio: 100,
			wantDiskRatio:   100,
		},
		{
			name: "new ACS apis",
			labels: map[string]string{
				LabelAlibabaCPUOverQuota:    "1.5",
				LabelAlibabaMemoryOverQuota: "2",
				LabelAlibabaDiskOverQuota:   "3",
			},
			wantCPURatio:    150,
			wantMemoryRatio: 200,
			wantDiskRatio:   300,
		},
		{
			name: "mix old and new apis",
			labels: map[string]string{
				LabelAlibabaCPUOverQuota:    "1.5",
				LabelAlibabaMemoryOverQuota: "2",
				LabelAlibabaDiskOverQuota:   "3",
				LabelCPUOverQuota:           "1.2",
				LabelMemoryOverQuota:        "1.3",
				LabelDiskOverQuota:          "1.4",
			},
			wantCPURatio:    150,
			wantMemoryRatio: 200,
			wantDiskRatio:   300,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCPURatio, gotMemoryRatio, gotDiskRatio := GetAlibabaResourceOverQuotaSpec(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: tt.labels,
				},
			})
			assert.Equal(t, tt.wantCPURatio, gotCPURatio)
			assert.Equal(t, tt.wantMemoryRatio, gotMemoryRatio)
			assert.Equal(t, tt.wantDiskRatio, gotDiskRatio)
		})
	}
}
