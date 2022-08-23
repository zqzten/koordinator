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

package reservation

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/utils"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type mockReservationBuilder struct {
	rr *sev1alpha1.Reservation
}

func newReservationBuilder() *mockReservationBuilder {
	m := &mockReservationBuilder{
		rr: &sev1alpha1.Reservation{},
	}
	m.rr.ResourceVersion = "999"
	return m
}

func (m *mockReservationBuilder) name(n string) *mockReservationBuilder {
	m.rr.Name = n
	return m
}

func (m *mockReservationBuilder) resourceVersion(n string) *mockReservationBuilder {
	m.rr.ResourceVersion = n
	return m
}

func (m *mockReservationBuilder) ownerReferences(n []metav1.OwnerReference) *mockReservationBuilder {
	m.rr.OwnerReferences = n
	return m
}

func (m *mockReservationBuilder) templateLabels(k, v string) *mockReservationBuilder {
	if m.rr.Spec.Template == nil {
		m.rr.Spec.Template = &corev1.PodTemplateSpec{}
	}
	if m.rr.Spec.Template.Labels == nil {
		m.rr.Spec.Template.Labels = map[string]string{}
	}
	m.rr.Spec.Template.Labels[k] = v
	return m
}

func (m *mockReservationBuilder) templateNamespace(n string) *mockReservationBuilder {
	if m.rr.Spec.Template == nil {
		m.rr.Spec.Template = &corev1.PodTemplateSpec{}
	}
	m.rr.Spec.Template.Namespace = n
	return m
}

func (m *mockReservationBuilder) templateOwnerReferences(n []metav1.OwnerReference) *mockReservationBuilder {
	if m.rr.Spec.Template == nil {
		m.rr.Spec.Template = &corev1.PodTemplateSpec{}
	}
	m.rr.Spec.Template.ObjectMeta.OwnerReferences = n
	return m
}

func (m *mockReservationBuilder) templateAffinity(n *corev1.Affinity) *mockReservationBuilder {
	if m.rr.Spec.Template == nil {
		m.rr.Spec.Template = &corev1.PodTemplateSpec{}
	}
	m.rr.Spec.Template.Spec.Affinity = n
	return m
}

func (m *mockReservationBuilder) allocateOnce(n bool) *mockReservationBuilder {
	m.rr.Spec.AllocateOnce = n
	return m
}

func (m *mockReservationBuilder) reservationOwner(n []sev1alpha1.ReservationOwner) *mockReservationBuilder {
	m.rr.Spec.Owners = n
	return m
}

func (m *mockReservationBuilder) label(k, v string) *mockReservationBuilder {
	if m.rr.Labels == nil {
		m.rr.Labels = map[string]string{}
	}
	m.rr.Labels[k] = v
	return m
}

func (m *mockReservationBuilder) object() Object {
	return &Reservation{Reservation: m.rr}
}

func reservationLess(s []sev1alpha1.Reservation) func(i, j int) bool {
	return func(i, j int) bool {
		return s[i].Name < s[j].Name
	}
}

func objectLess(s []Object) func(i, j int) bool {
	return func(i, j int) bool {
		return s[i].String() < s[j].String()
	}
}

