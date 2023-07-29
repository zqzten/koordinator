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
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func mustAllocateGPUByPartition(node *corev1.Node) bool {
	model := node.Labels[apiext.LabelGPUModel]
	_, ok := unified.PartitionTables[model]
	return ok
}

func getGPUPartitionTable(node *corev1.Node) map[int][]unified.GPUPartition {
	model := node.Labels[apiext.LabelGPUModel]
	return unified.PartitionTables[model]
}
