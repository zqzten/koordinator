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
	"encoding/json"

	sigmak8s "gitlab.alibaba-inc.com/sigma/sigma-k8s-api/pkg/api"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// constraint represent k8s LabelSelector.MatchExpression and maxCount.
type constraint struct {
	key      string
	op       metav1.LabelSelectorOperator
	value    string
	maxCount int64
}

// allocSpecToString json marshal sigmak8s.AllocSpec.
func allocSpecToString(allocSpec *sigmak8s.AllocSpec) string {
	as, err := json.Marshal(allocSpec)
	if err != nil {
		return ""
	}

	return string(as)
}

// allocSpecWithPodAffinity constructs AllocSpec with affinity.
func allocSpecWithPodAffinity(affinity *sigmak8s.Affinity) *sigmak8s.AllocSpec {
	return &sigmak8s.AllocSpec{
		Affinity: affinity,
	}
}

// podAffinityWithTerms constructs Affinity with PodAffinityTerms.
func podAffinityWithTerms(terms []sigmak8s.PodAffinityTerm) *sigmak8s.Affinity {
	return &sigmak8s.Affinity{
		PodAntiAffinity: &sigmak8s.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: terms,
		},
	}
}

// podAffinityTermWithConstraint constructs PodAffinityTerm with constraint.
func podAffinityTermWithConstraint(c constraint) sigmak8s.PodAffinityTerm {
	return sigmak8s.PodAffinityTerm{
		PodAffinityTerm: v1.PodAffinityTerm{
			LabelSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      c.key,
						Operator: c.op,
						Values:   []string{c.value},
					},
				},
			},
			TopologyKey: sigmak8s.LabelHostname,
		},
		MaxCount: c.maxCount,
	}
}

// allocSpecStrWithConstraints constructs AllocSpec with constraints.
func allocSpecStrWithConstraints(cs []constraint) string {
	return allocSpecToString(allocSpecWithPodAffinity(podAffinityWithTerms(constrainsToPodAffinityTerms(cs))))
}

func constrainsToPodAffinityTerms(cs []constraint) []sigmak8s.PodAffinityTerm {
	terms := []sigmak8s.PodAffinityTerm{}
	for _, c := range cs {
		terms = append(terms, podAffinityTermWithConstraint(c))
	}
	return terms
}
