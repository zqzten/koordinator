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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DrainNodeGroupSpec defines the desired state of DrainNodeGroup
type DrainNodeGroupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// NodeSelector decides whether to drain Nodes if the
	// Node matches the selector.
	// Default to the empty LabelSelector, which matches everything.
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`

	// TTL controls the DrainNodeGroup timeout duration.
	// +optional
	TTL *metav1.Duration `json:"ttl,omitempty"`

	// DrainNodePolicy decides how to identify drained nodes
	// +optional
	DrainNodePolicy *DrainNodePolicy `json:"drainNodePolicy,omitempty"`

	// ExecutionPolicy indicates how to execute in batches
	// +optional
	ExecutionPolicy *ExecutionPolicy `json:"executionPolicy,omitempty"`

	// TerminationPolicy, when the conditions are met, no new nodes are planned.
	// +optional
	TerminationPolicy *TerminationPolicy `json:"terminationPolicy,omitempty"`
}

type DrainNodeMode string

const (
	// DrainNodeModeTaint means adding Taint to planned node.
	DrainNodeModeTaint DrainNodeMode = "Taint"
)

type DrainNodePolicy struct {
	Mode DrainNodeMode `json:"mode,omitempty"`
}

type ExecutionPolicy struct {
	// Partition indicates the ordinal at which the DrainNodeGroup should be partitioned by default.
	// Partition indicates how many nodes need to perform draining.
	// Default value is 0.
	// +optional
	Partition *int32 `json:"partition,omitempty"`
	// Paused indicates that the DrainNodeGroup is paused.
	// Default value is false
	// +optional
	Paused bool `json:"paused,omitempty"`
}

type TerminationPolicy struct {
	// MaxNodeCount indicates how many nodes are planned at this time
	// +optional
	MaxNodeCount *int32 `json:"maxNodeCount,omitempty"`
	// PercentageOfResourceReserved indicates how many resources need to be reserved for this DrainNodeGroup.
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:validation:Minimum=1
	// +optional
	PercentageOfResourceReserved *int32 `json:"percentageOfResourceReserved,omitempty"`
}

// DrainNodeGroupStatus defines the observed state of DrainNodeGroup
type DrainNodeGroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase indicates what stage the DrainNodeGroup is in, such as Planning, Running, and Succeed.
	Phase DrainNodeGroupPhase `json:"phase,omitempty"`

	// TotalCount indicates the total number of nodes processed by the DrainNodeGroup.
	// TotalCount = UnavailableCount + AvailableCount
	TotalCount int32 `json:"totalCount,omitempty"`
	// UnavailableCount indicates the total number of nodes that the DrainNodeGroup cannot drain.
	UnavailableCount int32 `json:"unavailableCount,omitempty"`
	// AvailableCount indicates the total number of nodes that the DrainNodeGroup can drain.
	// AvailableCount = SucceedCount + FailedCount
	AvailableCount int32 `json:"availableCount,omitempty"`
	// RunningCount indicates the total number of nodes being drained by the DrainNodeGroup.
	RunningCount int32 `json:"runningCount,omitempty"`
	// SucceedCount indicates the total number of nodes that DrainNodeGroup has successfully drained.
	SucceedCount int32 `json:"succeedCount,omitempty"`
	// FailedCount indicates the total number of nodes that DrainNodeGroup failed to drain
	FailedCount int32 `json:"failedCount,omitempty"`

	// Conditions describes the errors, if any.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type DrainNodeGroupPhase string

const (
	// DrainNodeGroupPhasePending represents the initial status.
	DrainNodeGroupPhasePending DrainNodeGroupPhase = "Pending"
	// DrainNodeGroupPhasePlanning represents the drain node in the plan.
	DrainNodeGroupPhasePlanning DrainNodeGroupPhase = "Planning"
	// DrainNodeGroupPhaseWaiting represents the drain node plan to wait for confirmation.
	DrainNodeGroupPhaseWaiting DrainNodeGroupPhase = "Waiting"
	// DrainNodeGroupPhaseRunning represents that the drain node plan is being executed.
	DrainNodeGroupPhaseRunning DrainNodeGroupPhase = "Running"
	// DrainNodeGroupPhaseSucceed represents the successful execution of the drain node plan.
	DrainNodeGroupPhaseSucceed DrainNodeGroupPhase = "Succeed"
	// DrainNodeGroupPhaseFailed represents the failure of the drain node plan execution.
	DrainNodeGroupPhaseFailed DrainNodeGroupPhase = "Failed"
	// DrainNodeGroupPhaseAborted represents that the drain node plan has been cancelled.
	DrainNodeGroupPhaseAborted DrainNodeGroupPhase = "Aborted"
)

// DrainNodeGroup is the Schema for the DrainNodeGroup API
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="The phase of DrainNodeGroup"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="TTL",type="string",JSONPath=".spec.ttl"

// DrainNodeGroup is the Schema for the resourcepolicies API
type DrainNodeGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DrainNodeGroupSpec   `json:"spec,omitempty"`
	Status DrainNodeGroupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DrainNodeGroupList contains a list of DrainNodeGroup
type DrainNodeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DrainNodeGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DrainNodeGroup{}, &DrainNodeGroupList{})
}
