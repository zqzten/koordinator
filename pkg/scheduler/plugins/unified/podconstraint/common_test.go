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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/podconstraint/cache"
)

func Test_fillSelectorByMatchLabels(t *testing.T) {
	tests := []struct {
		name                  string
		pod                   *corev1.Pod
		spreadConstraints     []*cache.TopologySpreadConstraint
		wantSpreadConstraints []*cache.TopologySpreadConstraint
	}{
		{
			name: "normal flow",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-pod",
					Labels: map[string]string{"app": "app1"},
				},
			},
			spreadConstraints: []*cache.TopologySpreadConstraint{
				{
					MatchLabelKeys: []string{"app"},
				},
			},
			wantSpreadConstraints: []*cache.TopologySpreadConstraint{
				{
					MatchLabelKeys: []string{"app"},
					Selector:       labels.SelectorFromSet(map[string]string{"app": "app1"}),
				},
			},
		},
		{
			name: "disableMatchLabelKeysInPodConstraint",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-pod",
					Labels: map[string]string{"app": "app1"},
				},
			},
			spreadConstraints: []*cache.TopologySpreadConstraint{
				{},
			},
			wantSpreadConstraints: []*cache.TopologySpreadConstraint{
				{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fillSelectorByMatchLabels(tt.pod, tt.spreadConstraints)
			assert.Equal(t, tt.wantSpreadConstraints, tt.spreadConstraints)
		})
	}
}
