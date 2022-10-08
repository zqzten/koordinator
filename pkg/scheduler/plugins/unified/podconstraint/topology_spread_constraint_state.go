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
	"fmt"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	unischeduling "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
)

type TopologySpreadConstraintState struct {
	sync.RWMutex
	PodConstraint              *unischeduling.PodConstraint
	DefaultPodConstraint       bool
	SpreadTypeRequired         bool
	RequiredSpreadConstraints  []*TopologySpreadConstraint
	PreferredSpreadConstraints []*TopologySpreadConstraint
	// TpPairToMatchNum is keyed with topologyPair, and valued with the number of matching pods.
	TpPairToMatchNum     map[TopologyPair]int
	TpKeyToTotalMatchNum map[string]int
	TpKeyToCriticalPaths map[string]*TopologyCriticalPaths
}

func newTopologySpreadConstraintState(constraint *unischeduling.PodConstraint, spreadTypeRequired *bool) *TopologySpreadConstraintState {
	s := &TopologySpreadConstraintState{
		PodConstraint:        constraint,
		DefaultPodConstraint: IsPodConstraintDefault(constraint),
		TpKeyToCriticalPaths: make(map[string]*TopologyCriticalPaths),
		TpPairToMatchNum:     make(map[TopologyPair]int),
		TpKeyToTotalMatchNum: make(map[string]int),
	}
	s.updateWithPodConstraint(constraint, spreadTypeRequired)
	return s
}

type TopologyPair struct {
	TopologyKey   string
	TopologyValue string
}

func (p TopologyPair) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%s/%s", p.TopologyKey, p.TopologyValue)), nil
}

func (p *TopologyPair) UnmarshalText(text []byte) error {
	str := string(text)
	sepIndex := strings.LastIndex(str, "/")
	if sepIndex < 1 || sepIndex > len(str)-2 {
		return fmt.Errorf("want TopologyKey/TopologyValue, got %s", str)
	}
	p.TopologyKey = str[0:sepIndex]
	p.TopologyValue = str[sepIndex+1:]
	return nil
}

func (s *TopologySpreadConstraintState) copyRequiredSpreadConstraints() []*TopologySpreadConstraint {
	if len(s.RequiredSpreadConstraints) == 0 {
		return nil
	}
	spreadConstraints := make([]*TopologySpreadConstraint, len(s.RequiredSpreadConstraints))
	copy(spreadConstraints, s.RequiredSpreadConstraints)
	return spreadConstraints
}

func (s *TopologySpreadConstraintState) copyPreferredSpreadConstraints() []*TopologySpreadConstraint {
	if len(s.PreferredSpreadConstraints) == 0 {
		return nil
	}
	spreadConstraints := make([]*TopologySpreadConstraint, len(s.PreferredSpreadConstraints))
	copy(spreadConstraints, s.PreferredSpreadConstraints)
	return spreadConstraints
}

func (s *TopologySpreadConstraintState) copyTpPairToMatchNum() map[TopologyPair]int {
	if s.TpPairToMatchNum == nil {
		return nil
	}
	tpPairToMatchNum := map[TopologyPair]int{}
	for tpPair, matchNum := range s.TpPairToMatchNum {
		tpPairToMatchNum[tpPair] = matchNum
	}
	return tpPairToMatchNum
}

func (s *TopologySpreadConstraintState) copyTpKeyToTotalMatchNum() map[string]int {
	if s.TpKeyToTotalMatchNum == nil {
		return nil
	}
	tpKeyToTotalMatchNum := map[string]int{}
	for tpKey, totalMatchNum := range s.TpKeyToTotalMatchNum {
		tpKeyToTotalMatchNum[tpKey] = totalMatchNum
	}
	return tpKeyToTotalMatchNum
}

func (s *TopologySpreadConstraintState) copyTpKeyToCriticalPath() map[string]*TopologyCriticalPaths {
	if s.TpKeyToCriticalPaths == nil {
		return nil
	}
	TpKeyToCriticalPath := map[string]*TopologyCriticalPaths{}
	for tpKey, criticalPaths := range s.TpKeyToCriticalPaths {
		TpKeyToCriticalPath[tpKey] = &TopologyCriticalPaths{
			Min: CriticalPath{
				criticalPaths.Min.TopologyValue,
				criticalPaths.Min.MatchNum,
			},
			Max: CriticalPath{
				criticalPaths.Max.TopologyValue,
				criticalPaths.Max.MatchNum,
			},
		}
	}
	return TpKeyToCriticalPath
}

