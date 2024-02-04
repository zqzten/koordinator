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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// AnnotationLogicalResourceNodePodLabelSelector is the label selector that matches pod
	AnnotationLogicalResourceNodePodLabelSelector = "lrn.koordinator.sh/pod-label-selector"

	// AnnotationLogicalResourceNodePodLabelSelectorList is the ORed multiple label selector that matches pod.
	AnnotationLogicalResourceNodePodLabelSelectorList = "lrn.koordinator.sh/pod-label-selector-list"

	// LabelLogicalResourceNodePodAssign is the label on pod that indicates which LRN is the pod assigned to.
	LabelLogicalResourceNodePodAssign = "pod.lrn.koordinator.sh/assign-lrn"

	// LabelNodeNameOfLogicalResourceNode is the node name of LRN.
	LabelNodeNameOfLogicalResourceNode = "lrn.koordinator.sh/node-name"

	// AnnotationLogicalResourceNodeDevices is the devices that have allocated to the LRN.
	AnnotationLogicalResourceNodeDevices = "lrn.koordinator.sh/devices"

	// AnnotationVPCQoSThreshold is the threshold limits for the LRN.
	AnnotationVPCQoSThreshold = "lrn.koordinator.sh/vpc-qos-threshold"

	// LabelVPCQoSGroupID is the vpc qos group id for the LRN.
	LabelVPCQoSGroupID = "lrn.koordinator.sh/vpc-qos-group-id"

	// AnnotationForceSyncLabelRegex is the regular expressions that indicates which label keys should be
	// force synced and deleted from LRN to Reservation.
	AnnotationForceSyncLabelRegex = "lrn.koordinator.sh/force-sync-label-regex"
)

type LRNVPCQoSThreshold struct {
	Rx    string `json:"rx"`
	RxPps string `json:"rxPps"`
	Tx    string `json:"tx"`
	TxPps string `json:"txPps"`
}

// LogicalResourceNodeSpec defines the desired state of LogicalResourceNode
type LogicalResourceNodeSpec struct {
	// Requirements defines the requirements of LRN scheduling
	Requirements LogicalResourceNodeRequirements `json:"requirements"`
	// Unschedulable controls LRN schedulability of new pods. By default, LRN is schedulable.
	Unschedulable bool `json:"unschedulable,omitempty"`
}

// LogicalResourceNodeRequirements defines the requirements of LogicalResourceNode
type LogicalResourceNodeRequirements struct {
	// Resources describes the amount of compute resources required.
	Resources corev1.ResourceList `json:"resources"`

	// NodeSelector is a selector which must be true for the LRN to fit on a node.
	// Selector which must match a node's labels for the LRN to be scheduled on that node.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// If specified, the LRN's scheduling constraints.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// If specified, the LRN's tolerations.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// TopologySpreadConstraints describes how a group of LRNs ought to spread across topology
	// domains. Scheduler will schedule pods in a way which abides by the constraints.
	// All topologySpreadConstraints are ANDed.
	// +optional
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

type LogicalResourceNodePhase string

const (
	LogicalResourceNodePending       LogicalResourceNodePhase = "Pending"
	LogicalResourceNodeAvailable     LogicalResourceNodePhase = "Available"
	LogicalResourceNodeUnschedulable LogicalResourceNodePhase = "Unschedulable"
	LogicalResourceNodeUnknown       LogicalResourceNodePhase = "Unknown"
)

// LogicalResourceNodeStatus defines the observed state of LogicalResourceNode
type LogicalResourceNodeStatus struct {
	// Phase is the recently observed lifecycle phase of the LRN.
	// +optional
	Phase LogicalResourceNodePhase `json:"phase,omitempty"`
	// Message is a human readable message indicating details about why the LRN is in this condition.
	// +optional
	Message string `json:"message,omitempty"`
	// Conditions is an array of current observed LRN conditions.
	// +optional
	Conditions []corev1.NodeCondition `json:"conditions,omitempty"`
	// NodeName is a request to schedule this LRN onto a specific node.
	// +optional
	NodeName string `json:"nodeName,omitempty"`
	// NodeStatus is the status of the Node assigned to this LRN.
	// +optional
	NodeStatus *LRNNodeStatus `json:"nodeStatus,omitempty"`
	// Allocatable represents the resources of a LRN that are available for scheduling.
	// +optional
	Allocatable corev1.ResourceList `json:"allocatable,omitempty"`
}

// LRNNodeStatus is the status of the Node assigned to this LRN.
type LRNNodeStatus struct {
	// Unschedulable controls node schedulability of new pods. By default, node is schedulable.
	Unschedulable bool `json:"unschedulable,omitempty"`
	// Conditions is an array of current observed node conditions.
	// Currently only few conditions will be synced to here.
	Conditions []corev1.NodeCondition `json:"conditions,omitempty"`
	// PrintColumn is the print colunm of this Node of LRN.
	PrintColumn string `json:"printColumn,omitempty"`
}

type LogicalResourceNodeDevices map[DeviceType][]LogicalResourceNodeDeviceInfo

type LogicalResourceNodeDeviceInfo struct {
	Minor int32 `json:"minor"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:scope=Cluster,shortName=lrn
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NodeName",type="string",JSONPath=".status.nodeName",description="The node of LRN"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="The phase of LRN"
// +kubebuilder:printcolumn:name="NodeStatus",type="string",JSONPath=".status.nodeStatus.printColumn",description="The status of the Node assigned to LRN"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LogicalResourceNode is the Schema for the logicalresourcenode API
type LogicalResourceNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LogicalResourceNodeSpec   `json:"spec,omitempty"`
	Status LogicalResourceNodeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LogicalResourceNodeList contains a list of LogicalResourceNode
type LogicalResourceNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LogicalResourceNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LogicalResourceNode{}, &LogicalResourceNodeList{})
}
