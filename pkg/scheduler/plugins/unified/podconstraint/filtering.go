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
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	nodeaffinityhelper "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/nodeaffinity"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/podconstraint/cache"
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
	// ErrReasonTopologyValueNotMatch is used when the node topology value is not in TopologyRatios
	ErrReasonTopologyValueNotMatch = ErrReasonConstraintsNotMatch + " (topologyRatios)"
)

const (
	preFilterStateKey = "PreFilter" + Name
)

type preFilterState struct {
	items []*preFilterStateItem
}

type preFilterStateItem struct {
	PodConstraint             *unischeduling.PodConstraint
	RequiredSpreadConstraints []*cache.TopologySpreadConstraint
	// TpPairToMatchNum is keyed with topologyPair, and valued with the number of matching pods.
	TpPairToMatchNum     map[cache.TopologyPair]*int32
	TpKeyToTotalMatchNum map[string]*int32
	TpKeyToCriticalPaths map[string]*cache.TopologyCriticalPaths
}

func (s *preFilterState) Clone() framework.StateData {
	if s == nil {
		return nil
	}
	var clonedPreFilterState = &preFilterState{}
	for _, item := range s.items {
		cloneItem := &preFilterStateItem{
			PodConstraint:             item.PodConstraint,
			RequiredSpreadConstraints: make([]*cache.TopologySpreadConstraint, len(item.RequiredSpreadConstraints)),
			TpPairToMatchNum:          map[cache.TopologyPair]*int32{},
			TpKeyToTotalMatchNum:      map[string]*int32{},
			TpKeyToCriticalPaths:      map[string]*cache.TopologyCriticalPaths{},
		}
		copy(cloneItem.RequiredSpreadConstraints, item.RequiredSpreadConstraints)
		for tpPair, matchNum := range item.TpPairToMatchNum {
			copyCount := *matchNum
			cloneItem.TpPairToMatchNum[cache.TopologyPair{TopologyKey: tpPair.TopologyKey, TopologyValue: tpPair.TopologyValue}] = &copyCount
		}
		for tpKey, paths := range item.TpKeyToCriticalPaths {
			cloneItem.TpKeyToCriticalPaths[tpKey] = &cache.TopologyCriticalPaths{Min: paths.Min, Max: paths.Max}
		}
		for tpKey, matchNum := range item.TpKeyToTotalMatchNum {
			copyCount := *matchNum
			cloneItem.TpKeyToTotalMatchNum[tpKey] = &copyCount
		}
		clonedPreFilterState.items = append(clonedPreFilterState.items, cloneItem)
	}
	return clonedPreFilterState
}

func (s *preFilterState) updateWithPod(updatedPod, preemptorPod *corev1.Pod, node *corev1.Node, delta int32) {
	if s == nil || updatedPod.Namespace != preemptorPod.Namespace || node == nil {
		return
	}
	for _, item := range s.items {
		if !podHasConstraint(updatedPod, item.PodConstraint.Namespace, item.PodConstraint.Name) {
			continue
		}
		if !cache.NodeLabelsMatchSpreadConstraints(node.Labels, item.RequiredSpreadConstraints) {
			return
		}
		for _, constraint := range item.RequiredSpreadConstraints {
			k, v := constraint.TopologyKey, node.Labels[constraint.TopologyKey]
			pair := cache.TopologyPair{TopologyKey: k, TopologyValue: v}
			*item.TpPairToMatchNum[pair] += delta
			*item.TpKeyToTotalMatchNum[k] += delta

			item.TpKeyToCriticalPaths[k].Update(v, int(*item.TpPairToMatchNum[pair]))
		}
	}
}

func getPreFilterState(cycleState *framework.CycleState) (*preFilterState, *framework.Status) {
	value, err := cycleState.Read(preFilterStateKey)
	if err != nil {
		return nil, framework.AsStatus(err)
	}
	state := value.(*preFilterState)
	return state, nil
}

