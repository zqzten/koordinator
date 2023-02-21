package podconstraint

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/podconstraint/cache"
)

func countPodsMatchConstraint(podInfos []*framework.PodInfo, constraintNameSpace, constraintName string) int {
	count := 0
	for _, p := range podInfos {
		if podHasConstraint(p.Pod, constraintNameSpace, constraintName) {
			count++
		}
	}
	return count
}

func podHasConstraint(pod *corev1.Pod, constraintNameSpace, constraintName string) bool {
	// Bypass terminating Pod (see #87621).
	if pod.DeletionTimestamp != nil {
		return false
	}

	if weightedPodConstraints := extunified.GetWeightedPodConstraints(pod); len(weightedPodConstraints) != 0 {
		for _, weightedPodConstraint := range weightedPodConstraints {
			if weightedPodConstraint.Namespace == constraintNameSpace && weightedPodConstraint.Name == constraintName {
				return true
			}
		}
	} else if weightedSpreadUnits := extunified.GetWeighedSpreadUnits(pod); len(weightedSpreadUnits) != 0 {
		for _, weightedSpreadUnit := range weightedSpreadUnits {
			if weightedSpreadUnit.Name == "" {
				continue
			}
			defaultPodConstraintName := cache.GetDefaultPodConstraintName(weightedSpreadUnit.Name)
			if pod.Namespace == constraintNameSpace && defaultPodConstraintName == constraintName {
				return true
			}
		}
	}
	return false
}
