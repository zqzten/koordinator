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
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
)

const (
	Name = "UnifiedPodConstraint"
)

const (
	// ErrMissPodConstraint is used for PodConstraint pre-filter error.
	ErrMissPodConstraint = "miss PodConstraint"
	// ErrReasonConstraintsNotMatch is used for PodConstraint filter error.
	ErrReasonConstraintsNotMatch = "node(s) didn't match PodConstraint spread constraint"
	// ErrReasonNodeLabelNotMatch is used when the node doesn't hold the required label.
	ErrReasonNodeLabelNotMatch = ErrReasonConstraintsNotMatch + " (missing required label)"
	// ErrReasonMaxCountNotMatch is used when the count of Pods equal or greater the maxCount in the topology value
	ErrReasonMaxCountNotMatch = ErrReasonConstraintsNotMatch + " (exceed maxCount)"
	// ErrReasonMinTopologyValuesNotSatisfy is used when the min topology values is not satisfied
	ErrReasonMinTopologyValuesNotSatisfy = ErrReasonConstraintsNotMatch + " (min topology values not satisfy)"
	// ErrReasonTopologyValueNotMatch is used when the node topology value is not in TopologyRatios
	ErrReasonTopologyValueNotMatch = ErrReasonConstraintsNotMatch + " (topologyRatios)"
)

const (
	stateKey           = Name
	SigmaLabelSiteName = "sigma.ali/site"
	invalidScore       = -1
)

var defaultTopologyWeight = map[string]float64{
	SigmaLabelSiteName:            500,
	corev1.LabelTopologyZone:      500,
	corev1.LabelZoneFailureDomain: 500,
}

var (
	_ framework.PreFilterPlugin = &Plugin{}
	_ framework.FilterPlugin    = &Plugin{}
	_ framework.PreScorePlugin  = &Plugin{}
	_ framework.ScorePlugin     = &Plugin{}
	_ framework.ScoreExtensions = &Plugin{}
)

type Plugin struct {
	handle             framework.Handle
	pluginArgs         *schedulingconfig.UnifiedPodConstraintArgs
	podConstraintCache *PodConstraintCache
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	pluginArgs, ok := args.(*schedulingconfig.UnifiedPodConstraintArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type UnifiedPodConstraintArgs, got %T", args)
	}
	podConstraintCache := newPodConstraintCache(handle, *pluginArgs.EnableDefaultPodConstraint)
	if err := registerPodConstraintEventHandler(handle, podConstraintCache); err != nil {
		return nil, err
	}
	registerPodEventHandler(handle, podConstraintCache)
	return &Plugin{
		handle:             handle,
		pluginArgs:         pluginArgs,
		podConstraintCache: podConstraintCache,
	}, nil
}

func (p *Plugin) Name() string {
	return Name
}

type preFilterState struct {
	items []*preFilterStateItem
}

type preFilterStateItem struct {
	*TopologySpreadConstraintState
	Weight                    int
	TopologyNormalizingWeight []float64
	SpreadConstraintsInScore  []*TopologySpreadConstraint
	TpPairToMatchNumInScore   map[TopologyPair]int
	IgnoredNodes              sets.String
}

func (s *preFilterState) Clone() framework.StateData {

	var clonedPreFilterState = &preFilterState{}
	for _, item := range s.items {
		cloneItem := &preFilterStateItem{
			TopologySpreadConstraintState: item.TopologySpreadConstraintState,
			Weight:                        item.Weight,
			TopologyNormalizingWeight:     item.TopologyNormalizingWeight,
			IgnoredNodes:                  sets.NewString(item.IgnoredNodes.List()...),
		}
		clonedPreFilterState.items = append(clonedPreFilterState.items, cloneItem)
	}
	return clonedPreFilterState
}

func getPreFilterState(cycleState *framework.CycleState) (*preFilterState, *framework.Status) {
	value, err := cycleState.Read(stateKey)
	if err != nil {
		return nil, framework.AsStatus(err)
	}
	state := value.(*preFilterState)
	return state, nil
}

