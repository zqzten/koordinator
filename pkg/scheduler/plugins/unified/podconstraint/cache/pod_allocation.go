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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type podAllocation struct {
	namespace   string
	name        string
	labels      map[string]string
	annotations map[string]string
	nodeName    string
}

func allocationToPod(allocation *podAllocation) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        allocation.name,
			Namespace:   allocation.namespace,
			Labels:      allocation.labels,
			Annotations: allocation.annotations,
		},
		Spec: corev1.PodSpec{NodeName: allocation.nodeName},
	}
}

func podToAllocation(pod *corev1.Pod) *podAllocation {
	return &podAllocation{
		namespace:   pod.Namespace,
		name:        pod.Name,
		labels:      pod.Labels,
		annotations: pod.Annotations,
		nodeName:    pod.Spec.NodeName,
	}
}
