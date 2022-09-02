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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func GetNodeSelectorAndAffinity(podConstraintCache *PodConstraintCache, pod *corev1.Pod) (map[string]string, *corev1.Affinity, *framework.Status) {
	var nodeSelectorTerms []corev1.NodeSelectorTerm
	var preferredSchedulingTerms []corev1.PreferredSchedulingTerm
	nodeSelector := make(map[string]string)
	emptyNodeSelectTerms := false
	for k, v := range pod.Spec.NodeSelector {
		nodeSelector[k] = v
	}
	if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil {
		nodeAffinity := pod.Spec.Affinity.NodeAffinity
		if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
			emptyNodeSelectTerms = len(nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0
			nodeSelectorTerms = make([]corev1.NodeSelectorTerm, len(nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms))
			copy(nodeSelectorTerms, nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)
		}
		if len(nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
			preferredSchedulingTerms = make([]corev1.PreferredSchedulingTerm, len(nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution))
			copy(preferredSchedulingTerms, nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution)
		}
	}

	constraintKey := extunified.GetPodConstraintKey(pod)
	if constraintKey != "" {
		constraintState := podConstraintCache.GetState(constraintKey)
		if constraintState == nil {
			return nil, nil, framework.NewStatus(framework.Error, ErrMissPodConstraint)
		}
		affinity := constraintState.PodConstraint.Spec.Affinity
		affinityOrder := constraintState.PodConstraint.Spec.NodeAffinityLogicOrder
		if affinity != nil && affinity.NodeAffinity != nil {
			nodeAffinity := affinity.NodeAffinity
			if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
				emptyNodeSelectTerms = len(nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0
				nodeSelectorTerms = mergeRequiredNodeAffinity(nodeSelectorTerms, nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms, affinityOrder)
			}
			if len(nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
				preferredSchedulingTerms = append(preferredSchedulingTerms, nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution...)
			}
		}
		for k, v := range constraintState.PodConstraint.Spec.NodeSelector {
			nodeSelector[k] = v
		}
	}

	affinity := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: preferredSchedulingTerms,
		},
	}
	if len(nodeSelectorTerms) > 0 {
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: nodeSelectorTerms,
		}
	} else if emptyNodeSelectTerms {
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: nil,
		}
	}
	return nodeSelector, affinity, nil
}

func mergeRequiredNodeAffinity(fromPod []corev1.NodeSelectorTerm, fromConstraint []corev1.NodeSelectorTerm, order unischeduling.LogicOrder) []corev1.NodeSelectorTerm {
	if order == unischeduling.AND {
		for index, term := range fromPod {
			for _, toAdd := range fromConstraint {
				fromPod[index].MatchExpressions = append(term.MatchExpressions, toAdd.MatchExpressions...)
				fromPod[index].MatchFields = append(term.MatchFields, toAdd.MatchFields...)
			}
		}
		if len(fromPod) == 0 {
			fromPod = append(fromPod, fromConstraint...)
		}
	} else {
		fromPod = append(fromPod, fromConstraint...)
	}
	return fromPod
}

func MatchesNodeSelectorAndAffinityTerms(nodeSelector map[string]string, affinity *corev1.Affinity, node *corev1.Node) (bool, error) {
	if len(nodeSelector) > 0 {
		selector := labels.SelectorFromSet(nodeSelector)
		if !selector.Matches(labels.Set(node.Labels)) {
			return false, nil
		}
	}

	return nodeaffinity.GetRequiredNodeAffinity(&corev1.Pod{
		Spec: corev1.PodSpec{Affinity: affinity},
	}).Match(node)
}