func (p *Plugin) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) *framework.Status {
	state := &preFilterState{}
	if weightedPodConstraints := extunified.GetWeightedPodConstraints(pod); len(weightedPodConstraints) != 0 {
		for _, weightedPodConstraint := range weightedPodConstraints {
			constraintState := p.podConstraintCache.GetState(getNamespacedName(weightedPodConstraint.Namespace, weightedPodConstraint.Name))
			if constraintState == nil {
				return framework.NewStatus(framework.Error, ErrMissPodConstraint)
			}
			state.items = append(state.items, &preFilterStateItem{
				TopologySpreadConstraintState: constraintState,
				Weight:                        weightedPodConstraint.Weight,
				IgnoredNodes:                  sets.NewString(),
			})
		}
	} else if weightedSpreadUnits := extunified.GetWeighedSpreadUnits(pod); len(weightedSpreadUnits) != 0 {
		for _, weightedSpreadUnit := range weightedSpreadUnits {
			if weightedSpreadUnit.Name == "" {
				continue
			}
			constraintState := p.podConstraintCache.GetDefaultPodConstraintState(pod.Namespace, weightedSpreadUnit.Name)
			defaultPodConstraintName := GetDefaultPodConstraintName(weightedSpreadUnit.Name)
			spreadRequiredType := extunified.IsSpreadTypeRequire(pod)
			constraint := BuildDefaultPodConstraint(pod.Namespace, defaultPodConstraintName, spreadRequiredType)
			if constraintState == nil {
				// 默认constraint没有CRD，因此在Prefilter阶段构造默认PodConstraintState并存入preFilterState使得后续的Filter和Score流程正确
				constraintState = newTopologySpreadConstraintState(constraint, &spreadRequiredType)
			} else if constraintState.DefaultPodConstraint && constraintState.SpreadTypeRequired != spreadRequiredType {
				_, constraintState = p.podConstraintCache.UpdateStateIfNeed(constraint, &spreadRequiredType)
			}
			state.items = append(state.items, &preFilterStateItem{
				TopologySpreadConstraintState: constraintState,
				Weight:                        weightedSpreadUnit.Weight,
				IgnoredNodes:                  sets.NewString(),
			})
		}
	}

	status := p.reInitTopology(state, pod)
	if !status.IsSuccess() {
		return status
	}
	cycleState.Write(stateKey, state)
	return nil
}

// reInitTopology constructs the full-topology with all nodes to re-calc the min match number of criticalPath
func (p *Plugin) reInitTopology(state *preFilterState, pod *corev1.Pod) *framework.Status {
	var requiredPodConstraintStates []*preFilterStateItem
	for _, stateItem := range state.items {
		stateItem.RLock()
		if len(stateItem.RequiredSpreadConstraints) != 0 {
			requiredPodConstraintStates = append(requiredPodConstraintStates, stateItem)
		}
		stateItem.RUnlock()
	}
	if len(requiredPodConstraintStates) == 0 {
		return nil
	}

	nodeSelector, affinity, status := GetNodeSelectorAndAffinity(p.podConstraintCache, pod)
	if !status.IsSuccess() {
		return status
	}

	allNodes, err := p.handle.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("getting all nodes from Snapshot: %v", err))
	}
	var mutex sync.Mutex
	processNode := func(i int) {
		nodeInfo := allNodes[i]
		node := nodeInfo.Node()
		if node == nil {
			mutex.Lock()
			status = framework.NewStatus(framework.Error, fmt.Sprintf("node %s not found", node.Name))
			mutex.Unlock()
			return
		}
		isMatch, err := MatchesNodeSelectorAndAffinityTerms(nodeSelector, affinity, node)
		if err != nil {
			mutex.Lock()
			status = framework.NewStatus(framework.Error, fmt.Sprintf("affinity match error, err: %v", err))
			mutex.Unlock()
		}
		if !isMatch {
			return
		}
		for _, stateItem := range requiredPodConstraintStates {
			func() {
				stateItem.Lock()
				defer stateItem.Unlock()
				for _, c := range stateItem.RequiredSpreadConstraints {
					tpKey := c.TopologyKey
					tpVal := node.Labels[tpKey]
					if tpVal == "" {
						continue
					}
					pair := TopologyPair{TopologyKey: tpKey, TopologyValue: tpVal}
					if _, ok := stateItem.TpPairToMatchNum[pair]; !ok {
						criticalPaths := stateItem.TpKeyToCriticalPaths[tpKey]
						if criticalPaths == nil {
							criticalPaths = NewTopologyCriticalPaths()
							stateItem.TpKeyToCriticalPaths[tpKey] = criticalPaths
						}
						criticalPaths.Update(tpVal, 0)
					}
				}
			}()
		}
	}
	status = nil
	p.handle.Parallelizer().Until(context.Background(), len(allNodes), processNode)
	return status
}

