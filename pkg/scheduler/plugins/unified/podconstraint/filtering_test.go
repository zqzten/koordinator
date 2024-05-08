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

package podconstraint

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
	"k8s.io/utils/pointer"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	nodeaffinityhelper "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/nodeaffinity"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/podconstraint/cache"
)

func TestPlugin_PreFilter(t *testing.T) {
	type args struct {
		cycleState *framework.CycleState
		pod        *corev1.Pod
	}
	tests := []struct {
		name                 string
		args                 args
		podConstraint        *v1beta1.PodConstraint
		expectPrefilterState *preFilterState
		expectSuccess        bool
		expectReason         []string
	}{
		{
			name: "normal pod without podConstraint",
			args: args{
				cycleState: framework.NewCycleState(),
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-pod",
					},
				},
			},
			podConstraint:        nil,
			expectSuccess:        true,
			expectPrefilterState: &preFilterState{},
		},
		{
			name: "normal pod with non-exist podConstraint",
			args: args{
				cycleState: framework.NewCycleState(),
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-pod",
						Labels: map[string]string{
							extunified.LabelPodConstraint: "test-pod-constraint",
						},
					},
				},
			},
			podConstraint:        nil,
			expectSuccess:        false,
			expectReason:         []string{ErrMissPodConstraint},
			expectPrefilterState: nil,
		},
		{
			name: "normal pod with podConstraint",
			args: args{
				cycleState: framework.NewCycleState(),
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-pod",
						Labels: map[string]string{
							extunified.LabelPodConstraint: "test",
						},
					},
				},
			},
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "node-type",
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
			expectSuccess: true,
			expectReason:  nil,
			expectPrefilterState: &preFilterState{
				items: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nil, nil)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			if tt.podConstraint != nil {
				plg.podConstraintCache.SetPodConstraint(tt.podConstraint)
			}

			_, status := plg.PreFilter(context.TODO(), tt.args.cycleState, tt.args.pod)
			assert.Equal(t, tt.expectSuccess, status.IsSuccess())
			if !tt.expectSuccess {
				assert.Equal(t, tt.expectReason, status.Reasons())
			}

			state, status := getPreFilterState(tt.args.cycleState)
			assert.Equal(t, tt.expectSuccess, status.IsSuccess())
			assert.Equal(t, tt.expectPrefilterState, state)
		})
	}
}

func TestPlugin_FilterWithTemporaryAffinity(t *testing.T) {
	tests := []struct {
		name                 string
		podConstraint        *v1beta1.PodConstraint
		existingPods         []*corev1.Pod
		nodes                []*corev1.Node
		pod                  *corev1.Pod
		expectPrefilterState *preFilterState
		testNodes            []string
		filterStatus         []*framework.Status
	}{
		{
			name: "matchNum all equal, all satisfy maxSkew",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: (*v1beta1.NodeInclusionPolicy)(pointer.String(string(v1beta1.NodeInclusionPolicyHonor))),
							},
						},
					},
				},
			},
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+
			//  | na610 | na620 |
			//  |-------+-------+
			//  |  2    |  4    |
			//  +-------+-------+
			//
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-6").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").
				NodeAffinityIn(corev1.LabelTopologyZone, []string{"na610"}).Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(6),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(2),
						},
						TpKeyToCriticalPaths: nil,
					},
				},
			},
			testNodes:    []string{"test-node-1", "test-node-2"},
			filterStatus: []*framework.Status{nil, nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, tt.nodes, tt.existingPods)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			plg.enableNodeInclusionPolicyInPodConstraint = true
			suit.start()

			for _, node := range tt.nodes {
				_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
				assert.NoError(t, err)
			}
			plg.podConstraintCache.SetPodConstraint(tt.podConstraint)

			// normal prefilter
			cycleState := framework.NewCycleState()

			if tt.pod.Spec.Affinity != nil &&
				tt.pod.Spec.Affinity.NodeAffinity != nil &&
				tt.pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
				affinity := &nodeaffinityhelper.TemporaryNodeAffinity{
					NodeSelector: tt.pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution,
				}
				nodeaffinityhelper.SetTemporaryNodeAffinity(cycleState, affinity)
				tt.pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = nil
			}

			_, status := plg.PreFilter(context.TODO(), cycleState, tt.pod)
			assert.True(t, status.IsSuccess())
			state, status := getPreFilterState(cycleState)
			assert.True(t, status.IsSuccess())
			if tt.expectPrefilterState != nil {
				assert.Equal(t, tt.expectPrefilterState.items[0].RequiredSpreadConstraints, state.items[0].RequiredSpreadConstraints)
				assert.Equal(t, tt.expectPrefilterState.items[0].TpPairToMatchNum, state.items[0].TpPairToMatchNum)
				if tt.expectPrefilterState.items[0].TpKeyToCriticalPaths != nil {
					assert.Equal(t, tt.expectPrefilterState.items[0].TpKeyToCriticalPaths, state.items[0].TpKeyToCriticalPaths)
				}
				assert.Equal(t, tt.expectPrefilterState.items[0].TpKeyToTotalMatchNum, state.items[0].TpKeyToTotalMatchNum)
			}
			// normal filter
			for i, v := range tt.testNodes {
				nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(v)
				assert.NoError(t, err)
				status = plg.Filter(context.TODO(), cycleState, tt.pod, nodeInfo)
				assert.Equal(t, tt.filterStatus[i], status)
			}
		})
	}
}

