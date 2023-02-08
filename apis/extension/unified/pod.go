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

package unified

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	AnnotationVirtualNode = "alibabacloud.com/virtual-node-label"
	AnnotationODPSKata    = "odps-kata"

	LabelODPSKataType = "alibabacloud.com/odps-kata-type"
	LabelPseudoKata   = "pseudo-kata"

	ODPSPodNamespace = "asi-odps"
)

func IsODPSPseudoKataPod(pod *corev1.Pod) bool {
	if pod.Namespace != ODPSPodNamespace {
		return false
	}
	if v, ok := pod.Annotations[AnnotationVirtualNode]; !ok || v != AnnotationODPSKata {
		return false
	}
	if v, ok := pod.Labels[LabelODPSKataType]; !ok || v != LabelPseudoKata {
		return false
	}
	return true
}
