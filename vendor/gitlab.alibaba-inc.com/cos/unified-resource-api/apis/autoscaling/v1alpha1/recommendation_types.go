/*
Copyright 2021.

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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RecommendationConditionType are the valid conditions of a Recommender.
type RecommendationConditionType string

var (
	// RecommendationProvided indicates whether the recommender was able to calculate a client.
	RecommendationProvided RecommendationConditionType = "RecommendationProvided"
	// LowConfidence indicates whether the recommender has low confidence in the client for
	// some containers.
	LowConfidence RecommendationConditionType = "LowConfidence"
	// NoPodsMatched indicates that label selector used with object didn't match any pods.
	NoPodsMatched RecommendationConditionType = "NoPodsMatched"
	// FetchingHistory indicates that recommender is in the process of loading additional history samples.
	FetchingHistory RecommendationConditionType = "FetchingHistory"
	// ConfigDeprecated indicates that this recommender configuration is deprecated
	// and will stop being supported soon.
	ConfigDeprecated RecommendationConditionType = "ConfigDeprecated"
	// ConfigUnsupported indicates that this recommender is unsupported
	// and recommendations will not be provided for it.
	ConfigUnsupported RecommendationConditionType = "ConfigUnsupported"
	// MetricIsExpired indicates that the last metrics of container
	// is too old to give meaningful recommendation.
	MetricIsExpired RecommendationConditionType = "MetricIsExpired"
)

// RecommendationSpec is the specification of the client object.
type RecommendationSpec struct {
	// workloadRef of the workload object that stored Recommendation object.
	// Using string for reference is to support label
	// +optional
	WorkloadRef *CrossVersionObjectReference `json:"workloadRef,omitempty" protobuf:"bytes,1,opt,name=workloadRef"`
	// selector is a label query over pods that should match the replica count.
	// It must match the pod template's labels.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty" protobuf:"bytes,2,opt,name=selector"`
}

// CrossVersionObjectReference contains enough information to let you identify the referred resource.
type CrossVersionObjectReference struct {
	// Kind of the referent; More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds"
	Kind string `json:"kind" protobuf:"bytes,1,opt,name=kind"`
	// Name of the referent; More info: http://kubernetes.io/docs/user-guide/identifiers#names
	Name string `json:"name" protobuf:"bytes,2,opt,name=name"`
	// API version of the referent
	// +optional
	APIVersion string `json:"apiVersion,omitempty" protobuf:"bytes,3,opt,name=apiVersion"`
}

// RecommendedPodResources is the client of resources computed by
// autoscaler. It contains a client for each container in the pod
type RecommendedPodResources struct {
	// Resources recommended by the recommender for each container.
	// +optional
	ContainerRecommendations []RecommendedContainerResources `json:"containerRecommendations,omitempty" protobuf:"bytes,1,rep,name=containerRecommendations"`
}

// RecommendedContainerResources is the client of resources computed by
// recommender for a specific container. Respects the container resource policy
// if present in the spec.
type RecommendedContainerResources struct {
	// Name of the container.
	ContainerName string `json:"containerName,omitempty" protobuf:"bytes,1,opt,name=containerName"`

	// Recommendation of workload resource request.
	// Differ from the UncappedTarget because the target uses different quantile uncapped target,
	// which depends on workload type. For service type workload, use "P95 CPU" and "P99 Memory";
	// for batch job, use "Average CPU" and "P60 Memory".
	// Besides, target also includes buffer ratio(e.g. 15%) for safety reasons.
	Target v1.ResourceList `json:"target" protobuf:"bytes,2,rep,name=target,casttype=ResourceList,castkey=ResourceName"`

	// OriginalTarget records the recommendation based on actual resource usage,
	// not taking into account the calibration by workload type(e.g. service, batch job).
	// Use different quantile type as key, currently support "average", "p60", "p90", "p95" "p99", "max".
	// +optional
	OriginalTarget map[string]v1.ResourceList `json:"originalTarget"`
}

// RecommendationStatus defines the observed state of Recommendation
type RecommendationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The most recently computed amount of resources recommended by the
	// autoscaler for the controlled pods.
	// +optional
	Recommendation *RecommendedPodResources `json:"recommendResources,omitempty" protobuf:"bytes,1,opt,name=client"`

	// Conditions is the set of conditions required for this autoscaler to scale its target,
	// and indicates whether or not those conditions are met.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []RecommendationCondition `json:"conditions,omitempty" protobuf:"bytes,2,opt,name=conditions"`
}

type RecommendationCondition struct {
	// type describes the current condition
	Type RecommendationConditionType `json:"type" protobuf:"bytes,1,name=type"`
	// status is the status of the condition (True, False, Unknown)
	Status v1.ConditionStatus `json:"status" protobuf:"bytes,2,name=status"`
	// lastTransitionTime is the last time the condition transitioned from
	// one status to another
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,3,opt,name=lastTransitionTime"`
	// reason is the reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason"`
	// message is a human-readable explanation containing details about
	// the transition
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,5,opt,name=message"`
}

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=red
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Recommendation is the Schema for the recommendations API
type Recommendation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RecommendationSpec   `json:"spec,omitempty"`
	Status RecommendationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// RecommendationList contains a list of Recommendation
type RecommendationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Recommendation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Recommendation{}, &RecommendationList{})
}