func TestPodConstraintNotMatchNodeLabel(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-node",
			Labels: map[string]string{"test-tp-key": ""},
		},
	}
	suit := newPluginTestSuit(t, []*corev1.Node{node}, nil)
	p, err := suit.proxyNew(suit.args, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)

	plg := p.(*Plugin)
	suit.start()

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: "test-tp-key",
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	cycleState := framework.NewCycleState()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: "test",
			},
		},
	}
	_, status := plg.PreFilter(context.TODO(), cycleState, pod)
	assert.True(t, status.IsSuccess())
	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	expectPrefilterState := &preFilterState{
		items: []*preFilterStateItem{
			{
				PodConstraint: podConstraint,
				RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
					{
						TopologyKey:        "test-tp-key",
						MaxSkew:            1,
						NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
						NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
					},
				},
				TpKeyToTotalMatchNum: map[string]*int32{"test-tp-key": pointer.Int32(0)},
				TpPairToMatchNum:     map[cache.TopologyPair]*int32{},
				TpKeyToCriticalPaths: map[string]*cache.TopologyCriticalPaths{
					"test-tp-key": cache.NewTopologyCriticalPaths(),
				},
			},
		},
	}
	assert.Equal(t, expectPrefilterState, state)

	nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(node.Name)
	assert.NoError(t, err)
	status = plg.Filter(context.TODO(), cycleState, pod, nodeInfo)
	assert.False(t, status.IsSuccess())
	expectReason := []string{ErrReasonNodeLabelNotMatch}
	assert.Equal(t, expectReason, status.Reasons())
}

