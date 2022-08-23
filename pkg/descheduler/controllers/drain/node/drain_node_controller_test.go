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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ cache.Cache = &testCache{}

type testCache struct {
	nodeInfo *cache.NodeInfo
	pods     []*cache.PodInfo
}

func (t *testCache) GetNodeInfo(nodeName string) *cache.NodeInfo {
	return t.nodeInfo
}

func (t *testCache) GetPods(nodeName string) []*cache.PodInfo {
	return t.pods
}

func TestDrainNodeReconciler_Reconcile_init(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	type fields struct {
		objs []runtime.Object
	}
	type args struct {
		request reconcile.Request
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
		wantObj *v1alpha1.DrainNode
	}{
		{
			name: "init",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
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
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhasePending,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhasePending),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhasePending),
							Message: "Initialized",
						},
					},
				},
			},
		}, {
			name: "not found",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "1"}},
			},
			wantErr: false,
			wantObj: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.objs...).Build()
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: Name})

			r := &DrainNodeReconciler{
				Client:        c,
				eventRecorder: record.NewEventRecorderAdapter(recorder),
			}
			_, err := r.Reconcile(context.Background(), tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("DrainNodeReconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
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
				gotObj.Status.Conditions[0].LastTransitionTime =
					tt.wantObj.Status.Conditions[0].LastTransitionTime
				if !reflect.DeepEqual(gotObj, tt.wantObj) {
					t.Errorf("DrainNodeReconciler.Reconcile() = %v, want %v", gotObj, tt.wantObj)
				}
			}
		})
	}
}

