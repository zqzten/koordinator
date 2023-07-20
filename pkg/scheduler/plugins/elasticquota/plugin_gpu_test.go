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

package elasticquota

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	cosextension "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/util/transformer"
)

var _ framework.SharedLister = &testSharedLister{}

func TestPlugin_OnQuotaAdd2(t *testing.T) {
	suit := newPluginTestSuit(t, nil)
	p, _ := suit.proxyNew(suit.elasticQuotaArgs, suit.Handle)
	pl := p.(*Plugin)
	pl.GetGroupQuotaManager().UpdateClusterTotalResource(createResourceList(501952056, 0))
	gqm := pl.GetGroupQuotaManager()
	quota := CreateQuota2("1", "", 0, 0, 0, 0, 0, 0, false)
	quota.Spec.Min = corev1.ResourceList{
		corev1.ResourceCPU:                    *resource.NewMilliQuantity(100*1000, resource.DecimalSI),
		corev1.ResourceMemory:                 *resource.NewQuantity(1000, resource.BinarySI),
		extension.ResourceNvidiaGPU:           *resource.NewQuantity(8, resource.DecimalSI),
		extension.ResourceNvidiaGPU + "-a100": *resource.NewQuantity(8, resource.DecimalSI),
	}
	quota.Spec.Max = corev1.ResourceList{
		corev1.ResourceCPU:                    *resource.NewMilliQuantity(200*1000, resource.DecimalSI),
		corev1.ResourceMemory:                 *resource.NewQuantity(2000, resource.BinarySI),
		extension.ResourceNvidiaGPU:           *resource.NewQuantity(16, resource.DecimalSI),
		extension.ResourceNvidiaGPU + "-a100": *resource.NewQuantity(16, resource.DecimalSI),
	}
	sharedWeight := corev1.ResourceList{
		corev1.ResourceCPU:                    *resource.NewMilliQuantity(300*1000, resource.DecimalSI),
		corev1.ResourceMemory:                 *resource.NewQuantity(3000, resource.BinarySI),
		extension.ResourceNvidiaGPU:           *resource.NewQuantity(32, resource.DecimalSI),
		extension.ResourceNvidiaGPU + "-a100": *resource.NewQuantity(32, resource.DecimalSI),
	}
	sharedWeightStr, _ := json.Marshal(sharedWeight)
	quota.Annotations[extension.AnnotationSharedWeight] = string(sharedWeightStr)
	transformer.TransformElasticQuotaGPUResourcesToUnifiedCardRatio(quota)
	pl.OnQuotaAdd(quota)

	assert.NotNil(t, gqm.GetQuotaInfoByName("1"))
	quota.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	quota.Name = "2"
	pl.OnQuotaAdd(quota)
	assert.Nil(t, gqm.GetQuotaInfoByName("2"))

	quotaInfo := gqm.GetQuotaInfoByName("1")
	assert.Equal(t, quotaInfo.CalculateInfo.Min, corev1.ResourceList{
		corev1.ResourceCPU:                          *resource.NewMilliQuantity(100*1000, resource.DecimalSI),
		corev1.ResourceMemory:                       *resource.NewQuantity(1000, resource.BinarySI),
		extension.ResourceNvidiaGPU:                 *resource.NewQuantity(8, resource.DecimalSI),
		"nvidia.com/gpu-a100":                       *resource.NewQuantity(8, resource.DecimalSI),
		cosextension.GPUResourceCardRatio:           *resource.NewQuantity(800, resource.DecimalSI),
		cosextension.GPUResourceCardRatio + "-a100": *resource.NewQuantity(800, resource.DecimalSI),
	})
	assert.Equal(t, quotaInfo.CalculateInfo.Max, corev1.ResourceList{
		corev1.ResourceCPU:                          *resource.NewMilliQuantity(200*1000, resource.DecimalSI),
		corev1.ResourceMemory:                       *resource.NewQuantity(2000, resource.BinarySI),
		extension.ResourceNvidiaGPU:                 *resource.NewQuantity(16, resource.DecimalSI),
		"nvidia.com/gpu-a100":                       *resource.NewQuantity(16, resource.DecimalSI),
		cosextension.GPUResourceCardRatio:           *resource.NewQuantity(1600, resource.DecimalSI),
		cosextension.GPUResourceCardRatio + "-a100": *resource.NewQuantity(1600, resource.DecimalSI),
	})
	assert.True(t, v1.Equals(quotaInfo.CalculateInfo.SharedWeight, corev1.ResourceList{
		corev1.ResourceCPU:                          *resource.NewMilliQuantity(300*1000, resource.DecimalSI),
		corev1.ResourceMemory:                       *resource.NewQuantity(3000, resource.BinarySI),
		extension.ResourceNvidiaGPU:                 *resource.NewQuantity(32, resource.DecimalSI),
		"nvidia.com/gpu-a100":                       *resource.NewQuantity(32, resource.DecimalSI),
		cosextension.GPUResourceCardRatio:           *resource.NewQuantity(3200, resource.DecimalSI),
		cosextension.GPUResourceCardRatio + "-a100": *resource.NewQuantity(3200, resource.DecimalSI),
	}))

	//update
	quota = CreateQuota2("1", "", 0, 0, 0, 0, 0, 0, false)
	quota.Spec.Min = corev1.ResourceList{
		corev1.ResourceCPU:                    *resource.NewMilliQuantity(1000*1000, resource.DecimalSI),
		corev1.ResourceMemory:                 *resource.NewQuantity(10000, resource.BinarySI),
		extension.ResourceNvidiaGPU:           *resource.NewQuantity(80, resource.DecimalSI),
		extension.ResourceNvidiaGPU + "-a100": *resource.NewQuantity(80, resource.DecimalSI),
	}
	quota.Spec.Max = corev1.ResourceList{
		corev1.ResourceCPU:                    *resource.NewMilliQuantity(2000*1000, resource.DecimalSI),
		corev1.ResourceMemory:                 *resource.NewQuantity(20000, resource.BinarySI),
		extension.ResourceNvidiaGPU:           *resource.NewQuantity(160, resource.DecimalSI),
		extension.ResourceNvidiaGPU + "-a100": *resource.NewQuantity(160, resource.DecimalSI),
	}
	sharedWeight = corev1.ResourceList{
		corev1.ResourceCPU:                    *resource.NewMilliQuantity(3000*1000, resource.DecimalSI),
		corev1.ResourceMemory:                 *resource.NewQuantity(30000, resource.BinarySI),
		extension.ResourceNvidiaGPU:           *resource.NewQuantity(320, resource.DecimalSI),
		extension.ResourceNvidiaGPU + "-a100": *resource.NewQuantity(320, resource.DecimalSI),
	}
	sharedWeightStr, _ = json.Marshal(sharedWeight)
	quota.Annotations[extension.AnnotationSharedWeight] = string(sharedWeightStr)

	transformer.TransformElasticQuotaGPUResourcesToUnifiedCardRatio(quota)
	pl.OnQuotaUpdate(nil, quota)
	quotaInfo = gqm.GetQuotaInfoByName("1")
	assert.Equal(t, quotaInfo.CalculateInfo.Min, corev1.ResourceList{
		corev1.ResourceCPU:                          *resource.NewMilliQuantity(1000*1000, resource.DecimalSI),
		corev1.ResourceMemory:                       *resource.NewQuantity(10000, resource.BinarySI),
		extension.ResourceNvidiaGPU:                 *resource.NewQuantity(80, resource.DecimalSI),
		"nvidia.com/gpu-a100":                       *resource.NewQuantity(80, resource.DecimalSI),
		cosextension.GPUResourceCardRatio:           *resource.NewQuantity(8000, resource.DecimalSI),
		cosextension.GPUResourceCardRatio + "-a100": *resource.NewQuantity(8000, resource.DecimalSI),
	})
	assert.Equal(t, quotaInfo.CalculateInfo.Max, corev1.ResourceList{
		corev1.ResourceCPU:                          *resource.NewMilliQuantity(2000*1000, resource.DecimalSI),
		corev1.ResourceMemory:                       *resource.NewQuantity(20000, resource.BinarySI),
		extension.ResourceNvidiaGPU:                 *resource.NewQuantity(160, resource.DecimalSI),
		"nvidia.com/gpu-a100":                       *resource.NewQuantity(160, resource.DecimalSI),
		cosextension.GPUResourceCardRatio:           *resource.NewQuantity(16000, resource.DecimalSI),
		cosextension.GPUResourceCardRatio + "-a100": *resource.NewQuantity(16000, resource.DecimalSI),
	})
	assert.True(t, v1.Equals(quotaInfo.CalculateInfo.SharedWeight, corev1.ResourceList{
		corev1.ResourceCPU:                          *resource.NewMilliQuantity(3000*1000, resource.DecimalSI),
		corev1.ResourceMemory:                       *resource.NewQuantity(30000, resource.BinarySI),
		extension.ResourceNvidiaGPU:                 *resource.NewQuantity(320, resource.DecimalSI),
		"nvidia.com/gpu-a100":                       *resource.NewQuantity(320, resource.DecimalSI),
		cosextension.GPUResourceCardRatio:           *resource.NewQuantity(32000, resource.DecimalSI),
		cosextension.GPUResourceCardRatio + "-a100": *resource.NewQuantity(32000, resource.DecimalSI),
	}))
}

