/*
Copyright 2021 Alibaba Cloud.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ControllerKind string

const (
	// the workload of k8s
	CronJob               ControllerKind = "CronJob"
	DaemonSet             ControllerKind = "DaemonSet"
	Deployment            ControllerKind = "Deployment"
	Job                   ControllerKind = "Job"
	ReplicaSet            ControllerKind = "ReplicaSet"
	ReplicationController ControllerKind = "ReplicationController"
	StatefulSet           ControllerKind = "StatefulSet"

	// the workload of kruise
	CloneSet            ControllerKind = "CloneSet"
	AdvancedStatefulSet ControllerKind = "AdvancedStatefulSet"
	AdvancedDaemonset   ControllerKind = "AdvancedDaemonset"
	AdvancedCronJob     ControllerKind = "AdvancedCronJob"
)

type RecommendationProfileType string

const (
	Controller RecommendationProfileType = "controller"
	Pod        RecommendationProfileType = "pod"
)

// RecommendationProfileSpec defines the desired state of RecommendationProfile
type RecommendationProfileSpec struct {
	// Type indicates which pattern the pod managed by.
	// It can be a controller like deployment, statefulset, etc .
	// Of course, it may not be managed by any controllers and only is a pod.
	Type RecommendationProfileType `json:"type,omitempty"`
	// ControllerKind indicates which controller the pod managed by
	// when RuleType equals "controller".
	ControllerKinds []ControllerKind `json:"controllerKind,omitempty"`
	// EnabledNamespaces indicates which namespaces the RecommendationProfile
	// will generate recommendation cr for and "*" represents all namespaces.
	EnabledNamespaces []string `json:"enabledNamespaces,omitempty"`
	// DisabledNamespaces indicates which namesapces the RecommendationProfile
	// will not generate recommendation cr for and "*" represents all namespaces.
	DisabledNamespaces []string `json:"disabledNamespaces,omitempty"`
	// WorkloadRefLabelKeys indicates which pod's label key can distinguish from other type of pod.
	WorkloadRefLabelKeys []string `json:"workloadRefLabelKeys,omitempty"`
}

// RecommendationProfileStatus defines the observed state of RecommendationProfile
type RecommendationProfileStatus struct {
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:shortName=redprofile,scope=Cluster
// +kubebuilder:object:root=true
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// RecommendationProfile is the Schema for the recommendationprofile API

type RecommendationProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RecommendationProfileSpec   `json:"spec,omitempty"`
	Status RecommendationProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RecommendationProfileList contains a list of RecommendationProfile
type RecommendationProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RecommendationProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RecommendationProfile{}, &RecommendationProfileList{})
}
