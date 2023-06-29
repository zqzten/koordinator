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

package utils

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func IsAbort(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	if v, ok := labels[AbortKey]; ok && v == "true" {
		return true
	}
	if v, ok := labels[CleanKey]; ok && v == "true" {
		return true
	}
	return false
}

func NeedCleanTaint(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	if v, ok := labels[CleanKey]; ok && v == "true" {
		return true
	}
	return false
}

func UpdateCondition(status *v1alpha1.DrainNodeStatus, condition *v1alpha1.DrainNodeCondition) bool {
	condition.LastTransitionTime = metav1.Now()
	// Try to find this DrainNode condition.
	conditionIndex, oldCondition := GetCondition(status, condition.Type)

	if oldCondition == nil {
		// We are adding new PodMigrationJob condition.
		status.Conditions = append(status.Conditions, *condition)
		return true
	}
	// We are updating an existing condition, so we need to check if it has changed.
	if condition.Status == oldCondition.Status {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	}

	isEqual := condition.Status == oldCondition.Status &&
		condition.Reason == oldCondition.Reason &&
		condition.Message == oldCondition.Message &&
		condition.LastProbeTime.Equal(&oldCondition.LastProbeTime) &&
		condition.LastTransitionTime.Equal(&oldCondition.LastTransitionTime)

	status.Conditions[conditionIndex] = *condition
	// Return true if one of the fields have changed.
	return !isEqual
}

func GetCondition(status *v1alpha1.DrainNodeStatus, conditionType v1alpha1.DrainNodeConditionType) (int, *v1alpha1.DrainNodeCondition) {
	if len(status.Conditions) == 0 {
		return -1, nil
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return i, &status.Conditions[i]
		}
	}
	return -1, nil
}

func GetMigrationPolicy(pod *corev1.Pod) (*v1alpha1.MigrationPolicy, error) {
	rawMigrationPolicy, ok := pod.Annotations[MigrationPolicy]
	if !ok {
		return nil, nil
	}
	migrationPolicy := &v1alpha1.MigrationPolicy{}
	err := json.Unmarshal([]byte(rawMigrationPolicy), migrationPolicy)
	if err != nil {
		return nil, err
	}
	return migrationPolicy, nil
}

func PausedForPodAfterConfirmed(dn *v1alpha1.DrainNode) bool {
	_, ok := dn.Annotations[PausedAfterConfirmed]
	return ok
}
