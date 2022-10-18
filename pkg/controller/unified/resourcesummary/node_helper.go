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

package resourcesummary

import (
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
)

func IsNodeReady(node corev1.Node) bool {
	nodeReady := false
	for _, nodeCondition := range node.Status.Conditions {
		if nodeCondition.Type != corev1.NodeReady {
			continue
		}
		if nodeCondition.Status == corev1.ConditionTrue {
			nodeReady = true
		}
		break
	}
	return nodeReady
}

// GetRequiredNodeAffinity returns the parsing result of pod's nodeSelector and nodeAffinity.
func GetRequiredNodeAffinity(resourceSummary *v1beta1.ResourceSummary) nodeaffinity.RequiredNodeAffinity {
	return nodeaffinity.GetRequiredNodeAffinity(&corev1.Pod{
		Spec: corev1.PodSpec{Affinity: &corev1.Affinity{NodeAffinity: resourceSummary.Spec.NodeAffinity}},
	})
}
