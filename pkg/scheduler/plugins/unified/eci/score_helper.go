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
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// UnifiedECI 打分逻辑
// 1. 分数主要评估 Pod 请求使用的 ECIType 和节点的匹配程度，匹配程度高的分数高, 有如下基本假设
//	对于 VkNode 来说，Pod ECI Affinity ECIRequired 的语义要强于 ECI Preferred，所以对应的分数也要高；
//	对于 ECI Affinity 为 ECIPreferred 的 Pod，VKNode 的匹配程度要好于 NormalNode；
//	对于 ECI Affinity 为 ECIPreferredNot 的 Pod，NormalNode 的匹配程度要大于 VKNode；
//	对于 ECI 不感知的 Pod，应先尽量使用 NormalNode，所以 NormalNode 分数要大于 VkNode；
//	对于 VkNode来说，Pod ECIPreferredNot 比 ECI 无感知分数要高（ECI PreferredNot）
//	对于 NormalNode 来说，ECIPreferred、ECIRequiredNot、ECIPreferredNot、ECI无感知的分数是一样的
// 2. 基于以上基本假设，用score(eciAffinity, nodeType)表示具有eciAffinity的Pod和对应nodeType的Node的分数的话，score可排序如下：
//	score(ECIRequired, VKNode) > score(ECIPreferred, VKNode) > score(ECIPreferred, NormalNode) =
//	score(ECIRequiredNot, NormalNode) = score(ECI无感知，normalNode)）= score(ECIPreferredNot, NormalNode) >
//	score(ECIPreferredNot, VKNode) > score(ECI无感知, VKNode)
// 3. 由2可知，分数分为5档，为了使得插件之间可以完全通过权重来控制插件分数重要性，
//	这里采用标准最高分MaxNodeScore=100和标准最低分MinNodeScore=0来作为插件打分的最高分和最低分，中间三档分别给75，50，25

func ScoreVKNode(podLabels map[string]string) int64 {
	requestUseECIType := podLabels[uniext.LabelECIAffinity]
	switch requestUseECIType {
	case uniext.ECIRequired:
		return framework.MaxNodeScore
	case uniext.ECIPreferred:
		return framework.MaxNodeScore / 4 * 3
	case uniext.ECIPreferredNot:
		return framework.MaxNodeScore / 4
	default:
		return framework.MinNodeScore
	}
}

func ScoreNormalNode() int64 {
	return framework.MaxNodeScore / 2
}
