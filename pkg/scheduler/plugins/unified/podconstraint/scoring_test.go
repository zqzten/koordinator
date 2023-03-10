package podconstraint

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
	"k8s.io/utils/pointer"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/podconstraint/cache"
)

func TestScore(t *testing.T) {
	tests := []struct {
		name                 string
		podConstraint        *v1beta1.PodConstraint
		existingPods         []*corev1.Pod
		nodes                []*corev1.Node
		pod                  *corev1.Pod
		expectPreScoreState  *preScoreState
		testNodes            []string
		expectNodeScore      []int64
		expectNormalizeScore framework.NodeScoreList
	}{
		{
			//
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  1    |  2    |  2    |
			//  +-------+-------+-------+
			//
			name: "score with required podConstraint",
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
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Label(corev1.LabelHostname, "test-node-1").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Label(corev1.LabelHostname, "test-node-2").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Label(corev1.LabelHostname, "test-node-3").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPreScoreState: &preScoreState{
				items: []*preScoreStateItem{
					{
						PodConstraint: &v1beta1.PodConstraint{
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
						PreferredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(1),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(2),
						},
						IgnoredNodes: sets.NewString(),
					},
				},
			},
			testNodes:       []string{"test-node-1", "test-node-2", "test-node-3"},
			expectNodeScore: []int64{804, 1609, 1609},
			expectNormalizeScore: framework.NodeScoreList{
				{
					Name:  "test-node-1",
					Score: 100,
				},
				{
					Name:  "test-node-2",
					Score: 49,
				},
				{
					Name:  "test-node-3",
					Score: 49,
				},
			},
		},
		{
			//
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  1    |  2    |  2    |
			//  +-------+-------+-------+
			//
			name: "score with preferred podConstraint",
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
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Label(corev1.LabelHostname, "test-node-1").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Label(corev1.LabelHostname, "test-node-2").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Label(corev1.LabelHostname, "test-node-3").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPreScoreState: &preScoreState{
				items: []*preScoreStateItem{
					{
						PodConstraint: &v1beta1.PodConstraint{
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
						PreferredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(1),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(2),
						},
						IgnoredNodes: sets.NewString(),
					},
				},
			},
			testNodes:       []string{"test-node-1", "test-node-2", "test-node-3"},
			expectNodeScore: []int64{804, 1609, 1609},
			expectNormalizeScore: framework.NodeScoreList{
				{
					Name:  "test-node-1",
					Score: 100,
				},
				{
					Name:  "test-node-2",
					Score: 49,
				},
				{
					Name:  "test-node-3",
					Score: 49,
				},
			},
		},
		{
			//
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  1    |  2    |  2    |
			//  +-------+-------+-------+
			//
			name: "score with preferred podConstraint, one Ignored Node",
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
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Label(extunified.LabelPodConstraint, "test").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Label(corev1.LabelHostname, "test-node-1").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Label(corev1.LabelHostname, "test-node-2").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Label(corev1.LabelHostname, "test-node-3").Obj(),
				st.MakeNode().Name("test-node-4").Label(corev1.LabelHostname, "test-node-4").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPreScoreState: &preScoreState{
				items: []*preScoreStateItem{
					{
						PodConstraint: &v1beta1.PodConstraint{
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
						PreferredSpreadConstraints: []*cache.TopologySpreadConstraint{
							{
								TopologyKey:        corev1.LabelTopologyZone,
								MaxSkew:            1,
								NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
								NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
							},
						},
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(1),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(2),
						},
						IgnoredNodes: sets.NewString("test-node-4"),
					},
				},
			},
			testNodes:       []string{"test-node-1", "test-node-2", "test-node-3", "test-node-4"},
			expectNodeScore: []int64{804, 1609, 1609, 0},
			expectNormalizeScore: framework.NodeScoreList{
				{
					Name:  "test-node-1",
					Score: 100,
				},
				{
					Name:  "test-node-2",
					Score: 49,
				},
				{
					Name:  "test-node-3",
					Score: 49,
				},
				{
					Name:  "test-node-4",
					Score: 0,
				},
			},
		},
		{
			name: "score with preferred podConstraint, one Ignored Node",
			//
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
			//
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
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Label(corev1.LabelHostname, "test-node-1").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Label(corev1.LabelHostname, "test-node-2").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Label(corev1.LabelHostname, "test-node-3").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Label(extunified.LabelPodConstraint, "test").Obj(),
			expectPreScoreState: &preScoreState{
				items: []*preScoreStateItem{
					{
						PodConstraint: &v1beta1.PodConstraint{
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
						PreferredSpreadConstraints: []*cache.TopologySpreadConstraint{
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
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(2),
						},
						IgnoredNodes: sets.NewString(),
					},
				},
			},
			testNodes:       []string{"test-node-1", "test-node-2", "test-node-3"},
			expectNodeScore: []int64{1609, 1609, 1609},
			expectNormalizeScore: framework.NodeScoreList{
				{
					Name:  "test-node-1",
					Score: 100,
				},
				{
					Name:  "test-node-2",
					Score: 100,
				},
				{
					Name:  "test-node-3",
					Score: 100,
				},
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

			cycleState := framework.NewCycleState()
			status := plg.PreScore(context.TODO(), cycleState, tt.pod, tt.nodes)
			assert.True(t, status.IsSuccess())
			state, status := getPreScoreState(cycleState)
			assert.True(t, status.IsSuccess())
			assert.Equal(t, tt.expectPreScoreState.items[0].PreferredSpreadConstraints, state.items[0].PreferredSpreadConstraints)
			assert.Equal(t, tt.expectPreScoreState.items[0].IgnoredNodes, state.items[0].IgnoredNodes)
			assert.Equal(t, tt.expectPreScoreState.items[0].TpPairToMatchNum, state.items[0].TpPairToMatchNum)
			var gotNodeScores framework.NodeScoreList
			for i, v := range tt.testNodes {
				score, status := plg.Score(context.TODO(), cycleState, tt.pod, v)
				assert.True(t, status.IsSuccess())
				assert.Equal(t, tt.expectNodeScore[i], score)
				gotNodeScores = append(gotNodeScores, framework.NodeScore{
					Name:  v,
					Score: score,
				})
			}
			status = plg.NormalizeScore(context.TODO(), cycleState, tt.pod, gotNodeScores)
			assert.True(t, status.IsSuccess())
			assert.Equal(t, tt.expectNormalizeScore, gotNodeScores)
		})
	}
}

func TestScoreWithWeightedSpreadUnits(t *testing.T) {
	tests := []struct {
		name                 string
		weightedSpreadUnit   []extunified.WeightedSpreadUnit
		existingPods         []*corev1.Pod
		nodes                []*corev1.Node
		pod                  *corev1.Pod
		expectPreScoreState  *preScoreState
		testNodes            []string
		expectNodeScore      []int64
		expectNormalizeScore framework.NodeScoreList
	}{
		{
			//
			// constructs topology: topology.kubernetes.io/zone
			//  +-------+-------+-------+
			//  | na610 | na620 | na630 |
			//  |-------+-------+-------|
			//  |  1    |  2    |  2    |
			//  +-------+-------+-------+
			// constructs topology: kubernetes.io/hostname for appA-hostA
			//  +-------------+-------------+-------------+
			//  | test-node-1 | test-node-2 | test-node-3 |
			//  |-------------+-------------+-------------|
			//  |  1          |        2    |        2    |
			//  +-------------+-------------+-------------+
			//
			// constructs topology: kubernetes.io/hostname for appA-hostB
			//  +-------------+-------------+-------------+
			//  | test-node-1 | test-node-2 | test-node-3 |
			//  |-------------+-------------+-------------|
			//  |  0          |        0    |        0    |
			//  +-------------+-------------+-------------+
			//
			name: "",

			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("test-node-1").Obj(),
				st.MakePod().Namespace("default").Name("pod-2").Node("test-node-2").Obj(),
				st.MakePod().Namespace("default").Name("pod-3").Node("test-node-2").Obj(),
				st.MakePod().Namespace("default").Name("pod-4").Node("test-node-3").Obj(),
				st.MakePod().Namespace("default").Name("pod-5").Node("test-node-3").Obj(),
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("test-node-1").Label(corev1.LabelTopologyZone, "na610").Label(corev1.LabelHostname, "test-node-1").Obj(),
				st.MakeNode().Name("test-node-2").Label(corev1.LabelTopologyZone, "na620").Label(corev1.LabelHostname, "test-node-2").Obj(),
				st.MakeNode().Name("test-node-3").Label(corev1.LabelTopologyZone, "na630").Label(corev1.LabelHostname, "test-node-3").Obj(),
			},
			pod: st.MakePod().Namespace("default").Name("test-pod").Obj(),
			expectPreScoreState: &preScoreState{
				items: []*preScoreStateItem{
					{
						PreferredSpreadConstraints: []*cache.TopologySpreadConstraint{
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
						Weight: 100,
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(1),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(2),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(2),
						},
						IgnoredNodes: sets.NewString(),
					},
					{
						PreferredSpreadConstraints: []*cache.TopologySpreadConstraint{
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
						Weight: 10,
						TpPairToMatchNum: map[cache.TopologyPair]*int32{
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: pointer.Int32(0),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: pointer.Int32(0),
							{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: pointer.Int32(0),
						},
						IgnoredNodes: sets.NewString(),
					},
				},
			},
			testNodes:       []string{"test-node-1", "test-node-2", "test-node-3"},
			expectNodeScore: []int64{80600, 161200, 161200},
			expectNormalizeScore: framework.NodeScoreList{
				{
					Name:  "test-node-1",
					Score: 100,
				},
				{
					Name:  "test-node-2",
					Score: 50,
				},
				{
					Name:  "test-node-3",
					Score: 50,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range tt.existingPods {
				weightedSpreadUnit := []extunified.WeightedSpreadUnit{
					{
						Name:   "appA",
						Weight: 100,
					},
					{
						Name:   "appA-hostA",
						Weight: 10,
					},
				}
				weightedSpreadUnitsData, err := json.Marshal(weightedSpreadUnit)
				assert.NoError(t, err)
				assert.NotEmpty(t, weightedSpreadUnitsData)
				tt.existingPods[i].Annotations = map[string]string{}
				tt.existingPods[i].Annotations[extunified.AnnotationWeightedSpreadUnits] = string(weightedSpreadUnitsData)
			}
			weightedSpreadUnit := []extunified.WeightedSpreadUnit{
				{
					Name:   "appA",
					Weight: 100,
				},
				{
					Name:   "appA-hostB",
					Weight: 10,
				},
			}
			weightedSpreadUnitsData, err := json.Marshal(weightedSpreadUnit)
			assert.NoError(t, err)
			assert.NotEmpty(t, weightedSpreadUnitsData)
			tt.pod.Annotations = map[string]string{}
			tt.pod.Annotations[extunified.AnnotationWeightedSpreadUnits] = string(weightedSpreadUnitsData)
			suit := newPluginTestSuit(t, tt.nodes, tt.existingPods)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			cycleState := framework.NewCycleState()
			status := plg.PreScore(context.TODO(), cycleState, tt.pod, tt.nodes)
			assert.True(t, status.IsSuccess())
			state, status := getPreScoreState(cycleState)
			assert.True(t, status.IsSuccess())
			assert.Equal(t, tt.expectPreScoreState.items[0].PreferredSpreadConstraints, state.items[0].PreferredSpreadConstraints)
			assert.Equal(t, tt.expectPreScoreState.items[0].IgnoredNodes, state.items[0].IgnoredNodes)
			assert.Equal(t, tt.expectPreScoreState.items[0].TpPairToMatchNum, state.items[0].TpPairToMatchNum)
			var gotNodeScores framework.NodeScoreList
			for i, v := range tt.testNodes {
				score, status := plg.Score(context.TODO(), cycleState, tt.pod, v)
				assert.True(t, status.IsSuccess())
				assert.Equal(t, tt.expectNodeScore[i], score)
				gotNodeScores = append(gotNodeScores, framework.NodeScore{
					Name:  v,
					Score: score,
				})
			}
			status = plg.NormalizeScore(context.TODO(), cycleState, tt.pod, gotNodeScores)
			assert.True(t, status.IsSuccess())
			assert.Equal(t, tt.expectNormalizeScore, gotNodeScores)
		})
	}
}