func TestSpreadConstraintWithTopologyZone(t *testing.T) {
	tests := []struct {
		name                 string
		podConstraint        *v1beta1.PodConstraint
		existingPods         []*corev1.Pod
		nodes                []*corev1.Node
		pod                  *corev1.Pod
		expectPrefilterState *preFilterState
		testNodes            []string
		filterStatus         []*framework.Status
	}{
		{
			name: "matchNum all equal, all satisfy maxSkew",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey: corev1.LabelTopologyZone,
								MaxSkew:     1,
							},
						},
					},
				},
			},
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  2    |  2    |  2    |
			//  +-------+-------+-------+
			//
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-6").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Obj(),
				st.MakeNode().Name("test-node-4").Label(corev1.LabelTopologyZone, "na630").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(6),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(2),
						},
						TpKeyToCriticalPaths: nil,
					},
				},
			},
			testNodes:    []string{"test-node-1", "test-node-2", "test-node-3"},
			filterStatus: []*framework.Status{nil, nil, nil},
		},
		{

			name: "na630 has max matchNum, fail to satisfy maxSkew; other zones satisfy",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey: corev1.LabelTopologyZone,
								MaxSkew:     1,
							},
						},
					},
				},
			},
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  2    |  2    |  3    |
			//  +-------+-------+-------+
			//
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-6").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-7").Node("test-node-4").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Obj(),
				st.MakeNode().Name("test-node-4").Label(corev1.LabelTopologyZone, "na630").Obj(),
			},
			pod:                  st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: nil,
			testNodes:            []string{"test-node-1", "test-node-2", "test-node-3"},
			filterStatus:         []*framework.Status{nil, nil, framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch)},
		},
		{
			name: "validate maxCount",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey: corev1.LabelTopologyZone,
								MaxSkew:     1,
								MaxCount:    pointer.Int32Ptr(2),
							},
						},
					},
				},
			},
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  2    |  2    |  2    |
			//  +-------+-------+-------+
			//
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-6").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Obj(),
				st.MakeNode().Name("test-node-4").Label(corev1.LabelTopologyZone, "na630").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								MaxCount:           2,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(6),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(2),
						},
					},
				},
			},
			testNodes: []string{"test-node-1", "test-node-2", "test-node-3"},
			filterStatus: []*framework.Status{
				framework.NewStatus(framework.Unschedulable, ErrReasonMaxCountNotMatch),
				framework.NewStatus(framework.Unschedulable, ErrReasonMaxCountNotMatch),
				framework.NewStatus(framework.Unschedulable, ErrReasonMaxCountNotMatch)},
		},
		{

			name: "validate minTopologyValue",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey:      corev1.LabelTopologyZone,
								MaxSkew:          1,
								MinTopologyValue: pointer.Int32Ptr(4),
							},
						},
					},
				},
			},
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  2    |  2    |  2    |
			//  +-------+-------+-------+
			//
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-6").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Obj(),
				st.MakeNode().Name("test-node-4").Label(corev1.LabelTopologyZone, "na640").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								MinTopologyValues:  4,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(6),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na640"}: pointer.Int32(0),
						},
						TpKeyToCriticalPaths: nil,
					},
				},
			},
			testNodes: []string{"test-node-1", "test-node-2", "test-node-3", "test-node-4"},
			filterStatus: []*framework.Status{
				framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch),
				framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch),
				framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch),
				nil,
			},
		},
		{
			name: "one topology zone",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey: corev1.LabelTopologyZone,
								MaxSkew:     1,
							},
						},
					},
				},
			},
			//
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+
			//  | na610 |
			//  |-------+
			//  |  2    |
			//  +-------+
			//
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na610").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(2),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(0),
						},
						TpKeyToCriticalPaths: map[string]*cache.TopologyCriticalPaths{
							corev1.LabelTopologyZone: {
								Min: cache.CriticalPath{MatchNum: 0, TopologyValue: "na630"},
								Max: cache.CriticalPath{MatchNum: 2, TopologyValue: "na610"},
							},
						},
					},
				},
			},
			testNodes:    []string{"test-node-3"},
			filterStatus: []*framework.Status{nil},
		},
		{

			name: "Two topology zone, one invalid",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey: corev1.LabelTopologyZone,
								MaxSkew:     1,
							},
						},
					},
				},
			},
			//
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  2    |  2    |  0    |
			//  +-------+-------+-------+
			//
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(4),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(0),
						},
						TpKeyToCriticalPaths: nil,
					},
				},
			},
			testNodes: []string{"test-node-1", "test-node-2", "test-node-3"},
			filterStatus: []*framework.Status{
				framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch),
				framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch),
				nil,
			},
		},
		{
			name: "topology ratio",
			// constructs PodConstraint with TopologyRatio
			// topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  20%  |  20%  |  60%  |
			//  +-------+-------+-------+
			//
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey:   corev1.LabelTopologyZone,
								MaxSkew:       1,
								PodSpreadType: v1beta1.PodSpreadTypeRatio,
								TopologyRatios: []v1beta1.TopologyRatio{
									{
										TopologyValue: "na610",
										Ratio:         pointer.Int32Ptr(2),
									},
									{
										TopologyValue: "na620",
										Ratio:         pointer.Int32Ptr(2),
									},
									{
										TopologyValue: "na630",
										Ratio:         pointer.Int32Ptr(6),
									},
								},
							},
						},
					},
				},
			},
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  2    |  2    |  2    |
			//  +-------+-------+-------+
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-6").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Obj(),
				st.MakeNode().Name("test-node-4").Label(corev1.LabelTopologyZone, "na630").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey: corev1.LabelTopologyZone,
								MaxSkew:     1,
								TopologyRatios: map[string]int{
									"na610": 2,
									"na620": 2,
									"na630": 6,
								},
								TopologySumRatio:   10,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(6),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(2),
						},
						TpKeyToCriticalPaths: nil,
					},
				},
			},
			testNodes: []string{"test-node-1", "test-node-2", "test-node-3"},
			filterStatus: []*framework.Status{
				framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch),
				framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch),
				nil,
			},
		},
		{
			name: "topology ratio, all satisfy",
			// constructs PodConstraint with TopologyRatio
			// topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  20%  |  20%  |  60%  |
			//  +-------+-------+-------+
			//
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey:   corev1.LabelTopologyZone,
								MaxSkew:       1,
								PodSpreadType: v1beta1.PodSpreadTypeRatio,
								TopologyRatios: []v1beta1.TopologyRatio{
									{
										TopologyValue: "na610",
										Ratio:         pointer.Int32Ptr(2),
									},
									{
										TopologyValue: "na620",
										Ratio:         pointer.Int32Ptr(2),
									},
									{
										TopologyValue: "na630",
										Ratio:         pointer.Int32Ptr(6),
									},
								},
							},
						},
					},
				},
			},
			// add new Alloc in na630 align to 60%, all zone can be satisfied
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  2    |  2    |  6    |
			//  +-------+-------+-------+
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-6").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-7").Node("test-node-4").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-8").Node("test-node-4").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-9").Node("test-node-4").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-10").Node("test-node-4").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Obj(),
				st.MakeNode().Name("test-node-4").Label(corev1.LabelTopologyZone, "na630").Obj(),
			},
			pod:                  st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: nil,
			testNodes:            []string{"test-node-1", "test-node-2", "test-node-3"},
			filterStatus:         []*framework.Status{nil, nil, nil},
		},
		{

			name: "two constraints",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey: corev1.LabelTopologyZone,
								MaxSkew:     10,
							},
							{
								TopologyKey: corev1.LabelHostname,
								MaxSkew:     1,
								MaxCount:    pointer.Int32Ptr(2),
							},
						},
					},
				},
			},
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+-------+
			//  | na610 | na620 | na630 | na640 |
			//  |-------+-------+-------|-------|
			//  |  2    |  2    |  2    |  0    |
			//  +-------+-------+-------+-------+
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-6").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Label(corev1.LabelHostname, "test-node-1").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Label(corev1.LabelHostname, "test-node-2").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Label(corev1.LabelHostname, "test-node-3").Obj(),
				st.MakeNode().Name("test-node-4").Label(corev1.LabelTopologyZone, "na640").Label(corev1.LabelHostname, "test-node-4").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            10,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
							{
								TopologyKey:        corev1.LabelHostname,
								MaxSkew:            1,
								MaxCount:           2,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(6),
							corev1.LabelHostname:     pointer.Int32(6),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}:   pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}:   pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}:   pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na640"}:   pointer.Int32(0),
							{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-1"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-2"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-3"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-4"}: pointer.Int32(0),
						},
					},
				},
			},
			testNodes: []string{"test-node-1", "test-node-2", "test-node-3"},
			filterStatus: []*framework.Status{
				framework.NewStatus(framework.Unschedulable, ErrReasonMaxCountNotMatch),
				framework.NewStatus(framework.Unschedulable, ErrReasonMaxCountNotMatch),
				framework.NewStatus(framework.Unschedulable, ErrReasonMaxCountNotMatch),
			},
		},
		{

			name: "two constraints, one satisfy",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey: corev1.LabelTopologyZone,
								MaxSkew:     10,
							},
							{
								TopologyKey: corev1.LabelHostname,
								MaxSkew:     1,
								MaxCount:    pointer.Int32Ptr(2),
							},
						},
					},
				},
			},
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+-------+
			//  | na610 | na620 | na630 | na640 |
			//  |-------+-------+-------|-------|
			//  |  2    |  2    |  2    |  1    |
			//  +-------+-------+-------+-------+
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-6").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-7").Node("test-node-4").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Label(corev1.LabelHostname, "test-node-1").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Label(corev1.LabelHostname, "test-node-2").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Label(corev1.LabelHostname, "test-node-3").Obj(),
				st.MakeNode().Name("test-node-4").Label(corev1.LabelTopologyZone, "na640").Label(corev1.LabelHostname, "test-node-4").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            10,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
							{
								TopologyKey:        corev1.LabelHostname,
								MaxSkew:            1,
								MaxCount:           2,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(7),
							corev1.LabelHostname:     pointer.Int32(7),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}:   pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}:   pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}:   pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na640"}:   pointer.Int32(1),
							{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-1"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-2"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-3"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-4"}: pointer.Int32(1),
						},
					},
				},
			},
			testNodes: []string{"test-node-4"},
			filterStatus: []*framework.Status{
				nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, tt.nodes, tt.existingPods)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			for _, node := range tt.nodes {
				_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
				assert.NoError(t, err)
			}
			plg.podConstraintCache.SetPodConstraint(tt.podConstraint)

			// normal prefilter
			cycleState := framework.NewCycleState()
			_, status := plg.PreFilter(context.TODO(), cycleState, tt.pod)
			assert.True(t, status.IsSuccess())
			state, status := getPreFilterState(cycleState)
			assert.True(t, status.IsSuccess())
			if tt.expectPrefilterState != nil {
				assert.Equal(t, tt.expectPrefilterState.items[0].RequiredSpreadConstraints, state.items[0].RequiredSpreadConstraints)
				assert.Equal(t, tt.expectPrefilterState.items[0].TpPairToMatchNum, state.items[0].TpPairToMatchNum)
				if tt.expectPrefilterState.items[0].TpKeyToCriticalPaths != nil {
					assert.Equal(t, tt.expectPrefilterState.items[0].TpKeyToCriticalPaths, state.items[0].TpKeyToCriticalPaths)
				}
				assert.Equal(t, tt.expectPrefilterState.items[0].TpKeyToTotalMatchNum, state.items[0].TpKeyToTotalMatchNum)
			}
			// normal filter
			for i, v := range tt.testNodes {
				nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(v)
				assert.NoError(t, err)
				status = plg.Filter(context.TODO(), cycleState, tt.pod, nodeInfo)
				assert.Equal(t, tt.filterStatus[i], status)
			}
		})
	}
}

