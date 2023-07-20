package transformer

import (
	"sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

func init() {
	elasticQuotaTransformers = append(elasticQuotaTransformers,
		TransformElasticQuotaGPUResourcesToUnifiedCardRatio)
}

func TransformElasticQuotaGPUResourcesToUnifiedCardRatio(quota *v1alpha1.ElasticQuota) {
	quota.Spec.Max = NormalizeGpuResourcesToCardRatioForQuota(quota.Spec.Max)
	quota.Spec.Min = NormalizeGpuResourcesToCardRatioForQuota(quota.Spec.Min)
	if quota.Annotations != nil {
		quota.Annotations[extension.AnnotationSharedWeight] = NormalizeGpuResourcesToCardRatioForQuotaAnnotations(
			quota.Annotations[extension.AnnotationSharedWeight])
	}
}
