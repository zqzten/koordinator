/*
Copyright 2023 The Koordinator Authors.

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

package logicalresourcenode

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func GetPodLabelSelector(lrn *schedulingv1alpha1.LogicalResourceNode) ([]metav1.LabelSelector, error) {
	var ret []metav1.LabelSelector

	// Use the new annotation first
	multiPodSelectorStr := lrn.Annotations[schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelectorList]
	if multiPodSelectorStr != "" {
		labelSelectors := make([]map[string]string, 0)
		if err := json.Unmarshal([]byte(multiPodSelectorStr), &labelSelectors); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %v", schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelectorList, err)
		}
		for i := range labelSelectors {
			if len(labelSelectors[i]) == 0 {
				continue
			}
			ret = append(ret, metav1.LabelSelector{MatchLabels: labelSelectors[i]})
		}
		return ret, nil
	}

	labelSelector := map[string]string{}
	if podSelectorStr := lrn.Annotations[schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelector]; podSelectorStr == "" {
		return nil, fmt.Errorf("no found lrn.koordinator.sh/pod-label-selector in annotations")
	} else if err := json.Unmarshal([]byte(podSelectorStr), &labelSelector); err != nil {
		return nil, fmt.Errorf("failed to parse lrn.koordinator.sh/pod-label-selector in annotations: %s", err)
	}
	if len(labelSelector) != 0 {
		ret = append(ret, metav1.LabelSelector{MatchLabels: labelSelector})
	}
	return ret, nil
}

func GetReservationOwners(lrn *schedulingv1alpha1.LogicalResourceNode) ([]schedulingv1alpha1.ReservationOwner, error) {
	selectors, err := GetPodLabelSelector(lrn)
	if err != nil {
		return nil, err
	}
	owners := make([]schedulingv1alpha1.ReservationOwner, 0, len(selectors))
	for i := range selectors {
		selector := &selectors[i]
		owners = append(owners, schedulingv1alpha1.ReservationOwner{
			LabelSelector: selector,
		})
	}
	return owners, nil
}

func RequirementsToPodSpec(requirements *schedulingv1alpha1.LogicalResourceNodeRequirements) *corev1.PodSpec {
	// Add some required fields for validation
	return &corev1.PodSpec{
		NodeSelector:              requirements.NodeSelector,
		Affinity:                  requirements.Affinity,
		Tolerations:               requirements.Tolerations,
		TopologySpreadConstraints: requirements.TopologySpreadConstraints,
		Containers: []corev1.Container{
			{
				Name:            "mock",
				Image:           "mock",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources: corev1.ResourceRequirements{
					Requests: requirements.Resources,
					Limits:   requirements.Resources,
				},
				TerminationMessagePolicy: corev1.TerminationMessageReadFile,
			},
		},
		RestartPolicy: corev1.RestartPolicyAlways,
		DNSPolicy:     corev1.DNSClusterFirst,
	}
}

func GetReservationOwnerLRN(reservation *schedulingv1alpha1.Reservation) string {
	for _, ownerRef := range reservation.OwnerReferences {
		if ownerRef.APIVersion != schedulingv1alpha1.SchemeGroupVersion.String() || ownerRef.Kind != "LogicalResourceNode" {
			continue
		}
		return ownerRef.Name
	}
	return ""
}