func TestSpreadConstraintWithVK(t *testing.T) {
	tests := []struct {
		name                 string
		podConstraint        *v1beta1.PodConstraint
		existingPods         []*corev1.Pod
		nodes                []*corev1.Node
		pod                  *corev1.Pod
		expectPrefilterState *preFilterState
		testNodes            []string
		filterStatus         []*framework.Status
	}{
		{
			name: "vk node, host name ignore",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey: corev1.LabelHostname,
								MaxSkew:     1,
							},
						},
					},
				},
			},
			//
			// constructs topology: kubernetes.io/hostname
			//  +-------+-------+-------+
			//  |  vk1  |  vk2  |  vk3  |
			//  |-------+-------+-------|
			//  |  1    |  5    |  0    |
			//  +-------+-------+-------+
			//
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-vk-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-vk-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-vk-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-vk-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-vk-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-6").Node("test-vk-2").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-vk-1").Label(uniext.LabelCommonNodeType, uniext.VKType).Label(corev1.LabelHostname, "test-vk-1").Obj(),
				st.MakeNode().Name("test-vk-2").Label(uniext.LabelCommonNodeType, uniext.VKType).Label(corev1.LabelHostname, "test-vk-2").Obj(),
				st.MakeNode().Name("test-vk-3").Label(uniext.LabelCommonNodeType, uniext.VKType).Label(corev1.LabelHostname, "test-vk-3").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelHostname,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelHostname: pointer.Int32(6),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelHostname, TopologyValue: "test-vk-1"}: pointer.Int32(1),
							{TopologyKey: corev1.LabelHostname, TopologyValue: "test-vk-2"}: pointer.Int32(5),
							{TopologyKey: corev1.LabelHostname, TopologyValue: "test-vk-3"}: pointer.Int32(0),
						},
						TpKeyToCriticalPaths: map[string]*cache.TopologyCriticalPaths{
							corev1.LabelHostname: {
								Min: cache.CriticalPath{
									TopologyValue: "test-vk-3",
									MatchNum:      0,
								},
								Max: cache.CriticalPath{
									TopologyValue: "test-vk-1",
									MatchNum:      1,
								},
							},
						},
					},
				},
			},
			testNodes:    []string{"test-vk-1", "test-vk-2", "test-vk-3"},
			filterStatus: []*framework.Status{nil, nil, nil},
		},
		{
			name: "honor taint toleration and obey vk taint toleration rules",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey:        corev1.LabelHostname,
								MaxSkew:            1,
								NodeAffinityPolicy: (*v1beta1.NodeInclusionPolicy)(pointer.String(string(v1beta1.NodeInclusionPolicyHonor))),
								NodeTaintsPolicy:   (*v1beta1.NodeInclusionPolicy)(pointer.String(string(v1beta1.NodeInclusionPolicyHonor))),
							},
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: (*v1beta1.NodeInclusionPolicy)(pointer.String(string(v1beta1.NodeInclusionPolicyHonor))),
								NodeTaintsPolicy:   (*v1beta1.NodeInclusionPolicy)(pointer.String(string(v1beta1.NodeInclusionPolicyHonor))),
							},
						},
					},
				},
			},
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-a-1").Node("node-a").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-a-2").Node("node-a").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-b-1").Node("node-b").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-y-1").Node("node-y").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-y-2").Node("node-y").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-y-3").Node("node-y").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-a",
						Labels: map[string]string{
							corev1.LabelTopologyZone: "zone1",
							corev1.LabelHostname:     "node-a",
						},
					},
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							{
								Key:    "unTolerated",
								Value:  "unTolerated",
								Effect: corev1.TaintEffectNoSchedule,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-b",
						Labels: map[string]string{
							corev1.LabelTopologyZone: "zone1",
							corev1.LabelHostname:     "node-b",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-x",
						Labels: map[string]string{
							corev1.LabelTopologyZone: "zone2",
							corev1.LabelHostname:     "node-x",
						},
					},
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							{
								Key:    "unTolerated",
								Value:  "unTolerated",
								Effect: corev1.TaintEffectNoSchedule,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-y",
						Labels: map[string]string{
							corev1.LabelTopologyZone:   "zone2",
							corev1.LabelHostname:       "node-y",
							uniext.LabelCommonNodeType: uniext.VKType,
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
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Label(uniext.LabelECIAffinity, uniext.ECIRequired).Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelHostname,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyHonor,
							},
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyHonor,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelHostname:     pointer.Int32(6),
							corev1.LabelTopologyZone: pointer.Int32(6)},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "zone2"}: pointer.Int32(3),
							{TopologyKey: corev1.LabelHostname, TopologyValue: "node-y"}:    pointer.Int32(3),
						},
					},
				},
			},
			testNodes:    []string{"node-a", "node-b", "node-x", "node-y"},
			filterStatus: []*framework.Status{nil, nil, nil, nil},
		},
		{
			name: "honor node affinity and obey vk node affinity rules",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey: corev1.LabelHostname,
								MaxSkew:     1,
							},
							{
								TopologyKey: corev1.LabelTopologyZone,
								MaxSkew:     1,
							},
						},
					},
				},
			},
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-a-1").Node("node-a").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-a-2").Node("node-a").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-b-1").Node("node-b").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-y-1").Node("node-y").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-y-2").Node("node-y").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-y-3").Node("node-y").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("node-a").Label(corev1.LabelTopologyZone, "zone1").Label(corev1.LabelHostname, "node-a").Obj(),
				st.MakeNode().Name("node-b").Label(corev1.LabelTopologyZone, "zone1").Label(corev1.LabelHostname, "node-b").Label("not-allowed", "not-allowed").Obj(),
				st.MakeNode().Name("node-x").Label(corev1.LabelTopologyZone, "zone2").Label(corev1.LabelHostname, "node-x").Obj(),
				st.MakeNode().Name("node-y").Label(corev1.LabelTopologyZone, "zone2").Label(corev1.LabelHostname, "node-y").Label(uniext.LabelCommonNodeType, uniext.VKType).Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Label(uniext.LabelECIAffinity, uniext.ECIRequired).NodeSelector(map[string]string{"not-allowed": "not-allowed"}).Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelHostname,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelHostname:     pointer.Int32(6),
							corev1.LabelTopologyZone: pointer.Int32(6)},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "zone2"}: pointer.Int32(3),
							{TopologyKey: corev1.LabelHostname, TopologyValue: "node-y"}:    pointer.Int32(3),
						},
					},
				},
			},
			testNodes:    []string{"node-a", "node-b", "node-x", "node-y"},
			filterStatus: []*framework.Status{nil, nil, nil, nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, tt.nodes, tt.existingPods)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			for _, node := range tt.nodes {
				_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
				assert.NoError(t, err)
			}
			plg.podConstraintCache.SetPodConstraint(tt.podConstraint)

			// normal prefilter
			cycleState := framework.NewCycleState()
			_, status := plg.PreFilter(context.TODO(), cycleState, tt.pod)
			assert.True(t, status.IsSuccess())
			state, status := getPreFilterState(cycleState)
			assert.True(t, status.IsSuccess())
			if tt.expectPrefilterState != nil {
				assert.Equal(t, tt.expectPrefilterState.items[0].RequiredSpreadConstraints, state.items[0].RequiredSpreadConstraints)
				assert.Equal(t, tt.expectPrefilterState.items[0].TpPairToMatchNum, state.items[0].TpPairToMatchNum)
				if tt.expectPrefilterState.items[0].TpKeyToCriticalPaths != nil {
					assert.Equal(t, tt.expectPrefilterState.items[0].TpKeyToCriticalPaths, state.items[0].TpKeyToCriticalPaths)
				}
				assert.Equal(t, tt.expectPrefilterState.items[0].TpKeyToTotalMatchNum, state.items[0].TpKeyToTotalMatchNum)
			}
			// normal filter
			for i, v := range tt.testNodes {
				nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(v)
				assert.NoError(t, err)
				status = plg.Filter(context.TODO(), cycleState, tt.pod, nodeInfo)
				assert.Equal(t, tt.filterStatus[i], status)
			}
		})
	}
}

