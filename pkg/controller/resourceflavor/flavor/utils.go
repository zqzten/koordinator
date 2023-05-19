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

package flavor

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

var GpuModel = extension.LabelGPUModel
var PointOfDelivery = unified.LabelNodePointOfDelivery
var ASWID = unified.LabelNodeASWID
var ForceWanted = extension.NodeDomainPrefix + "/quota-node-flavor-force-wanted"
var ForceWantedType = extension.NodeDomainPrefix + "/quota-node-flavor-force-wanted-type"

var GetNodeGpuModel = func(node *corev1.Node) string {
	if node != nil && node.Labels != nil {
		return node.Labels[GpuModel]
	}

	return ""
}

var GetNodePointOfDelivery = func(node *corev1.Node) string {
	if node != nil && node.Labels != nil {
		return node.Labels[PointOfDelivery]
	}

	return ""
}

var GetNodeASW = func(node *corev1.Node) string {
	if node != nil && node.Labels != nil {
		return node.Labels[ASWID]
	}

	return ""
}

func GetNodeAffinity(nodeAffinity *corev1.NodeAffinity) *corev1.Affinity {
	var nodeSelectorTerms []corev1.NodeSelectorTerm

	if nodeAffinity != nil {
		if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
			nodeSelectorTerms = make([]corev1.NodeSelectorTerm, len(nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms))
			copy(nodeSelectorTerms, nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)
		}
	}

	affinity := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{},
	}
	if len(nodeSelectorTerms) > 0 {
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: nodeSelectorTerms,
		}
	} else {
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: nil,
		}
	}
	return affinity
}

func MatchesNodeSelectorAndAffinityTerms(affinity *corev1.Affinity, node *corev1.Node) (bool, error) {
	return nodeaffinity.GetRequiredNodeAffinity(&corev1.Pod{
		Spec: corev1.PodSpec{Affinity: affinity},
	}).Match(node)
}
