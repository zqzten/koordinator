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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	schedulinglisterv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"

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

func GetQuotaName(pod *v1.Pod, quotaLister schedulinglisterv1alpha1.ElasticQuotaLister) string {
	quotaName := extension.GetQuotaName(pod)
	if quotaName != "" {
		return quotaName
	}

	eq, err := quotaLister.ElasticQuotas(pod.Namespace).Get(pod.Namespace)
	if err == nil && eq != nil {
		return eq.Name
	} else if !errors.IsNotFound(err) {
		klog.Errorf("Failed to Get ElasticQuota %s, err: %v", pod.Namespace, err)
	}

	eqList, err := quotaLister.List(labels.Everything())
	if err != nil {
		return extension.DefaultQuotaName
	}
	for _, eq := range eqList {
		namespaces := strings.Split(eq.Annotations[ack.AnnotationQuotaNamespaces], ",")
		for _, namespace := range namespaces {
			if pod.Namespace == namespace {
				return namespace
			}
		}
	}

	return extension.DefaultQuotaName
}
