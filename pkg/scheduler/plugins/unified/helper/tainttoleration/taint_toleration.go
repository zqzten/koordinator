package tainttoleration

import (
	corev1 "k8s.io/api/core/v1"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func TolerationsToleratesECI(pod *corev1.Pod) []corev1.Toleration {
	var tolerations []corev1.Toleration
	copy(tolerations, pod.Spec.Tolerations)
	tolerations = append(tolerations, corev1.Toleration{
		Key:      extunified.VKTaintKey,
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	})
	return tolerations
}
