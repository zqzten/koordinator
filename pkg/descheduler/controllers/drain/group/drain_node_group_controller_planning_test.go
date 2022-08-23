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

package group

import (
	"context"
	"reflect"
	"testing"

	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/cache"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestDrainNodeGroupReconciler_Reconcile_Planning(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	type fields struct {
		objs  []runtime.Object
		cache cache.Cache
	}
	type args struct {
		request types.NamespacedName
	}
	tests := []struct {
		name          string
		fields        fields
		args          args
		want          reconcile.Result
		wantErr       bool
		wantObj       *v1alpha1.DrainNodeGroup
		wantNodes     []*corev1.Node
		wantDrainNode []*v1alpha1.DrainNode
	}{
		{
			name: "planning max node count",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNodeGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123",
						},
						Spec: v1alpha1.DrainNodeGroupSpec{
							NodeSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"nodepool": "001"},
							},
							TerminationPolicy: &v1alpha1.TerminationPolicy{
								MaxNodeCount: pointer.Int32Ptr(1),
							},
						},
						Status: v1alpha1.DrainNodeGroupStatus{
							Phase: v1alpha1.DrainNodeGroupPhasePlanning,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
							Labels: map[string]string{
								"nodepool":     "001",
								utils.GroupKey: "dng-123",
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node2",
							Labels: map[string]string{
								"nodepool":     "001",
								utils.GroupKey: "dng-123",
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node3",
						},
					},
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123-node1",
							Labels: map[string]string{
								utils.GroupKey: "dng-123",
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseAvailable,
						},
					},
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123-node2",
							Labels: map[string]string{
								utils.GroupKey: "dng-123",
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseAvailable,
						},
					},
				},
				cache: &testCache{
					nodeInfo: map[string]*cache.NodeInfo{
						"node1": {Name: "node1", Score: 1},
						"node2": {Name: "node2", Score: 100},
					},
				},
			},
			args: args{
				request: types.NamespacedName{Name: "dng-123"},
			},
			wantObj: &v1alpha1.DrainNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dng-123",
				},
				Spec: v1alpha1.DrainNodeGroupSpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"nodepool": "001"},
					},
					TerminationPolicy: &v1alpha1.TerminationPolicy{
						MaxNodeCount: pointer.Int32Ptr(1),
					},
				},
				Status: v1alpha1.DrainNodeGroupStatus{
					Phase:          v1alpha1.DrainNodeGroupPhaseWaiting,
					TotalCount:     2,
					AvailableCount: 2,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodeGroupPhaseWaiting),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeGroupPhaseWaiting),
							Message: "meet MaxNodeCount TerminationPolicy",
						},
					},
				},
			},
			wantNodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"nodepool":     "001",
							utils.GroupKey: "dng-123",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"nodepool":     "001",
							utils.GroupKey: "dng-123",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
					},
				},
			},
			wantDrainNode: []*v1alpha1.DrainNode{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node1",
						Labels: map[string]string{
							utils.GroupKey: "dng-123",
						},
					},
					Status: v1alpha1.DrainNodeStatus{
						Phase: v1alpha1.DrainNodePhaseAvailable,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node2",
						Labels: map[string]string{
							utils.GroupKey: "dng-123",
						},
					},
					Status: v1alpha1.DrainNodeStatus{
						Phase: v1alpha1.DrainNodePhaseAvailable,
					},
				},
			},
		},
		{
			name: "planning reserved resource",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNodeGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123",
						},
						Spec: v1alpha1.DrainNodeGroupSpec{
							NodeSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"nodepool": "001"},
							},
							TerminationPolicy: &v1alpha1.TerminationPolicy{
								PercentageOfResourceReserved: pointer.Int32Ptr(50),
							},
						},
						Status: v1alpha1.DrainNodeGroupStatus{
							Phase: v1alpha1.DrainNodeGroupPhasePlanning,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
							Labels: map[string]string{
								"nodepool":     "001",
								utils.GroupKey: "dng-123",
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node2",
							Labels: map[string]string{
								"nodepool":     "001",
								utils.GroupKey: "dng-123",
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node3",
						},
					},
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123-node1",
							Labels: map[string]string{
								utils.GroupKey: "dng-123",
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseAvailable,
						},
					},
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123-node2",
							Labels: map[string]string{
								utils.GroupKey: "dng-123",
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseAvailable,
						},
					},
				},
				cache: &testCache{
					nodeInfo: map[string]*cache.NodeInfo{
						"node1": {Name: "node1", Score: 1,
							Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10")},
							Free:        corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("5")},
						},
						"node2": {Name: "node2", Score: 100,
							Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10")},
							Free:        corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("6")},
						},
					},
				},
			},
			args: args{
				request: types.NamespacedName{Name: "dng-123"},
			},
			wantObj: &v1alpha1.DrainNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dng-123",
				},
				Spec: v1alpha1.DrainNodeGroupSpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"nodepool": "001"},
					},
					TerminationPolicy: &v1alpha1.TerminationPolicy{
						PercentageOfResourceReserved: pointer.Int32Ptr(50),
					},
				},
				Status: v1alpha1.DrainNodeGroupStatus{
					Phase:          v1alpha1.DrainNodeGroupPhaseWaiting,
					TotalCount:     2,
					AvailableCount: 2,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodeGroupPhaseWaiting),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeGroupPhaseWaiting),
							Message: "meet PercentageOfResourceReserved TerminationPolicy",
						},
					},
				},
			},
			wantNodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"nodepool":     "001",
							utils.GroupKey: "dng-123",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"nodepool":     "001",
							utils.GroupKey: "dng-123",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
					},
				},
			},
			wantDrainNode: []*v1alpha1.DrainNode{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node1",
						Labels: map[string]string{
							utils.GroupKey: "dng-123",
						},
					},
					Status: v1alpha1.DrainNodeStatus{
						Phase: v1alpha1.DrainNodePhaseAvailable,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node2",
						Labels: map[string]string{
							utils.GroupKey: "dng-123",
						},
					},
					Status: v1alpha1.DrainNodeStatus{
						Phase: v1alpha1.DrainNodePhaseAvailable,
					},
				},
			},
		},
		{
			name: "planning create DrainNode",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNodeGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123",
						},
						Spec: v1alpha1.DrainNodeGroupSpec{
							NodeSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"nodepool": "001"},
							},
						},
						Status: v1alpha1.DrainNodeGroupStatus{
							Phase: v1alpha1.DrainNodeGroupPhasePlanning,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
							Labels: map[string]string{
								"nodepool":     "001",
								utils.GroupKey: "dng-123",
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node2",
							Labels: map[string]string{
								"nodepool":     "001",
								utils.GroupKey: "dng-123",
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node3",
						},
					},
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123-node1",
							Labels: map[string]string{
								utils.GroupKey: "dng-123",
							},
						},
						Status: v1alpha1.DrainNodeStatus{
							Phase: v1alpha1.DrainNodePhaseAvailable,
						},
					},
				},
				cache: &testCache{
					nodeInfo: map[string]*cache.NodeInfo{
						"node1": {Name: "node1", Score: 1,
							Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10")},
							Free:        corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("5")},
						},
						"node2": {Name: "node2", Score: 100,
							Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10")},
							Free:        corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("6")},
						},
					},
				},
			},
			args: args{
				request: types.NamespacedName{Name: "dng-123"},
			},
			wantObj: &v1alpha1.DrainNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dng-123",
				},
				Spec: v1alpha1.DrainNodeGroupSpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"nodepool": "001"},
					},
				},
				Status: v1alpha1.DrainNodeGroupStatus{
					Phase:          v1alpha1.DrainNodeGroupPhasePlanning,
					TotalCount:     1,
					AvailableCount: 1,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodeGroupPhasePlanning),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeGroupPhasePlanning),
							Message: "Planning",
						},
					},
				},
			},
			wantNodes: []*corev1.Node{},
			wantDrainNode: []*v1alpha1.DrainNode{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node2",
						Labels: map[string]string{
							utils.GroupKey:    "dng-123",
							utils.NodeNameKey: "node2",
						},
						OwnerReferences: []metav1.OwnerReference{
							{APIVersion: "scheduling.koordinator.sh/v1alpha1",
								Kind:               "DrainNodeGroup",
								Name:               "dng-123",
								Controller:         pointer.Bool(true),
								BlockOwnerDeletion: pointer.Bool(true),
							},
						},
					},
					Spec: v1alpha1.DrainNodeSpec{
						NodeName:     "node2",
						ConfirmState: v1alpha1.ConfirmStateWait,
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
			r := &DrainNodeGroupReconciler{
				Client:        c,
				eventRecorder: record.NewEventRecorderAdapter(recorder),
				cache:         tt.fields.cache,
			}
			got, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: tt.args.request})
			if (err != nil) != tt.wantErr {
				t.Errorf("DrainNodeGroupReconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DrainNodeGroupReconciler.Reconcile() = %v, want %v", got, tt.want)
			}
			if tt.wantObj != nil {
				gotObj := &v1alpha1.DrainNodeGroup{}
				err := c.Get(context.Background(), types.NamespacedName{Name: tt.wantObj.Name}, gotObj)
				if err != nil {
					t.Errorf("DrainNodeGroupReconciler.Reconcile() got err = %v", err)
					return
				}
				tt.wantObj.TypeMeta = gotObj.TypeMeta
				tt.wantObj.ResourceVersion = gotObj.ResourceVersion
				if len(gotObj.Status.Conditions) > 0 && len(tt.wantObj.Status.Conditions) > 0 {
					gotObj.Status.Conditions[0].LastTransitionTime =
						tt.wantObj.Status.Conditions[0].LastTransitionTime
				}
				if !reflect.DeepEqual(gotObj, tt.wantObj) {
					t.Errorf("DrainNodeGroupReconciler.Reconcile() = %v, want %v", gotObj, tt.wantObj)
				}
			}
			for _, wantN := range tt.wantNodes {
				n := &corev1.Node{}
				if err := c.Get(context.Background(), types.NamespacedName{Name: wantN.Name}, n); err != nil {
					t.Errorf("DrainNodeGroupReconciler.Reconcile() get nodes err %v", err)
				}
				n.TypeMeta = wantN.TypeMeta
				n.ResourceVersion = wantN.ResourceVersion
				if !reflect.DeepEqual(n, wantN) {
					t.Errorf("DrainNodeReconciler.Reconcile() get node = %v, want %v", n, wantN)
				}
			}
			for _, wantN := range tt.wantDrainNode {
				n := &v1alpha1.DrainNode{}
				if err := c.Get(context.Background(), types.NamespacedName{Name: wantN.Name}, n); err != nil {
					t.Errorf("DrainNodeGroupReconciler.Reconcile() get DrainNode err %v", err)
				}
				n.TypeMeta = wantN.TypeMeta
				n.ResourceVersion = wantN.ResourceVersion
				if len(n.Status.Conditions) > 0 && len(wantN.Status.Conditions) > 0 {
					n.Status.Conditions[0].LastTransitionTime =
						wantN.Status.Conditions[0].LastTransitionTime
				}
				if !reflect.DeepEqual(n, wantN) {
					t.Errorf("DrainNodeReconciler.Reconcile() get DrainNode = %v, want %v", n, wantN)
				}
			}
		})
	}
}
