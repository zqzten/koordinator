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
	"encoding/json"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	LabelPodConstraint            = "alibabacloud.com/pod-constraint-name"
	AnnotationMultiPodConstraints = "alibabacloud.com/multi-pod-constraints"
	AnnotationWeightedSpreadUnits = "alibabacloud.com/weighted-spread-units"
	SigmaLabelSpreadGroupName     = "sigma.ali/spread-group-name"
	SigmaLabelServiceUnitName     = "sigma.ali/deploy-unit"
	SigmaLabelInstanceGroupName   = "sigma.ali/instance-group"

	LabelSpreadType         = "scheduler.alibabacloud.com/spread-type"
	LabelSpreadTypeRequired = "Required"
)

type WeightedPodConstraint struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Weight    int    `json:"weight"`
}

func GetWeightedPodConstraints(pod *corev1.Pod) []WeightedPodConstraint {
	var weightedPodConstraints []WeightedPodConstraint
	if annotation := pod.Annotations[AnnotationMultiPodConstraints]; annotation != "" {
		err := json.Unmarshal([]byte(annotation), &weightedPodConstraints)
		if err != nil {
			return nil
		}
	} else if constraintName := pod.Labels[LabelPodConstraint]; constraintName != "" {
		weightedPodConstraints = append(weightedPodConstraints, WeightedPodConstraint{
			Namespace: pod.Namespace,
			Name:      constraintName,
			Weight:    1,
		})
	}
	for i := range weightedPodConstraints {
		weightedPodConstraint := &weightedPodConstraints[i]
		if weightedPodConstraint.Weight <= 0 {
			weightedPodConstraint.Weight = 1
		}
		if weightedPodConstraint.Namespace == "" {
			weightedPodConstraint.Namespace = pod.Namespace
		}
	}
	return weightedPodConstraints
}

type WeightedSpreadUnit struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
}

func GetWeighedSpreadUnits(pod *corev1.Pod) []WeightedSpreadUnit {
	var weightedSpreadUnits []WeightedSpreadUnit
	if annotation := pod.Annotations[AnnotationWeightedSpreadUnits]; annotation != "" {
		err := json.Unmarshal([]byte(annotation), &weightedSpreadUnits)
		if err != nil {
			return nil
		}
	} else {
		unit := getSpreadUnit(pod)
		weightedSpreadUnits = append(weightedSpreadUnits, WeightedSpreadUnit{
			Name:   unit,
			Weight: 1,
		})
	}
	for i := range weightedSpreadUnits {
		if weightedSpreadUnits[i].Weight <= 0 {
			weightedSpreadUnits[i].Weight = 1
		}
	}
	return weightedSpreadUnits
}

func getSpreadUnit(pod *corev1.Pod) string {
	if spreadGroupName, ok := pod.Labels[SigmaLabelSpreadGroupName]; ok {
		return spreadGroupName
	}
	return getServiceUnit(pod.Labels)
}

func getServiceUnit(label map[string]string) string {
	_, value := getDeployUnitKeyValue(label)
	return value
}

func getDeployUnitKeyValue(label map[string]string) (string, string) {
	if value, ok := label[uniext.LabelDeployUnit]; ok {
		return uniext.LabelDeployUnit, value
	} else if value, ok = label[uniext.LabelHippoRoleName]; ok {
		return uniext.LabelHippoRoleName, value
	} else if value, ok = label[SigmaLabelServiceUnitName]; ok {
		return SigmaLabelServiceUnitName, value
	} else if value, ok = label[SigmaLabelInstanceGroupName]; ok {
		return SigmaLabelInstanceGroupName, value
	}
	return "", ""
}

// GetPodConstraintKey
// 先不考虑label不一样，但是PodConstraint一样的case，这种情况不多。
// 所以只考虑label，不看具体PodConstraint crd的内容了
func GetPodConstraintKey(pod *corev1.Pod) string {
	constraintName := pod.Labels[LabelPodConstraint]
	if constraintName == "" {
		return ""
	}
	return getNamespacedName(pod.Namespace, constraintName)
}

func getNamespacedName(namespace, name string) string {
	result := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	return result.String()
}

func IsSpreadTypeRequire(pod *corev1.Pod) bool {
	return pod.Labels[LabelSpreadType] == LabelSpreadTypeRequired
}
