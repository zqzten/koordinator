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

package unified

import (
	"strings"

	cosextension "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota/gpumodel"
)

func init() {
	elasticquota.GetQuotaName = GetQuotaName
	gpumodel.GPUResourceMemRatio = cosextension.GPUResourceMemRatio
	gpumodel.GPUResourceMem = cosextension.GPUResourceMem
	gpumodel.GPUResourceCardRatio = cosextension.GPUResourceCardRatio
	gpumodel.GPUResourceCore = cosextension.GPUResourceCore
	gpumodel.NormalGPUNamesForNode = NormalGPUNamesForNode
	gpumodel.PercentageGPUNamesForNode = PercentageGPUNamesForNode
	gpumodel.NormalGPUNamesForQuota = NormalGPUNamesForQuota
	gpumodel.NormalGPUNamesForPod = NormalGPUNamesForPod
	gpumodel.PercentageGPUNamesForPod = PercentageGPUNamesForPod
	gpumodel.PercentageGPUNamesForQuota = PercentageGPUNamesForQuota
}

var NormalGPUNamesForNode = sets.NewString(
	cosextension.GPUResourceNvidia,
)

var PercentageGPUNamesForNode = sets.NewString(
	cosextension.GPUResourceMemRatio,
	cosextension.GPUResourceAlibaba,
)

var NormalGPUNamesForQuota = sets.NewString(
	cosextension.GPUResourceNvidia,
)

var PercentageGPUNamesForQuota = sets.NewString(
	cosextension.GPUResourceAlibaba,
	cosextension.GPUResourceCardRatio,
)

var NormalGPUNamesForPod = sets.NewString(
	cosextension.GPUResourceNvidia,
)

var PercentageGPUNamesForPod = sets.NewString(
	cosextension.GPUResourceAlibaba,
	cosextension.GPUResourceMemRatio,
	cosextension.GPUResourceCore,
)

func GetQuotaName(quotaLister v1alpha1.ElasticQuotaLister, pod *v1.Pod) string {
	eqList, err := quotaLister.List(labels.Everything())
	if err != nil {
		return extension.DefaultQuotaName
	}
	for _, eq := range eqList {
		namespaces := eq.Annotations[ack.AnnotationQuotaNamespaces]
		if strings.Contains(namespaces, pod.Namespace) {
			return eq.Name
		}
	}

	list, err := quotaLister.ElasticQuotas(pod.Namespace).List(labels.Everything())
	if err != nil {
		runtime.HandleError(err)
		return extension.DefaultQuotaName
	}
	if len(list) == 0 {
		return extension.DefaultQuotaName
	}
	// todo when elastic quota supports multiple instances in a namespace, modify this
	return list[0].Name
}
