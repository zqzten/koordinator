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

package nodeaffinity

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/eci"
)

type RequiredNodeSelectorAndAffinity struct {
	pod                     *corev1.Pod
	requiredNodeAffinity    nodeaffinity.RequiredNodeAffinity
	requiredECINodeAffinity nodeaffinity.RequiredNodeAffinity
}

func (r RequiredNodeSelectorAndAffinity) Match(node *corev1.Node) bool {
	if extunified.AffinityECI(r.pod) && extunified.IsVirtualKubeletNode(node) {
		match, _ := r.requiredECINodeAffinity.Match(node)
		return match
	}
	match, _ := r.requiredNodeAffinity.Match(node)
	return match
}

func GetRequiredNodeAffinity(pod *corev1.Pod) RequiredNodeSelectorAndAffinity {
	requiredNodeAffinity := nodeaffinity.GetRequiredNodeAffinity(pod)
	var requiredECINodeAffinity nodeaffinity.RequiredNodeAffinity
	if len(eci.DefaultECIProfile.AllowedAffinityKeys) > 0 {
		requiredECINodeAffinity = GetRequiredECINodeAffinity(pod)
	}
	return RequiredNodeSelectorAndAffinity{
		pod:                     pod,
		requiredNodeAffinity:    requiredNodeAffinity,
		requiredECINodeAffinity: requiredECINodeAffinity,
	}
}

func GetRequiredECINodeAffinity(pod *corev1.Pod) nodeaffinity.RequiredNodeAffinity {
	modifiedPod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Affinity:     pod.Spec.Affinity.DeepCopy(),
			NodeSelector: pod.Spec.NodeSelector,
		},
	}
	if len(modifiedPod.Spec.NodeSelector) > 0 {
		modifiedPod.Spec.NodeSelector = getECINodeSelector(modifiedPod.Spec.NodeSelector, eci.DefaultECIProfile.AllowedAffinityKeys)
	}
	if modifiedPod.Spec.Affinity != nil &&
		modifiedPod.Spec.Affinity.NodeAffinity != nil &&
		modifiedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		required := modifiedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		modifiedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = getRequiredECINodeSelector(required, eci.DefaultECIProfile.AllowedAffinityKeys)
	}
	return nodeaffinity.GetRequiredNodeAffinity(modifiedPod)
}

func getECINodeSelector(nodeSelector map[string]string, allowedAffinityKeys []string) map[string]string {
	modifiedNodeSelector := map[string]string{}
	for k, v := range nodeSelector {
		if isAllowedAffinityKey(k, allowedAffinityKeys) {
			modifiedNodeSelector[k] = v
		}
	}
	return modifiedNodeSelector
}

func getRequiredECINodeSelector(required *corev1.NodeSelector, allowedAffinityKeys []string) *corev1.NodeSelector {
	if matchNothing(required) {
		return &corev1.NodeSelector{}
	}
	var terms []corev1.NodeSelectorTerm
	for _, term := range required.NodeSelectorTerms {
		if isEmptyNodeSelectorTerm(&term) {
			continue
		}
		expressions := make([]corev1.NodeSelectorRequirement, 0)
		for _, expression := range term.MatchExpressions {
			if isAllowedAffinityKey(expression.Key, allowedAffinityKeys) {
				expressions = append(expressions, expression)
			}
		}
		term.MatchExpressions = expressions
		if !isEmptyNodeSelectorTerm(&term) {
			terms = append(terms, term)
		}
	}
	if len(terms) > 0 {
		return &corev1.NodeSelector{NodeSelectorTerms: terms}
	}
	return nil
}

func matchNothing(required *corev1.NodeSelector) bool {
	if len(required.NodeSelectorTerms) == 0 {
		return true
	}
	allEmpty := true
	for _, term := range required.NodeSelectorTerms {
		if !isEmptyNodeSelectorTerm(&term) {
			allEmpty = false
			break
		}
	}
	return allEmpty
}

func isEmptyNodeSelectorTerm(term *corev1.NodeSelectorTerm) bool {
	return len(term.MatchExpressions) == 0 && len(term.MatchFields) == 0
}

func isAllowedAffinityKey(key string, allowedAffinityKeys []string) bool {
	for _, v := range allowedAffinityKeys {
		if v == key {
			return true
		}
	}
	return false
}