func (p *Plugin) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	state, status := p.calPrefilterState(cycleState, pod)
	if !status.IsSuccess() {
		return nil, status
	}
	cycleState.Write(preFilterStateKey, state)
	return nil, nil
}

// calPrefilterState constructs the full-topology with all nodes to re-calc the min match number of criticalPath
func (p *Plugin) calPrefilterState(cycleState *framework.CycleState, pod *corev1.Pod) (*preFilterState, *framework.Status) {
	state := &preFilterState{}
	if weightedPodConstraints := extunified.GetWeightedPodConstraints(pod); len(weightedPodConstraints) != 0 {
		for _, weightedPodConstraint := range weightedPodConstraints {
			constraintState := p.podConstraintCache.GetState(cache.GetNamespacedName(weightedPodConstraint.Namespace, weightedPodConstraint.Name))
			if constraintState == nil {
				return nil, framework.NewStatus(framework.Error, ErrMissPodConstraint)
			}
			constraintState.RLock()
			constraint := constraintState.PodConstraint
			requiredPodConstraints := constraintState.CopyRequiredSpreadConstraints()
			constraintState.RUnlock()
			if len(requiredPodConstraints) == 0 {
				continue
			}
			fillSelectorByMatchLabels(pod, requiredPodConstraints)
			state.items = append(state.items, &preFilterStateItem{
				PodConstraint:             constraint,
				RequiredSpreadConstraints: requiredPodConstraints,
				TpPairToMatchNum:          make(map[cache.TopologyPair]*int32),
				TpKeyToTotalMatchNum:      make(map[string]*int32),
				TpKeyToCriticalPaths:      make(map[string]*cache.TopologyCriticalPaths),
			})
		}
	} else if weightedSpreadUnits := extunified.GetWeighedSpreadUnits(pod); len(weightedSpreadUnits) != 0 {
		for _, weightedSpreadUnit := range weightedSpreadUnits {
			if weightedSpreadUnit.Name == "" {
				continue
			}
			defaultPodConstraintName := cache.GetDefaultPodConstraintName(weightedSpreadUnit.Name)
			spreadRequiredType := extunified.IsSpreadTypeRequire(pod)
			if !spreadRequiredType {
				continue
			}
			constraint := cache.BuildDefaultPodConstraint(pod.Namespace, defaultPodConstraintName, spreadRequiredType)
			requiredPodConstraints := cache.SpreadRulesToTopologySpreadConstraint(constraint.Spec.SpreadRule.Requires)
			fillSelectorByMatchLabels(pod, requiredPodConstraints)
			state.items = append(state.items, &preFilterStateItem{
				PodConstraint:             constraint,
				RequiredSpreadConstraints: requiredPodConstraints,
				TpPairToMatchNum:          make(map[cache.TopologyPair]*int32),
				TpKeyToTotalMatchNum:      make(map[string]*int32),
				TpKeyToCriticalPaths:      make(map[string]*cache.TopologyCriticalPaths),
			})
		}
	}
	if len(state.items) == 0 {
		return &preFilterState{}, nil
	}

	allNodes, err := p.handle.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return nil, framework.NewStatus(framework.Error, fmt.Sprintf("getting all nodes from Snapshot: %v", err))
	}
	nodeSelector, affinity, status := GetNodeSelectorAndAffinity(p.podConstraintCache, pod)
	if !status.IsSuccess() {
		return nil, status
	}
	requiredNodeAffinity := nodeaffinityhelper.GetRequiredNodeAffinity(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: pod.Labels}, Spec: corev1.PodSpec{Affinity: affinity, NodeSelector: nodeSelector}})
	for _, stateItem := range state.items {
		for _, c := range stateItem.RequiredSpreadConstraints {
			stateItem.TpKeyToTotalMatchNum[c.TopologyKey] = new(int32)
		}
	}
	temporaryNodeAffinity := nodeaffinityhelper.GetTemporaryNodeAffinity(cycleState)

	for _, n := range allNodes {
		node := n.Node()
		if node == nil {
			continue
		}
		for _, stateItem := range state.items {
			func() {
				if !cache.NodeLabelsMatchSpreadConstraints(node.Labels, stateItem.RequiredSpreadConstraints) {
					return
				}
				for _, c := range stateItem.RequiredSpreadConstraints {
					tpKey := c.TopologyKey
					tpVal := node.Labels[tpKey]
					if tpVal == "" {
						continue
					}
					if p.enableNodeInclusionPolicyInPodConstraint && !c.MatchNodeInclusionPolicies(pod, node, requiredNodeAffinity, temporaryNodeAffinity) {
						return
					}
					pair := cache.TopologyPair{TopologyKey: c.TopologyKey, TopologyValue: node.Labels[c.TopologyKey]}
					stateItem.TpPairToMatchNum[pair] = new(int32)
				}
			}()
		}
	}
	processNode := func(i int) {
		nodeInfo := allNodes[i]
		node := nodeInfo.Node()
		if node == nil {
			return
		}
		for _, stateItem := range state.items {
			for _, c := range stateItem.RequiredSpreadConstraints {
				tpKey := c.TopologyKey
				tpVal := node.Labels[tpKey]
				if tpVal == "" {
					continue
				}
				totalCount := stateItem.TpKeyToTotalMatchNum[tpKey]
				if totalCount == nil {
					continue
				}
				count := countPodsMatchConstraint(nodeInfo.Pods, stateItem.PodConstraint.Namespace, stateItem.PodConstraint.Name, c.Selector)
				atomic.AddInt32(totalCount, int32(count))
				pair := cache.TopologyPair{TopologyKey: tpKey, TopologyValue: tpVal}
				tpCount := stateItem.TpPairToMatchNum[pair]
				if tpCount == nil {
					continue
				}
				atomic.AddInt32(tpCount, int32(count))
			}
		}
	}
	p.handle.Parallelizer().Until(context.Background(), len(allNodes), processNode)
	for _, stateItem := range state.items {
		func() {
			for _, c := range stateItem.RequiredSpreadConstraints {
				stateItem.TpKeyToCriticalPaths[c.TopologyKey] = cache.NewTopologyCriticalPaths()
			}
			for pair, num := range stateItem.TpPairToMatchNum {
				stateItem.TpKeyToCriticalPaths[pair.TopologyKey].Update(pair.TopologyValue, int(*num))
			}
		}()
	}
	return state, nil
}

