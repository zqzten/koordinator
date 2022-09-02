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
	unischeduling "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
)

// TopologySpreadConstraint
// 对于每个TopologyKey
// 1. 没到MinTopologyValues时尽量让每个TopologyValue都有一个
// 2. 到了之后再按照MaxCount、MaxSkew、TopologyValueToRatios去匹配
type TopologySpreadConstraint struct {
	TopologyKey       string
	MinTopologyValues int
	MaxCount          int
	MaxSkew           int
	TopologyRatios    map[string]int
	TopologySumRatio  int
}

func spreadRulesToTopologySpreadConstraint(spreadRules []unischeduling.SpreadRuleItem) []*TopologySpreadConstraint {
	var constraints []*TopologySpreadConstraint
	uniqueKeys := sets.NewString()
	for _, rule := range spreadRules {
		// remove duplicate topologyKey spread rule
		if uniqueKeys.Has(rule.TopologyKey) {
			continue
		}
		uniqueKeys.Insert(rule.TopologyKey)

		var constraint TopologySpreadConstraint
		constraint.TopologyKey = rule.TopologyKey
		if rule.MaxCount != nil {
			constraint.MaxCount = int(*rule.MaxCount)
		}
		if rule.MinTopologyValue != nil {
			constraint.MinTopologyValues = int(*rule.MinTopologyValue)
		}
		maxSkew := rule.MaxSkew
		if maxSkew <= 0 {
			maxSkew = 1
		}
		constraint.MaxSkew = int(maxSkew)
		if rule.PodSpreadType == unischeduling.PodSpreadTypeRatio {
			sumRatio := 0
			for _, ruleTopologyRatio := range rule.TopologyRatios {
				ratio := ruleTopologyRatio.Ratio
				if ratio == nil {
					ratio = pointer.Int32Ptr(1)
				}
				if constraint.TopologyRatios == nil {
					constraint.TopologyRatios = make(map[string]int)
				}
				constraint.TopologyRatios[ruleTopologyRatio.TopologyValue] = int(*ratio)
				sumRatio += int(*ratio)
			}
			if sumRatio == 0 {
				sumRatio = 1
			}
			constraint.TopologySumRatio = sumRatio
		}
		constraints = append(constraints, &constraint)
	}
	return constraints
}
