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

package cache

import (
	unischeduling "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	v1helper "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/utils/pointer"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/eci"
	nodeaffinityhelper "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/nodeaffinity"
	tainttolerationhelper "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/tainttoleration"
)

// TopologySpreadConstraint
// 对于每个TopologyKey
// 1. 没到MinTopologyValues时尽量让每个TopologyValue都有一个
// 2. 到了之后再按照MaxCount、MaxSkew、TopologyValueToRatios去匹配
type TopologySpreadConstraint struct {
	TopologyKey        string                            `json:"topologyKey,omitempty"`
	MinTopologyValues  int                               `json:"minTopologyValues,omitempty"`
	MaxCount           int                               `json:"maxCount,omitempty"`
	MaxSkew            int                               `json:"maxSkew,omitempty"`
	TopologyRatios     map[string]int                    `json:"topologyRatios,omitempty"`
	TopologySumRatio   int                               `json:"topologySumRatio,omitempty"`
	MatchLabelKeys     []string                          `json:"matchLabelKeys,omitempty"`
	Selector           labels.Selector                   `json:"selector,omitempty"`
	NodeAffinityPolicy unischeduling.NodeInclusionPolicy `json:"nodeAffinityPolicy,omitempty"`
	NodeTaintsPolicy   unischeduling.NodeInclusionPolicy `json:"nodeTaintsPolicy,omitempty"`
}

func (tsc *TopologySpreadConstraint) MatchNodeInclusionPolicies(pod *corev1.Pod, node *corev1.Node, requiredNodeAffinity nodeaffinityhelper.RequiredNodeSelectorAndAffinity) bool {
	if tsc.NodeAffinityPolicy == unischeduling.NodeInclusionPolicyHonor {
		isMatch := requiredNodeAffinity.Match(node)
		if !isMatch {
			return false
		}
	}
	if tsc.NodeTaintsPolicy == unischeduling.NodeInclusionPolicyHonor {
		tolerations := pod.Spec.Tolerations
		if extunified.AffinityECI(pod) && extunified.IsVirtualKubeletNode(node) {
			tolerations = tainttolerationhelper.TolerationsToleratesECI(pod)
		}
		_, isUnTolerated := v1helper.FindMatchingUntoleratedTaint(node.Spec.Taints, tolerations, func(t *corev1.Taint) bool {
			// PodToleratesNodeTaints is only interested in NoSchedule and NoExecute taints.
			return t.Effect == corev1.TaintEffectNoSchedule || t.Effect == corev1.TaintEffectNoExecute
		})
		if isUnTolerated {
			return false
		}
	}
	return eci.FilterByECIAffinity(pod, node)
}

func SpreadRulesToTopologySpreadConstraint(spreadRules []unischeduling.SpreadRuleItem) []*TopologySpreadConstraint {
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
		constraint.NodeAffinityPolicy = unischeduling.NodeInclusionPolicyHonor
		if rule.NodeAffinityPolicy != nil {
			constraint.NodeAffinityPolicy = *rule.NodeAffinityPolicy
		}
		constraint.NodeTaintsPolicy = unischeduling.NodeInclusionPolicyIgnore
		if k8sfeature.DefaultFeatureGate.Enabled(features.DefaultHonorTaintTolerationInPodConstraint) {
			constraint.NodeTaintsPolicy = unischeduling.NodeInclusionPolicyHonor
		}
		if rule.NodeTaintsPolicy != nil {
			constraint.NodeTaintsPolicy = *rule.NodeTaintsPolicy
		}
		constraint.MatchLabelKeys = rule.MatchLabelKeys
		constraints = append(constraints, &constraint)
	}
	return constraints
}
