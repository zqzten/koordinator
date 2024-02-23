package transformer

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"

	"github.com/koordinator-sh/koordinator/apis/extension"
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
