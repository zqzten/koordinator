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
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	LabelEnableOverQuota             = "sigma.ali/is-over-quota"
	LabelCPUOverQuota                = "sigma.ali/cpu-over-quota"
	LabelMemoryOverQuota             = "sigma.ali/memory-over-quota"
	LabelDiskOverQuota               = "sigma.ali/disk-over-quota"
	AnnotationDisableOverQuotaFilter = "sigma.ali/disable-over-quota-filter"
)

// The following new over-quota APIs defined by ACS
const (
	LabelAlibabaCPUOverQuota    = "alibabacloud.com/cpu-over-quota"
	LabelAlibabaMemoryOverQuota = "alibabacloud.com/memory-over-quota"
	LabelAlibabaDiskOverQuota   = "alibabacloud.com/disk-over-quota"
)

func IsNodeEnableOverQuota(node *corev1.Node) bool {
	return node.Labels[LabelEnableOverQuota] == "true"
}

func IsPodDisableOverQuotaFilter(pod *corev1.Pod) bool {
	return pod.Annotations[AnnotationDisableOverQuotaFilter] == "true"
}

func IsPodRequireOverQuotaNode(pod *corev1.Pod) bool {
	var nodeSelectorTerms []corev1.NodeSelectorTerm
	if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil {
		nodeAffinity := pod.Spec.Affinity.NodeAffinity
		if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
			nodeSelectorTerms = nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		}
	}
	if len(nodeSelectorTerms) == 0 {
		return false
	}
	for _, req := range nodeSelectorTerms {
		if len(req.MatchExpressions) == 0 {
			continue
		}

		labelSelector, err := nodeSelectorRequirementsAsSelector(req.MatchExpressions)
		if err != nil {
			continue
		}
		value, found := labelSelector.RequiresExactMatch(LabelEnableOverQuota)
		if found && value == "true" {
			return true
		}
	}
	return false
}

// nodeSelectorRequirementsAsSelector converts the []NodeSelectorRequirement api type into a struct that implements
// labels.Selector.
func nodeSelectorRequirementsAsSelector(nsm []corev1.NodeSelectorRequirement) (labels.Selector, error) {
	if len(nsm) == 0 {
		return labels.Nothing(), nil
	}
	selector := labels.NewSelector()
	for _, expr := range nsm {
		var op selection.Operator
		switch expr.Operator {
		case corev1.NodeSelectorOpIn:
			op = selection.In
		case corev1.NodeSelectorOpNotIn:
			op = selection.NotIn
		case corev1.NodeSelectorOpExists:
			op = selection.Exists
		case corev1.NodeSelectorOpDoesNotExist:
			op = selection.DoesNotExist
		case corev1.NodeSelectorOpGt:
			op = selection.GreaterThan
		case corev1.NodeSelectorOpLt:
			op = selection.LessThan
		default:
			return nil, fmt.Errorf("%q is not a valid node selector operator", expr.Operator)
		}
		r, err := labels.NewRequirement(expr.Key, op, expr.Values)
		if err != nil {
			return nil, err
		}
		selector = selector.Add(*r)
	}
	return selector, nil
}

func GetResourceOverQuotaSpec(node *corev1.Node) (cpuOverQuotaRatio, memoryOverQuotaRatio, diskOverQuotaRatio int64) {
	cpuOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelAlibabaCPUOverQuota], node.Labels[LabelCPUOverQuota])
	memoryOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelAlibabaMemoryOverQuota], node.Labels[LabelMemoryOverQuota])
	diskOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelAlibabaDiskOverQuota], node.Labels[LabelDiskOverQuota])
	return
}

func GetSigmaResourceOverQuotaSpec(node *corev1.Node) (cpuOverQuotaRatio, memoryOverQuotaRatio, diskOverQuotaRatio int64) {
	cpuOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelCPUOverQuota], "")
	memoryOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelMemoryOverQuota], "")
	diskOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelDiskOverQuota], "")
	return
}

func GetAlibabaResourceOverQuotaSpec(node *corev1.Node) (cpuOverQuotaRatio, memoryOverQuotaRatio, diskOverQuotaRatio int64) {
	cpuOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelAlibabaCPUOverQuota], "")
	memoryOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelAlibabaMemoryOverQuota], "")
	diskOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelAlibabaDiskOverQuota], "")
	return
}

func CPUMaxRefCount(node *corev1.Node) int {
	cpuOverQuotaRatio := parseOverQuotaRatio(node.Labels[LabelCPUOverQuota], node.Labels[LabelAlibabaCPUOverQuota])
	return int((cpuOverQuotaRatio + 99) / 100)
}

func parseOverQuotaRatio(overQuota string, alternative string) int64 {
	f := parseOverQuotaRatioToFloat64(overQuota, alternative)
	return int64(f * 100)
}

func parseOverQuotaRatioToFloat64(overQuota, alternative string) float64 {
	if overQuota != "" {
		f, err := strconv.ParseFloat(overQuota, 64)
		if err == nil {
			return f
		}
	}

	if alternative != "" {
		f, err := strconv.ParseFloat(alternative, 64)
		if err == nil {
			return f
		}
	}

	return 1.0
}

func NewAmplificationRatiosByOverQuota(labels map[string]string) map[corev1.ResourceName]apiext.Ratio {
	return map[corev1.ResourceName]apiext.Ratio{
		corev1.ResourceCPU:              apiext.Ratio(parseOverQuotaRatioToFloat64(labels[LabelAlibabaCPUOverQuota], labels[LabelCPUOverQuota])),
		corev1.ResourceMemory:           apiext.Ratio(parseOverQuotaRatioToFloat64(labels[LabelAlibabaMemoryOverQuota], labels[LabelMemoryOverQuota])),
		corev1.ResourceEphemeralStorage: apiext.Ratio(parseOverQuotaRatioToFloat64(labels[LabelAlibabaDiskOverQuota], labels[LabelDiskOverQuota])),
	}
}

func GetAllocatableByOverQuota(node *corev1.Node) corev1.ResourceList {
	allocatable := node.Status.Allocatable.DeepCopy()
	if allocatable == nil {
		return nil
	}
	amplificationRatios := NewAmplificationRatiosByOverQuota(node.Labels)
	for resourceName, ratio := range amplificationRatios {
		if ratio > 1 {
			if quantity := allocatable[resourceName]; !quantity.IsZero() {
				apiext.AmplifyResourceList(allocatable, amplificationRatios, resourceName)
			}
		}
	}
	return allocatable
}