func (p *Plugin) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (p *Plugin) Filter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	state, status := getPreFilterState(cycleState)
	if !status.IsSuccess() {
		return status
	}
	if len(state.items) == 0 {
		return nil
	}
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	for _, item := range state.items {
		status = p.filterWithPodConstraint(item, node)
		if !status.IsSuccess() {
			return status
		}
	}
	return nil
}

func (p *Plugin) filterWithPodConstraint(s *preFilterStateItem, node *corev1.Node) *framework.Status {
	if s.TopologySpreadConstraintState == nil {
		return nil
	}
	s.RLock()
	defer s.RUnlock()
	if len(s.RequiredSpreadConstraints) == 0 {
		return nil
	}
	for _, c := range s.RequiredSpreadConstraints {
		tpKey := c.TopologyKey
		tpVal := node.Labels[tpKey]
		if tpVal == "" {
			klog.V(5).Infof("[PodConstraint] node '%s' doesn't have required label '%s'", node.Name, tpKey)
			return framework.NewStatus(framework.Unschedulable, ErrReasonNodeLabelNotMatch)
		}

		paths, ok := s.TpKeyToCriticalPaths[tpKey]
		if !ok {
			// error which should not happen
			klog.Errorf("[PodConstraint] internal error: get paths from key %q of %#v", tpKey, s.TpKeyToCriticalPaths)
			continue
		}

		matchNum := 0
		pair := TopologyPair{TopologyKey: tpKey, TopologyValue: tpVal}
		if tpCount, ok := s.TpPairToMatchNum[pair]; ok {
			matchNum = tpCount
		}

		if c.MaxCount > 0 && c.MaxCount <= matchNum {
			return framework.NewStatus(framework.Unschedulable, ErrReasonMaxCountNotMatch)
		}

		minMatchNum := paths.Min.MatchNum
		expectReplicas := math.MaxInt
		tpValNum := 0
		for tpPair := range s.TpPairToMatchNum {
			if tpPair.TopologyKey == tpKey {
				tpValNum++
			}
		}
		if c.MinTopologyValues > 0 && c.MinTopologyValues > tpValNum {
			minMatchNum = 0
			expectReplicas = 0
		}

		skew := 0
		selfMatchNum := 1
		if len(c.TopologyRatios) > 0 {
			ratio, ok := c.TopologyRatios[tpVal]
			if !ok {
				return framework.NewStatus(framework.Unschedulable, ErrReasonTopologyValueNotMatch)
			}
			sumRatio := c.TopologySumRatio
			totalMatchNum := s.TpKeyToTotalMatchNum[tpKey]
			ratioExpectReplicas := totalMatchNum * ratio / sumRatio
			if expectReplicas > ratioExpectReplicas {
				expectReplicas = ratioExpectReplicas
			}
			skew = matchNum + selfMatchNum - expectReplicas
		} else {
			skew = matchNum + selfMatchNum - minMatchNum
		}

		if c.MaxSkew > 0 {
			if skew > c.MaxSkew {
				return framework.NewStatus(framework.Unschedulable, ErrReasonConstraintsNotMatch)
			}
		}
	}
	return nil
}

