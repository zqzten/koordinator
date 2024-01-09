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
	"fmt"
	"strings"

	cosextension "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	v1 "k8s.io/api/core/v1"
	apires "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	DeprecatedLabelNodeGPUModel = extension.DomainPrefix + "gpu-model"
	GpuModelLabel               = "alibabacloud.com/gpu-card-model"
	GpuModelDetailLabel         = "alibabacloud.com/gpu-card-model-detail"
)

var NormalGPUNamesForNode = sets.NewString(
	string(extension.ResourceNvidiaGPU),
)

var PercentageGPUNamesForNode = sets.NewString(
	string(extension.ResourceGPUMemoryRatio),
	string(extension.ResourceGPU),
)

func MaxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func GetNodeGPUModel(nodeLabels map[string]string) string {
	if nodeLabels[extension.LabelGPUModel] != "" {
		return nodeLabels[extension.LabelGPUModel]
	}
	if nodeLabels[DeprecatedLabelNodeGPUModel] != "" {
		return nodeLabels[DeprecatedLabelNodeGPUModel]
	}
	if nodeLabels[GpuModelDetailLabel] != "" {
		return nodeLabels[GpuModelDetailLabel]
	}
	return nodeLabels[GpuModelLabel]
}

func NodeAllocatableContainsGPU(allocatable v1.ResourceList) bool {
	for rname := range NormalGPUNamesForNode.Union(PercentageGPUNamesForNode) {
		if _, ok := allocatable[v1.ResourceName(rname)]; ok {
			return true
		}
	}
	return false
}

func ParseGPUResourcesByModel(allocatable v1.ResourceList, nodeLabels map[string]string) v1.ResourceList {
	gpuModel := GetNodeGPUModel(nodeLabels)
	result := NormalizeGPUResourcesToCardRatioForNode(allocatable, gpuModel)
	return result
}

func NormalizeGPUResourcesToCardRatioForNode(res v1.ResourceList, gpuModel string) v1.ResourceList {
	if len(res) == 0 {
		return res
	}

	var quantity int64
	for rname, q := range res {
		var transRatio int64
		if NormalGPUNamesForNode.Has(string(rname)) {
			transRatio = q.Value() * 100
		}
		if PercentageGPUNamesForNode.Has(string(rname)) {
			transRatio = q.Value()
		}
		if transRatio <= 0 {
			continue
		}
		quantity = MaxInt64(quantity, transRatio)
	}

	if quantity <= 0 {
		return res
	}

	q := apires.NewQuantity(quantity, apires.DecimalSI)
	res[cosextension.GPUResourceCardRatio] = *q
	if gpuModel != "" {
		res[v1.ResourceName(strings.ToLower(fmt.Sprintf("%s-%s", cosextension.GPUResourceCardRatio, gpuModel)))] = *q
	}
	return res
}
