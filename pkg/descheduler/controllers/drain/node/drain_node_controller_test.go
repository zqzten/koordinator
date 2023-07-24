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
	"fmt"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/cache"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/reservation"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/utils"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/evictions"
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

func TestDrainNodeReconciler_Reconcile_Init(t *testing.T) {
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
					Phase:      v1alpha1.DrainNodePhasePending,
					Conditions: nil,
				},
			},
		},
		{
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
				if !reflect.DeepEqual(gotObj, tt.wantObj) {
					t.Errorf("DrainNodeReconciler.Reconcile() = %v, want %v", gotObj, tt.wantObj)
				}
			}
		})
	}
}

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
			name: "cordon policy label mode",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName: "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{
								Mode: v1alpha1.CordonNodeModeLabel,
								Labels: map[string]string{
									"pool": "cold",
								},
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhasePending,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName: "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{
						Mode: v1alpha1.CordonNodeModeLabel,
						Labels: map[string]string{
							"pool": "cold",
						},
					},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseRunning,
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
					Labels: map[string]string{
						utils.PlanningKey: "",
						"pool":            "cold",
					},
				},
				Spec: corev1.NodeSpec{},
			},
			wantReservation: []*v1alpha1.Reservation{},
		},
		{
			name: "cordon policy taint mode",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName: "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{
								Mode: v1alpha1.CordonNodeModeTaint,
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhasePending,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			wantErr: false,
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName: "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{
						Mode: v1alpha1.CordonNodeModeTaint,
					},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseRunning,
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
					Labels: map[string]string{
						utils.PlanningKey: "",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
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
					gotObj.Status.Conditions[0].LastProbeTime =
						tt.wantObj.Status.Conditions[0].LastProbeTime
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

func TestDrainNodeReconciler_Reconcile_Running(t *testing.T) {
	podsOnNode := []*corev1.Pod{
		{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				UID: uuid.NewUUID(),
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         "apps/v1",
					Kind:               "ReplicaSet",
					Name:               "test-replicaset",
					UID:                uuid.NewUUID(),
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				}},
				Name:      "test-pod-0",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				NodeName: "123",
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
			Status: corev1.PodStatus{},
		},
		{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				UID:         uuid.NewUUID(),
				Annotations: map[string]string{"Unmigratable": "Unmigratable"},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         "apps/v1",
					Kind:               "ReplicaSet",
					Name:               "test-replicaset",
					UID:                uuid.NewUUID(),
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				}},
				Name:      "test-pod-1",
				Namespace: "default",
			},
			Spec:   corev1.PodSpec{},
			Status: corev1.PodStatus{},
		},
		{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				UID: uuid.NewUUID(),
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         "apps/v1",
					Kind:               "ReplicaSet",
					Name:               "test-replicaset",
					UID:                uuid.NewUUID(),
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				}},
				Name:      "test-pod-1",
				Namespace: "default",
			},
			Spec:   corev1.PodSpec{},
			Status: corev1.PodStatus{},
		},
	}
	now := metav1.Now()

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
		name              string
		fields            fields
		args              args
		requeue           bool
		funcBeforeRequeue func(reconciler *DrainNodeReconciler)
		want              reconcile.Result
		wantErr           bool
		wantObj           *v1alpha1.DrainNode
		wantNode          *corev1.Node
		wantReservation   []*v1alpha1.Reservation
		wantJob           []*v1alpha1.PodMigrationJob
	}{
		{
			name: "migrationMode WaitFirst, no unmigratable pod, no unexpected reservation",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "123",
							CreationTimestamp: now,
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeWaitFirst,
								WaitDuration: &metav1.Duration{Duration: time.Hour},
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Name:        "123",
						Reservation: nil,
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "123",
					CreationTimestamp: now,
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeWaitFirst,
						WaitDuration: &metav1.Duration{Duration: time.Hour},
					},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseRunning,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:         podsOnNode[0].UID,
							Namespace:      podsOnNode[0].Namespace,
							PodName:        podsOnNode[0].Name,
							Phase:          v1alpha1.PodMigrationPhaseWaiting,
							StartTimestamp: metav1.Time{},
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 0),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
							Message: fmt.Sprintf("unmigratable pod count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
							Message: fmt.Sprintf("unavailable reservation count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("failed job count: %d", 0),
							Message: fmt.Sprintf("failed job count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionOnceAvailable,
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeConditionOnceAvailable),
							Message: "no unmigratable, waiting, ready and unavailable migration",
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 0,
						v1alpha1.PodMigrationPhaseWaiting:      1,
						v1alpha1.PodMigrationPhaseReady:        0,
						v1alpha1.PodMigrationPhaseAvailable:    0,
						v1alpha1.PodMigrationPhaseUnavailable:  0,
						v1alpha1.PodMigrationPhaseMigrating:    0,
						v1alpha1.PodMigrationPhaseSucceed:      0,
						v1alpha1.PodMigrationPhaseFailed:       0,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: nil,
		},
		{
			name: "migrationMode evictDirectly, one normal to pending reservation",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
								WaitDuration: nil,
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Name:        "123",
						Reservation: nil,
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
						WaitDuration: nil,
					},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseRunning,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:               podsOnNode[0].UID,
							Namespace:            podsOnNode[0].Namespace,
							PodName:              podsOnNode[0].Name,
							Phase:                v1alpha1.PodMigrationPhaseReady,
							StartTimestamp:       metav1.Time{},
							ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							ReservationPhase:     "",
							TargetNode:           "",
							PodMigrationJobName:  "",
							PodMigrationJobPhase: "",
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 0),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
							Message: fmt.Sprintf("unmigratable pod count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
							Message: fmt.Sprintf("unavailable reservation count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("failed job count: %d", 0),
							Message: fmt.Sprintf("failed job count: %d", 0),
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 0,
						v1alpha1.PodMigrationPhaseWaiting:      0,
						v1alpha1.PodMigrationPhaseReady:        1,
						v1alpha1.PodMigrationPhaseAvailable:    0,
						v1alpha1.PodMigrationPhaseUnavailable:  0,
						v1alpha1.PodMigrationPhaseMigrating:    0,
						v1alpha1.PodMigrationPhaseSucceed:      0,
						v1alpha1.PodMigrationPhaseFailed:       0,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
						Labels: map[string]string{
							utils.DrainNodeKey:    "123",
							utils.PodNamespaceKey: podsOnNode[0].Namespace,
							utils.PodNameKey:      podsOnNode[0].Name,
						},
						OwnerReferences: []metav1.OwnerReference{
							{APIVersion: "scheduling.koordinator.sh/v1alpha1",
								Kind:               "DrainNode",
								Name:               "123",
								UID:                "",
								Controller:         pointer.Bool(true),
								BlockOwnerDeletion: pointer.Bool(true),
							},
						},
					},
					Spec: v1alpha1.ReservationSpec{
						Template: &corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: podsOnNode[0].Namespace,
								Labels: map[string]string{
									utils.PodNameKey: podsOnNode[0].Name,
								},
								OwnerReferences: podsOnNode[0].OwnerReferences,
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
								OwnerReference: podsOnNode[0].OwnerReferences[0],
								Namespace:      podsOnNode[0].Namespace,
							}},
						},
						AllocateOnce: pointer.Bool(true),
					},
				},
			},
		},
		{
			name: "migrationMode evictDirectly, one normal to pending reservation, one unmigratable pod, one unexpected reservation",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
								WaitDuration: nil,
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Name:        "123",
						Reservation: map[string]struct{}{"unexpectedReservation": {}},
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
						{
							UID: podsOnNode[1].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[1].Namespace,
								Name:      podsOnNode[1].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[1],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
						WaitDuration: nil,
					},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseRunning,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:               podsOnNode[0].UID,
							Namespace:            podsOnNode[0].Namespace,
							PodName:              podsOnNode[0].Name,
							Phase:                v1alpha1.PodMigrationPhaseReady,
							StartTimestamp:       metav1.Time{},
							ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							ReservationPhase:     "",
							TargetNode:           "",
							PodMigrationJobName:  "",
							PodMigrationJobPhase: "",
						},
						{
							PodUID:               podsOnNode[1].UID,
							Namespace:            podsOnNode[1].Namespace,
							PodName:              podsOnNode[1].Name,
							Phase:                v1alpha1.PodMigrationPhaseUnmigratable,
							StartTimestamp:       metav1.Time{},
							ReservationName:      "",
							ReservationPhase:     "",
							TargetNode:           "",
							PodMigrationJobName:  "",
							PodMigrationJobPhase: "",
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionTrue,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 1),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{"unexpectedReservation"}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionTrue,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 1),
							Message: fmt.Sprintf("unmigratable pod count: %d", 1),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
							Message: fmt.Sprintf("unavailable reservation count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("failed job count: %d", 0),
							Message: fmt.Sprintf("failed job count: %d", 0),
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 1,
						v1alpha1.PodMigrationPhaseWaiting:      0,
						v1alpha1.PodMigrationPhaseReady:        1,
						v1alpha1.PodMigrationPhaseAvailable:    0,
						v1alpha1.PodMigrationPhaseUnavailable:  0,
						v1alpha1.PodMigrationPhaseMigrating:    0,
						v1alpha1.PodMigrationPhaseSucceed:      0,
						v1alpha1.PodMigrationPhaseFailed:       0,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
						Labels: map[string]string{
							utils.DrainNodeKey:    "123",
							utils.PodNamespaceKey: podsOnNode[0].Namespace,
							utils.PodNameKey:      podsOnNode[0].Name,
						},
						OwnerReferences: []metav1.OwnerReference{
							{APIVersion: "scheduling.koordinator.sh/v1alpha1",
								Kind:               "DrainNode",
								Name:               "123",
								UID:                "",
								Controller:         pointer.Bool(true),
								BlockOwnerDeletion: pointer.Bool(true),
							},
						},
					},
					Spec: v1alpha1.ReservationSpec{
						Template: &corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: podsOnNode[0].Namespace,
								Labels: map[string]string{
									utils.PodNameKey: podsOnNode[0].Name,
								},
								OwnerReferences: podsOnNode[0].OwnerReferences,
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
								OwnerReference: podsOnNode[0].OwnerReferences[0],
								Namespace:      podsOnNode[0].Namespace,
							}},
						},
						AllocateOnce: pointer.Bool(true),
					},
				},
			},
		},
		{
			name: "migrationMode evictDirectly, one normal to pending reservation, one unmigratable pod, one unexpected reservation; unmigratable pod changed to wait after requeue",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
								WaitDuration: nil,
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Name:        "123",
						Reservation: map[string]struct{}{"unexpectedReservation": {}},
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
						{
							UID: podsOnNode[1].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[1].Namespace,
								Name:      podsOnNode[1].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[1],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			requeue: true,
			funcBeforeRequeue: func(reconciler *DrainNodeReconciler) {
				reconciler.podFilter = func(pod *corev1.Pod) bool {
					return true
				}
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
						WaitDuration: nil,
					},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseRunning,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:               podsOnNode[0].UID,
							Namespace:            podsOnNode[0].Namespace,
							PodName:              podsOnNode[0].Name,
							Phase:                v1alpha1.PodMigrationPhaseReady,
							StartTimestamp:       metav1.Time{},
							ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							ReservationPhase:     "",
							TargetNode:           "",
							PodMigrationJobName:  "",
							PodMigrationJobPhase: "",
						},
						{
							PodUID:               podsOnNode[1].UID,
							Namespace:            podsOnNode[1].Namespace,
							PodName:              podsOnNode[1].Name,
							Phase:                v1alpha1.PodMigrationPhaseReady,
							StartTimestamp:       metav1.Time{},
							ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[1].UID),
							ReservationPhase:     "",
							TargetNode:           "",
							PodMigrationJobName:  "",
							PodMigrationJobPhase: "",
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionTrue,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 1),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{"unexpectedReservation"}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
							Message: fmt.Sprintf("unmigratable pod count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
							Message: fmt.Sprintf("unavailable reservation count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("failed job count: %d", 0),
							Message: fmt.Sprintf("failed job count: %d", 0),
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 0,
						v1alpha1.PodMigrationPhaseWaiting:      0,
						v1alpha1.PodMigrationPhaseReady:        2,
						v1alpha1.PodMigrationPhaseAvailable:    0,
						v1alpha1.PodMigrationPhaseUnavailable:  0,
						v1alpha1.PodMigrationPhaseMigrating:    0,
						v1alpha1.PodMigrationPhaseSucceed:      0,
						v1alpha1.PodMigrationPhaseFailed:       0,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
						Labels: map[string]string{
							utils.DrainNodeKey:    "123",
							utils.PodNamespaceKey: podsOnNode[0].Namespace,
							utils.PodNameKey:      podsOnNode[0].Name,
						},
						OwnerReferences: []metav1.OwnerReference{
							{APIVersion: "scheduling.koordinator.sh/v1alpha1",
								Kind:               "DrainNode",
								Name:               "123",
								UID:                "",
								Controller:         pointer.Bool(true),
								BlockOwnerDeletion: pointer.Bool(true),
							},
						},
					},
					Spec: v1alpha1.ReservationSpec{
						Template: &corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: podsOnNode[0].Namespace,
								Labels: map[string]string{
									utils.PodNameKey: podsOnNode[0].Name,
								},
								OwnerReferences: podsOnNode[0].OwnerReferences,
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
								OwnerReference: podsOnNode[0].OwnerReferences[0],
								Namespace:      podsOnNode[0].Namespace,
							}},
						},
						AllocateOnce: pointer.Bool(true),
					},
				},
			},
		},
		{
			name: "migrationMode evictDirectly, one normal to pending reservation, one unmigratable pod changed to wait because dn have annotation, one unexpected reservation",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
							Annotations: map[string]string{
								evictions.EvictPodAnnotationKey: "true",
							},
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
								WaitDuration: nil,
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Name:        "123",
						Reservation: map[string]struct{}{"unexpectedReservation": {}},
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
						{
							UID: podsOnNode[1].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[1].Namespace,
								Name:      podsOnNode[1].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[1],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
						WaitDuration: nil,
					},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseRunning,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:               podsOnNode[0].UID,
							Namespace:            podsOnNode[0].Namespace,
							PodName:              podsOnNode[0].Name,
							Phase:                v1alpha1.PodMigrationPhaseReady,
							StartTimestamp:       metav1.Time{},
							ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							ReservationPhase:     "",
							TargetNode:           "",
							PodMigrationJobName:  "",
							PodMigrationJobPhase: "",
						},
						{
							PodUID:               podsOnNode[1].UID,
							Namespace:            podsOnNode[1].Namespace,
							PodName:              podsOnNode[1].Name,
							Phase:                v1alpha1.PodMigrationPhaseReady,
							StartTimestamp:       metav1.Time{},
							ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[1].UID),
							ReservationPhase:     "",
							TargetNode:           "",
							PodMigrationJobName:  "",
							PodMigrationJobPhase: "",
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionTrue,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 1),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{"unexpectedReservation"}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
							Message: fmt.Sprintf("unmigratable pod count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
							Message: fmt.Sprintf("unavailable reservation count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("failed job count: %d", 0),
							Message: fmt.Sprintf("failed job count: %d", 0),
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 0,
						v1alpha1.PodMigrationPhaseWaiting:      0,
						v1alpha1.PodMigrationPhaseReady:        2,
						v1alpha1.PodMigrationPhaseAvailable:    0,
						v1alpha1.PodMigrationPhaseUnavailable:  0,
						v1alpha1.PodMigrationPhaseMigrating:    0,
						v1alpha1.PodMigrationPhaseSucceed:      0,
						v1alpha1.PodMigrationPhaseFailed:       0,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
						Labels: map[string]string{
							utils.DrainNodeKey:    "123",
							utils.PodNamespaceKey: podsOnNode[0].Namespace,
							utils.PodNameKey:      podsOnNode[0].Name,
						},
						OwnerReferences: []metav1.OwnerReference{
							{APIVersion: "scheduling.koordinator.sh/v1alpha1",
								Kind:               "DrainNode",
								Name:               "123",
								UID:                "",
								Controller:         pointer.Bool(true),
								BlockOwnerDeletion: pointer.Bool(true),
							},
						},
					},
					Spec: v1alpha1.ReservationSpec{
						Template: &corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: podsOnNode[0].Namespace,
								Labels: map[string]string{
									utils.PodNameKey: podsOnNode[0].Name,
								},
								OwnerReferences: podsOnNode[0].OwnerReferences,
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
								OwnerReference: podsOnNode[0].OwnerReferences[0],
								Namespace:      podsOnNode[0].Namespace,
							}},
						},
						AllocateOnce: pointer.Bool(true),
					},
				},
			},
		},
		{
			name: "migrationMode evictDirectly, one unavailable reservation",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
								WaitDuration: nil,
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: podsOnNode[0].Namespace,
								utils.PodNameKey:      podsOnNode[0].Name,
							},
							OwnerReferences: []metav1.OwnerReference{
								{APIVersion: "scheduling.koordinator.sh/v1alpha1",
									Kind:               "DrainNode",
									Name:               "123",
									UID:                "",
									Controller:         pointer.Bool(true),
									BlockOwnerDeletion: pointer.Bool(true),
								},
							},
						},
						Spec: v1alpha1.ReservationSpec{
							Template: &corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: podsOnNode[0].Namespace,
									Labels: map[string]string{
										utils.PodNameKey: podsOnNode[0].Name,
									},
									OwnerReferences: podsOnNode[0].OwnerReferences,
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
														MatchFields: []corev1.NodeSelectorRequirement{{
															Key:      "metadata.name",
															Operator: corev1.NodeSelectorOpNotIn,
															Values:   []string{podsOnNode[0].Spec.NodeName},
														}},
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
															Key:      utils.DrainNodeKey,
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
									OwnerReference: podsOnNode[0].OwnerReferences[0],
									Namespace:      podsOnNode[0].Namespace,
								}},
							},
							AllocateOnce: pointer.Bool(true),
						},
						Status: v1alpha1.ReservationStatus{
							Phase: v1alpha1.ReservationSucceeded,
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Name:        "123",
						Reservation: nil,
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
						WaitDuration: nil,
					},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseComplete,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:               podsOnNode[0].UID,
							Namespace:            podsOnNode[0].Namespace,
							PodName:              podsOnNode[0].Name,
							Phase:                v1alpha1.PodMigrationPhaseUnavailable,
							StartTimestamp:       metav1.Time{},
							ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							ReservationPhase:     v1alpha1.ReservationSucceeded,
							TargetNode:           "",
							PodMigrationJobName:  "",
							PodMigrationJobPhase: "",
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 0),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
							Message: fmt.Sprintf("unmigratable pod count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionTrue,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 1),
							Message: fmt.Sprintf("unavailable reservation count: %d", 1),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("failed job count: %d", 0),
							Message: fmt.Sprintf("failed job count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedPodAfterCompleteExists,
							Status:  metav1.ConditionFalse,
							Reason:  "",
							Message: "",
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 0,
						v1alpha1.PodMigrationPhaseWaiting:      0,
						v1alpha1.PodMigrationPhaseReady:        0,
						v1alpha1.PodMigrationPhaseAvailable:    0,
						v1alpha1.PodMigrationPhaseUnavailable:  1,
						v1alpha1.PodMigrationPhaseMigrating:    0,
						v1alpha1.PodMigrationPhaseSucceed:      0,
						v1alpha1.PodMigrationPhaseFailed:       0,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{},
		},
		{
			name: "migrationMode evictDirectly, one ready migration to Available",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
								WaitDuration: nil,
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: podsOnNode[0].Namespace,
								utils.PodNameKey:      podsOnNode[0].Name,
							},
							OwnerReferences: []metav1.OwnerReference{
								{APIVersion: "scheduling.koordinator.sh/v1alpha1",
									Kind:               "DrainNode",
									Name:               "123",
									UID:                "",
									Controller:         pointer.Bool(true),
									BlockOwnerDeletion: pointer.Bool(true),
								},
							},
						},
						Spec: v1alpha1.ReservationSpec{
							Template: &corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: podsOnNode[0].Namespace,
									Labels: map[string]string{
										utils.PodNameKey: podsOnNode[0].Name,
									},
									OwnerReferences: podsOnNode[0].OwnerReferences,
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
														MatchFields: []corev1.NodeSelectorRequirement{{
															Key:      "metadata.name",
															Operator: corev1.NodeSelectorOpNotIn,
															Values:   []string{podsOnNode[0].Spec.NodeName},
														}},
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
															Key:      utils.DrainNodeKey,
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
									OwnerReference: podsOnNode[0].OwnerReferences[0],
									Namespace:      podsOnNode[0].Namespace,
								}},
							},
							AllocateOnce: pointer.Bool(true),
						},
						Status: v1alpha1.ReservationStatus{
							Phase:    v1alpha1.ReservationAvailable,
							NodeName: "456",
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Name:        "123",
						Reservation: nil,
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
						WaitDuration: nil,
					},
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseRunning,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:               podsOnNode[0].UID,
							Namespace:            podsOnNode[0].Namespace,
							PodName:              podsOnNode[0].Name,
							Phase:                v1alpha1.PodMigrationPhaseAvailable,
							StartTimestamp:       metav1.Time{},
							ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							ReservationPhase:     v1alpha1.ReservationAvailable,
							TargetNode:           "456",
							PodMigrationJobName:  "",
							PodMigrationJobPhase: "",
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 0),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
							Message: fmt.Sprintf("unmigratable pod count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
							Message: fmt.Sprintf("unavailable reservation count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("failed job count: %d", 0),
							Message: fmt.Sprintf("failed job count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionOnceAvailable,
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeConditionOnceAvailable),
							Message: "no unmigratable, waiting, ready and unavailable migration",
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 0,
						v1alpha1.PodMigrationPhaseWaiting:      0,
						v1alpha1.PodMigrationPhaseReady:        0,
						v1alpha1.PodMigrationPhaseAvailable:    1,
						v1alpha1.PodMigrationPhaseUnavailable:  0,
						v1alpha1.PodMigrationPhaseMigrating:    0,
						v1alpha1.PodMigrationPhaseSucceed:      0,
						v1alpha1.PodMigrationPhaseFailed:       0,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{},
		},
		{
			name: "migrationMode evictDirectly, one ready migration to pending job",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
							Annotations: map[string]string{
								utils.PausedAfterConfirmed: "true",
							},
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
								WaitDuration: nil,
							},
							ConfirmState: v1alpha1.ConfirmStateConfirmed,
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: podsOnNode[0].Namespace,
								utils.PodNameKey:      podsOnNode[0].Name,
							},
							OwnerReferences: []metav1.OwnerReference{
								{APIVersion: "scheduling.koordinator.sh/v1alpha1",
									Kind:               "DrainNode",
									Name:               "123",
									UID:                "",
									Controller:         pointer.Bool(true),
									BlockOwnerDeletion: pointer.Bool(true),
								},
							},
						},
						Spec: v1alpha1.ReservationSpec{
							Template: &corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: podsOnNode[0].Namespace,
									Labels: map[string]string{
										utils.PodNameKey: podsOnNode[0].Name,
									},
									OwnerReferences: podsOnNode[0].OwnerReferences,
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
														MatchFields: []corev1.NodeSelectorRequirement{{
															Key:      "metadata.name",
															Operator: corev1.NodeSelectorOpNotIn,
															Values:   []string{podsOnNode[0].Spec.NodeName},
														}},
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
															Key:      utils.DrainNodeKey,
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
									OwnerReference: podsOnNode[0].OwnerReferences[0],
									Namespace:      podsOnNode[0].Namespace,
								}},
							},
							AllocateOnce: pointer.Bool(true),
						},
						Status: v1alpha1.ReservationStatus{
							Phase:    v1alpha1.ReservationAvailable,
							NodeName: "456",
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Name:        "123",
						Reservation: nil,
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
					Annotations: map[string]string{
						utils.PausedAfterConfirmed: "true",
					},
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
						WaitDuration: nil,
					},
					ConfirmState: v1alpha1.ConfirmStateConfirmed,
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseRunning,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:                         podsOnNode[0].UID,
							Namespace:                      podsOnNode[0].Namespace,
							PodName:                        podsOnNode[0].Name,
							Phase:                          v1alpha1.PodMigrationPhaseMigrating,
							StartTimestamp:                 metav1.Time{},
							ChartedAfterDrainNodeConfirmed: true,
							ReservationName:                getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							ReservationPhase:               v1alpha1.ReservationAvailable,
							TargetNode:                     "456",
							PodMigrationJobName:            getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							PodMigrationJobPaused:          pointer.Bool(true),
							PodMigrationJobPhase:           "",
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 0),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
							Message: fmt.Sprintf("unmigratable pod count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
							Message: fmt.Sprintf("unavailable reservation count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("failed job count: %d", 0),
							Message: fmt.Sprintf("failed job count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionOnceAvailable,
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeConditionOnceAvailable),
							Message: "no unmigratable, waiting, ready and unavailable migration",
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 0,
						v1alpha1.PodMigrationPhaseWaiting:      0,
						v1alpha1.PodMigrationPhaseReady:        0,
						v1alpha1.PodMigrationPhaseAvailable:    0,
						v1alpha1.PodMigrationPhaseUnavailable:  0,
						v1alpha1.PodMigrationPhaseMigrating:    1,
						v1alpha1.PodMigrationPhaseSucceed:      0,
						v1alpha1.PodMigrationPhaseFailed:       0,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{},
		},
		{
			name: "migrationMode evictDirectly, one ready migration to pending job, available condition has been satisfied, but new pod scheduled to this node",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
								WaitDuration: nil,
							},
							ConfirmState: v1alpha1.ConfirmStateConfirmed,
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
							PodMigrations: []v1alpha1.PodMigration{
								{
									PodUID:               podsOnNode[0].UID,
									Namespace:            podsOnNode[0].Namespace,
									PodName:              podsOnNode[0].Name,
									Phase:                v1alpha1.PodMigrationPhaseMigrating,
									StartTimestamp:       metav1.Time{},
									ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[0].UID),
									ReservationPhase:     v1alpha1.ReservationAvailable,
									TargetNode:           "456",
									PodMigrationJobName:  getReservationOrMigrationJobName("123", podsOnNode[0].UID),
									PodMigrationJobPhase: "",
								},
							},
							Conditions: []v1alpha1.DrainNodeCondition{
								{
									Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
									Status:  metav1.ConditionFalse,
									Reason:  fmt.Sprintf("unexpected reservation count: %d,", 0),
									Message: fmt.Sprintf("unexpectedRR: %v", []string{}),
								},
								{
									Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
									Status:  metav1.ConditionFalse,
									Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
									Message: fmt.Sprintf("unmigratable pod count: %d", 0),
								},
								{
									Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
									Status:  metav1.ConditionFalse,
									Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
									Message: fmt.Sprintf("unavailable reservation count: %d", 0),
								},
								{
									Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
									Status:  metav1.ConditionFalse,
									Reason:  fmt.Sprintf("failed job count: %d", 0),
									Message: fmt.Sprintf("failed job count: %d", 0),
								},
								{
									Type:    v1alpha1.DrainNodeConditionOnceAvailable,
									Status:  metav1.ConditionFalse,
									Reason:  string(v1alpha1.DrainNodeConditionOnceAvailable),
									Message: "no unmigratable, waiting, ready and unavailable migration",
								},
							},
							PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
								v1alpha1.PodMigrationPhaseUnmigratable: 0,
								v1alpha1.PodMigrationPhaseWaiting:      0,
								v1alpha1.PodMigrationPhaseReady:        0,
								v1alpha1.PodMigrationPhaseAvailable:    1,
								v1alpha1.PodMigrationPhaseUnavailable:  0,
								v1alpha1.PodMigrationPhaseMigrating:    0,
								v1alpha1.PodMigrationPhaseSucceed:      0,
								v1alpha1.PodMigrationPhaseFailed:       0,
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: podsOnNode[0].Namespace,
								utils.PodNameKey:      podsOnNode[0].Name,
							},
							OwnerReferences: []metav1.OwnerReference{
								{APIVersion: "scheduling.koordinator.sh/v1alpha1",
									Kind:               "DrainNode",
									Name:               "123",
									UID:                "",
									Controller:         pointer.Bool(true),
									BlockOwnerDeletion: pointer.Bool(true),
								},
							},
						},
						Spec: v1alpha1.ReservationSpec{
							Template: &corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: podsOnNode[0].Namespace,
									Labels: map[string]string{
										utils.PodNameKey: podsOnNode[0].Name,
									},
									OwnerReferences: podsOnNode[0].OwnerReferences,
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
														MatchFields: []corev1.NodeSelectorRequirement{{
															Key:      "metadata.name",
															Operator: corev1.NodeSelectorOpNotIn,
															Values:   []string{podsOnNode[0].Spec.NodeName},
														}},
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
															Key:      utils.DrainNodeKey,
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
									OwnerReference: podsOnNode[0].OwnerReferences[0],
									Namespace:      podsOnNode[0].Namespace,
								}},
							},
							AllocateOnce: pointer.Bool(true),
						},
						Status: v1alpha1.ReservationStatus{
							Phase:    v1alpha1.ReservationAvailable,
							NodeName: "456",
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Name:        "123",
						Reservation: nil,
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
						{
							UID: podsOnNode[2].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[2].Namespace,
								Name:      podsOnNode[2].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[2],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
						WaitDuration: nil,
					},
					ConfirmState: v1alpha1.ConfirmStateConfirmed,
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseRunning,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:                podsOnNode[0].UID,
							Namespace:             podsOnNode[0].Namespace,
							PodName:               podsOnNode[0].Name,
							Phase:                 v1alpha1.PodMigrationPhaseMigrating,
							StartTimestamp:        metav1.Time{},
							ReservationName:       getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							ReservationPhase:      v1alpha1.ReservationAvailable,
							TargetNode:            "456",
							PodMigrationJobName:   getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							PodMigrationJobPaused: pointer.Bool(false),
							PodMigrationJobPhase:  "",
						},
						{
							PodUID:                         podsOnNode[2].UID,
							Namespace:                      podsOnNode[2].Namespace,
							PodName:                        podsOnNode[2].Name,
							Phase:                          v1alpha1.PodMigrationPhaseReady,
							ChartedAfterDrainNodeConfirmed: true,
							StartTimestamp:                 metav1.Time{},
							ReservationName:                getReservationOrMigrationJobName("123", podsOnNode[2].UID),
							ReservationPhase:               "",
							TargetNode:                     "",
							PodMigrationJobName:            "",
							PodMigrationJobPhase:           "",
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 0),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
							Message: fmt.Sprintf("unmigratable pod count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
							Message: fmt.Sprintf("unavailable reservation count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("failed job count: %d", 0),
							Message: fmt.Sprintf("failed job count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionOnceAvailable,
							Status:  metav1.ConditionFalse,
							Reason:  string(v1alpha1.DrainNodeConditionOnceAvailable),
							Message: "no unmigratable, waiting, ready and unavailable migration",
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 0,
						v1alpha1.PodMigrationPhaseWaiting:      0,
						v1alpha1.PodMigrationPhaseReady:        1,
						v1alpha1.PodMigrationPhaseAvailable:    0,
						v1alpha1.PodMigrationPhaseUnavailable:  0,
						v1alpha1.PodMigrationPhaseMigrating:    1,
						v1alpha1.PodMigrationPhaseSucceed:      0,
						v1alpha1.PodMigrationPhaseFailed:       0,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{},
		},
		{
			name: "migrationMode evictDirectly, one ready migration, but user rejected",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
								WaitDuration: nil,
							},
							ConfirmState: v1alpha1.ConfirmStateRejected,
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
							PodMigrations: []v1alpha1.PodMigration{
								{
									PodUID:               podsOnNode[0].UID,
									Namespace:            podsOnNode[0].Namespace,
									PodName:              podsOnNode[0].Name,
									Phase:                v1alpha1.PodMigrationPhaseMigrating,
									StartTimestamp:       metav1.Time{},
									ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[0].UID),
									ReservationPhase:     v1alpha1.ReservationAvailable,
									TargetNode:           "456",
									PodMigrationJobName:  "",
									PodMigrationJobPhase: "",
								},
							},
							Conditions: []v1alpha1.DrainNodeCondition{
								{
									Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
									Status:  metav1.ConditionFalse,
									Reason:  fmt.Sprintf("unexpected reservation count: %d,", 0),
									Message: fmt.Sprintf("unexpectedRR: %v", []string{}),
								},
								{
									Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
									Status:  metav1.ConditionFalse,
									Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
									Message: fmt.Sprintf("unmigratable pod count: %d", 0),
								},
								{
									Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
									Status:  metav1.ConditionFalse,
									Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
									Message: fmt.Sprintf("unavailable reservation count: %d", 0),
								},
								{
									Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
									Status:  metav1.ConditionFalse,
									Reason:  fmt.Sprintf("failed job count: %d", 0),
									Message: fmt.Sprintf("failed job count: %d", 0),
								},
								{
									Type:    v1alpha1.DrainNodeConditionOnceAvailable,
									Status:  metav1.ConditionTrue,
									Reason:  string(v1alpha1.DrainNodeConditionOnceAvailable),
									Message: "no unmigratable, waiting, ready and unavailable migration",
								},
							},
							PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
								v1alpha1.PodMigrationPhaseUnmigratable: 0,
								v1alpha1.PodMigrationPhaseWaiting:      0,
								v1alpha1.PodMigrationPhaseReady:        0,
								v1alpha1.PodMigrationPhaseAvailable:    1,
								v1alpha1.PodMigrationPhaseUnavailable:  0,
								v1alpha1.PodMigrationPhaseMigrating:    0,
								v1alpha1.PodMigrationPhaseSucceed:      0,
								v1alpha1.PodMigrationPhaseFailed:       0,
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: podsOnNode[0].Namespace,
								utils.PodNameKey:      podsOnNode[0].Name,
							},
							OwnerReferences: []metav1.OwnerReference{
								{APIVersion: "scheduling.koordinator.sh/v1alpha1",
									Kind:               "DrainNode",
									Name:               "123",
									UID:                "",
									Controller:         pointer.Bool(true),
									BlockOwnerDeletion: pointer.Bool(true),
								},
							},
						},
						Spec: v1alpha1.ReservationSpec{
							Template: &corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: podsOnNode[0].Namespace,
									Labels: map[string]string{
										utils.PodNameKey: podsOnNode[0].Name,
									},
									OwnerReferences: podsOnNode[0].OwnerReferences,
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
														MatchFields: []corev1.NodeSelectorRequirement{{
															Key:      "metadata.name",
															Operator: corev1.NodeSelectorOpNotIn,
															Values:   []string{podsOnNode[0].Spec.NodeName},
														}},
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
															Key:      utils.DrainNodeKey,
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
									OwnerReference: podsOnNode[0].OwnerReferences[0],
									Namespace:      podsOnNode[0].Namespace,
								}},
							},
							AllocateOnce: pointer.Bool(true),
						},
						Status: v1alpha1.ReservationStatus{
							Phase:    v1alpha1.ReservationAvailable,
							NodeName: "456",
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Name:        "123",
						Reservation: nil,
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
						WaitDuration: nil,
					},
					ConfirmState: v1alpha1.ConfirmStateRejected,
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseAborted,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:               podsOnNode[0].UID,
							Namespace:            podsOnNode[0].Namespace,
							PodName:              podsOnNode[0].Name,
							Phase:                v1alpha1.PodMigrationPhaseMigrating,
							StartTimestamp:       metav1.Time{},
							ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							ReservationPhase:     v1alpha1.ReservationAvailable,
							TargetNode:           "456",
							PodMigrationJobName:  "",
							PodMigrationJobPhase: "",
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 0),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
							Message: fmt.Sprintf("unmigratable pod count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
							Message: fmt.Sprintf("unavailable reservation count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("failed job count: %d", 0),
							Message: fmt.Sprintf("failed job count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionOnceAvailable,
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeConditionOnceAvailable),
							Message: "no unmigratable, waiting, ready and unavailable migration",
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 0,
						v1alpha1.PodMigrationPhaseWaiting:      0,
						v1alpha1.PodMigrationPhaseReady:        0,
						v1alpha1.PodMigrationPhaseAvailable:    1,
						v1alpha1.PodMigrationPhaseUnavailable:  0,
						v1alpha1.PodMigrationPhaseMigrating:    0,
						v1alpha1.PodMigrationPhaseSucceed:      0,
						v1alpha1.PodMigrationPhaseFailed:       0,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{},
			},
			wantReservation: []*v1alpha1.Reservation{},
		},
		{
			name: "migrationMode evictDirectly, one migrating migration to pending job",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
								WaitDuration: nil,
							},
							ConfirmState: v1alpha1.ConfirmStateConfirmed,
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
							PodMigrations: []v1alpha1.PodMigration{
								{
									PodUID:               podsOnNode[0].UID,
									Namespace:            podsOnNode[0].Namespace,
									PodName:              podsOnNode[0].Name,
									Phase:                v1alpha1.PodMigrationPhaseMigrating,
									StartTimestamp:       metav1.Time{},
									ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[0].UID),
									ReservationPhase:     v1alpha1.ReservationAvailable,
									TargetNode:           "456",
									PodMigrationJobName:  "",
									PodMigrationJobPhase: "",
								},
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: podsOnNode[0].Namespace,
								utils.PodNameKey:      podsOnNode[0].Name,
							},
							OwnerReferences: []metav1.OwnerReference{
								{APIVersion: "scheduling.koordinator.sh/v1alpha1",
									Kind:               "DrainNode",
									Name:               "123",
									UID:                "",
									Controller:         pointer.Bool(true),
									BlockOwnerDeletion: pointer.Bool(true),
								},
							},
						},
						Spec: v1alpha1.ReservationSpec{
							Template: &corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: podsOnNode[0].Namespace,
									Labels: map[string]string{
										utils.PodNameKey: podsOnNode[0].Name,
									},
									OwnerReferences: podsOnNode[0].OwnerReferences,
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
														MatchFields: []corev1.NodeSelectorRequirement{{
															Key:      "metadata.name",
															Operator: corev1.NodeSelectorOpNotIn,
															Values:   []string{podsOnNode[0].Spec.NodeName},
														}},
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
															Key:      utils.DrainNodeKey,
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
									OwnerReference: podsOnNode[0].OwnerReferences[0],
									Namespace:      podsOnNode[0].Namespace,
								}},
							},
							AllocateOnce: pointer.Bool(true),
						},
						Status: v1alpha1.ReservationStatus{
							Phase:    v1alpha1.ReservationAvailable,
							NodeName: "456",
						},
					},
				},
				cache: &testCache{
					nodeInfo: &cache.NodeInfo{
						Name:        "123",
						Reservation: nil,
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
						WaitDuration: nil,
					},
					ConfirmState: v1alpha1.ConfirmStateConfirmed,
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseRunning,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:                podsOnNode[0].UID,
							Namespace:             podsOnNode[0].Namespace,
							PodName:               podsOnNode[0].Name,
							Phase:                 v1alpha1.PodMigrationPhaseMigrating,
							StartTimestamp:        metav1.Time{},
							ReservationName:       getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							ReservationPhase:      v1alpha1.ReservationAvailable,
							TargetNode:            "456",
							PodMigrationJobName:   getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							PodMigrationJobPaused: pointer.Bool(false),
							PodMigrationJobPhase:  "",
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 0),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
							Message: fmt.Sprintf("unmigratable pod count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
							Message: fmt.Sprintf("unavailable reservation count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("failed job count: %d", 0),
							Message: fmt.Sprintf("failed job count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionOnceAvailable,
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeConditionOnceAvailable),
							Message: "no unmigratable, waiting, ready and unavailable migration",
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 0,
						v1alpha1.PodMigrationPhaseWaiting:      0,
						v1alpha1.PodMigrationPhaseReady:        0,
						v1alpha1.PodMigrationPhaseAvailable:    0,
						v1alpha1.PodMigrationPhaseUnavailable:  0,
						v1alpha1.PodMigrationPhaseMigrating:    1,
						v1alpha1.PodMigrationPhaseSucceed:      0,
						v1alpha1.PodMigrationPhaseFailed:       0,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{},
			wantJob: []*v1alpha1.PodMigrationJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
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
							Namespace: podsOnNode[0].Namespace,
							Name:      podsOnNode[0].Name,
						},
						ReservationOptions: &v1alpha1.PodMigrateReservationOptions{
							ReservationRef: &corev1.ObjectReference{
								Name:       getReservationOrMigrationJobName("123", podsOnNode[0].UID),
								UID:        "",
								APIVersion: v1alpha1.SchemeGroupVersion.Version,
								Kind:       "Reservation",
							},
						},
					},
				},
			},
		},
		{
			name: "migrationMode evictDirectly, one migrating migration to failed job",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
								WaitDuration: nil,
							},
							ConfirmState: v1alpha1.ConfirmStateConfirmed,
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
							PodMigrations: []v1alpha1.PodMigration{
								{
									PodUID:               podsOnNode[0].UID,
									Namespace:            podsOnNode[0].Namespace,
									PodName:              podsOnNode[0].Name,
									Phase:                v1alpha1.PodMigrationPhaseMigrating,
									StartTimestamp:       metav1.Time{},
									ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[0].UID),
									ReservationPhase:     v1alpha1.ReservationAvailable,
									TargetNode:           "456",
									PodMigrationJobName:  "",
									PodMigrationJobPhase: "",
								},
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: podsOnNode[0].Namespace,
								utils.PodNameKey:      podsOnNode[0].Name,
							},
							OwnerReferences: []metav1.OwnerReference{
								{APIVersion: "scheduling.koordinator.sh/v1alpha1",
									Kind:               "DrainNode",
									Name:               "123",
									UID:                "",
									Controller:         pointer.Bool(true),
									BlockOwnerDeletion: pointer.Bool(true),
								},
							},
						},
						Spec: v1alpha1.ReservationSpec{
							Template: &corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: podsOnNode[0].Namespace,
									Labels: map[string]string{
										utils.PodNameKey: podsOnNode[0].Name,
									},
									OwnerReferences: podsOnNode[0].OwnerReferences,
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
														MatchFields: []corev1.NodeSelectorRequirement{{
															Key:      "metadata.name",
															Operator: corev1.NodeSelectorOpNotIn,
															Values:   []string{podsOnNode[0].Spec.NodeName},
														}},
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
															Key:      utils.DrainNodeKey,
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
									OwnerReference: podsOnNode[0].OwnerReferences[0],
									Namespace:      podsOnNode[0].Namespace,
								}},
							},
							AllocateOnce: pointer.Bool(true),
						},
						Status: v1alpha1.ReservationStatus{
							Phase:    v1alpha1.ReservationAvailable,
							NodeName: "456",
						},
					},
					&v1alpha1.PodMigrationJob{
						ObjectMeta: metav1.ObjectMeta{
							Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
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
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							ReservationOptions: &v1alpha1.PodMigrateReservationOptions{
								ReservationRef: &corev1.ObjectReference{
									Name:       getReservationOrMigrationJobName("123", podsOnNode[0].UID),
									UID:        "",
									APIVersion: v1alpha1.SchemeGroupVersion.Version,
									Kind:       "Reservation",
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
						Name:        "123",
						Reservation: nil,
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
						WaitDuration: nil,
					},
					ConfirmState: v1alpha1.ConfirmStateConfirmed,
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseComplete,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:                podsOnNode[0].UID,
							Namespace:             podsOnNode[0].Namespace,
							PodName:               podsOnNode[0].Name,
							Phase:                 v1alpha1.PodMigrationPhaseFailed,
							StartTimestamp:        metav1.Time{},
							ReservationName:       getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							ReservationPhase:      v1alpha1.ReservationAvailable,
							TargetNode:            "456",
							PodMigrationJobName:   getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							PodMigrationJobPaused: pointer.Bool(false),
							PodMigrationJobPhase:  v1alpha1.PodMigrationJobFailed,
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 0),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
							Message: fmt.Sprintf("unmigratable pod count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
							Message: fmt.Sprintf("unavailable reservation count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionTrue,
							Reason:  fmt.Sprintf("failed job count: %d", 1),
							Message: fmt.Sprintf("failed job count: %d", 1),
						},
						{
							Type:    v1alpha1.DrainNodeConditionOnceAvailable,
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeConditionOnceAvailable),
							Message: "no unmigratable, waiting, ready and unavailable migration",
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedPodAfterCompleteExists,
							Status:  metav1.ConditionFalse,
							Reason:  "",
							Message: "",
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 0,
						v1alpha1.PodMigrationPhaseWaiting:      0,
						v1alpha1.PodMigrationPhaseReady:        0,
						v1alpha1.PodMigrationPhaseAvailable:    0,
						v1alpha1.PodMigrationPhaseUnavailable:  0,
						v1alpha1.PodMigrationPhaseMigrating:    0,
						v1alpha1.PodMigrationPhaseSucceed:      0,
						v1alpha1.PodMigrationPhaseFailed:       1,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantReservation: []*v1alpha1.Reservation{},
		},
		{
			name: "migrationMode evictDirectly, one migrating migration to succeed job",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "123",
						},
						Spec: v1alpha1.DrainNodeSpec{
							NodeName:     "node123",
							CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
							MigrationPolicy: v1alpha1.MigrationPolicy{
								Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
								WaitDuration: nil,
							},
							ConfirmState: v1alpha1.ConfirmStateConfirmed,
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseRunning,
							PodMigrations: []v1alpha1.PodMigration{
								{
									PodUID:               podsOnNode[0].UID,
									Namespace:            podsOnNode[0].Namespace,
									PodName:              podsOnNode[0].Name,
									Phase:                v1alpha1.PodMigrationPhaseMigrating,
									StartTimestamp:       metav1.Time{},
									ReservationName:      getReservationOrMigrationJobName("123", podsOnNode[0].UID),
									ReservationPhase:     v1alpha1.ReservationAvailable,
									TargetNode:           "456",
									PodMigrationJobName:  getReservationOrMigrationJobName("123", podsOnNode[0].UID),
									PodMigrationJobPhase: "",
								},
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node123",
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:    utils.DrainNodeKey,
									Value:  "123",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
					&v1alpha1.Reservation{
						ObjectMeta: metav1.ObjectMeta{
							Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							Labels: map[string]string{
								utils.DrainNodeKey:    "123",
								utils.PodNamespaceKey: podsOnNode[0].Namespace,
								utils.PodNameKey:      podsOnNode[0].Name,
							},
							OwnerReferences: []metav1.OwnerReference{
								{APIVersion: "scheduling.koordinator.sh/v1alpha1",
									Kind:               "DrainNode",
									Name:               "123",
									UID:                "",
									Controller:         pointer.Bool(true),
									BlockOwnerDeletion: pointer.Bool(true),
								},
							},
						},
						Spec: v1alpha1.ReservationSpec{
							Template: &corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: podsOnNode[0].Namespace,
									Labels: map[string]string{
										utils.PodNameKey: podsOnNode[0].Name,
									},
									OwnerReferences: podsOnNode[0].OwnerReferences,
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
														MatchFields: []corev1.NodeSelectorRequirement{{
															Key:      "metadata.name",
															Operator: corev1.NodeSelectorOpNotIn,
															Values:   []string{podsOnNode[0].Spec.NodeName},
														}},
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
															Key:      utils.DrainNodeKey,
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
									OwnerReference: podsOnNode[0].OwnerReferences[0],
									Namespace:      podsOnNode[0].Namespace,
								}},
							},
							AllocateOnce: pointer.Bool(true),
						},
						Status: v1alpha1.ReservationStatus{
							Phase:    v1alpha1.ReservationAvailable,
							NodeName: "456",
						},
					},
					&v1alpha1.PodMigrationJob{
						ObjectMeta: metav1.ObjectMeta{
							Name: getReservationOrMigrationJobName("123", podsOnNode[0].UID),
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
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							ReservationOptions: &v1alpha1.PodMigrateReservationOptions{
								ReservationRef: &corev1.ObjectReference{
									Name:       getReservationOrMigrationJobName("123", podsOnNode[0].UID),
									UID:        "",
									APIVersion: v1alpha1.SchemeGroupVersion.Version,
									Kind:       "Reservation",
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
						Name:        "123",
						Reservation: nil,
					},
					pods: []*cache.PodInfo{
						{
							UID: podsOnNode[0].UID,
							NamespacedName: types.NamespacedName{
								Namespace: podsOnNode[0].Namespace,
								Name:      podsOnNode[0].Name,
							},
							Ignore:     false,
							Migratable: true,
							Pod:        podsOnNode[0],
						},
					},
				},
			},
			args: args{
				request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "123"}},
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: defaultRequeueAfter,
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "123",
				},
				Spec: v1alpha1.DrainNodeSpec{
					NodeName:     "node123",
					CordonPolicy: &v1alpha1.CordonNodePolicy{Mode: v1alpha1.CordonNodeModeTaint},
					MigrationPolicy: v1alpha1.MigrationPolicy{
						Mode:         v1alpha1.MigrationPodModeMigrateDirectly,
						WaitDuration: nil,
					},
					ConfirmState: v1alpha1.ConfirmStateConfirmed,
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseComplete,
					PodMigrations: []v1alpha1.PodMigration{
						{
							PodUID:                podsOnNode[0].UID,
							Namespace:             podsOnNode[0].Namespace,
							PodName:               podsOnNode[0].Name,
							Phase:                 v1alpha1.PodMigrationPhaseSucceed,
							StartTimestamp:        metav1.Time{},
							ReservationName:       getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							ReservationPhase:      v1alpha1.ReservationAvailable,
							TargetNode:            "456",
							PodMigrationJobName:   getReservationOrMigrationJobName("123", podsOnNode[0].UID),
							PodMigrationJobPaused: pointer.Bool(false),
							PodMigrationJobPhase:  v1alpha1.PodMigrationJobSucceeded,
						},
					},
					Conditions: []v1alpha1.DrainNodeCondition{
						{
							Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unexpected reservation count: %d,", 0),
							Message: fmt.Sprintf("unexpectedRR: %v", []string{}),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unmigratable pod count: %d", 0),
							Message: fmt.Sprintf("unmigratable pod count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("unavailable reservation count: %d", 0),
							Message: fmt.Sprintf("unavailable reservation count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
							Status:  metav1.ConditionFalse,
							Reason:  fmt.Sprintf("failed job count: %d", 0),
							Message: fmt.Sprintf("failed job count: %d", 0),
						},
						{
							Type:    v1alpha1.DrainNodeConditionOnceAvailable,
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeConditionOnceAvailable),
							Message: "no unmigratable, waiting, ready and unavailable migration",
						},
						{
							Type:   v1alpha1.DrainNodeConditionUnexpectedPodAfterCompleteExists,
							Status: metav1.ConditionFalse,
						},
					},
					PodMigrationSummary: map[v1alpha1.PodMigrationPhase]int32{
						v1alpha1.PodMigrationPhaseUnmigratable: 0,
						v1alpha1.PodMigrationPhaseWaiting:      0,
						v1alpha1.PodMigrationPhaseReady:        0,
						v1alpha1.PodMigrationPhaseAvailable:    0,
						v1alpha1.PodMigrationPhaseUnavailable:  0,
						v1alpha1.PodMigrationPhaseMigrating:    0,
						v1alpha1.PodMigrationPhaseSucceed:      1,
						v1alpha1.PodMigrationPhaseFailed:       0,
					},
				},
			},
			wantNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node123",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    utils.DrainNodeKey,
							Value:  "123",
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
				Client:        c,
				eventRecorder: record.NewEventRecorderAdapter(recorder),
				podFilter: func(pod *corev1.Pod) bool {
					return pod.Annotations["Unmigratable"] != "Unmigratable"
				},
				cache:                  tt.fields.cache,
				reservationInterpreter: reservation.NewInterpreter(c, c),
			}
			got, err := r.Reconcile(context.Background(), tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("DrainNodeReconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.requeue {
				tt.funcBeforeRequeue(r)
				got, err = r.Reconcile(context.Background(), tt.args.request)
				if (err != nil) != tt.wantErr {
					t.Errorf("DrainNodeReconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
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
				tt.wantObj.CreationTimestamp = gotObj.CreationTimestamp
				tt.wantObj.TypeMeta = gotObj.TypeMeta
				tt.wantObj.ResourceVersion = gotObj.ResourceVersion
				for i := range gotObj.Status.Conditions {
					gotCondition := gotObj.Status.Conditions[i]
					tt.wantObj.Status.Conditions[i].LastTransitionTime = gotCondition.LastTransitionTime
					tt.wantObj.Status.Conditions[i].LastProbeTime = gotCondition.LastProbeTime
				}
				for i := range gotObj.Status.PodMigrations {
					gotPodMigration := gotObj.Status.PodMigrations[i]
					tt.wantObj.Status.PodMigrations[i].StartTimestamp = gotPodMigration.StartTimestamp
				}
				if !reflect.DeepEqual(gotObj.Spec, tt.wantObj.Spec) {
					t.Errorf("DrainNodeReconciler.Reconcile() = %v, want %v", gotObj, tt.wantObj)
				}
				if !reflect.DeepEqual(gotObj.Status.Phase, tt.wantObj.Status.Phase) {
					t.Errorf("DrainNodeReconciler.Reconcile() = %v, want %v", gotObj.Status.Phase, tt.wantObj.Status.Phase)
				}
				if !reflect.DeepEqual(gotObj.Status.PodMigrations, tt.wantObj.Status.PodMigrations) {
					t.Errorf("DrainNodeReconciler.Reconcile() = %v, want %v", gotObj.Status.PodMigrations, tt.wantObj.Status.PodMigrations)
				}
				if !reflect.DeepEqual(gotObj.Status.Conditions, tt.wantObj.Status.Conditions) {
					t.Errorf("DrainNodeReconciler.Reconcile() = %v, want %v", gotObj.Status.Conditions, tt.wantObj.Status.Conditions)
				}
				if !reflect.DeepEqual(gotObj.Status.PodMigrationSummary, tt.wantObj.Status.PodMigrationSummary) {
					t.Errorf("DrainNodeReconciler.Reconcile() = %v, want %v", gotObj.Status.PodMigrationSummary, tt.wantObj.Status.PodMigrationSummary)
				}
			}
			for _, wantR := range tt.wantReservation {
				r := &v1alpha1.Reservation{}
				if err := c.Get(context.Background(), types.NamespacedName{Name: wantR.Name}, r); err != nil {
					t.Errorf("DrainNodeReconciler.Reconcile() get reservation %v err %v", wantR.Name, err)
				}
				r.TypeMeta = wantR.TypeMeta
				r.ResourceVersion = wantR.ResourceVersion
				if !reflect.DeepEqual(r, wantR) {
					t.Errorf("DrainNodeReconciler.Reconcile() get reservation = %v, want %v", r, wantR)
				}
			}
			for _, wantJob := range tt.wantJob {
				r := &v1alpha1.PodMigrationJob{}
				if err := c.Get(context.Background(), types.NamespacedName{Name: wantJob.Name}, r); err != nil {
					t.Errorf("DrainNodeReconciler.Reconcile() get job %v err %v", wantJob.Name, err)
				}
				r.TypeMeta = wantJob.TypeMeta
				r.ResourceVersion = wantJob.ResourceVersion
				if !reflect.DeepEqual(r.Spec, wantJob.Spec) {
					t.Errorf("DrainNodeReconciler.Reconcile() get job = %v, want %v", r, wantJob)
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