func (s *TopologySpreadConstraintState) updateWithPodConstraint(constraint *unischeduling.PodConstraint, spreadTypeRequired *bool) (hasNewTopologyKey bool) {
	s.Lock()
	defer s.Unlock()
	constraintKey := getNamespacedName(constraint.Namespace, constraint.Name)
	if constraintKey != getNamespacedName(s.PodConstraint.Namespace, s.PodConstraint.Name) {
		return
	}
	if s.DefaultPodConstraint && spreadTypeRequired != nil && *spreadTypeRequired != s.SpreadTypeRequired {
		s.SpreadTypeRequired = *spreadTypeRequired
	}
	requiredSpreadConstraints := spreadRulesToTopologySpreadConstraint(constraint.Spec.SpreadRule.Requires)
	preferredSpreadConstraints := spreadRulesToTopologySpreadConstraint(constraint.Spec.SpreadRule.Affinities)

	topologyKeys := sets.NewString()
	for _, constraints := range [][]*TopologySpreadConstraint{requiredSpreadConstraints, preferredSpreadConstraints} {
		for _, spreadConstraint := range constraints {
			topologyKeys.Insert(spreadConstraint.TopologyKey)
			_, ok := s.TpKeyToCriticalPaths[spreadConstraint.TopologyKey]
			if !ok {
				hasNewTopologyKey = true
				s.TpKeyToCriticalPaths[spreadConstraint.TopologyKey] = NewTopologyCriticalPaths()
			}
		}
	}

	invalidTopologyKeys := sets.NewString()
	for _, constraints := range [][]*TopologySpreadConstraint{s.RequiredSpreadConstraints, s.PreferredSpreadConstraints} {
		for _, spreadConstraint := range constraints {
			if !topologyKeys.Has(spreadConstraint.TopologyKey) {
				invalidTopologyKeys.Insert(spreadConstraint.TopologyKey)
			}
		}
	}
	for topologyKey := range invalidTopologyKeys {
		delete(s.TpKeyToCriticalPaths, topologyKey)
		delete(s.TpKeyToTotalMatchNum, topologyKey)
		for pair := range s.TpPairToMatchNum {
			if pair.TopologyKey == topologyKey {
				delete(s.TpPairToMatchNum, pair)
			}
		}
	}
	s.PodConstraint = constraint
	s.RequiredSpreadConstraints = requiredSpreadConstraints
	s.PreferredSpreadConstraints = preferredSpreadConstraints
	return
}

func (s *TopologySpreadConstraintState) update(node *corev1.Node, podReplicasDelta int) {
	if !(nodeLabelsMatchSpreadConstraints(node.Labels, s.RequiredSpreadConstraints) &&
		nodeLabelsMatchSpreadConstraints(node.Labels, s.PreferredSpreadConstraints)) {
		return
	}
	topologyKeySet := sets.NewString()
	for _, constraints := range [][]*TopologySpreadConstraint{s.RequiredSpreadConstraints, s.PreferredSpreadConstraints} {
		for _, constraint := range constraints {
			if topologyKeySet.Has(constraint.TopologyKey) {
				continue
			}
			topologyKeySet.Insert(constraint.TopologyKey)

			topologyPair := TopologyPair{TopologyKey: constraint.TopologyKey, TopologyValue: node.Labels[constraint.TopologyKey]}
			matchNum := s.TpPairToMatchNum[topologyPair]
			matchNum += podReplicasDelta
			if matchNum <= 0 {
				delete(s.TpPairToMatchNum, topologyPair)
			} else {
				s.TpPairToMatchNum[topologyPair] = matchNum
			}
			matchNum = s.TpKeyToTotalMatchNum[constraint.TopologyKey]
			matchNum += podReplicasDelta
			if matchNum <= 0 {
				delete(s.TpKeyToTotalMatchNum, constraint.TopologyKey)
				delete(s.TpKeyToCriticalPaths, constraint.TopologyKey)
			} else {
				s.TpKeyToTotalMatchNum[constraint.TopologyKey] = matchNum
				if _, ok := s.TpKeyToCriticalPaths[constraint.TopologyKey]; !ok {
					s.TpKeyToCriticalPaths[constraint.TopologyKey] = NewTopologyCriticalPaths()
				}
			}
		}
	}

	for pair, matchNum := range s.TpPairToMatchNum {
		topologyCriticalPaths := s.TpKeyToCriticalPaths[pair.TopologyKey]
		if topologyCriticalPaths != nil {
			topologyCriticalPaths.Update(pair.TopologyValue, matchNum)
		}
	}
}

func nodeLabelsMatchSpreadConstraints(nodeLabels map[string]string, constraints []*TopologySpreadConstraint) bool {
	// if node has all topology constraint key, return true
	for _, c := range constraints {
		if _, ok := nodeLabels[c.TopologyKey]; !ok {
			return false
		}
	}
	return true
}
