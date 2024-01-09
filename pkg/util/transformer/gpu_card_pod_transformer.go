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

var NormalGPUNamesForPod = sets.NewString(
	string(extension.ResourceNvidiaGPU),
)

var PercentageGPUNamesForPod = sets.NewString(
	string(extension.ResourceGPUMemoryRatio),
	string(extension.ResourceGPUCore),
	string(extension.ResourceGPU),
	cosextension.GPUResourceMemRatio,
	cosextension.GPUResourceCore,
	cosextension.GPUResourceAlibaba,
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
	res[cosextension.GPUResourceCardRatio] = *q
	if gpuModel != "" {
		res[v1.ResourceName(strings.ToLower(fmt.Sprintf("%s-%s", cosextension.GPUResourceCardRatio, gpuModel)))] = *q
	}

	return res
}

// GetPodGPUModel if the pod is assigned, get model from GPUModelCache
func GetPodGPUModel(pod *v1.Pod) string {
	if pod == nil {
		return ""
	}

	if pod.Spec.NodeSelector != nil {
		if model, ok := pod.Spec.NodeSelector[extension.LabelGPUModel]; ok {
			return model
		}
	}

	if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil &&
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil &&
		len(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) > 0 {
		for i := range pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			term := &pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[i]
			for ei := range term.MatchExpressions {
				e := term.MatchExpressions[ei]
				gpuModel := ""
				if e.Key == extension.LabelGPUModel && e.Operator == v1.NodeSelectorOpIn && len(e.Values) == 1 {
					gpuModel = e.Values[0]
				}
				if gpuModel != "" {
					return gpuModel
				}
			}
		}
	}
	return ""
}