func TestPlugin_OnNodeAdd2(t *testing.T) {
	tests := []struct {
		name     string
		nodes    []*corev1.Node
		totalRes corev1.ResourceList
	}{
		{
			name: "add gpu node",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							extension.LabelGPUModel: "A100",
						},
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:               *resource.NewMilliQuantity(100*1000, resource.DecimalSI),
							corev1.ResourceMemory:            *resource.NewQuantity(1000, resource.BinarySI),
							cosextension.GPUResourceMemRatio: *resource.NewQuantity(800, resource.DecimalSI),
						},
					},
				},
			},
			totalRes: corev1.ResourceList{
				corev1.ResourceCPU:                          *resource.NewMilliQuantity(100*1000, resource.DecimalSI),
				corev1.ResourceMemory:                       *resource.NewQuantity(1000, resource.BinarySI),
				extension.ResourceNvidiaGPU:                 *resource.NewQuantity(8, resource.DecimalSI),
				cosextension.GPUResourceMemRatio:            *resource.NewQuantity(800, resource.DecimalSI),
				cosextension.GPUResourceCardRatio:           *resource.NewQuantity(800, resource.DecimalSI),
				cosextension.GPUResourceCardRatio + "-a100": *resource.NewQuantity(800, resource.DecimalSI),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nil)
			p, err := suit.proxyNew(suit.elasticQuotaArgs, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)
			eQP := p.(*Plugin)
			for _, node := range tt.nodes {
				transformer.TransformNodeAllocatableWithUnifiedGPUMemoryRatio(node)
				transformer.TransformNodeAllocatableToUnifiedCardRatio(node)
				eQP.OnNodeAdd(node)
			}
			gqm := eQP.GetGroupQuotaManager()
			assert.NotNil(t, gqm)
			assert.Equal(t, tt.totalRes, gqm.GetClusterTotalResource())
		})
	}
}

