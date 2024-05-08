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
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/thirdparty/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
)

const (
	LabelInstanceType      = "alibabacloud.com/instance-type"
	BestEffortInstanceType = "best-effort"
)

func init() {
	elasticQuotaTransformers = append(elasticQuotaTransformers,
		TransformElasticQuotaGPUResourcesToUnifiedCardRatio, TransformElasticQuotaForACS)
}

func TransformElasticQuotaGPUResourcesToUnifiedCardRatio(quota *v1alpha1.ElasticQuota) {
	quota.Spec.Max = NormalizeGpuResourcesToCardRatioForQuota(quota.Spec.Max)
	quota.Spec.Min = NormalizeGpuResourcesToCardRatioForQuota(quota.Spec.Min)
	if quota.Annotations != nil {
		quota.Annotations[extension.AnnotationSharedWeight] = NormalizeGpuResourcesToCardRatioForQuotaAnnotations(
			quota.Annotations[extension.AnnotationSharedWeight])
	}
}

func TransformElasticQuotaForACS(quota *v1alpha1.ElasticQuota) {
	if quota.Labels[LabelInstanceType] != BestEffortInstanceType {
		return
	}

	// convert max.
	replaceAndEraseResource(quota.Spec.Max, corev1.ResourceCPU, extension.BatchCPU)
	replaceAndEraseResource(quota.Spec.Max, corev1.ResourceMemory, extension.BatchMemory)

	// convert min.
	replaceAndEraseResource(quota.Spec.Min, corev1.ResourceCPU, extension.BatchCPU)
	replaceAndEraseResource(quota.Spec.Min, corev1.ResourceMemory, extension.BatchMemory)

	convertQuotaResourceAnnotations(quota, extension.AnnotationSharedWeight)
	convertQuotaResourceAnnotations(quota, extension.AnnotationTotalResource)
}

func convertQuotaResourceAnnotations(quota *v1alpha1.ElasticQuota, key string) {
	val := quota.Annotations[key]
	if val == "" {
		return
	}
	res := corev1.ResourceList{}
	err := json.Unmarshal([]byte(val), &res)
	if err == nil {
		replaceAndEraseResource(res, corev1.ResourceCPU, extension.BatchCPU)
		replaceAndEraseResource(res, corev1.ResourceMemory, extension.BatchMemory)
	}
	data, err := json.Marshal(res)
	if err == nil {
		quota.Annotations[key] = string(data)
	}
}
