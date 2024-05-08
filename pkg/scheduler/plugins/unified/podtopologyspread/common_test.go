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

package podtopologyspread

import (
	"testing"

	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/util/feature"
)

func Test_tolerationTolerateNodeFn(t *testing.T) {
	tests := []struct {
		name                        string
		defaultHonorTaintToleration bool
		pod                         *corev1.Pod
		node                        *corev1.Node
		want                        bool
	}{
		{
			name:                        "DefaultHonorTaintTolerationInTopologySpread disabled, pass",
			defaultHonorTaintToleration: false,
			pod: &corev1.Pod{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec:       corev1.PodSpec{},
				Status:     corev1.PodStatus{},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "test-taint",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			want: true,
		},
		{
			name:                        "DefaultHonorTaintTolerationInTopologySpread enabled, normal node, tolerated",
			defaultHonorTaintToleration: true,
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{
						{
							Key:      "test-taint",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "test-taint",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			want: true,
		},
		{
			name:                        "DefaultHonorTaintTolerationInTopologySpread enabled, normal node, unTolerated",
			defaultHonorTaintToleration: true,
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "test-taint",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			want: false,
		},
		{
			name:                        "DefaultHonorTaintTolerationInTopologySpread enabled, vknode, tolerated",
			defaultHonorTaintToleration: true,
			pod: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						uniext.LabelECIAffinity: uniext.ECIRequired,
					},
				},
				Spec:   corev1.PodSpec{},
				Status: corev1.PodStatus{},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						uniext.LabelNodeType: uniext.VKType,
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    extunified.VKTaintKey,
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			want: true,
		},
		{
			name:                        "DefaultHonorTaintTolerationInTopologySpread enabled, vknode, unTolerated",
			defaultHonorTaintToleration: true,
			pod: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						uniext.LabelECIAffinity: uniext.ECIRequired,
					},
				},
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{},
				},
				Status: corev1.PodStatus{},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						uniext.LabelNodeType: uniext.VKType,
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    extunified.VKTaintKey,
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    "test-taint",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer feature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.DefaultHonorTaintTolerationInTopologySpread, tt.defaultHonorTaintToleration)()
			tolerationTolerateNode := tolerationTolerateNodeFn(tt.pod)
			assert.Equal(t, tt.want, tolerationTolerateNode(tt.node))
		})
	}
}