func TestPreFilterStateAddPod(t *testing.T) {
	tests := []struct {
		name                      string
		podConstraint             *v1beta1.PodConstraint
		existingPods              []*corev1.Pod
		nodes                     []*corev1.Node
		pod                       *corev1.Pod
		expectPrefilterState      *preFilterState
		testNodes                 []string
		filterStatus              []*framework.Status
		toAddPod                  *corev1.Pod
		prefilterStateAfterAddPod *preFilterState
		filterStatusAfterAddPod   []*framework.Status
	}{
		{
			name: "matchNum all equal, all satisfy maxSkew",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey: corev1.LabelTopologyZone,
								MaxSkew:     1,
							},
						},
					},
				},
			},
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  2    |  2    |  2    |
			//  +-------+-------+-------+
			//
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Obj(),
				st.MakeNode().Name("test-node-4").Label(corev1.LabelTopologyZone, "na630").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(5),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(1),
						},
						TpKeyToCriticalPaths: nil,
					},
				},
			},
			testNodes:    []string{"test-node-1", "test-node-2", "test-node-3"},
			filterStatus: []*framework.Status{framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch), framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch), nil},
			toAddPod:     st.MakePod().Namespace("default").Name("pod-6").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
			prefilterStateAfterAddPod: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(6),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(2),
						},
						TpKeyToCriticalPaths: nil,
					},
				},
			},
			filterStatusAfterAddPod: []*framework.Status{nil, nil, nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, tt.nodes, tt.existingPods)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			for _, node := range tt.nodes {
				_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
				assert.NoError(t, err)
			}
			plg.podConstraintCache.SetPodConstraint(tt.podConstraint)

			// normal prefilter
			cycleState := framework.NewCycleState()
			_, status := plg.PreFilter(context.TODO(), cycleState, tt.pod)
			assert.True(t, status.IsSuccess())
			state, status := getPreFilterState(cycleState)
			assert.True(t, status.IsSuccess())
			if tt.expectPrefilterState != nil {
				assert.Equal(t, tt.expectPrefilterState.items[0].RequiredSpreadConstraints, state.items[0].RequiredSpreadConstraints)
				assert.Equal(t, tt.expectPrefilterState.items[0].TpPairToMatchNum, state.items[0].TpPairToMatchNum)
				if tt.expectPrefilterState.items[0].TpKeyToCriticalPaths != nil {
					assert.Equal(t, tt.expectPrefilterState.items[0].TpKeyToCriticalPaths, state.items[0].TpKeyToCriticalPaths)
				}
				assert.Equal(t, tt.expectPrefilterState.items[0].TpKeyToTotalMatchNum, state.items[0].TpKeyToTotalMatchNum)
			}
			// normal filter
			for i, v := range tt.testNodes {
				nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(v)
				assert.NoError(t, err)
				status = plg.Filter(context.TODO(), cycleState, tt.pod, nodeInfo)
				assert.Equal(t, tt.filterStatus[i], status)
			}

			nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(tt.toAddPod.Spec.NodeName)
			assert.NoError(t, err)
			podInfo, _ := framework.NewPodInfo(tt.toAddPod)
			status = plg.AddPod(context.TODO(), cycleState, tt.pod, podInfo, nodeInfo)
			assert.True(t, status.IsSuccess())
			if tt.prefilterStateAfterAddPod != nil {
				assert.Equal(t, tt.prefilterStateAfterAddPod.items[0].RequiredSpreadConstraints, state.items[0].RequiredSpreadConstraints)
				assert.Equal(t, tt.prefilterStateAfterAddPod.items[0].TpPairToMatchNum, state.items[0].TpPairToMatchNum)
				if tt.prefilterStateAfterAddPod.items[0].TpKeyToCriticalPaths != nil {
					assert.Equal(t, tt.prefilterStateAfterAddPod.items[0].TpKeyToCriticalPaths, state.items[0].TpKeyToCriticalPaths)
				}
				assert.Equal(t, tt.prefilterStateAfterAddPod.items[0].TpKeyToTotalMatchNum, state.items[0].TpKeyToTotalMatchNum)
			}
			// normal filter after add pod
			for i, v := range tt.testNodes {
				nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(v)
				assert.NoError(t, err)
				status = plg.Filter(context.TODO(), cycleState, tt.pod, nodeInfo)
				assert.Equal(t, tt.filterStatusAfterAddPod[i], status)
			}
		})
	}
}

