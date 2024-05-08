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

package logicalresourcenode

import (
	"fmt"
	"testing"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

func TestGetReservationOwners(t *testing.T) {
	cases := []struct {
		lrn            *schedulingv1alpha1.LogicalResourceNode
		expectedOwners []schedulingv1alpha1.ReservationOwner
	}{
		{
			lrn: &schedulingv1alpha1.LogicalResourceNode{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelectorList: `[]`,
					},
				},
			},
			expectedOwners: []schedulingv1alpha1.ReservationOwner{},
		},
		{
			lrn: &schedulingv1alpha1.LogicalResourceNode{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelector: "{}",
					},
				},
			},
			expectedOwners: []schedulingv1alpha1.ReservationOwner{},
		},
		{
			lrn: &schedulingv1alpha1.LogicalResourceNode{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelectorList: `[{"k1": "v1", "k2": "v2"}]`,
					},
				},
			},
			expectedOwners: []schedulingv1alpha1.ReservationOwner{
				{LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"k1": "v1", "k2": "v2"}}},
			},
		},
		{
			lrn: &schedulingv1alpha1.LogicalResourceNode{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelector: `{"k3": "v3"}`,
					},
				},
			},
			expectedOwners: []schedulingv1alpha1.ReservationOwner{
				{LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"k3": "v3"}}},
			},
		},
		{
			lrn: &schedulingv1alpha1.LogicalResourceNode{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelectorList: `[{"k1":"v1", "k2": "v2"},{"k4": "v4"}]`,
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelector:     `{"k3": "v3"}`,
					},
				},
			},
			expectedOwners: []schedulingv1alpha1.ReservationOwner{
				{LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"k1": "v1", "k2": "v2"}}},
				{LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"k4": "v4"}}},
			},
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			gotOwners, err := GetReservationOwners(tc.lrn)
			if err != nil {
				t.Fatal(err)
			}
			if !apiequality.Semantic.DeepEqual(gotOwners, tc.expectedOwners) {
				t.Fatalf("expected:\n%v\ngot:\n%v", util.DumpJSON(tc.expectedOwners), util.DumpJSON(gotOwners))
			}
		})
	}
}