func (p *Plugin) PreScore(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodes []*corev1.Node) *framework.Status {
	state, status := getPreFilterState(cycleState)
	if !status.IsSuccess() {
		return status
	}
	if len(state.items) == 0 {
		return nil
	}
	for _, v := range state.items {
		status = p.preScoreWithPodConstraint(v, nodes)
		if !status.IsSuccess() {
			return status
		}
	}
	return nil
}

func (p *Plugin) preScoreWithPodConstraint(item *preFilterStateItem, nodes []*corev1.Node) *framework.Status {
	if item.TopologySpreadConstraintState == nil {
		return nil
	}

	// Must copy the constraints from state,
	// otherwise deadlock happened between TopologySpreadConstraintState with NodeInfo
	item.RLock()
	item.SpreadConstraintsInScore = item.copyPreferredSpreadConstraints()
	if len(item.SpreadConstraintsInScore) == 0 {
		item.SpreadConstraintsInScore = item.copyRequiredSpreadConstraints()
	}
	item.TpPairToMatchNumInScore = item.copyTpPairToMatchNum()
	item.RUnlock()

	if len(item.SpreadConstraintsInScore) == 0 {
		return nil
	}

	topologyValNums := make([]int, len(item.SpreadConstraintsInScore))
	topologyPairSets := make(map[TopologyPair]struct{})
	var mutex sync.Mutex
	processNode := func(nodeIndex int) {
		node := nodes[nodeIndex]
		for i, constraint := range item.SpreadConstraintsInScore {
			tpVal := node.Labels[constraint.TopologyKey]
			if tpVal == "" {
				mutex.Lock()
				item.IgnoredNodes.Insert(node.Name)
				mutex.Unlock()
				continue
			}

			// per-node counts are calculated during Score.
			if constraint.TopologyKey == corev1.LabelHostname {
				continue
			}

			pair := TopologyPair{TopologyKey: constraint.TopologyKey, TopologyValue: tpVal}
			mutex.Lock()
			if _, ok := topologyPairSets[pair]; !ok {
				topologyPairSets[pair] = struct{}{}
				topologyValNums[i]++
			}
			mutex.Unlock()
		}
	}
	p.handle.Parallelizer().Until(context.Background(), len(nodes), processNode)

	item.TopologyNormalizingWeight = make([]float64, len(item.SpreadConstraintsInScore))
	for i, c := range item.SpreadConstraintsInScore {
		topologyValNum := topologyValNums[i]
		baseWeight := defaultTopologyWeight[c.TopologyKey]
		if c.TopologyKey == corev1.LabelHostname {
			topologyValNum = len(nodes) - item.IgnoredNodes.Len()
		}
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
	state, status := getPreFilterState(cycleState)
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
		score, status := p.scoreWithPodConstraint(item, node)
		if !status.IsSuccess() {
			return 0, status
		}
		totalScore += score * int64(item.Weight)
	}
	return totalScore, nil
}

func (p *Plugin) scoreWithPodConstraint(item *preFilterStateItem, node *corev1.Node) (int64, *framework.Status) {
	if item.TopologySpreadConstraintState == nil {
		return 0, nil
	}
	if item.IgnoredNodes.Has(node.Name) {
		return 0, nil
	}

	item.RLock()
	defer item.RUnlock()
	if len(item.SpreadConstraintsInScore) == 0 {
		return 0, nil
	}

	var score float64
	for i, constraint := range item.SpreadConstraintsInScore {
		if i > len(item.TopologyNormalizingWeight) {
			break
		}
		if tpVal, ok := node.Labels[constraint.TopologyKey]; ok {
			pair := TopologyPair{TopologyKey: constraint.TopologyKey, TopologyValue: tpVal}
			matchNum := item.TpPairToMatchNumInScore[pair]
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
	state, status := getPreFilterState(cycleState)
	if !status.IsSuccess() {
		return status
	}
	if len(state.items) == 0 {
		return nil
	}

	var maxScore int64
	var minScore int64 = math.MaxInt64
	for _, item := range state.items {
		if item.TopologySpreadConstraintState == nil {
			continue
		}
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
