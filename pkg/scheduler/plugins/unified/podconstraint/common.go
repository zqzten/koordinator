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

package podconstraint

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/podconstraint/cache"
)

func fillSelectorByMatchLabels(pod *corev1.Pod, spreadConstraints []*cache.TopologySpreadConstraint) {
	for i := range spreadConstraints {
		spreadConstraint := spreadConstraints[i]
		matchLabels := make(labels.Set)
		for _, labelKey := range spreadConstraint.MatchLabelKeys {
			if value, ok := pod.Labels[labelKey]; ok {
				matchLabels[labelKey] = value
			}
		}
		if len(matchLabels) > 0 {
			spreadConstraint.Selector = labels.SelectorFromSet(matchLabels)
		}
	}
}

func countPodsMatchConstraint(podInfos []*framework.PodInfo, constraintNameSpace, constraintName string, selector labels.Selector) int {
	count := 0
	for _, podInfo := range podInfos {
		if podHasConstraint(podInfo.Pod, constraintNameSpace, constraintName) && podMatchLabels(podInfo.Pod, selector) {
			count++
		}
	}
	return count
}

func podMatchLabels(pod *corev1.Pod, selector labels.Selector) bool {
	return selector == nil || selector.Matches(labels.Set(pod.Labels))
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
