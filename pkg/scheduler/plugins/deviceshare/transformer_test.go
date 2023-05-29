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

package deviceshare

import (
	"testing"

	"github.com/stretchr/testify/assert"
	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func Test_transformNodeInfoAllocatable(t *testing.T) {
	tests := []struct {
		name                string
		nodeAllocatable     corev1.ResourceList
		numGPU              int
		wantResult          bool
		wantScalarResources map[corev1.ResourceName]int64
	}{
		{
			name: "kubelet reports 0 gpu-mem-ratio but device CRD obj has 1 GPU",
			nodeAllocatable: corev1.ResourceList{
				unifiedresourceext.GPUResourceMemRatio: resource.MustParse("0"),
			},
			numGPU:     1,
			wantResult: false,
			wantScalarResources: map[corev1.ResourceName]int64{
				unifiedresourceext.GPUResourceMemRatio: 0,
			},
		},
		{
			name: "kubelet and device-plugins both report 2 GPU",
			nodeAllocatable: corev1.ResourceList{
				unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
			},
			numGPU:     2,
			wantResult: true,
			wantScalarResources: map[corev1.ResourceName]int64{
				unifiedresourceext.GPUResourceMemRatio: 200,
				unifiedresourceext.GPUResourceCore:     200,
				unifiedresourceext.GPUResourceMem:      2 * 4 * 1024 * 1024 * 1024,
				apiext.ResourceNvidiaGPU:               2,
			},
		},
		{
			name: "kubelet reports gpu-mem-ratio 200 but device has 100",
			nodeAllocatable: corev1.ResourceList{
				unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
			},
			numGPU:     1,
			wantResult: true,
			wantScalarResources: map[corev1.ResourceName]int64{
				unifiedresourceext.GPUResourceMemRatio: 200,
				unifiedresourceext.GPUResourceCore:     100,
				unifiedresourceext.GPUResourceMem:      4 * 1024 * 1024 * 1024,
				apiext.ResourceNvidiaGPU:               1,
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
			nodeInfo := framework.NewNodeInfo()
			nodeInfo.SetNode(node)

			device := newNodeDevice()
			if tt.numGPU > 0 {
				resources := deviceResources{}
				for i := 0; i < tt.numGPU; i++ {
					resources[i] = corev1.ResourceList{
						apiext.ResourceGPUCore:        resource.MustParse("100"),
						apiext.ResourceGPUMemoryRatio: resource.MustParse("100"),
						apiext.ResourceGPUMemory:      resource.MustParse("4Gi"),
					}
				}
				device.resetDeviceTotal(map[schedulingv1alpha1.DeviceType]deviceResources{schedulingv1alpha1.GPU: resources})
			}

			got := transformNodeInfoAllocatable(nodeInfo, device)
			assert.Equal(t, tt.wantResult, got)
			assert.Equal(t, tt.wantScalarResources, nodeInfo.Allocatable.ScalarResources)
		})
	}
}
