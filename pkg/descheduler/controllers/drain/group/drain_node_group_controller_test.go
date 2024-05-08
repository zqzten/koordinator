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
	nodeInfo map[string]*cache.NodeInfo
}

func (t *testCache) GetNodeInfo(nodeName string) *cache.NodeInfo {
	return t.nodeInfo[nodeName]
}

func (t *testCache) GetPods(nodeName string) []*cache.PodInfo {
	return nil
}

func TestDrainNodeGroupReconciler_Reconcile(t *testing.T) {
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
			name: "init",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNodeGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123",
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
				Status: v1alpha1.DrainNodeGroupStatus{
					Phase: v1alpha1.DrainNodeGroupPhasePending,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodeGroupPhasePending),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeGroupPhasePending),
							Message: "Initialized",
						},
					},
				},
			},
		},
		{
			name: "requeue",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNodeGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123",
						},
						Status: v1alpha1.DrainNodeGroupStatus{
							Phase: v1alpha1.DrainNodeGroupPhasePending,
						},
					},
					&v1alpha1.DrainNodeGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-321",
						},
						Status: v1alpha1.DrainNodeGroupStatus{
							Phase: v1alpha1.DrainNodeGroupPhasePending,
						},
					},
				},
			},
			args: args{
				request: types.NamespacedName{Name: "dng-321"},
			},
			want: reconcile.Result{RequeueAfter: defaultRequeueAfter},
			wantObj: &v1alpha1.DrainNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dng-123",
				},
				Status: v1alpha1.DrainNodeGroupStatus{
					Phase: v1alpha1.DrainNodeGroupPhasePending,
				},
			},
		},
		{
			name: "pending",
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
							Phase: v1alpha1.DrainNodeGroupPhasePending,
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
							Labels: map[string]string{
								"nodepool":     "001",
								utils.GroupKey: "123",
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node2",
							Labels: map[string]string{
								"nodepool":        "001",
								utils.GroupKey:    "123",
								utils.PlanningKey: "123",
							},
						},
					},
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node3",
							Labels: map[string]string{
								utils.GroupKey:    "123",
								utils.PlanningKey: "123",
							},
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
					Phase: v1alpha1.DrainNodeGroupPhasePlanning,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodeGroupPhasePlanning),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeGroupPhasePlanning),
							Message: "Start Planning",
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
		},
		{
			name: "waiting",
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
							Phase: v1alpha1.DrainNodeGroupPhaseWaiting,
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
							Phase: DrainNodePhaseAvailable,
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
							Phase: DrainNodePhaseAvailable,
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
							Message: "Waiting",
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
					Spec: v1alpha1.DrainNodeSpec{
						ConfirmState: v1alpha1.ConfirmStateConfirmed,
					},
					Status: v1alpha1.DrainNodeStatus{
						Phase: DrainNodePhaseAvailable,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node2",
						Labels: map[string]string{
							utils.GroupKey: "dng-123",
						},
					},
					Spec: v1alpha1.DrainNodeSpec{ConfirmState: v1alpha1.ConfirmStateConfirmed},
					Status: v1alpha1.DrainNodeStatus{
						Phase: DrainNodePhaseAvailable,
					},
				},
			},
		},
		{
			name: "waiting to running",
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
							Phase: v1alpha1.DrainNodeGroupPhaseWaiting,
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
							Phase: v1alpha1.DrainNodePhaseRunning,
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
							Phase: DrainNodePhaseAvailable,
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
				},
				Status: v1alpha1.DrainNodeGroupStatus{
					Phase:          v1alpha1.DrainNodeGroupPhaseRunning,
					TotalCount:     2,
					AvailableCount: 2,
					RunningCount:   1,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodeGroupPhaseRunning),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeGroupPhaseRunning),
							Message: "Running, 1 DrainNode start running",
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
						Phase: v1alpha1.DrainNodePhaseRunning,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node2",
						Labels: map[string]string{
							utils.GroupKey: "dng-123",
						},
					},
					Spec: v1alpha1.DrainNodeSpec{ConfirmState: v1alpha1.ConfirmStateConfirmed},
					Status: v1alpha1.DrainNodeStatus{
						Phase: DrainNodePhaseAvailable,
					},
				},
			},
		},
		{
			name: "running",
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
							Phase: v1alpha1.DrainNodeGroupPhaseRunning,
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
							Phase: v1alpha1.DrainNodePhaseRunning,
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
							Phase: DrainNodePhaseAvailable,
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
				},
				Status: v1alpha1.DrainNodeGroupStatus{
					Phase:          v1alpha1.DrainNodeGroupPhaseRunning,
					TotalCount:     2,
					AvailableCount: 2,
					RunningCount:   1,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodeGroupPhaseRunning),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeGroupPhaseRunning),
							Message: "Running",
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
						Phase: v1alpha1.DrainNodePhaseRunning,
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
						Phase: DrainNodePhaseAvailable,
					},
				},
			},
		},
		{
			name: "running to waiting",
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
							Phase: v1alpha1.DrainNodeGroupPhaseRunning,
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
							Phase: v1alpha1.DrainNodePhaseComplete,
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
							Phase: DrainNodePhaseAvailable,
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
				},
				Status: v1alpha1.DrainNodeGroupStatus{
					Phase:          v1alpha1.DrainNodeGroupPhaseWaiting,
					TotalCount:     2,
					AvailableCount: 2,
					SucceededCount: 1,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodeGroupPhaseWaiting),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeGroupPhaseWaiting),
							Message: "Waiting",
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
						Phase: v1alpha1.DrainNodePhaseComplete,
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
						Phase: DrainNodePhaseAvailable,
					},
				},
			},
		},
		{
			name: "running to Succeeded",
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
							Phase: v1alpha1.DrainNodeGroupPhaseRunning,
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
							Phase: v1alpha1.DrainNodePhaseComplete,
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
							Phase: v1alpha1.DrainNodePhaseComplete,
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
				},
				Status: v1alpha1.DrainNodeGroupStatus{
					Phase:          v1alpha1.DrainNodeGroupPhaseSucceeded,
					TotalCount:     2,
					AvailableCount: 2,
					SucceededCount: 2,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodeGroupPhaseSucceeded),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeGroupPhaseSucceeded),
							Message: "Succeeded, 2 DrainNode Succeeded",
						},
					},
				},
			},
			wantNodes:     []*corev1.Node{},
			wantDrainNode: []*v1alpha1.DrainNode{},
		},
		{
			name: "running abort",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNodeGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123",
							Labels: map[string]string{
								utils.AbortKey: "true",
							},
						},
						Spec: v1alpha1.DrainNodeGroupSpec{
							NodeSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"nodepool": "001"},
							},
						},
						Status: v1alpha1.DrainNodeGroupStatus{
							Phase: v1alpha1.DrainNodeGroupPhaseRunning,
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
							Phase: v1alpha1.DrainNodePhaseRunning,
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
							Phase: DrainNodePhaseAvailable,
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
			wantErr: true,
			wantObj: &v1alpha1.DrainNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dng-123",
					Labels: map[string]string{
						utils.AbortKey: "true",
					},
				},
				Spec: v1alpha1.DrainNodeGroupSpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"nodepool": "001"},
					},
				},
				Status: v1alpha1.DrainNodeGroupStatus{
					Phase: v1alpha1.DrainNodeGroupPhaseRunning,
				},
			},
			wantNodes: []*corev1.Node{},
			wantDrainNode: []*v1alpha1.DrainNode{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node1",
						Labels: map[string]string{
							utils.GroupKey: "dng-123",
						},
					},
					Spec: v1alpha1.DrainNodeSpec{ConfirmState: v1alpha1.ConfirmStateAborted},
					Status: v1alpha1.DrainNodeStatus{
						Phase: v1alpha1.DrainNodePhaseRunning,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node2",
						Labels: map[string]string{
							utils.GroupKey: "dng-123",
						},
					},
					Spec: v1alpha1.DrainNodeSpec{ConfirmState: v1alpha1.ConfirmStateAborted},
					Status: v1alpha1.DrainNodeStatus{
						Phase: DrainNodePhaseAvailable,
					},
				},
			},
		},
		{
			name: "running abort success",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNodeGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123",
							Labels: map[string]string{
								utils.AbortKey: "true",
							},
						},
						Spec: v1alpha1.DrainNodeGroupSpec{
							NodeSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"nodepool": "001"},
							},
						},
						Status: v1alpha1.DrainNodeGroupStatus{
							Phase: v1alpha1.DrainNodeGroupPhaseRunning,
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
							Phase: v1alpha1.DrainNodePhaseComplete,
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
							Phase: v1alpha1.DrainNodePhaseAborted,
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
			wantErr: false,
			wantObj: &v1alpha1.DrainNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dng-123",
					Labels: map[string]string{
						utils.AbortKey: "true",
					},
				},
				Spec: v1alpha1.DrainNodeGroupSpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"nodepool": "001"},
					},
				},
				Status: v1alpha1.DrainNodeGroupStatus{
					Phase: v1alpha1.DrainNodeGroupPhaseAborted,
					Conditions: []metav1.Condition{
						{
							Type:    string(v1alpha1.DrainNodeGroupPhaseAborted),
							Status:  metav1.ConditionTrue,
							Reason:  string(v1alpha1.DrainNodeGroupPhaseAborted),
							Message: "DrainNodeGroup Aborted",
						},
					},
				},
			},
			wantNodes: []*corev1.Node{},
			wantDrainNode: []*v1alpha1.DrainNode{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node1",
						Labels: map[string]string{
							utils.GroupKey: "dng-123",
						},
					},
					Spec: v1alpha1.DrainNodeSpec{ConfirmState: v1alpha1.ConfirmStateAborted},
					Status: v1alpha1.DrainNodeStatus{
						Phase: v1alpha1.DrainNodePhaseComplete,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node2",
						Labels: map[string]string{
							utils.GroupKey: "dng-123",
						},
					},
					Spec: v1alpha1.DrainNodeSpec{ConfirmState: v1alpha1.ConfirmStateAborted},
					Status: v1alpha1.DrainNodeStatus{
						Phase: v1alpha1.DrainNodePhaseAborted,
					},
				},
			},
		},
		{
			name: "clean taint",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNodeGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dng-123",
							Labels: map[string]string{
								utils.CleanKey: "true",
							},
						},
						Spec: v1alpha1.DrainNodeGroupSpec{
							NodeSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"nodepool": "001"},
							},
						},
						Status: v1alpha1.DrainNodeGroupStatus{
							Phase: v1alpha1.DrainNodeGroupPhaseSucceeded,
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
							Phase: v1alpha1.DrainNodePhaseComplete,
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
							Phase: v1alpha1.DrainNodePhaseAborted,
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
			wantErr: false,
			wantObj: &v1alpha1.DrainNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dng-123",
					Labels: map[string]string{
						utils.CleanKey: "true",
					},
				},
				Spec: v1alpha1.DrainNodeGroupSpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"nodepool": "001"},
					},
				},
				Status: v1alpha1.DrainNodeGroupStatus{
					Phase: v1alpha1.DrainNodeGroupPhaseSucceeded,
				},
			},
			wantNodes: []*corev1.Node{},
			wantDrainNode: []*v1alpha1.DrainNode{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node1",
						Labels: map[string]string{
							utils.GroupKey: "dng-123",
						},
					},
					Spec: v1alpha1.DrainNodeSpec{
						ConfirmState: v1alpha1.ConfirmStateAborted,
					},
					Status: v1alpha1.DrainNodeStatus{
						Phase: v1alpha1.DrainNodePhaseComplete,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dng-123-node2",
						Labels: map[string]string{
							utils.GroupKey: "dng-123",
						},
					},
					Spec: v1alpha1.DrainNodeSpec{
						ConfirmState: v1alpha1.ConfirmStateAborted,
					},
					Status: v1alpha1.DrainNodeStatus{
						Phase: v1alpha1.DrainNodePhaseAborted,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&v1alpha1.DrainNodeGroup{}).
				WithRuntimeObjects(tt.fields.objs...).Build()
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
