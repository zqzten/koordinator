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
	"fmt"
	"math"
	"sync/atomic"

	unischeduling "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	nodeaffinityhelper "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/nodeaffinity"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/podconstraint/cache"
)

const (
	preScoreStateKey = "PreScore" + Name
	invalidScore     = -1
)

const (
	SigmaLabelSiteName = "sigma.ali/site"
)

var defaultTopologyWeight = map[string]float64{
	SigmaLabelSiteName:            500,
	corev1.LabelTopologyZone:      500,
	corev1.LabelZoneFailureDomain: 500,
}

type preScoreState struct {
	items []*preScoreStateItem
}

type preScoreStateItem struct {
	PodConstraint              *unischeduling.PodConstraint
	PreferredSpreadConstraints []*cache.TopologySpreadConstraint
	Weight                     int
	TpPairToMatchNum           map[cache.TopologyPair]*int32
	TopologyNormalizingWeight  []float64
	IgnoredNodes               sets.String
}

func (s *preScoreState) Clone() framework.StateData {
	return s
}

func getPreScoreState(cycleState *framework.CycleState) (*preScoreState, *framework.Status) {
	value, err := cycleState.Read(preScoreStateKey)
	if err != nil {
		return nil, framework.AsStatus(err)
	}
	state := value.(*preScoreState)
	return state, nil
}

func (p *Plugin) PreScore(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodes []*corev1.Node) *framework.Status {
	allNodes, err := p.handle.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("getting all nodes from Snapshot: %v", err))
	}
	if len(nodes) == 0 || len(allNodes) == 0 {
		// No nodes to score.
		return nil
	}
	state := &preScoreState{}
	if weightedPodConstraints := extunified.GetWeightedPodConstraints(pod); len(weightedPodConstraints) != 0 {
		for _, weightedPodConstraint := range weightedPodConstraints {
			constraintState := p.podConstraintCache.GetState(cache.GetNamespacedName(weightedPodConstraint.Namespace, weightedPodConstraint.Name))
			if constraintState == nil {
				return framework.NewStatus(framework.Error, ErrMissPodConstraint)
			}
			constraintState.RLock()
			constraint := constraintState.PodConstraint
			preferredSpreadConstraints := constraintState.CopyPreferredSpreadConstraints()
			if len(preferredSpreadConstraints) == 0 {
				preferredSpreadConstraints = constraintState.CopyRequiredSpreadConstraints()
			}
			constraintState.RUnlock()
			if len(preferredSpreadConstraints) == 0 {
				continue
			}
			state.items = append(state.items, &preScoreStateItem{
				PodConstraint:              constraint,
				Weight:                     weightedPodConstraint.Weight,
				PreferredSpreadConstraints: preferredSpreadConstraints,
				TpPairToMatchNum:           map[cache.TopologyPair]*int32{},
				IgnoredNodes:               sets.NewString(),
			})
		}
	} else if weightedSpreadUnits := extunified.GetWeighedSpreadUnits(pod); len(weightedSpreadUnits) != 0 {
		for _, weightedSpreadUnit := range weightedSpreadUnits {
			if weightedSpreadUnit.Name == "" {
				continue
			}
			defaultPodConstraintName := cache.GetDefaultPodConstraintName(weightedSpreadUnit.Name)
			spreadRequiredType := extunified.IsSpreadTypeRequire(pod)
			constraint := cache.BuildDefaultPodConstraint(pod.Namespace, defaultPodConstraintName, spreadRequiredType)
			preferredSpreadConstraints := cache.SpreadRulesToTopologySpreadConstraint(constraint.Spec.SpreadRule.Affinities)
			if len(preferredSpreadConstraints) == 0 {
				preferredSpreadConstraints = cache.SpreadRulesToTopologySpreadConstraint(constraint.Spec.SpreadRule.Requires)
			}
			if len(preferredSpreadConstraints) == 0 {
				continue
			}
			state.items = append(state.items, &preScoreStateItem{
				PodConstraint:              constraint,
				PreferredSpreadConstraints: preferredSpreadConstraints,
				Weight:                     weightedSpreadUnit.Weight,
				TpPairToMatchNum:           map[cache.TopologyPair]*int32{},
				IgnoredNodes:               sets.NewString(),
			})
		}
	}
	if len(state.items) == 0 {
		cycleState.Write(preScoreStateKey, state)
		return nil
	}
	for _, v := range state.items {
		status := p.preScoreWithPodConstraint(v, nodes)
		if !status.IsSuccess() {
			return status
		}
	}

	nodeSelector, affinity, status := GetNodeSelectorAndAffinity(p.podConstraintCache, pod)
	if !status.IsSuccess() {
		return status
	}
	requiredNodeAffinity := nodeaffinityhelper.GetRequiredNodeAffinity(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: pod.Labels}, Spec: corev1.PodSpec{Affinity: affinity, NodeSelector: nodeSelector}})

	processNode := func(i int) {
		nodeInfo := allNodes[i]
		node := nodeInfo.Node()
		if node == nil {
			return
		}

		for _, stateItem := range state.items {
			if !cache.NodeLabelsMatchSpreadConstraints(node.Labels, stateItem.PreferredSpreadConstraints) {
				continue
			}
			for _, c := range stateItem.PreferredSpreadConstraints {

				tpKey := c.TopologyKey
				tpVal := node.Labels[tpKey]
				if tpVal == "" {
					continue
				}
				pair := cache.TopologyPair{TopologyKey: tpKey, TopologyValue: tpVal}
				tpCount := stateItem.TpPairToMatchNum[pair]
				if tpCount == nil {
					continue
				}
				if p.enableNodeInclusionPolicyInPodConstraint && !c.MatchNodeInclusionPolicies(pod, node, requiredNodeAffinity) {
					return
				}
				count := countPodsMatchConstraint(nodeInfo.Pods, stateItem.PodConstraint.Namespace, stateItem.PodConstraint.Name)
				atomic.AddInt32(tpCount, int32(count))
			}
		}
	}
	p.handle.Parallelizer().Until(context.Background(), len(allNodes), processNode)
	cycleState.Write(preScoreStateKey, state)
	return nil
}

