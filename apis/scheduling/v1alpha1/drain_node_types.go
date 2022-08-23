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

// DrainNodeSpec defines the desired state of DrainNode
type DrainNodeSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Node name that needs to be drained
	// +required
	NodeName string `json:"nodeName,omitempty"`

	// TTL controls the DrainNode timeout duration.
	// +optional
	TTL *metav1.Duration `json:"ttl,omitempty"`

	// DrainNodePolicy decides how to identify drained nodes
	// +optional
	DrainNodePolicy *DrainNodePolicy `json:"drainNodePolicy,omitempty"`

	// ConfirmState indicates the confirmation state.
	// +required
	ConfirmState ConfirmState `json:"confirmState,omitempty"`
}

type ConfirmState string

const (
	// ConfirmStateWait means waiting for confirmation to drain the node.
	ConfirmStateWait ConfirmState = "WaitForConfirm"
	// ConfirmStateConfirmed agrees to drain the node.
	ConfirmStateConfirmed ConfirmState = "Confirmed"
	// ConfirmStateRejected indicates refusal to drain the node.
	ConfirmStateRejected ConfirmState = "Rejected"
)

// DrainNodeStatus defines the observed state of DrainNode
type DrainNodeStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase indicates what stage the DrainNode is in, such as Waiting, Running, and Succeed.
	Phase DrainNodePhase `json:"phase,omitempty"`

	// MigrationJobStatus indicates the migration status of pods on the node.
	MigrationJobStatus []MigrationJobStatus `json:"migrationJobStatus,omitempty"`

	// Conditions describes the errors, if any.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type DrainNodePhase string

const (
	// DrainNodePhasePending represents the initial status.
	DrainNodePhasePending DrainNodePhase = "Pending"
	// DrainNodePhaseUnavailable represents insufficient resources to drain the node.
	DrainNodePhaseUnavailable DrainNodePhase = "Unavailable"
	// DrainNodePhaseAvailable represents sufficient resources to drain the node.
	DrainNodePhaseAvailable DrainNodePhase = "Available"
	// DrainNodePhaseWaiting represents waiting for confirmation to drain node.
	DrainNodePhaseWaiting DrainNodePhase = "Waiting"
	// DrainNodePhaseRunning represents the node being drained.
	DrainNodePhaseRunning DrainNodePhase = "Running"
	// DrainNodePhaseSucceeded represents the successful execution of the drain node.
	DrainNodePhaseSucceeded DrainNodePhase = "Succeeded"
	// DrainNodePhaseFailed represents a drain node execution failure.
	DrainNodePhaseFailed DrainNodePhase = "Failed"
	// DrainNodePhaseAborted represents that the drain node has been cancelled.
	DrainNodePhaseAborted DrainNodePhase = "Aborted"
)

type MigrationJobStatus struct {
	// Namespace where the pod is in
	Namespace string `json:"namespace,omitempty"`
	// The name of the Pod.
	PodName string `json:"podName,omitempty"`
	// The name of the Reservation.
	ReservationName string `json:"reservationName,omitempty"`
	// Status of the Reservation.
	ReservationPhase ReservationPhase `json:"reservationPhase,omitempty"`
	// The target node for pod migration.
	TargetNode string `json:"targetNode,omitempty"`
	// The name of the PodMigrationJob.
	PodMigrationJobName string `json:"podMigrationJobName,omitempty"`
	// Status of the PodMigrationJob.
	PodMigrationJobPhase PodMigrationJobPhase `json:"podMigrationJobPhase,omitempty"`
}

// DrainNode is the Schema for the DrainNode API
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:scope=Cluster,shortName=dn
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="The phase of DrainNode"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="TTL",type="string",JSONPath=".spec.ttl"

// DrainNode is the Schema for the resourcepolicies API
type DrainNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DrainNodeSpec   `json:"spec,omitempty"`
	Status DrainNodeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DrainNodeList contains a list of DrainNode
type DrainNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DrainNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DrainNode{}, &DrainNodeList{})
}
