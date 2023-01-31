/*
Copyright 2020 The Kubernetes Authors.

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

package nodeaffinity

import (
	"testing"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/eci"
)

func TestPodMatchesNodeSelectorAndAffinityTerms(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		labels   map[string]string
		nodeName string
		want     bool
	}{
		{
			name: "no selector",
			pod:  &corev1.Pod{},
			want: true,
		},
		{
			name: "missing labels",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo": "bar",
					},
				},
			},
			want: false,
		},
		{
			name: "same labels",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo": "bar",
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "node labels are superset",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo": "bar",
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
				"baz": "blah",
			},
			want: true,
		},
		{
			name: "node labels are subset",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo": "bar",
						"baz": "blah",
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: false,
		},
		{
			name: "Pod with matchExpressions using In operator that matches the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "Pod with matchExpressions using Gt operator that matches the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "kernel-version",
												Operator: corev1.NodeSelectorOpGt,
												Values:   []string{"0204"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				// We use two digit to denote major version and two digit for minor version.
				"kernel-version": "0206",
			},
			want: true,
		},
		{
			name: "Pod with matchExpressions using NotIn operator that matches the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "mem-type",
												Operator: corev1.NodeSelectorOpNotIn,
												Values:   []string{"DDR", "DDR2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"mem-type": "DDR3",
			},
			want: true,
		},
		{
			name: "Pod with matchExpressions using Exists operator that matches the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "GPU",
												Operator: corev1.NodeSelectorOpExists,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"GPU": "NVIDIA-GRID-K1",
			},
			want: true,
		},
		{
			name: "Pod with affinity that don't match node's labels won't schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"value1", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: false,
		},
		{
			name: "Pod with a nil []NodeSelectorTerm in affinity, can't match the node's labels and won't schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: nil,
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: false,
		},
		{
			name: "Pod with an empty []NodeSelectorTerm in affinity, can't match the node's labels and won't schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: false,
		},
		{
			name: "Pod with one empty and one not-empty MatchExpressions is not a valid value will match no objects and won't schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{},
									},
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "Pod with empty MatchExpressions is not a valid value will match no objects and won't schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: false,
		},
		{
			name: "Pod with no Affinity will schedule onto a node",
			pod:  &corev1.Pod{},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "Pod with Affinity but nil NodeSelector will schedule onto a node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: nil,
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "Pod with multiple matchExpressions ANDed that matches the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "GPU",
												Operator: corev1.NodeSelectorOpExists,
											}, {
												Key:      "GPU",
												Operator: corev1.NodeSelectorOpNotIn,
												Values:   []string{"AMD", "INTER"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"GPU": "NVIDIA-GRID-K1",
			},
			want: true,
		},
		{
			name: "Pod with multiple matchExpressions ANDed that doesn't match the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "GPU",
												Operator: corev1.NodeSelectorOpExists,
											}, {
												Key:      "GPU",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"AMD", "INTER"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"GPU": "NVIDIA-GRID-K1",
			},
			want: false,
		},
		{
			name: "Pod with multiple NodeSelectorTerms ORed in affinity, matches the node's labels and will schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "diffkey",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"wrong", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "Pod with an Affinity and a PodSpec.NodeSelector(the old thing that we are deprecating) " +
				"both are satisfied, will schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo": "bar",
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpExists,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "Pod with an Affinity matches node's labels but the PodSpec.NodeSelector(the old thing that we are deprecating) " +
				"is not satisfied, won't schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo": "bar",
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpExists,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "barrrrrr",
			},
			want: false,
		},
		{
			name: "Pod with an invalid value in Affinity term won't be scheduled onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpNotIn,
												Values:   []string{"invalid value: ___@#$%^"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: false,
		},
		{
			name: "Pod with matchFields using In operator that matches the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"node_1"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node_1",
			want:     true,
		},
		{
			name: "Pod with matchFields using In operator that does not match the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"node_1"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node_2",
			want:     false,
		},
		{
			name: "Pod with two terms: matchFields does not match, but matchExpressions matches",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"node_1"},
											},
										},
									},
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node_2",
			labels:   map[string]string{"foo": "bar"},
			want:     true,
		},
		{
			name: "Pod with one term: matchFields does not match, but matchExpressions matches",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"node_1"},
											},
										},
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node_2",
			labels:   map[string]string{"foo": "bar"},
			want:     false,
		},
		{
			name: "Pod with one term: both matchFields and matchExpressions match",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"node_1"},
											},
										},
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node_1",
			labels:   map[string]string{"foo": "bar"},
			want:     true,
		},
		{
			name: "Pod with two terms: both matchFields and matchExpressions do not match",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"node_1"},
											},
										},
									},
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"not-match-to-bar"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node_2",
			labels:   map[string]string{"foo": "bar"},
			want:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := corev1.Node{ObjectMeta: metav1.ObjectMeta{
				Name:   test.nodeName,
				Labels: test.labels,
			}}
			got := GetRequiredNodeAffinity(test.pod).Match(&node)
			if test.want != got {
				t.Errorf("expected: %v got %v", test.want, got)
			}
		})
	}
}

func TestECIPodMatchesNodeSelectorAndAffinityTerms(t *testing.T) {
	eci.DefaultECIProfile.AllowedAffinityKeys = []string{"foo", "baz", "kernel-version", "GPU", "diffkey", "mem-type"}
	tests := []struct {
		name     string
		pod      *corev1.Pod
		labels   map[string]string
		nodeName string
		want     bool
	}{
		{
			name: "no selector",
			pod:  &corev1.Pod{},
			want: true,
		},
		{
			name: "missing labels",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo":         "bar",
						"not-allowed": "not-allowed",
					},
				},
			},
			want: false,
		},
		{
			name: "same labels",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo":         "bar",
						"not-allowed": "not-allowed",
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "node labels are superset",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo":         "bar",
						"not-allowed": "not-allowed",
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
				"baz": "blah",
			},
			want: true,
		},
		{
			name: "node labels are subset",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo":         "bar",
						"baz":         "blah",
						"not-allowed": "not-allowed",
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: false,
		},
		{
			name: "Pod with matchExpressions using In operator that matches the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "Pod with matchExpressions using Gt operator that matches the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "kernel-version",
												Operator: corev1.NodeSelectorOpGt,
												Values:   []string{"0204"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				// We use two digit to denote major version and two digit for minor version.
				"kernel-version": "0206",
			},
			want: true,
		},
		{
			name: "Pod with matchExpressions using NotIn operator that matches the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "mem-type",
												Operator: corev1.NodeSelectorOpNotIn,
												Values:   []string{"DDR", "DDR2"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"mem-type": "DDR3",
			},
			want: true,
		},
		{
			name: "Pod with matchExpressions using Exists operator that matches the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "GPU",
												Operator: corev1.NodeSelectorOpExists,
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"GPU": "NVIDIA-GRID-K1",
			},
			want: true,
		},
		{
			name: "Pod with affinity that don't match node's labels won't schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"value1", "value2"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: false,
		},
		{
			name: "Pod with a nil []NodeSelectorTerm in affinity, can't match the node's labels and won't schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: nil,
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: false,
		},
		{
			name: "Pod with an empty []NodeSelectorTerm in affinity, can't match the node's labels and won't schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: false,
		},

		{
			name: "Pod with one empty and one not-empty MatchExpressions is not a valid value will match no objects and won't schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{},
									},
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "Pod with empty MatchExpressions is not a valid value will match no objects and won't schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: false,
		},
		{
			name: "Pod with no Affinity will schedule onto a node",
			pod:  &corev1.Pod{},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "Pod with Affinity but nil NodeSelector will schedule onto a node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: nil,
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "Pod with multiple matchExpressions ANDed that matches the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "GPU",
												Operator: corev1.NodeSelectorOpExists,
											}, {
												Key:      "GPU",
												Operator: corev1.NodeSelectorOpNotIn,
												Values:   []string{"AMD", "INTER"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"GPU": "NVIDIA-GRID-K1",
			},
			want: true,
		},
		{
			name: "Pod with multiple matchExpressions ANDed that doesn't match the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "GPU",
												Operator: corev1.NodeSelectorOpExists,
											}, {
												Key:      "GPU",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"AMD", "INTER"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"GPU": "NVIDIA-GRID-K1",
			},
			want: false,
		},
		{
			name: "Pod with multiple NodeSelectorTerms ORed in affinity, matches the node's labels and will schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "diffkey",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"wrong", "value2"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "Pod with an Affinity and a PodSpec.NodeSelector(the old thing that we are deprecating) " +
				"both are satisfied, will schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo": "bar",
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpExists,
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: true,
		},
		{
			name: "Pod with an Affinity matches node's labels but the PodSpec.NodeSelector(the old thing that we are deprecating) " +
				"is not satisfied, won't schedule onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo": "bar",
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpExists,
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "barrrrrr",
			},
			want: false,
		},
		{
			name: "Pod with an invalid value in Affinity term won't be scheduled onto the node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpNotIn,
												Values:   []string{"invalid value: ___@#$%^"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			want: false,
		},
		{
			name: "Pod with matchFields using In operator that matches the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"node_1"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node_1",
			want:     true,
		},
		{
			name: "Pod with matchFields using In operator that does not match the existing node",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"node_1"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node_2",
			want:     false,
		},
		{
			name: "Pod with two terms: matchFields does not match, but matchExpressions matches",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"node_1"},
											},
										},
									},
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node_2",
			labels:   map[string]string{"foo": "bar"},
			want:     true,
		},
		{
			name: "Pod with one term: matchFields does not match, but matchExpressions matches",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"node_1"},
											},
										},
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node_2",
			labels:   map[string]string{"foo": "bar"},
			want:     false,
		},
		{
			name: "Pod with one term: both matchFields and matchExpressions match",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"node_1"},
											},
										},
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node_1",
			labels:   map[string]string{"foo": "bar"},
			want:     true,
		},
		{
			name: "Pod with two terms: both matchFields and matchExpressions do not match",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"node_1"},
											},
										},
									},
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"not-match-to-bar"},
											},
											{
												Key:      "not-allowed",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"bar", "value2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node_2",
			labels:   map[string]string{"foo": "bar"},
			want:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := corev1.Node{ObjectMeta: metav1.ObjectMeta{
				Name:   test.nodeName,
				Labels: test.labels,
			}}
			if node.Labels == nil {
				node.Labels = map[string]string{}
			}
			node.Labels[uniext.LabelNodeType] = uniext.VKType
			if test.pod.Labels == nil {
				test.pod.Labels = map[string]string{}
			}
			test.pod.Labels[uniext.LabelECIAffinity] = uniext.ECIRequired
			got := GetRequiredNodeAffinity(test.pod).Match(&node)
			if test.want != got {
				t.Errorf("expected: %v got %v", test.want, got)
			}
		})
	}
}
