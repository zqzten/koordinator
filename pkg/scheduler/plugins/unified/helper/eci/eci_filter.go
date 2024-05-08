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

package eci

import (
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func FilterByECIAffinity(pod *corev1.Pod, node *corev1.Node) bool {
	podECIAffinity := pod.Labels[uniext.LabelECIAffinity]
	switch podECIAffinity {
	case uniext.ECIRequired:
		return extunified.IsVirtualKubeletNode(node)
	case uniext.ECIRequiredNot:
		return !extunified.IsVirtualKubeletNode(node)
	default:
		return true
	}
}
