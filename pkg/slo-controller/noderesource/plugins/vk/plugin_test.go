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

package vk

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension"
	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/noderesource/framework"
)

func TestPlugin(t *testing.T) {
	t.Run("test VKResource", func(t *testing.T) {
		p := &Plugin{}
		assert.Equal(t, PluginName, p.Name())

		var got []framework.ResourceItem
		assert.Equal(t, got, p.Reset(nil, ""))
	})
}

func TestPluginCalculate(t *testing.T) {
	type args struct {
		strategy *extension.ColocationStrategy
		node     *corev1.Node
		podList  *corev1.PodList
		metrics  *framework.ResourceMetrics
	}
	tests := []struct {
		name    string
		args    args
		want    []framework.ResourceItem
		wantErr bool
	}{
		{
			name: "skip for non-VK node",
			args: args{
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "normal-node",
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "vk node",
			args: args{
				strategy: &extension.ColocationStrategy{
					Enable:                        pointer.BoolPtr(true),
					CPUReclaimThresholdPercent:    pointer.Int64Ptr(70),
					MemoryReclaimThresholdPercent: pointer.Int64Ptr(80),
				},
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node1",
						Labels: map[string]string{
							"type": "virtual-kubelet",
						},
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100"),
							corev1.ResourceMemory: resource.MustParse("120G"),
						},
						Capacity: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100"),
							corev1.ResourceMemory: resource.MustParse("120G"),
						},
					},
				},
				podList: &corev1.PodList{
					Items: []corev1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "podA",
								Namespace: "test",
								Labels: map[string]string{
									extension.LabelPodQoS: string(extension.QoSLS),
								},
							},
							Spec: corev1.PodSpec{
								NodeName: "test-node1",
								Containers: []corev1.Container{
									{
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("20"),
												corev1.ResourceMemory: resource.MustParse("20G"),
											},
											Limits: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("20"),
												corev1.ResourceMemory: resource.MustParse("20G"),
											},
										},
									},
								},
							},
							Status: corev1.PodStatus{
								Phase: corev1.PodRunning,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "podB",
								Namespace: "test",
								Labels: map[string]string{
									extension.LabelPodQoS: string(extension.QoSBE),
								},
							},
							Spec: corev1.PodSpec{
								NodeName:   "test-node1",
								Containers: []corev1.Container{{}},
							},
							Status: corev1.PodStatus{
								Phase: corev1.PodRunning,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "podC",
								Namespace: "test",
							},
							Spec: corev1.PodSpec{
								NodeName: "test-node1",
								Containers: []corev1.Container{
									{
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("10"),
												corev1.ResourceMemory: resource.MustParse("20G"),
											},
											Limits: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("10"),
												corev1.ResourceMemory: resource.MustParse("20G"),
											},
										},
									}, {
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("10"),
												corev1.ResourceMemory: resource.MustParse("20G"),
											},
											Limits: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("10"),
												corev1.ResourceMemory: resource.MustParse("20G"),
											},
										},
									},
								},
							},
							Status: corev1.PodStatus{
								Phase: corev1.PodPending,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "podD",
								Namespace: "test",
								Labels: map[string]string{
									extension.LabelPodQoS: string(extension.QoSBE),
								},
							},
							Spec: corev1.PodSpec{
								NodeName: "test-node1",
								Containers: []corev1.Container{
									{
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("10"),
												corev1.ResourceMemory: resource.MustParse("10G"),
											},
											Limits: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("10"),
												corev1.ResourceMemory: resource.MustParse("10G"),
											},
										},
									},
								},
							},
							Status: corev1.PodStatus{
								Phase: corev1.PodSucceeded,
							},
						},
					},
				},
				metrics: &framework.ResourceMetrics{
					NodeMetric: &slov1alpha1.NodeMetric{
						Status: slov1alpha1.NodeMetricStatus{
							UpdateTime: &metav1.Time{Time: time.Now()},
							NodeMetric: &slov1alpha1.NodeMetricInfo{
								NodeUsage: slov1alpha1.ResourceMap{
									ResourceList: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("50"),
										corev1.ResourceMemory: resource.MustParse("55G"),
									},
								},
							},
							PodsMetric: []*slov1alpha1.PodMetricInfo{
								{
									Namespace: "test",
									Name:      "podA",
									PodUsage: slov1alpha1.ResourceMap{
										ResourceList: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("11"),
											corev1.ResourceMemory: resource.MustParse("11G"),
										},
									},
								}, {
									Namespace: "test",
									Name:      "podB",
									PodUsage: slov1alpha1.ResourceMap{
										ResourceList: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("10"),
											corev1.ResourceMemory: resource.MustParse("10G"),
										},
									},
								},
								{
									Namespace: "test",
									Name:      "podC",
									PodUsage: slov1alpha1.ResourceMap{
										ResourceList: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("22"),
											corev1.ResourceMemory: resource.MustParse("22G"),
										},
									},
								},
							},
						},
					},
				},
			},
			want: []framework.ResourceItem{
				{
					Name:     extension.BatchCPU,
					Quantity: resource.NewQuantity(30000, resource.DecimalSI),
					Message:  "batchAllocatable[CPU(Milli-Core)]:30000 = nodeAllocatable:100000 - nodeReservation:30000 - podRequest(Non-BE):40000",
				},
				{
					Name:     extension.BatchMemory,
					Quantity: resource.NewScaledQuantity(36, 9),
					Message:  "batchAllocatable[Mem(GB)]:36 = nodeAllocatable:120 - nodeReservation:24 - podRequest(Non-BE):60",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plugin{}
			got, gotErr := p.Calculate(tt.args.strategy, tt.args.node, tt.args.podList, tt.args.metrics)
			assert.Equal(t, tt.wantErr, gotErr != nil)
			testingCorrectResourceItems(t, tt.want, got)
		})
	}
}

func testingCorrectResourceItems(t *testing.T, want, got []framework.ResourceItem) {
	assert.Equal(t, len(want), len(got))
	for i := range want {
		qWant, qGot := want[i].Quantity, got[i].Quantity
		want[i].Quantity, got[i].Quantity = nil, nil
		assert.Equal(t, want[i], got[i], "equal fields for resource "+want[i].Name)
		assert.Equal(t, qWant.MilliValue(), qGot.MilliValue(), "equal values for resource "+want[i].Name)
		want[i].Quantity, got[i].Quantity = qWant, qGot
	}
}
