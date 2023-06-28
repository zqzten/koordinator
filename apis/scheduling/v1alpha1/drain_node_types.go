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
	"k8s.io/apimachinery/pkg/types"
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

	// CordonPolicy decides how to cordon drained nodes
	// +optional
	CordonPolicy *CordonNodePolicy `json:"cordonPolicy,omitempty"`

	// MigrationPolicy decides how the migrations progress
	// +optional
	MigrationPolicy MigrationPolicy `json:"migrationPolicy,omitempty"`

	// ConfirmState indicates the confirmation state.
	// +required
	ConfirmState ConfirmState `json:"confirmState,omitempty"`
}

type MigrationPolicy struct {
	Mode         MigrationPodMode `json:"mode,omitempty"`
	WaitDuration *metav1.Duration `json:"waitDuration,omitempty"`
}

type MigrationPodMode string

const (
	MigrationPodModeWaitFirst       MigrationPodMode = "WaitFirst"
	MigrationPodModeMigrateDirectly MigrationPodMode = "MigrateDirectly"
)

type ConfirmState string

const (
	// ConfirmStateWait means waiting for confirmation to drain the node.
	ConfirmStateWait ConfirmState = "WaitForConfirm"
	// ConfirmStateConfirmed agrees to drain the node.
	ConfirmStateConfirmed ConfirmState = "Confirmed"
	// ConfirmStateRejected indicates refusal to drain the node.
	ConfirmStateRejected ConfirmState = "Rejected"
	// ConfirmStateAborted drainNode aborted
	ConfirmStateAborted ConfirmState = "Aborted"
)

// DrainNodeStatus defines the observed state of DrainNode
type DrainNodeStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase indicates what stage the DrainNode is in, such as Waiting, Running, and Succeed.
	Phase DrainNodePhase `json:"phase,omitempty"`

	// PodMigrations indicates the migration status of pods on the node.
	PodMigrations []PodMigration `json:"pogMigrations,omitempty"`

	// PodMigrationSummary provides summary information of migrations
	PodMigrationSummary map[PodMigrationPhase]int32 `json:"pogMigrationSummary,omitempty"`

	// Conditions describes the errors, if any.
	Conditions []DrainNodeCondition `json:"conditions,omitempty"`
}

type DrainNodePhase string

const (
	// DrainNodePhasePending represents the initial status.
	DrainNodePhasePending DrainNodePhase = "Pending"
	// DrainNodePhaseRunning represents the node being drained.
	DrainNodePhaseRunning DrainNodePhase = "Running"
	// DrainNodePhaseComplete represents the successful execution of the drain node.
	DrainNodePhaseComplete DrainNodePhase = "Complete"
	// DrainNodePhaseAborted represents that the drain node has been cancelled.
	DrainNodePhaseAborted DrainNodePhase = "Aborted"
)

type DrainNodeCondition struct {
	// Type is the type of the condition.
	Type DrainNodeConditionType `json:"type"`
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	Status metav1.ConditionStatus `json:"status"`
	// Last time we probed the condition.
	// +nullable
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	// +nullable
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Unique, one-word, CamelCase reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition.
	Message string `json:"message,omitempty"`
}

type DrainNodeConditionType string

// These are valid conditions of DrainNode.
const (
	DrainNodeConditionOnceAvailable                    DrainNodeConditionType = "OnceAvailable"
	DrainNodeConditionUnmigratablePodExists            DrainNodeConditionType = "UnmigratablePodExists"
	DrainNodeConditionUnexpectedReservationExists      DrainNodeConditionType = "UnexpectedReservationExists"
	DrainNodeConditionUnavailableReservationExists     DrainNodeConditionType = "UnavailableReservationExists"
	DrainNodeConditionFailedMigrationJobExists         DrainNodeConditionType = "FailedMigrationJobExists"
	DrainNodeConditionUnexpectedPodAfterCompleteExists DrainNodeConditionType = "UnexpectedPodAfterCompleteExists"
)

type PodMigration struct {
	// The UID of the Pod
	PodUID types.UID `json:"uid,omitempty"`
	// Namespace where the pod is in
	Namespace string `json:"namespace,omitempty"`
	// The name of the Pod.
	PodName string `json:"podName,omitempty"`
	// The phase of the PodMigration
	Phase PodMigrationPhase `json:"phase,omitempty"`
	// The startTime of the PodMigration
	StartTimestamp metav1.Time `json:"startTimestamp,omitempty"`
	// If pod migration start after user confirms the DrainNode
	ChartedAfterDrainNodeConfirmed bool `json:"chartedAfterDrainNodeConfirmed,omitempty"`
	// The name of the Reservation.
	ReservationName string `json:"reservationName,omitempty"`
	// Status of the Reservation.
	ReservationPhase ReservationPhase `json:"reservationPhase,omitempty"`
	// The target node for pod migration.
	TargetNode string `json:"targetNode,omitempty"`
	// The name of the PodMigrationJob.
	PodMigrationJobName string `json:"podMigrationJobName,omitempty"`
	// If PodMigrationJob paused
	PodMigrationJobPaused *bool `json:"podMigrationJobPaused,omitempty"`
	// Status of the PodMigrationJob.
	PodMigrationJobPhase PodMigrationJobPhase `json:"podMigrationJobPhase,omitempty"`
}

type PodMigrationPhase string

const (
	// PodMigrationPhaseUnmigratable represents the pod is Unmigratable
	PodMigrationPhaseUnmigratable PodMigrationPhase = "Unmigratable"
	// PodMigrationPhaseWaiting wait for pod completion
	PodMigrationPhaseWaiting PodMigrationPhase = "Waiting"
	// PodMigrationPhaseReady pod completed or the maximum waiting time has been reached.
	PodMigrationPhaseReady PodMigrationPhase = "Ready"
	// PodMigrationPhaseAvailable represents sufficient resources to migrate the pod.
	PodMigrationPhaseAvailable PodMigrationPhase = "Available"
	// PodMigrationPhaseUnavailable represents pod migration unavailable.
	PodMigrationPhaseUnavailable PodMigrationPhase = "Unavailable"
	// PodMigrationPhaseMigrating represents pod is migrating.
	PodMigrationPhaseMigrating PodMigrationPhase = "Migrating"
	// PodMigrationPhaseSucceed represents pod migration has finished.
	PodMigrationPhaseSucceed PodMigrationPhase = "Succeed"
	// PodMigrationPhaseFailed represents pod migration failed.
	PodMigrationPhaseFailed PodMigrationPhase = "Failed"
)

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
