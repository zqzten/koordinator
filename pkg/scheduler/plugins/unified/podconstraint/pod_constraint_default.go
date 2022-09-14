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
	"fmt"
	"strings"

	unischeduling "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const defaultConstraintPrefix = "__default__"

func GetDefaultPodConstraintName(spreadUnit string) string {
	return fmt.Sprintf("%s__spread__%s", defaultConstraintPrefix, spreadUnit)
}

func IsPodConstraintDefault(constraint *unischeduling.PodConstraint) bool {
	return IsConstraintNameDefault(constraint.Name)
}

func IsConstraintNameDefault(constraintName string) bool {
	return strings.HasPrefix(constraintName, defaultConstraintPrefix)
}

func BuildDefaultPodConstraint(namespace, name string, required bool) *unischeduling.PodConstraint {
	if required {
		return buildRequiredDefaultPodConstraint(namespace, name)
	} else {
		return buildAffinityDefaultPodConstraint(namespace, name)
	}
}

func buildRequiredDefaultPodConstraint(namespace, name string) *unischeduling.PodConstraint {
	minNumOfTopologyValue := int32(2)
	return &unischeduling.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: unischeduling.PodConstraintSpec{
			SpreadRule: unischeduling.SpreadRule{
				Requires: []unischeduling.SpreadRuleItem{
					{
						TopologyKey:      corev1.LabelTopologyZone,
						PodSpreadType:    unischeduling.PodSpreadTypeDefault,
						MaxSkew:          1,
						MinTopologyValue: &minNumOfTopologyValue,
					},
				},
				Affinities: []unischeduling.SpreadRuleItem{
					{
						TopologyKey:   corev1.LabelHostname,
						PodSpreadType: unischeduling.PodSpreadTypeDefault,
						MaxSkew:       1,
					},
				},
			},
		},
	}
}

func buildAffinityDefaultPodConstraint(namespace, name string) *unischeduling.PodConstraint {
	return &unischeduling.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: unischeduling.PodConstraintSpec{
			SpreadRule: unischeduling.SpreadRule{
				Affinities: []unischeduling.SpreadRuleItem{
					{
						TopologyKey:   corev1.LabelHostname,
						PodSpreadType: unischeduling.PodSpreadTypeDefault,
						MaxSkew:       1,
					},
					{
						TopologyKey:   corev1.LabelTopologyZone,
						PodSpreadType: unischeduling.PodSpreadTypeDefault,
						MaxSkew:       1,
					},
				},
			},
		},
	}
}
