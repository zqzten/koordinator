package profile

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/quota/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/util/transformer"
)

func init() {
	resourceDecorators = append(resourceDecorators, DecorateResourceByACSQuota)
}

func DecorateResourceByACSQuota(profile *v1alpha1.ElasticQuotaProfile, total corev1.ResourceList) {
	// TODO: import acs vendor.
	switch profile.Labels[transformer.LabelInstanceType] {
	case transformer.BestEffortInstanceType:
		quantity, ok := total[extension.BatchCPU]
		if ok {
			total[corev1.ResourceCPU] = *resource.NewMilliQuantity(quantity.Value(), resource.DecimalSI)
		} else {
			total[corev1.ResourceCPU] = *resource.NewMilliQuantity(0, resource.DecimalSI)
		}

		quantity, ok = total[extension.BatchMemory]
		if ok {
			total[corev1.ResourceMemory] = quantity
		} else {
			total[corev1.ResourceMemory] = *resource.NewQuantity(0, resource.BinarySI)
		}
		delete(total, extension.BatchCPU)
		delete(total, extension.BatchMemory)
		delete(total, extension.MidCPU)
		delete(total, extension.MidMemory)
	case "standard", "exclusive":
		// clean batch/mid resource which change frequently, but we don't need it now.
		delete(total, extension.BatchCPU)
		delete(total, extension.BatchMemory)
		delete(total, extension.MidCPU)
		delete(total, extension.MidMemory)
	}
}
