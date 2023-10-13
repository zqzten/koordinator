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

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	cpuOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelCPUOverQuota])
	memoryOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelMemoryOverQuota])
	diskOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelDiskOverQuota])
	return
}

func CPUMaxRefCount(node *corev1.Node) int {
	cpuOverQuotaRatio, _, _ := GetResourceOverQuotaSpec(node)
	return int((cpuOverQuotaRatio + 99) / 100)
}

func parseOverQuotaRatio(overQuota string) int64 {
	f := parseOverQuotaRatioToFloat64(overQuota)
	return int64(f * 100)
}

func parseOverQuotaRatioToFloat64(overQuota string) float64 {
	if overQuota == "" {
		return 1
	}

	f, err := strconv.ParseFloat(overQuota, 64)
	if err != nil {
		f = 1.0
	}
	return f
}

func NewAmplificationRatiosByOverQuota(labels map[string]string) map[corev1.ResourceName]apiext.Ratio {
	return map[corev1.ResourceName]apiext.Ratio{
		corev1.ResourceCPU:              apiext.Ratio(parseOverQuotaRatioToFloat64(labels[LabelCPUOverQuota])),
		corev1.ResourceMemory:           apiext.Ratio(parseOverQuotaRatioToFloat64(labels[LabelMemoryOverQuota])),
		corev1.ResourceEphemeralStorage: apiext.Ratio(parseOverQuotaRatioToFloat64(labels[LabelDiskOverQuota])),
	}
}

func GetAllocatableByOverQuota(node *corev1.Node) corev1.ResourceList {
	allocatable := node.Status.Allocatable.DeepCopy()
	cpuOverQuotaRatioSpec, memoryOverQuotaRatioSpec, diskOverQuotaRatioSpec := GetResourceOverQuotaSpec(node)
	if cpu, found := allocatable[corev1.ResourceCPU]; found {
		allocatable[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpu.MilliValue()*cpuOverQuotaRatioSpec/100, resource.DecimalSI)
	}
	if acu, found := allocatable[uniext.ResourceACU]; found {
		allocatable[uniext.ResourceACU] = *resource.NewMilliQuantity(acu.MilliValue()*cpuOverQuotaRatioSpec/100, resource.DecimalSI)
	}
	if memory, found := allocatable[corev1.ResourceMemory]; found {
		allocatable[corev1.ResourceMemory] = *resource.NewQuantity(memory.Value()*memoryOverQuotaRatioSpec/100, resource.BinarySI)
	}
	if ephemeralStorage, found := allocatable[corev1.ResourceEphemeralStorage]; found {
		allocatable[corev1.ResourceEphemeralStorage] = *resource.NewQuantity(ephemeralStorage.Value()*diskOverQuotaRatioSpec/100, resource.BinarySI)
	}
	return allocatable
}
