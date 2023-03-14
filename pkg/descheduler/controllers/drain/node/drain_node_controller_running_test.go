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

func TestDrainNodeReconciler_Reconcile_Running(t *testing.T) {
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
		wantJob         []*v1alpha1.PodMigrationJob
	}{
		{
			name: "create job",
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
							Phase: v1alpha1.DrainNodePhaseRunning,
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
							UID: "abcd",
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
					Phase: v1alpha1.DrainNodePhaseRunning,
					MigrationJobStatus: []v1alpha1.MigrationJobStatus{
						{
							Namespace:           "testns",
							PodName:             "testname",
							ReservationName:     "123-456",
							ReservationPhase:    "Available",
							PodMigrationJobName: "123-456",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseRunning),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseRunning),
							Message: "Running",
						},
					},
				},
			},
			wantNode:        nil,
			wantReservation: []*v1alpha1.Reservation{},
			wantJob: []*v1alpha1.PodMigrationJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "123-456",
						Labels: map[string]string{
							utils.DrainNodeKey: "123",
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
					Spec: v1alpha1.PodMigrationJobSpec{
						PodRef: &corev1.ObjectReference{
							Namespace: "testns",
							Name:      "testname",
						},
						ReservationOptions: &v1alpha1.PodMigrateReservationOptions{
							ReservationRef: &corev1.ObjectReference{
								Name: "123-456",
								UID:  "abcd",
							},
						},
					},
				},
			},
		},
		{
			name: "failed job",
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
							Phase: v1alpha1.DrainNodePhaseRunning,
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
							UID: "abcd",
						},
						Status: v1alpha1.ReservationStatus{
							Phase: v1alpha1.ReservationAvailable,
						},
					},
					&v1alpha1.PodMigrationJob{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-456",
							Labels: map[string]string{
								utils.DrainNodeKey: "123",
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
						Spec: v1alpha1.PodMigrationJobSpec{
							PodRef: &corev1.ObjectReference{
								Namespace: "testns",
								Name:      "testname",
							},
							ReservationOptions: &v1alpha1.PodMigrateReservationOptions{
								ReservationRef: &corev1.ObjectReference{
									Name: "123-456",
									UID:  "abcd",
								},
							},
						},
						Status: v1alpha1.PodMigrationJobStatus{
							Phase: v1alpha1.PodMigrationJobFailed,
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
					Phase: v1alpha1.DrainNodePhaseFailed,
					MigrationJobStatus: []v1alpha1.MigrationJobStatus{
						{
							Namespace:            "testns",
							PodName:              "testname",
							ReservationName:      "123-456",
							ReservationPhase:     "Available",
							PodMigrationJobName:  "123-456",
							PodMigrationJobPhase: "Failed",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseFailed),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseFailed),
							Message: "Failed, 1 PodMigrationJob failed",
						},
					},
				},
			},
			wantNode:        nil,
			wantReservation: []*v1alpha1.Reservation{},
			wantJob:         []*v1alpha1.PodMigrationJob{},
		},
		{
			name: "success job",
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
							Phase: v1alpha1.DrainNodePhaseRunning,
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
							UID: "abcd",
						},
						Status: v1alpha1.ReservationStatus{
							Phase: v1alpha1.ReservationAvailable,
						},
					},
					&v1alpha1.PodMigrationJob{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-456",
							Labels: map[string]string{
								utils.DrainNodeKey: "123",
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
						Spec: v1alpha1.PodMigrationJobSpec{
							PodRef: &corev1.ObjectReference{
								Namespace: "testns",
								Name:      "testname",
							},
							ReservationOptions: &v1alpha1.PodMigrateReservationOptions{
								ReservationRef: &corev1.ObjectReference{
									Name: "123-456",
									UID:  "abcd",
								},
							},
						},
						Status: v1alpha1.PodMigrationJobStatus{
							Phase: v1alpha1.PodMigrationJobSucceeded,
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
					Phase: v1alpha1.DrainNodePhaseSucceeded,
					MigrationJobStatus: []v1alpha1.MigrationJobStatus{
						{
							Namespace:            "testns",
							PodName:              "testname",
							ReservationName:      "123-456",
							ReservationPhase:     "Available",
							PodMigrationJobName:  "123-456",
							PodMigrationJobPhase: "Succeeded",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseSucceeded),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseSucceeded),
							Message: "Succeeded",
						},
					},
				},
			},
			wantNode:        nil,
			wantReservation: []*v1alpha1.Reservation{},
			wantJob:         []*v1alpha1.PodMigrationJob{},
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
			for _, wantJob := range tt.wantJob {
				r := &v1alpha1.PodMigrationJob{}
				if err := c.Get(context.Background(), types.NamespacedName{Name: wantJob.Name}, r); err != nil {
					t.Errorf("DrainNodeReconciler.Reconcile() get reservation %v err %v", tt.wantNode.Name, err)
				}
				r.TypeMeta = wantJob.TypeMeta
				r.ResourceVersion = wantJob.ResourceVersion
				if !reflect.DeepEqual(r, wantJob) {
					t.Errorf("DrainNodeReconciler.Reconcile() get node = %v, want %v", r, wantJob)
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
