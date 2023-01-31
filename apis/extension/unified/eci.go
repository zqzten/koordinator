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
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/koordinator-sh/koordinator/pkg/features"
)

const (
	VKTaintKey = "virtual-kubelet.io/provider"
)

func IsVirtualKubeletNode(node *corev1.Node) bool {
	return node.Labels[uniext.LabelNodeType] == uniext.VKType || node.Labels[uniext.LabelCommonNodeType] == uniext.VKType
}

func AffinityECI(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	eciAffinityLabel := pod.Labels[uniext.LabelECIAffinity]
	return eciAffinityLabel == uniext.ECIRequired || eciAffinityLabel == uniext.ECIPreferred || k8sfeature.DefaultFeatureGate.Enabled(features.EnableDefaultECIProfile)
}