func TestDrainNodeReconciler_Reconcile_Available_Waiting_Abnormal_Succeeded(t *testing.T) {
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
		name               string
		fields             fields
		args               args
		want               reconcile.Result
		wantErr            bool
		wantObj            *v1alpha1.DrainNode
		wantNode           *corev1.Node
		deletedReservation []string
		deletedJob         []string
	}{
		{
			name: "available new pod",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseAvailable,
						},
					},
				},
				cache: &testCache{
					pods: []*cache.PodInfo{
						{
							Ignore: true,
						},
						{
							NamespacedName: types.NamespacedName{Namespace: "ns", Name: "n1"},
							Ignore:         false,
						},
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
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseFailed,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseFailed),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseFailed),
							Message: "new pod ns/n1 scheduled to node ",
						},
					},
				},
			},
		}, {
			name: "available allocated reservation",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseAvailable,
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-321",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "ns",
								utils.PodNameKey:      "n1",
							},
						},
						Status: v1alpha1.ReservationStatus{
							CurrentOwners: []corev1.ObjectReference{{}},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-test",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "ns",
								utils.PodNameKey:      "n2",
							},
						},
					},
				},
				cache: &testCache{
					pods: []*cache.PodInfo{
						{
							Ignore: true,
						},
						{
							NamespacedName: types.NamespacedName{Namespace: "ns", Name: "n1"},
							Ignore:         false,
							UID:            "321",
						},
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
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseFailed,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseFailed),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseFailed),
							Message: "reservation 123-321 is used",
						},
					},
				},
			},
			deletedReservation: []string{"123-test"},
		}, {
			name: "waiting new pod",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseWaiting,
						},
					},
				},
				cache: &testCache{
					pods: []*cache.PodInfo{
						{
							Ignore: true,
						},
						{
							NamespacedName: types.NamespacedName{Namespace: "ns", Name: "n1"},
							Ignore:         false,
						},
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
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseFailed,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseFailed),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseFailed),
							Message: "new pod ns/n1 scheduled to node ",
						},
					},
				},
			},
		}, {
			name: "waiting allocated reservation",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseWaiting,
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-321",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "ns",
								utils.PodNameKey:      "n1",
							},
						},
						Status: v1alpha1.ReservationStatus{
							CurrentOwners: []corev1.ObjectReference{{}},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-test",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "ns",
								utils.PodNameKey:      "n2",
							},
						},
					},
				},
				cache: &testCache{
					pods: []*cache.PodInfo{
						{
							Ignore: true,
						},
						{
							NamespacedName: types.NamespacedName{Namespace: "ns", Name: "n1"},
							Ignore:         false,
							UID:            "321",
						},
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
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseFailed,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseFailed),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseFailed),
							Message: "reservation 123-321 is used",
						},
					},
				},
			},
			deletedReservation: []string{"123-test"},
		}, {
			name: "waiting confirmed running",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							ConfirmState: v1alpha1.ConfirmStateConfirmed,
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseWaiting,
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-321",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "ns",
								utils.PodNameKey:      "n1",
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-test",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "ns",
								utils.PodNameKey:      "n2",
							},
						},
					},
				},
				cache: &testCache{
					pods: []*cache.PodInfo{
						{
							Ignore: true,
						},
						{
							NamespacedName: types.NamespacedName{Namespace: "ns", Name: "n1"},
							Ignore:         false,
							UID:            "321",
						},
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
				},
				Spec: v1alpha1.DrainNodeSpec{
					ConfirmState: v1alpha1.ConfirmStateConfirmed,
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseRunning,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseRunning),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseRunning),
							Message: "ConfirmState Updated",
						},
					},
				},
			},
			deletedReservation: []string{"123-test"},
		}, {
			name: "waiting confirmed rejected",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							ConfirmState: v1alpha1.ConfirmStateRejected,
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseWaiting,
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-321",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "ns",
								utils.PodNameKey:      "n1",
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-test",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "ns",
								utils.PodNameKey:      "n2",
							},
						},
					},
				},
				cache: &testCache{
					pods: []*cache.PodInfo{
						{
							Ignore: true,
						},
						{
							NamespacedName: types.NamespacedName{Namespace: "ns", Name: "n1"},
							Ignore:         false,
							UID:            "321",
						},
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
				},
				Spec: v1alpha1.DrainNodeSpec{
					ConfirmState: v1alpha1.ConfirmStateRejected,
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseAborted,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseAborted),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseAborted),
							Message: "ConfirmState Updated",
						},
					},
				},
			},
			deletedReservation: []string{"123-test"},
		}, {
			name: "waiting abort",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
							Labels: map[string]string{
								utils.CleanKey: "true",
								utils.GroupKey: "group",
							},
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:        "node123",
							DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseWaiting,
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
									Key:    utils.GroupKey,
									Value:  "group",
									Effect: corev1.TaintEffectNoSchedule,
								},
								{
									Key:    "test",
									Value:  "group",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
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
						utils.CleanKey: "true",
						utils.GroupKey: "group",
					},
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:        "node123",
					DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseAborted,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodePhaseAborted),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodePhaseAborted),
							Message: "DrainNode Aborted",
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
		}, {
			name: "unavailable",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
							Labels: map[string]string{
								utils.CleanKey: "true",
								utils.GroupKey: "group",
							},
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:        "node123",
							DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseUnavailable,
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-321",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "ns",
								utils.PodNameKey:      "n1",
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-test",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "ns",
								utils.PodNameKey:      "n2",
							},
						},
					},
					&v1alpha1.PodMigrationJob{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123-test",
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: "ns",
								utils.PodNameKey:      "n2",
							},
						},
					},
					&v1alpha1.PodMigrationJob{
						ObjectMeta: metav1.ObjectMeta{
							Name: "456-test",
							Labels: map[string]string{
								utils.DrainNodeKey:    "456",
								utils.PodNamespaceKey: "ns",
								utils.PodNameKey:      "n1",
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
									Key:    utils.GroupKey,
									Value:  "group",
									Effect: corev1.TaintEffectNoSchedule,
								},
								{
									Key:    "test",
									Value:  "group",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
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
						utils.CleanKey: "true",
						utils.GroupKey: "group",
					},
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:        "node123",
					DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseUnavailable,
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
			deletedReservation: []string{"123-test", "123-321"},
			deletedJob:         []string{"123-test"},
		}, {
			name: "Succeeded",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
							Labels: map[string]string{
								utils.CleanKey: "true",
								utils.GroupKey: "group",
							},
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:        "node123",
							DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseSucceeded,
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
									Key:    utils.GroupKey,
									Value:  "group",
									Effect: corev1.TaintEffectNoSchedule,
								},
								{
									Key:    "test",
									Value:  "group",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
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
						utils.CleanKey: "true",
						utils.GroupKey: "group",
					},
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:        "node123",
					DrainNodePolicy: &v1alpha1.DrainNodePolicy{Mode: v1alpha1.DrainNodeModeTaint},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseSucceeded,
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
			for _, name := range tt.deletedReservation {
				r := &v1alpha1.Reservation{}
				if err := c.Get(context.Background(), types.NamespacedName{Name: name}, r); err != nil && errors.IsNotFound(err) {
					continue
				}
				t.Errorf("DrainNodeReconciler.Reconcile() found deleted reservation %v value %v", name, r)
			}
			for _, name := range tt.deletedJob {
				r := &v1alpha1.PodMigrationJob{}
				if err := c.Get(context.Background(), types.NamespacedName{Name: name}, r); err != nil && errors.IsNotFound(err) {
					continue
				}
				t.Errorf("DrainNodeReconciler.Reconcile() found deleted podMigrationJob %v value %v", name, r)
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