func Test_interpreterImpl_GetReservation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = sev1alpha1.AddToScheme(scheme)
	type fields struct {
		cacheObjs []runtime.Object
		apiObjs   []runtime.Object
	}
	type args struct {
		drainNode string
		podUID    types.UID
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    Object
		wantErr bool
	}{
		{
			name: "from cache",
			fields: fields{
				cacheObjs: []runtime.Object{
					newReservationBuilder().name("1-1").rr,
				},
			},
			args: args{
				drainNode: "1",
				podUID:    "1",
			},
			want:    newReservationBuilder().name("1-1").object(),
			wantErr: false,
		}, {
			name: "from api",
			fields: fields{
				apiObjs: []runtime.Object{
					newReservationBuilder().name("1-2").rr,
				},
			},
			args: args{
				drainNode: "1",
				podUID:    "2",
			},
			want:    newReservationBuilder().name("1-2").object(),
			wantErr: false,
		}, {
			name:   "none",
			fields: fields{},
			args: args{
				drainNode: "1",
				podUID:    "2",
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.cacheObjs...).Build()
			apiClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.apiObjs...).Build()
			p := &interpreterImpl{
				apiReader: apiClient,
				Client:    cacheClient,
			}
			got, err := p.GetReservation(context.Background(), tt.args.drainNode, tt.args.podUID)
			if (err != nil) != tt.wantErr {
				t.Errorf("interpreterImpl.GetReservation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil && got != nil {
				tt.want.GetObjectKind().SetGroupVersionKind(got.GetObjectKind().GroupVersionKind())
				tt.want.SetResourceVersion(got.GetResourceVersion())
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("interpreterImpl.GetReservation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_interpreterImpl_DeleteReservations(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = sev1alpha1.AddToScheme(scheme)
	type fields struct {
		cacheObjs []runtime.Object
		apiObjs   []runtime.Object
	}
	type args struct {
		drainNode string
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		wantErr  bool
		wantList []sev1alpha1.Reservation
	}{
		{
			name: "success",
			fields: fields{
				cacheObjs: []runtime.Object{
					newReservationBuilder().name("1-1").label(utils.DrainNodeKey, "test1").rr,
					newReservationBuilder().name("1-2").label(utils.DrainNodeKey, "test1").rr,
					newReservationBuilder().name("1-3").label(utils.DrainNodeKey, "test1").rr,
					newReservationBuilder().name("3-1").label(utils.DrainNodeKey, "test3").rr,
					newReservationBuilder().name("2-1").label(utils.DrainNodeKey, "test2").rr,
				},
			},
			args: args{
				drainNode: "test1",
			},
			wantErr: false,
			wantList: []sev1alpha1.Reservation{
				*newReservationBuilder().name("3-1").label(utils.DrainNodeKey, "test3").rr,
				*newReservationBuilder().name("2-1").label(utils.DrainNodeKey, "test2").rr,
			},
		}, {
			name: "error",
			fields: fields{
				cacheObjs: []runtime.Object{},
			},
			args: args{
				drainNode: "test1",
			},
			wantErr:  false,
			wantList: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.cacheObjs...).Build()
			apiClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.apiObjs...).Build()
			p := &interpreterImpl{
				apiReader: apiClient,
				Client:    cacheClient,
			}
			if err := p.DeleteReservations(context.Background(), tt.args.drainNode); (err != nil) != tt.wantErr {
				t.Errorf("interpreterImpl.DeleteReservations() error = %v, wantErr %v", err, tt.wantErr)
			}
			list := &sev1alpha1.ReservationList{}
			cacheClient.List(context.Background(), list)
			sort.SliceStable(list.Items, reservationLess(list.Items))
			sort.SliceStable(tt.wantList, reservationLess(tt.wantList))
			if !reflect.DeepEqual(list.Items, tt.wantList) {
				t.Errorf("interpreterImpl.DeleteReservations() = %v, want %v", list.Items, tt.wantList)
			}
		})
	}
}