func TestPreFilterStateRemovePod(t *testing.T) {
	tests := []struct {
		name                         string
		podConstraint                *v1beta1.PodConstraint
		existingPods                 []*corev1.Pod
		nodes                        []*corev1.Node
		pod                          *corev1.Pod
		expectPrefilterState         *preFilterState
		testNodes                    []string
		filterStatus                 []*framework.Status
		toRemovePod                  *corev1.Pod
		prefilterStateAfterRemovePod *preFilterState
		filterStatusAfterRemovePod   []*framework.Status
	}{
		{
			name: "matchNum all equal, all satisfy maxSkew",
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					SpreadRule: v1beta1.SpreadRule{
						Requires: []v1beta1.SpreadRuleItem{
							{
								TopologyKey: corev1.LabelTopologyZone,
								MaxSkew:     1,
							},
						},
					},
				},
			},
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  2    |  2    |  2    |
			//  +-------+-------+-------+
			//
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-6").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Obj(),
				st.MakeNode().Name("test-node-4").Label(corev1.LabelTopologyZone, "na630").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(6),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(2),
						},
						TpKeyToCriticalPaths: nil,
					},
				},
			},
			testNodes:    []string{"test-node-1", "test-node-2", "test-node-3"},
			filterStatus: []*framework.Status{nil, nil, nil},
			toRemovePod:  st.MakePod().Namespace("default").Name("pod-6").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
			prefilterStateAfterRemovePod: &preFilterState{
				items: []*preFilterStateItem{
					{
						RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpKeyToTotalMatchNum: map[string]*int32{
							corev1.LabelTopologyZone: pointer.Int32(5),
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(1),
						},
						TpKeyToCriticalPaths: nil,
					},
				},
			},
			filterStatusAfterRemovePod: []*framework.Status{framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch), framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch), nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, tt.nodes, tt.existingPods)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			for _, node := range tt.nodes {
				_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
				assert.NoError(t, err)
			}
			plg.podConstraintCache.SetPodConstraint(tt.podConstraint)

			// normal prefilter
			cycleState := framework.NewCycleState()
			_, status := plg.PreFilter(context.TODO(), cycleState, tt.pod)
			assert.True(t, status.IsSuccess())
			state, status := getPreFilterState(cycleState)
			assert.True(t, status.IsSuccess())
			if tt.expectPrefilterState != nil {
				assert.Equal(t, tt.expectPrefilterState.items[0].RequiredSpreadConstraints, state.items[0].RequiredSpreadConstraints)
				assert.Equal(t, tt.expectPrefilterState.items[0].TpPairToMatchNum, state.items[0].TpPairToMatchNum)
				if tt.expectPrefilterState.items[0].TpKeyToCriticalPaths != nil {
					assert.Equal(t, tt.expectPrefilterState.items[0].TpKeyToCriticalPaths, state.items[0].TpKeyToCriticalPaths)
				}
				assert.Equal(t, tt.expectPrefilterState.items[0].TpKeyToTotalMatchNum, state.items[0].TpKeyToTotalMatchNum)
			}
			// normal filter
			for i, v := range tt.testNodes {
				nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(v)
				assert.NoError(t, err)
				status = plg.Filter(context.TODO(), cycleState, tt.pod, nodeInfo)
				assert.Equal(t, tt.filterStatus[i], status)
			}

			nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(tt.toRemovePod.Spec.NodeName)
			assert.NoError(t, err)
			podInfo, _ := framework.NewPodInfo(tt.toRemovePod)
			status = plg.RemovePod(context.TODO(), cycleState, tt.pod, podInfo, nodeInfo)
			assert.True(t, status.IsSuccess())
			if tt.prefilterStateAfterRemovePod != nil {
				assert.Equal(t, tt.prefilterStateAfterRemovePod.items[0].RequiredSpreadConstraints, state.items[0].RequiredSpreadConstraints)
				assert.Equal(t, tt.prefilterStateAfterRemovePod.items[0].TpPairToMatchNum, state.items[0].TpPairToMatchNum)
				if tt.prefilterStateAfterRemovePod.items[0].TpKeyToCriticalPaths != nil {
					assert.Equal(t, tt.prefilterStateAfterRemovePod.items[0].TpKeyToCriticalPaths, state.items[0].TpKeyToCriticalPaths)
				}
				assert.Equal(t, tt.prefilterStateAfterRemovePod.items[0].TpKeyToTotalMatchNum, state.items[0].TpKeyToTotalMatchNum)
			}
			// normal filter after add pod
			for i, v := range tt.testNodes {
				nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(v)
				assert.NoError(t, err)
				status = plg.Filter(context.TODO(), cycleState, tt.pod, nodeInfo)
				assert.Equal(t, tt.filterStatusAfterRemovePod[i], status)
			}
		})
	}
}
