/*
Copyright 2023 The Koordinator Authors.

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

package validating

import (
	"testing"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestValidateRequirements(t *testing.T) {
	successCases := []schedulingv1alpha1.LogicalResourceNodeRequirements{
		{},
		{
			NodeSelector: map[string]string{"tenant": "abc"},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "type", Operator: corev1.NodeSelectorOpIn, Values: []string{"a", "b"}}},
						}},
					},
				},
			},
			Tolerations: []corev1.Toleration{
				{Key: "tenant", Operator: corev1.TolerationOpEqual, Value: "abc", Effect: corev1.TaintEffectNoSchedule},
			},
			Resources: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("32"),
				corev1.ResourceMemory: resource.MustParse("128Gi"),
				"koordinator.sh/gpu":  resource.MustParse("400"),
			},
		},
	}

	for i, sc := range successCases {
		if err := validateRequirements(&sc); err != nil {
			t.Fatalf("Success cases #%d failed: %s", i, err)
		}
	}
}

func TestValidateResources(t *testing.T) {
	successCases := []corev1.ResourceList{
		{
			corev1.ResourceCPU: resource.MustParse("1"),
		},
		{
			apiext.ResourceNvidiaGPU: resource.MustParse("1"),
		},
	}
	failCases := []corev1.ResourceList{
		{
			corev1.ResourceCPU: resource.MustParse("100m"),
		},
		{
			corev1.ResourceCPU: resource.MustParse("0"),
		},
		{
			apiext.ResourceNvidiaGPU: resource.MustParse("100m"),
		},
	}

	for i, sc := range successCases {
		if err := validateResources(sc); err != nil {
			t.Fatalf("Success cases #%d unexpected failed: %v", i, err)
		}
	}

	for i, sc := range failCases {
		if err := validateResources(sc); err == nil {
			t.Fatalf("Fail cases #%d unexpected succeeded", i)
		}
	}
}
