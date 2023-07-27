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
	"testing"

	"github.com/stretchr/testify/assert"
	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func TestTransformNodeAllocatableWithOverQuota(t *testing.T) {
	tests := []struct {
		name                       string
		nodeLabels                 map[string]string
		nodeAllocatable            corev1.ResourceList
		wantTransformedAllocatable corev1.ResourceList
	}{
		{
			name: "cpu 1.5, and memory 2",
			nodeLabels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "2",
			},
			nodeAllocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("2"),
			},
			wantTransformedAllocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("3"),
				corev1.ResourceMemory: resource.MustParse("4"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node",
					Labels: tt.nodeLabels,
				},
				Status: corev1.NodeStatus{
					Allocatable: tt.nodeAllocatable,
				},
			}
			jsonData, err := json.Marshal(tt.nodeAllocatable)
			assert.NoError(t, err)
			TransformNodeAllocatableWithOverQuota(node)
			assert.Equal(t, string(jsonData), node.Annotations[extunified.AnnotationOriginalNodeAllocatable])
			assert.True(t, quotav1.Equals(tt.wantTransformedAllocatable, node.Status.Allocatable))
		})
	}
}

func Test_transformNodeInfoAllocatable(t *testing.T) {
	tests := []struct {
		name                string
		nodeAllocatable     corev1.ResourceList
		wantScalarResources map[corev1.ResourceName]int64
	}{
		{
			name: "kubelet reports 0 gpu-mem-ratio but device CRD obj has 1 GPU",
			nodeAllocatable: corev1.ResourceList{
				unifiedresourceext.GPUResourceMemRatio: resource.MustParse("0"),
			},
			wantScalarResources: map[corev1.ResourceName]int64{
				unifiedresourceext.GPUResourceMemRatio: 0,
			},
		},
		{
			name: "kubelet and device-plugins both report 2 GPU",
			nodeAllocatable: corev1.ResourceList{
				unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
			},
			wantScalarResources: map[corev1.ResourceName]int64{
				unifiedresourceext.GPUResourceMemRatio: 200,
				apiext.ResourceNvidiaGPU:               2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Status: corev1.NodeStatus{
					Allocatable: tt.nodeAllocatable,
				},
			}
			obj, err := TransformNode(node)
			assert.NoError(t, err)
			node = obj.(*corev1.Node)

			nodeInfo := framework.NewNodeInfo()
			nodeInfo.SetNode(node)

			assert.Equal(t, tt.wantScalarResources, nodeInfo.Allocatable.ScalarResources)
		})
	}
}
