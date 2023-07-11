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
	corev1 "k8s.io/api/core/v1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

// 一些高级的 NVIDIA GPU 型号要求按照一定的组合关系才能获得最佳的 NVLINK 联通带宽。而这种组合关系称为 Partition。
// 分配时也要按照这个 Partition 分配，Partition 之外的组合关系是无效的，调度器会拒绝调度。

type GPUPartition struct {
	ID           int
	NumberOfGPUs int
	ModuleIDs    []int
}

var H800PartitionTables = map[int][]GPUPartition{
	8: {
		{ID: 0, NumberOfGPUs: 8, ModuleIDs: []int{1, 2, 3, 4, 5, 6, 7, 8}},
	},
	4: {
		{ID: 1, NumberOfGPUs: 4, ModuleIDs: []int{1, 2, 3, 4}},
		{ID: 2, NumberOfGPUs: 4, ModuleIDs: []int{5, 6, 7, 8}},
	},
	2: {
		{ID: 3, NumberOfGPUs: 2, ModuleIDs: []int{1, 3}},
		{ID: 4, NumberOfGPUs: 2, ModuleIDs: []int{2, 4}},
		{ID: 5, NumberOfGPUs: 2, ModuleIDs: []int{5, 7}},
		{ID: 6, NumberOfGPUs: 2, ModuleIDs: []int{6, 8}},
	},
	1: {
		// keep the following order to reduce fragments
		{ID: 7, NumberOfGPUs: 1, ModuleIDs: []int{1}},
		{ID: 8, NumberOfGPUs: 1, ModuleIDs: []int{3}},
		{ID: 9, NumberOfGPUs: 1, ModuleIDs: []int{2}},
		{ID: 10, NumberOfGPUs: 1, ModuleIDs: []int{4}},
		{ID: 11, NumberOfGPUs: 1, ModuleIDs: []int{5}},
		{ID: 12, NumberOfGPUs: 1, ModuleIDs: []int{7}},
		{ID: 13, NumberOfGPUs: 1, ModuleIDs: []int{6}},
		{ID: 14, NumberOfGPUs: 1, ModuleIDs: []int{8}},
	},
}

var PartitionTables = map[string]map[int][]GPUPartition{
	"H800": H800PartitionTables,
}

func mustAllocateGPUByPartition(node *corev1.Node) bool {
	model := node.Labels[apiext.LabelGPUModel]
	_, ok := PartitionTables[model]
	return ok
}

func getGPUPartitionTable(node *corev1.Node) map[int][]GPUPartition {
	model := node.Labels[apiext.LabelGPUModel]
	return PartitionTables[model]
}
