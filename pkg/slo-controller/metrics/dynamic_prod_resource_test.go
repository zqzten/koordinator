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

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

func TestDynamicProdResourceCollector(t *testing.T) {
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
				extension.BatchCPU:    resource.MustParse("30000"),
				extension.BatchMemory: resource.MustParse("60Gi"),
			},
		},
	}
	assert.NotPanics(t, func() {
		RecordNodeProdResourceEstimatedOvercommitRatio(testNode, string(corev1.ResourceCPU), 1.0)
		RecordNodeProdResourceEstimatedAllocatable(testNode, string(corev1.ResourceMemory), UnitByte, 100<<30)
		RecordNodeProdResourceReclaimable(testNode, string(corev1.ResourceCPU), UnitCore, 16)
	})
}