func (p *Plugin) preScoreWithPodConstraint(item *preScoreStateItem, nodes []*corev1.Node) *framework.Status {
	topologyValNums := make([]int, len(item.PreferredSpreadConstraints))
	for _, node := range nodes {
		if !cache.NodeLabelsMatchSpreadConstraints(node.Labels, item.PreferredSpreadConstraints) {
			item.IgnoredNodes.Insert(node.Name)
			continue
		}
		for i, constraint := range item.PreferredSpreadConstraints {
			tpVal := node.Labels[constraint.TopologyKey]
			if tpVal == "" {
				item.IgnoredNodes.Insert(node.Name)
				continue
			}
			// per-node counts are calculated during Score.
			if constraint.TopologyKey == corev1.LabelHostname {
				continue
			}
			pair := cache.TopologyPair{TopologyKey: constraint.TopologyKey, TopologyValue: tpVal}
			if _, ok := item.TpPairToMatchNum[pair]; !ok {
				item.TpPairToMatchNum[pair] = new(int32)
				topologyValNums[i]++
			}
		}
	}
	item.TopologyNormalizingWeight = make([]float64, len(item.PreferredSpreadConstraints))
	for i, c := range item.PreferredSpreadConstraints {
		topologyValNum := topologyValNums[i]
		if c.TopologyKey == corev1.LabelHostname {
			topologyValNum = len(nodes) - item.IgnoredNodes.Len()
		}
		baseWeight := defaultTopologyWeight[c.TopologyKey]
		if baseWeight == 0 {
			baseWeight = 1
		}
		item.TopologyNormalizingWeight[i] = topologyNormalizingWeight(topologyValNum) * baseWeight
	}
	return nil
}

func topologyNormalizingWeight(topologyValNum int) float64 {
	return math.Log(float64(topologyValNum + 2))
}

func (p *Plugin) Score(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	state, status := getPreScoreState(cycleState)
	if !status.IsSuccess() {
		return int64(0), status
	}
	if len(state.items) == 0 {
		return int64(0), nil
	}
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()
	if node == nil {
		return 0, framework.NewStatus(framework.Error, "node not found")
	}

	var totalScore int64
	for _, item := range state.items {
		score, status := p.scoreWithPodConstraint(item, nodeInfo)
		if !status.IsSuccess() {
			return 0, status
		}
		totalScore += score * int64(item.Weight)
	}
	return totalScore, nil
}

func (p *Plugin) scoreWithPodConstraint(item *preScoreStateItem, nodeInfo *framework.NodeInfo) (int64, *framework.Status) {
	if item.IgnoredNodes.Has(nodeInfo.Node().Name) {
		return 0, nil
	}
	if len(item.PreferredSpreadConstraints) == 0 {
		return 0, nil
	}

	var score float64
	for i, constraint := range item.PreferredSpreadConstraints {
		if i > len(item.TopologyNormalizingWeight) {
			break
		}
		if tpVal, ok := nodeInfo.Node().Labels[constraint.TopologyKey]; ok {
			var matchNum int
			if constraint.TopologyKey == corev1.LabelHostname {
				matchNum = countPodsMatchConstraint(nodeInfo.Pods, item.PodConstraint.Namespace, item.PodConstraint.Name)
			} else {
				pair := cache.TopologyPair{TopologyKey: constraint.TopologyKey, TopologyValue: tpVal}
				matchNum = int(*item.TpPairToMatchNum[pair])
			}
			score += scoreForMatchNum(matchNum, constraint.MaxSkew, item.TopologyNormalizingWeight[i])
		}
	}
	return int64(score), nil
}

func scoreForMatchNum(matchNum int, maxSkew int, tpWeight float64) float64 {
	return float64(matchNum)*tpWeight + float64(maxSkew-1)
}

func (p *Plugin) ScoreExtensions() framework.ScoreExtensions {
	return p
}

func (p *Plugin) NormalizeScore(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, scores framework.NodeScoreList) *framework.Status {
	state, status := getPreScoreState(cycleState)
	if !status.IsSuccess() {
		return status
	}
	if len(state.items) == 0 {
		return nil
	}

	var maxScore int64
	var minScore int64 = math.MaxInt64
	for _, item := range state.items {
		for i, score := range scores {
			if item.IgnoredNodes.Has(score.Name) {
				scores[i].Score = invalidScore
				continue
			}
			if score.Score == invalidScore {
				continue
			}

			if score.Score < minScore {
				minScore = score.Score
			}
			if score.Score > maxScore {
				maxScore = score.Score
			}
		}
	}

	for i := range scores {
		if scores[i].Score == invalidScore {
			scores[i].Score = 0
			continue
		}
		if maxScore == 0 {
			scores[i].Score = framework.MaxNodeScore
			continue
		}
		s := scores[i].Score
		// the lower s, the higher score
		scores[i].Score = framework.MaxNodeScore * (maxScore + minScore - s) / maxScore
	}
	return nil
}