func (p *Plugin) PreFilterExtensions() framework.PreFilterExtensions {
	return p
}

func (p *Plugin) AddPod(ctx context.Context, state *framework.CycleState, podToSchedule *corev1.Pod, podInfoToAdd *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	s, status := getPreFilterState(state)
	if !status.IsSuccess() {
		return status
	}
	s.updateWithPod(podInfoToAdd.Pod, podToSchedule, nodeInfo.Node(), 1)
	return nil
}

func (p *Plugin) RemovePod(ctx context.Context, state *framework.CycleState, podToSchedule *corev1.Pod, podInfoToRemove *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	s, status := getPreFilterState(state)
	if !status.IsSuccess() {
		return status
	}
	s.updateWithPod(podInfoToRemove.Pod, podToSchedule, nodeInfo.Node(), 1)
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
	if len(s.RequiredSpreadConstraints) == 0 {
		return nil
	}
	for _, c := range s.RequiredSpreadConstraints {
		tpKey := c.TopologyKey
		if extunified.IsVirtualKubeletNode(node) && tpKey == corev1.LabelHostname {
			continue
		}
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
		pair := cache.TopologyPair{TopologyKey: tpKey, TopologyValue: tpVal}
		if tpCount := s.TpPairToMatchNum[pair]; tpCount != nil {
			matchNum = int(*tpCount)
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
			var totalMatchNum int
			if s.TpKeyToTotalMatchNum[tpKey] == nil {
				totalMatchNum = 0
			} else {
				totalMatchNum = int(*s.TpKeyToTotalMatchNum[tpKey])
			}
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