func Test_interpreterImpl_DeleteReservation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = sev1alpha1.AddToScheme(scheme)
	type fields struct {
		cacheObjs []runtime.Object
		apiObjs   []runtime.Object
	}
	type args struct {
		reservationName string
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		wantErr  bool
		wantList []sev1alpha1.Reservation
	}{
		{
			name: "success",
			fields: fields{
				cacheObjs: []runtime.Object{
					newReservationBuilder().name("1-1").label(utils.DrainNodeKey, "test1").rr,
					newReservationBuilder().name("1-2").label(utils.DrainNodeKey, "test1").rr,
					newReservationBuilder().name("1-3").label(utils.DrainNodeKey, "test1").rr,
					newReservationBuilder().name("3-1").label(utils.DrainNodeKey, "test3").rr,
					newReservationBuilder().name("2-1").label(utils.DrainNodeKey, "test2").rr,
				},
			},
			args: args{
				reservationName: "1-3",
			},
			wantErr: false,
			wantList: []sev1alpha1.Reservation{
				*newReservationBuilder().name("1-1").label(utils.DrainNodeKey, "test1").rr,
				*newReservationBuilder().name("1-2").label(utils.DrainNodeKey, "test1").rr,
				*newReservationBuilder().name("3-1").label(utils.DrainNodeKey, "test3").rr,
				*newReservationBuilder().name("2-1").label(utils.DrainNodeKey, "test2").rr,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.cacheObjs...).Build()
			apiClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.apiObjs...).Build()
			p := &interpreterImpl{
				apiReader: apiClient,
				Client:    cacheClient,
			}
			if err := p.DeleteReservation(context.Background(), tt.args.reservationName); (err != nil) != tt.wantErr {
				t.Errorf("interpreterImpl.DeleteReservation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_interpreterImpl_ListReservation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = sev1alpha1.AddToScheme(scheme)
	type fields struct {
		cacheObjs []runtime.Object
		apiObjs   []runtime.Object
	}
	type args struct {
		drainNode string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []Object
		wantErr bool
	}{
		{
			name: "success",
			fields: fields{
				cacheObjs: []runtime.Object{
					newReservationBuilder().name("1-1").label(utils.DrainNodeKey, "test1").rr,
					newReservationBuilder().name("1-2").label(utils.DrainNodeKey, "test1").rr,
					newReservationBuilder().name("1-3").label(utils.DrainNodeKey, "test1").rr,
					newReservationBuilder().name("3-1").label(utils.DrainNodeKey, "test3").rr,
					newReservationBuilder().name("2-1").label(utils.DrainNodeKey, "test2").rr,
				},
			},
			args: args{
				drainNode: "test1",
			},
			wantErr: false,
			want: []Object{
				newReservationBuilder().name("1-1").label(utils.DrainNodeKey, "test1").object(),
				newReservationBuilder().name("1-2").label(utils.DrainNodeKey, "test1").object(),
				newReservationBuilder().name("1-3").label(utils.DrainNodeKey, "test1").object(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.cacheObjs...).Build()
			apiClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.apiObjs...).Build()
			p := &interpreterImpl{
				apiReader: apiClient,
				Client:    cacheClient,
			}
			got, err := p.ListReservation(context.Background(), tt.args.drainNode)
			if (err != nil) != tt.wantErr {
				t.Errorf("interpreterImpl.ListReservation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil && got != nil {
				sort.SliceStable(got, objectLess(got))
				sort.SliceStable(tt.want, objectLess(tt.want))
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("interpreterImpl.ListReservation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_interpreterImpl_CreateReservation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = sev1alpha1.AddToScheme(scheme)
	type fields struct {
		cacheObjs []runtime.Object
		apiObjs   []runtime.Object
	}
	type args struct {
		dn  *sev1alpha1.DrainNode
		pod *corev1.Pod
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    Object
		wantErr bool
	}{
		{
			name:   "success",
			fields: fields{},
			args: args{
				dn: &sev1alpha1.DrainNode{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dn-1",
						UID:  "123",
					},
				},
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "testns",
						Name:      "dn-1",
						UID:       "123",
						OwnerReferences: []metav1.OwnerReference{
							{
								Controller: pointer.Bool(true),
							},
						},
					},
				},
			},
			want: newReservationBuilder().
				name("dn-1-123").
				label(utils.DrainNodeKey, "dn-1").
				label(utils.PodNamespaceKey, "testns").
				label(utils.PodNameKey, "dn-1").
				ownerReferences([]metav1.OwnerReference{
					{APIVersion: "scheduling.koordinator.sh/v1alpha1",
						Kind:               "DrainNode",
						Name:               "dn-1",
						UID:                "123",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					},
				}).
				templateLabels(utils.PodNameKey, "dn-1").
				templateNamespace("testns").
				templateOwnerReferences([]metav1.OwnerReference{
					{Controller: pointer.Bool(true)},
				}).
				templateAffinity(&corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
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
										Key:      utils.GroupKey,
										Operator: corev1.NodeSelectorOpDoesNotExist,
									}},
								},
							},
						},
					},
				}).
				reservationOwner([]sev1alpha1.ReservationOwner{
					{
						Controller: &sev1alpha1.ReservationControllerReference{
							Namespace: "testns",
							OwnerReference: metav1.OwnerReference{
								Controller: pointer.Bool(true),
							},
						},
					},
				}).
				allocateOnce(true).
				resourceVersion("1").object(),
			wantErr: false,
		}, {
			name:   "merge success",
			fields: fields{},
			args: args{
				dn: &sev1alpha1.DrainNode{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dn-1",
						UID:  "123",
					},
				},
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "testns",
						Name:      "dn-1",
						UID:       "123",
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
			},
			want: newReservationBuilder().
				name("dn-1-123").
				label(utils.DrainNodeKey, "dn-1").
				label(utils.PodNamespaceKey, "testns").
				label(utils.PodNameKey, "dn-1").
				ownerReferences([]metav1.OwnerReference{
					{APIVersion: "scheduling.koordinator.sh/v1alpha1",
						Kind:               "DrainNode",
						Name:               "dn-1",
						UID:                "123",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					},
				}).
				templateNamespace("testns").
				templateLabels(utils.PodNameKey, "dn-1").
				templateOwnerReferences([]metav1.OwnerReference{
					{Controller: pointer.Bool(true)},
				}).
				templateAffinity(&corev1.Affinity{
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
				}).
				reservationOwner([]sev1alpha1.ReservationOwner{
					{
						Controller: &sev1alpha1.ReservationControllerReference{
							Namespace: "testns",
							OwnerReference: metav1.OwnerReference{
								Controller: pointer.Bool(true),
							},
						},
					},
				}).
				allocateOnce(true).
				resourceVersion("1").object(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.cacheObjs...).Build()
			apiClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.apiObjs...).Build()
			p := &interpreterImpl{
				apiReader: apiClient,
				Client:    cacheClient,
			}
			got, err := p.CreateReservation(context.Background(), tt.args.dn, tt.args.pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("interpreterImpl.CreateReservation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != nil && tt.want != nil {
				got.SetCreationTimestamp(tt.want.GetCreationTimestamp())
			}
			if !reflect.DeepEqual(got, tt.want) {
				gotObj := got.OriginObject().(*sev1alpha1.Reservation)
				wantObj := tt.want.OriginObject().(*sev1alpha1.Reservation)
				if !reflect.DeepEqual(gotObj.Spec.Template, wantObj.Spec.Template) {
					t.Errorf("interpreterImpl.CreateReservation() Template = %v, want %v", gotObj.Spec, wantObj.Spec)
				}
				if !reflect.DeepEqual(gotObj.Spec.Owners, wantObj.Spec.Owners) {
					t.Errorf("interpreterImpl.CreateReservation() Owners = %v, want %v", gotObj.Spec, wantObj.Spec)
				}
				gotjson, _ := json.Marshal(got)
				wantjson, _ := json.Marshal(tt.want)
				t.Errorf("interpreterImpl.CreateReservation() = %v, want %v", string(gotjson), string(wantjson))
				return
			}
			if tt.want != nil {
				cacheObj := tt.want.DeepCopyObject().(client.Object)
				cacheClient.Get(context.Background(), types.NamespacedName{}, cacheObj)
				if !reflect.DeepEqual(cacheObj, tt.want.OriginObject()) {
					t.Errorf("interpreterImpl.CreateReservation() = %v, want %v", cacheObj, tt.want.OriginObject())
				}
			}
		})
	}
}
