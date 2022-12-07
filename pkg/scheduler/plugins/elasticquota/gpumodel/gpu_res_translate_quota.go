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
	"encoding/json"
	"sort"
	"strings"

	v1 "k8s.io/api/core/v1"
	apires "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

var NormalGPUNamesForQuota = sets.NewString(
	string(extension.NvidiaGPU),
)

var PercentageGPUNamesForQuota = sets.NewString(
	string(GPUResourceCardRatio),
	string(extension.KoordGPU),
)

//解析quota的配置维度尝试翻译成card-ratio：
//nvida/gpu:20                      ------>  alibabacloud.com/gpu-card-ratio:2000
//alibabacloud.com/gpu:20           ------>  alibabacloud.com/gpu-card-ratio:2000
//nvida/gpu-v100-16gb               ------>  alibabacloud.com/gpu-card-ratio-v100-16gb:2000
//alibabacloud.com/gpu-v100-16gb:20 ------>  alibabacloud.com/gpu-card-ratio-v100-16gb:2000
//老的key不删掉，仅用于显示使用
//我们暂时不考虑quota组直接配置gpucore\membyte\memratio等，目前以及可见未来没有这种需求，我们暂时也不额外处理
func NormalizeGpuResourcesToCardRatioForQuota(res v1.ResourceList) v1.ResourceList {
	if len(res) == 0 {
		return res
	}

	resCopy := res.DeepCopy()
	cardRatioValue := int64(0)

	for rName, q := range res {
		resName := string(rName)
		if NormalGPUNamesForQuota.Has(resName) {
			cardRatioValue = MaxInt64(cardRatioValue, q.Value()*100)
		}
		if PercentageGPUNamesForQuota.Has(resName) {
			cardRatioValue = MaxInt64(cardRatioValue, q.Value())
		}
	}

	modeledCardRatioSum := int64(0)
	for rname, rquant := range res {
		if prefix, maybe := MaybeGPUResourceWithCardModel(rname, NormalGPUNamesForQuota.Union(PercentageGPUNamesForQuota)); maybe {
			q := rquant.DeepCopy()
			if NormalGPUNamesForQuota.Has(prefix) {
				q.Set(q.Value() * 100)
			}
			newRname := v1.ResourceName(strings.Replace(string(rname), prefix, string(GPUResourceCardRatio), -1))
			resCopy[newRname] = q
			modeledCardRatioSum += q.Value()
		}
	}
	cardRatioValue = MaxInt64(cardRatioValue, modeledCardRatioSum)
	if cardRatioValue > 0 {
		resCopy[GPUResourceCardRatio] = *apires.NewQuantity(cardRatioValue, apires.DecimalSI)
	}
	return resCopy
}

func NormalizeGpuResourcesToCardRatioForQuotaAnnotations(quotaResStr string) string {
	if quotaResStr != "" {
		quotaRes := v1.ResourceList{}
		err := json.Unmarshal([]byte(quotaResStr), &quotaRes)
		if err == nil {
			quotaRes = NormalizeGpuResourcesToCardRatioForQuota(quotaRes)
			newQuotaResStr, _ := json.Marshal(quotaRes)
			return string(newQuotaResStr)
		}
	}

	return quotaResStr
}

// MaybeGPUResourceWithCardModel parses gpu resources suffixed with gpu-model to its gpu-card-ratio
// formulations, e.g:
// nvidia/gpu-tesla-v100 -> alibabacloud.com/gpu-card-ratio-tesla-v100
// alibabacloud.com/gpu-tesla-v100 -> alibabacloud.com/gpu-card-ratio-tesla-v100
func MaybeGPUResourceWithCardModel(rname v1.ResourceName, potentialResourceNames sets.String) (prefix string, maybe bool) {
	if potentialResourceNames.Has(string(rname)) {
		return "", false
	}

	// Ordered by desc length to match longer prefix first.
	rNames := potentialResourceNames.List()
	sort.Slice(rNames, func(i, j int) bool {
		return len(rNames[i]) > len(rNames[j])
	})
	for _, prefixedName := range rNames {
		if strings.HasPrefix(string(rname), prefixedName+"-") {
			return prefixedName, true
		}
	}

	return "", false
}
