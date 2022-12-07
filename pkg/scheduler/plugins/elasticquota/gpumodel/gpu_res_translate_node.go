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

package gpumodel

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	apires "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

var GPUResourceMemRatio = extension.GPUMemoryRatio
var GPUResourceCardRatio = unified.GPUCardRatio

var NormalGPUNamesForNode = sets.NewString(
	string(extension.NvidiaGPU),
)

var PercentageGPUNamesForNode = sets.NewString(
	string(GPUResourceMemRatio),
	string(extension.KoordGPU),
)

func MaxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func NodeAllocatableContainsGpu(allocatable v1.ResourceList) bool {
	for rname := range NormalGPUNamesForNode.Union(PercentageGPUNamesForNode) {
		if _, ok := allocatable[v1.ResourceName(rname)]; ok {
			return true
		}
	}
	return false
}

func ParseGPUResourcesByModel(nodeName string, allocatable v1.ResourceList, nodeLabels map[string]string) v1.ResourceList {
	gpuModel := GetNodeGPUModel(nodeLabels)
	if gpuModel == "" {
		klog.Warningf("node %s reports gpu resource in allocatable but has not gpu model", nodeName)
		return allocatable
	}

	result := NormalizeGPUResourcesToCardRatioForNode(allocatable, gpuModel)
	return result
}

// NormalizeGPUResourcesToCardRatioForNode 从node上解析的gpu资源最终应该翻译成：alibabacloud.com/gpu-mem-ratio->
// alibabacloud.com/gpu-card-ratio-tesla-v100-sxm2-16gb
func NormalizeGPUResourcesToCardRatioForNode(res v1.ResourceList, gpuModel string) v1.ResourceList {
	if len(res) == 0 || gpuModel == "" {
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
	res[GPUResourceCardRatio] = *q
	res[v1.ResourceName(strings.ToLower(fmt.Sprintf("%s-%s", GPUResourceCardRatio, gpuModel)))] = *q
	return res
}
