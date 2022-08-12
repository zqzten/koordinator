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

	"github.com/koordinator-sh/koordinator/apis/extension"
)

func GetPodQoSClass(pod *corev1.Pod) extension.QoSClass {
	if pod == nil || pod.Labels == nil {
		return extension.QoSNone
	}
	if _, ok := pod.Labels[extension.LabelPodQoS]; ok {
		return extension.GetPodQoSClass(pod)
	}
	if qos, ok := pod.Labels[LabelPodQoSClass]; ok {
		return getPodQoSClassByName(qos)
	}
	// If pod has not  qos label, default as none-qos pod.
	return extension.QoSNone
}
func getPodQoSClassByName(qos string) extension.QoSClass {
	q := extension.QoSClass(qos)

	switch q {
	case extension.QoSLSE, extension.QoSLSR, extension.QoSLS, extension.QoSBE, extension.QoSSystem:
		return q
	}

	return extension.QoSNone
}
