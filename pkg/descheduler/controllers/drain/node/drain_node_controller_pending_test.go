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

package node

import (
	"context"
	"reflect"
	"testing"

	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/cache"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/reservation"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/utils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestDrainNodeReconciler_Reconcile_Pending(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	type fields struct {
		objs  []runtime.Object
		cache cache.Cache
	}
	type args struct {
		request reconcile.Request
	}
	tests := []struct {
		name            string
		fields          fields
		args            args
		want            reconcile.Result
		wantErr         bool
		wantObj         *v1alpha1.DrainNode
		wantNode        *corev1.Node
		wantReservation []*v1alpha1.Reservation
	}{
		{
			name: "node unmigratable",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
							Labels: map[string]string{
								utils.GroupKey: "group",
							},
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:        "node123",
							DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhasePending,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
							Labels: map[string]string{
								utils.DrainNodeKey: "123",
							},
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    "test",
									Value:  "group",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Drainable: false,
					},
					pods: []*cache.PodInfo{},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			wantErr: true,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
					Labels: map[string]string{
						utils.GroupKey: "group",
					},
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:        "node123",
					DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseUnavailable,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseUnavailable),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseUnavailable),
							Message: "Unavailable,  node is not migrated",
						},
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
					Labels: map[string]string{
						utils.DrainNodeKey: "123",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "test",
							Value:  "group",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{},
		},
		{
			name: "pod unmigratable",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
							Labels: map[string]string{
								utils.GroupKey: "group",
							},
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:        "node123",
							DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhasePending,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
							Labels: map[string]string{
								utils.DrainNodeKey: "123",
							},
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    "test",
									Value:  "group",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Drainable: true,
					},
					pods: []*cache.PodInfo{
						{Migratable: false},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
					Labels: map[string]string{
						utils.GroupKey: "group",
					},
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:        "node123",
					DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseUnavailable,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseUnavailable),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseUnavailable),
							Message: "Unavailable, / pod on node node123 is not migrated",
						},
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
					Labels: map[string]string{
						utils.DrainNodeKey: "123",
						utils.PlanningKey:  "",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "test",
							Value:  "group",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    utils.GroupKey,
							Value:  "group",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{},
		},
		{
			name: "create reservation",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
							Labels: map[string]string{
								utils.GroupKey: "group",
							},
							UID: "DrainNode-123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:        "node123",
							DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhasePending,
						},
					},
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "testns",
							Name:      "testname",
							UID:       "456",
							OwnerReferences: []metav1.OwnerReference{
								{
									Controller: pointer.Bool(true),
								},
							},
						},
						Spec: corev1.PodSpec{
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "test",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"test1"},
													},
												},
											},
										},
									},
									PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
										{
											Weight: 100,
											Preference: corev1.NodeSelectorTerm{
												MatchExpressions: []corev1.NodeSelectorRequirement{{
													Key:      "key2",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"test1"},
												}},
											},
										},
									},
								},
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
							Labels: map[string]string{
								utils.DrainNodeKey: "123",
							},
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    "test",
									Value:  "group",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Drainable: true,
					},
					pods: []*cache.PodInfo{
						{UID: "456", Migratable: true, NamespacedName: types.NamespacedName{Namespace: "testns", Name: "testname"}},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
					Labels: map[string]string{
						utils.GroupKey: "group",
					},
					UID: "DrainNode-123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:        "node123",
					DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhasePending,
					MigrationJobStatus: []v1alpha1.MigrationJobStatus{
						{
							Namespace:       "testns",
							PodName:         "testname",
							ReservationName: "123-456",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhasePending),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhasePending),
							Message: "Pending",
						},
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
					Labels: map[string]string{
						utils.DrainNodeKey: "123",
						utils.PlanningKey:  "",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "test",
							Value:  "group",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    utils.GroupKey,
							Value:  "group",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "123-456",
						Labels: map[string]string{
							utils.DrainNodeKey:    "123",
							utils.PodNamespaceKey: "testns",
							utils.PodNameKey:      "testname",
						},
						OwnerReferences: []metav1.OwnerReference{
							{APIVersion: "scheduling.koordinator.sh/v1alpha1",
								Kind:               "DrainNode",
								Name:               "123",
								UID:                "DrainNode-123",
								Controller:         pointer.Bool(true),
								BlockOwnerDeletion: pointer.Bool(true),
							},
						},
					},
					Spec: v1alpha1.ReservationSpec{
						Template: &corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "testns",
								Labels: map[string]string{
									utils.PodNameKey: "testname",
								},
								OwnerReferences: []metav1.OwnerReference{
									{
										Controller: pointer.Bool(true),
									},
								},
							},
							Spec: corev1.PodSpec{
								Affinity: &corev1.Affinity{
									NodeAffinity: &corev1.NodeAffinity{
										RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
											NodeSelectorTerms: []corev1.NodeSelectorTerm{
												{
													MatchExpressions: []corev1.NodeSelectorRequirement{
														{
															Key:      "test",
															Operator: corev1.NodeSelectorOpIn,
															Values:   []string{"test1"},
														},
														{
															Key:      utils.PlanningKey,
															Operator: corev1.NodeSelectorOpDoesNotExist,
														},
													},
												},
											},
										},
										PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
											{
												Weight: 100,
												Preference: corev1.NodeSelectorTerm{
													MatchExpressions: []corev1.NodeSelectorRequirement{{
														Key:      "key2",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"test1"},
													}},
												},
											},
											{
												Weight: 100,
												Preference: corev1.NodeSelectorTerm{
													MatchExpressions: []corev1.NodeSelectorRequirement{{
														Key:      utils.GroupKey,
														Operator: corev1.NodeSelectorOpDoesNotExist,
													}},
												},
											},
										},
									},
								},
							},
						},
						Owners: []v1alpha1.ReservationOwner{
							{Controller: &v1alpha1.ReservationControllerReference{
								OwnerReference: metav1.OwnerReference{
									Controller: pointer.Bool(true),
								},
								Namespace: "testns",
							}},
						},
						AllocateOnce: true,
					},
				},
			},
		},
		{
			name: "reservation failed",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
							Labels: map[string]string{
								utils.GroupKey: "group",
							},
							UID: "DrainNode-123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:        "node123",
							DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhasePending,
						},
					},
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "testns",
							Name:      "testname",
							UID:       "456",
							OwnerReferences: []metav1.OwnerReference{
								{
									Controller: pointer.Bool(true),
								},
							},
						},
						Spec: corev1.PodSpec{
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "test",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"test1"},
													},
												},
											},
										},
									},
									PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
										{
											Weight: 100,
											Preference: corev1.NodeSelectorTerm{
												MatchExpressions: []corev1.NodeSelectorRequirement{{
													Key:      "key2",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"test1"},
												}},
											},
										},
									},
								},
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
							Labels: map[string]string{
								utils.DrainNodeKey: "123",
							},
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    "test",
									Value:  "group",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-456",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "testns",
								utils.PodNameKey:      "testname",
							},
							OwnerReferences: []metav1.OwnerReference{
								{APIVersion: "scheduling.koordinator.sh/v1alpha1",
									Kind:               "DrainNode",
									Name:               "123",
									UID:                "DrainNode-123",
									Controller:         pointer.Bool(true),
									BlockOwnerDeletion: pointer.Bool(true),
								},
							},
						},
						Spec: v1alpha1.ReservationSpec{
							Template: &corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "testns",
									OwnerReferences: []metav1.OwnerReference{
										{
											Controller: pointer.Bool(true),
										},
									},
								},
								Spec: corev1.PodSpec{
									Affinity: &corev1.Affinity{
										NodeAffinity: &corev1.NodeAffinity{
											RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
												NodeSelectorTerms: []corev1.NodeSelectorTerm{
													{
														MatchExpressions: []corev1.NodeSelectorRequirement{
															{
																Key:      "test",
																Operator: corev1.NodeSelectorOpIn,
																Values:   []string{"test1"},
															},
															{
																Key:      utils.PlanningKey,
																Operator: corev1.NodeSelectorOpDoesNotExist,
															},
														},
													},
												},
											},
											PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
												{
													Weight: 100,
													Preference: corev1.NodeSelectorTerm{
														MatchExpressions: []corev1.NodeSelectorRequirement{{
															Key:      "key2",
															Operator: corev1.NodeSelectorOpIn,
															Values:   []string{"test1"},
														}},
													},
												},
												{
													Weight: 100,
													Preference: corev1.NodeSelectorTerm{
														MatchExpressions: []corev1.NodeSelectorRequirement{{
															Key:      utils.GroupKey,
															Operator: corev1.NodeSelectorOpDoesNotExist,
														}},
													},
												},
											},
										},
									},
								},
							},
							Owners: []v1alpha1.ReservationOwner{
								{Controller: &v1alpha1.ReservationControllerReference{
									OwnerReference: metav1.OwnerReference{
										Controller: pointer.Bool(true),
									},
									Namespace: "testns",
								}},
							},
							AllocateOnce: true,
						},
						Status: v1alpha1.ReservationStatus{
							Conditions: []v1alpha1.ReservationCondition{
								{
									Type:   v1alpha1.ReservationConditionScheduled,
									Reason: v1alpha1.ReasonReservationUnschedulable,
								},
							},
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Drainable: true,
					},
					pods: []*cache.PodInfo{
						{UID: "456", Migratable: true, NamespacedName: types.NamespacedName{Namespace: "testns", Name: "testname"}},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
					Labels: map[string]string{
						utils.GroupKey: "group",
					},
					UID: "DrainNode-123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:        "node123",
					DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseUnavailable,
					MigrationJobStatus: []v1alpha1.MigrationJobStatus{
						{
							Namespace:       "testns",
							PodName:         "testname",
							ReservationName: "123-456",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseUnavailable),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseUnavailable),
							Message: "Unavailable, Pod testns/testname Reservation 123-456 scheduled fail",
						},
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
					Labels: map[string]string{
						utils.DrainNodeKey: "123",
						utils.PlanningKey:  "",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "test",
							Value:  "group",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    utils.GroupKey,
							Value:  "group",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{},
		},
		{
			name: "reservation available",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
							Labels: map[string]string{
								utils.GroupKey: "group",
							},
							UID: "DrainNode-123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:        "node123",
							DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhasePending,
						},
					},
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "testns",
							Name:      "testname",
							UID:       "456",
							OwnerReferences: []metav1.OwnerReference{
								{
									Controller: pointer.Bool(true),
								},
							},
						},
						Spec: corev1.PodSpec{
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "test",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"test1"},
													},
												},
											},
										},
									},
									PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
										{
											Weight: 100,
											Preference: corev1.NodeSelectorTerm{
												MatchExpressions: []corev1.NodeSelectorRequirement{{
													Key:      "key2",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"test1"},
												}},
											},
										},
									},
								},
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
							Labels: map[string]string{
								utils.DrainNodeKey: "123",
							},
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    "test",
									Value:  "group",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-456",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "testns",
								utils.PodNameKey:      "testname",
							},
							OwnerReferences: []metav1.OwnerReference{
								{APIVersion: "scheduling.koordinator.sh/v1alpha1",
									Kind:               "DrainNode",
									Name:               "123",
									UID:                "DrainNode-123",
									Controller:         pointer.Bool(true),
									BlockOwnerDeletion: pointer.Bool(true),
								},
							},
						},
						Spec: v1alpha1.ReservationSpec{
							Template: &corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "testns",
									OwnerReferences: []metav1.OwnerReference{
										{
											Controller: pointer.Bool(true),
										},
									},
								},
								Spec: corev1.PodSpec{
									Affinity: &corev1.Affinity{
										NodeAffinity: &corev1.NodeAffinity{
											RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
												NodeSelectorTerms: []corev1.NodeSelectorTerm{
													{
														MatchExpressions: []corev1.NodeSelectorRequirement{
															{
																Key:      "test",
																Operator: corev1.NodeSelectorOpIn,
																Values:   []string{"test1"},
															},
															{
																Key:      utils.PlanningKey,
																Operator: corev1.NodeSelectorOpDoesNotExist,
															},
														},
													},
												},
											},
											PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
												{
													Weight: 100,
													Preference: corev1.NodeSelectorTerm{
														MatchExpressions: []corev1.NodeSelectorRequirement{{
															Key:      "key2",
															Operator: corev1.NodeSelectorOpIn,
															Values:   []string{"test1"},
														}},
													},
												},
												{
													Weight: 100,
													Preference: corev1.NodeSelectorTerm{
														MatchExpressions: []corev1.NodeSelectorRequirement{{
															Key:      utils.GroupKey,
															Operator: corev1.NodeSelectorOpDoesNotExist,
														}},
													},
												},
											},
										},
									},
								},
							},
							Owners: []v1alpha1.ReservationOwner{
								{Controller: &v1alpha1.ReservationControllerReference{
									OwnerReference: metav1.OwnerReference{
										Controller: pointer.Bool(true),
									},
									Namespace: "testns",
								}},
							},
							AllocateOnce: true,
						},
						Status: v1alpha1.ReservationStatus{
							Phase: v1alpha1.ReservationAvailable,
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Drainable: true,
					},
					pods: []*cache.PodInfo{
						{UID: "456", Migratable: true, NamespacedName: types.NamespacedName{Namespace: "testns", Name: "testname"}},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
					Labels: map[string]string{
						utils.GroupKey: "group",
					},
					UID: "DrainNode-123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:        "node123",
					DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseAvailable,
					MigrationJobStatus: []v1alpha1.MigrationJobStatus{
						{
							Namespace:        "testns",
							PodName:          "testname",
							ReservationName:  "123-456",
							ReservationPhase: "Available",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseAvailable),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseAvailable),
							Message: "Available",
						},
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
					Labels: map[string]string{
						utils.DrainNodeKey: "123",
						utils.PlanningKey:  "",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "test",
							Value:  "group",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    utils.GroupKey,
							Value:  "group",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.objs...).Build()
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: Name})
			r := &DrainNodeReconciler{
				Client:                 c,
				eventRecorder:          record.NewEventRecorderAdapter(recorder),
				cache:                  tt.fields.cache,
				reservationInterpreter: reservation.NewInterpreter(c, c),
			}
			got, err := r.Reconcile(context.Background(), tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("DrainNodeReconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DrainNodeReconciler.Reconcile() = %v, want %v", got, tt.want)
			}
			if tt.wantObj != nil {
				gotObj := &v1alpha1.DrainNode{}
				err := c.Get(context.Background(), types.NamespacedName{Name: tt.wantObj.Name}, gotObj)
				if err != nil {
					t.Errorf("DrainNodeReconciler.Reconcile() got err = %v", err)
					return
				}
				tt.wantObj.TypeMeta = gotObj.TypeMeta
				tt.wantObj.ResourceVersion = gotObj.ResourceVersion
				if len(gotObj.Status.Conditions) > 0 {
					gotObj.Status.Conditions[0].LastTransitionTime =
						tt.wantObj.Status.Conditions[0].LastTransitionTime
				}
				if !reflect.DeepEqual(gotObj, tt.wantObj) {
					t.Errorf("DrainNodeReconciler.Reconcile() = %v, want %v", gotObj, tt.wantObj)
				}
			}
			for _, wantR := range tt.wantReservation {
				r := &v1alpha1.Reservation{}
				if err := c.Get(context.Background(), types.NamespacedName{Name: wantR.Name}, r); err != nil {
					t.Errorf("DrainNodeReconciler.Reconcile() get reservation %v err %v", tt.wantNode.Name, err)
				}
				r.TypeMeta = wantR.TypeMeta
				r.ResourceVersion = wantR.ResourceVersion
				if !reflect.DeepEqual(r, wantR) {
					t.Errorf("DrainNodeReconciler.Reconcile() get node = %v, want %v", r, wantR)
				}
			}
			if tt.wantNode != nil {
				n := &corev1.Node{}
				if err := c.Get(context.Background(), types.NamespacedName{Name: tt.wantNode.Name}, n); err != nil {
					t.Errorf("DrainNodeReconciler.Reconcile() get node %v err %v", tt.wantNode.Name, err)
				}
				tt.wantNode.TypeMeta = n.TypeMeta
				tt.wantNode.ResourceVersion = n.ResourceVersion
				if !reflect.DeepEqual(n, tt.wantNode) {
					t.Errorf("DrainNodeReconciler.Reconcile() get node = %v, want %v", n, tt.wantNode)
				}
			}
		})
	}
}