func TestPlugin_OnNodeUpdate2(t *testing.T) {
	nodes := []*corev1.Node{defaultCreateNodeWithResourceVersion("1"), defaultCreateNodeWithResourceVersion("2"),
		defaultCreateNodeWithResourceVersion("3")}
	tests := []struct {
		name     string
		nodes    []*corev1.Node
		totalRes corev1.ResourceList
	}{
		{
			name: "add gpu node",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "1",
						Labels: map[string]string{
							extension.LabelGPUModel: "A100",
						},
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:               *resource.NewMilliQuantity(50*1000, resource.DecimalSI),
							corev1.ResourceMemory:            *resource.NewQuantity(500, resource.BinarySI),
							cosextension.GPUResourceMemRatio: *resource.NewQuantity(800, resource.DecimalSI),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "2",
						Labels: map[string]string{
							extension.LabelGPUModel: "A100",
						},
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:               *resource.NewMilliQuantity(50*1000, resource.DecimalSI),
							corev1.ResourceMemory:            *resource.NewQuantity(500, resource.BinarySI),
							cosextension.GPUResourceMemRatio: *resource.NewQuantity(800, resource.DecimalSI),
						},
					},
				},
			},
			totalRes: corev1.ResourceList{
				corev1.ResourceCPU:                          *resource.NewMilliQuantity(200*1000, resource.DecimalSI),
				corev1.ResourceMemory:                       *resource.NewQuantity(2000, resource.BinarySI),
				extension.ResourceNvidiaGPU:                 *resource.NewQuantity(16, resource.DecimalSI),
				cosextension.GPUResourceMemRatio:            *resource.NewQuantity(1600, resource.DecimalSI),
				cosextension.GPUResourceCardRatio:           *resource.NewQuantity(1600, resource.DecimalSI),
				cosextension.GPUResourceCardRatio + "-a100": *resource.NewQuantity(1600, resource.DecimalSI),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nil)
			p, _ := suit.proxyNew(suit.elasticQuotaArgs, suit.Handle)
			plugin := p.(*Plugin)
			for _, node := range nodes {
				transformer.TransformNodeAllocatableWithUnifiedGPUMemoryRatio(node)
				transformer.TransformNodeAllocatableToUnifiedCardRatio(node)
				plugin.OnNodeAdd(node)
			}
			for i, node := range tt.nodes {
				transformer.TransformNodeAllocatableWithUnifiedGPUMemoryRatio(node)
				transformer.TransformNodeAllocatableToUnifiedCardRatio(node)
				plugin.OnNodeUpdate(nodes[i], node)
			}
			assert.Equal(t, p.(*Plugin).GetGroupQuotaManager().GetClusterTotalResource(), tt.totalRes)
		})
	}
}

