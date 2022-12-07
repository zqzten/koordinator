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

	"github.com/koordinator-sh/koordinator/apis/extension"
)

var GPUResourceCore = extension.GPUCore

var NormalGPUNamesForPod = sets.NewString(
	string(extension.NvidiaGPU),
)

var PercentageGPUNamesForPod = sets.NewString(
	string(GPUResourceMemRatio),
	string(GPUResourceCore),
	string(extension.KoordGPU),
)

func NormalizeGPUResourcesToCardRatioForPod(res v1.ResourceList, gpuModel string) v1.ResourceList {
	var gpuCardRatio int64
	for rname, q := range res {
		var transRatio int64
		if NormalGPUNamesForPod.Has(string(rname)) {
			transRatio = q.Value() * 100
		}
		if PercentageGPUNamesForPod.Has(string(rname)) {
			transRatio = q.Value()
		}
		if transRatio <= 0 {
			continue
		}
		gpuCardRatio = MaxInt64(gpuCardRatio, transRatio)
		if gpuModel != "" {
			res[v1.ResourceName(strings.ToLower(fmt.Sprintf("%s-%s", rname, gpuModel)))] = q.DeepCopy()
		}
	}
	if gpuCardRatio <= 0 {
		return res
	}

	// 填充card-ratio的值
	q := apires.NewQuantity(gpuCardRatio, apires.DecimalSI)
	res[GPUResourceCardRatio] = *q
	if gpuModel != "" {
		res[v1.ResourceName(strings.ToLower(fmt.Sprintf("%s-%s", GPUResourceCardRatio, gpuModel)))] = *q
	}

	return res
}