func TestPlugin_OnNodeDelete2(t *testing.T) {
	suit := newPluginTestSuit(t, nil)
	p, err := suit.proxyNew(suit.elasticQuotaArgs, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)
	eQP := p.(*Plugin)
	gqp := eQP.GetGroupQuotaManager()
	assert.NotNil(t, gqp)
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "1",
				Labels: map[string]string{
					extension.LabelGPUModel: "A100",
				},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:               *resource.NewMilliQuantity(25*1000, resource.DecimalSI),
					corev1.ResourceMemory:            *resource.NewQuantity(250, resource.BinarySI),
					cosextension.GPUResourceMemRatio: *resource.NewQuantity(400, resource.DecimalSI),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "2",
				Labels: map[string]string{
					extension.LabelGPUModel: "A100",
				},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:               *resource.NewMilliQuantity(25*1000, resource.DecimalSI),
					corev1.ResourceMemory:            *resource.NewQuantity(250, resource.BinarySI),
					cosextension.GPUResourceMemRatio: *resource.NewQuantity(400, resource.DecimalSI),
				},
			},
		},
	}
	for _, node := range nodes {
		transformer.TransformNodeAllocatableWithUnifiedGPUMemoryRatio(node)
		transformer.TransformNodeAllocatableToUnifiedCardRatio(node)
		eQP.OnNodeAdd(node)
	}
	for i, node := range nodes {
		transformer.TransformNodeAllocatableWithUnifiedGPUMemoryRatio(node)
		transformer.TransformNodeAllocatableToUnifiedCardRatio(node)
		eQP.OnNodeDelete(node)
		assert.Equal(t, gqp.GetClusterTotalResource(), corev1.ResourceList{
			corev1.ResourceCPU:                          *resource.NewMilliQuantity(int64(50-25*(i+1))*1000, resource.DecimalSI),
			corev1.ResourceMemory:                       *resource.NewQuantity(int64(500-250*(i+1)), resource.BinarySI),
			extension.ResourceNvidiaGPU:                 *resource.NewQuantity(int64(8-4*(i+1)), resource.DecimalSI),
			cosextension.GPUResourceMemRatio:            *resource.NewQuantity(int64(800-400*(i+1)), resource.DecimalSI),
			cosextension.GPUResourceCardRatio:           *resource.NewQuantity(int64(800-400*(i+1)), resource.DecimalSI),
			cosextension.GPUResourceCardRatio + "-a100": *resource.NewQuantity(int64(800-400*(i+1)), resource.DecimalSI),
		})
	}
}
