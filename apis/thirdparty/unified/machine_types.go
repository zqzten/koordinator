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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "kubenode.alibabacloud.com", Version: "v1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// MachineSpec defines the desired state of Machine
type MachineSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Ip of the machine
	// +optional
	Ip string `json:"ip,omitempty"`

	// nodeName quote to the name of nodename
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// ExtraConfigs, such as ssh-key-ref
	// +optional
	ExtraConfigs map[string]string `json:"extraConfigs,omitempty"`

	// init config for machine
	// +optional
	InitConfig *InitConfig `json:"initConfig,omitempty"`

	// The list of the taints to be applied to the corresponding Node in additive
	// manner. This list will not overwrite any other taints added to the Node on
	// an ongoing basis by other entities. These taints should be actively reconciled
	// e.g. if you ask the machine controller to apply a taint and then manually remove
	// the taint the machine controller will put it back) but not have the machine controller
	// remove any taints
	// +optional
	TaintSpec TaintSpec `json:"taintSpec,omitempty"`

	//The list of the labels to be applied to the corresponding Node.
	// +optional
	LabelSpec LabelSpec `json:"labelSpec,omitempty"`

	// MachineComponentsSpec details package infos for the machine
	// +optional
	// +patchMergeKey=componentName
	// +patchStrategy=merge
	MachineComponentsSpec []MachineComponentRef `json:"machineComponentsSpec,omitempty" patchStrategy:"merge" patchMergeKey:"componentName"`
}

type InitConfig struct {
	InitScriptContent *string `json:"initScriptContent,omitempty"`

	InitScriptConfigMapRef *ConfigMapRef `json:"initScriptConfigMapRef,omitempty"`
}

type ConfigMapRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
}

type LabelSpec struct {
	// The list of the labels to be applied to the corresponding Node.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// The list of the labels which should be excluded in node's labels
	// +optional
	ExcludedLabels map[string]string `json:"excludedLabels,omitempty"`
}

type TaintSpec struct {
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`
	// taints should be excluded in the node
	// +optional
	ExcludedTaints []corev1.Taint `json:"excludedTaints,omitempty"`
}

type MachineComponentRef struct {
	// Name is the name of the package
	ComponentName string `json:"componentName"`
	// Component version
	ComponentVersion string `json:"componentVersion"`
}

type MachinePhase string

// These are the valid phases of machine.
// more info: https://yuque.antfin-inc.com/sigmahost/laow2e/qn916s#Fgvyn
const (
	// MachinePending means the machine has been created/added by the system, but not configured.
	MachinePending MachinePhase = "Pending"
	// MachineReady means the machine has been initialized, and it is ready to install components
	MachineReady MachinePhase = "Ready"
	// MachineRunning means the machine has been configured and all services are running.
	MachineRunning MachinePhase = "Running"
	// MachineError means the machine has one or more services are un-normal
	MachineError MachinePhase = "Error"
	// MachineTerminating means the machine has been removed from the cluster.
	MachineTerminating MachinePhase = "Terminating"
)

const ConditionUpgrading corev1.ConditionStatus = "Upgrading"

type MachineCondition struct {
	// Type of machine condition.
	Type string `json:"type"`
	// field for component condition, indicates the version of component
	// +optional
	Version string `json:"version"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time we got an update on a given condition.
	// +optional
	LastHeartbeatTime metav1.Time `json:"lastHeartbeatTime,omitempty"`
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
	// Current version exception count
	ExceptionCount int `json:"exceptionCount"`
}

// MachineStatus defines the observed state of Machine
type MachineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase is the recently observed lifecycle phase of the machine.
	Phase MachinePhase `json:"phase"`
	// Conditions is an array of current observed machine conditions.
	// +optional
	Conditions []MachineCondition `json:"conditions,omitempty"`

	// health status of the machine
	// +optional
	HealthStatus *HealthStatus `json:"healthStatus,omitempty"`
}

type HealthStatusSource string

const (
	HealthStatusSourceServer HealthStatusSource = "Server"
	HealthStatusSourceAgent  HealthStatusSource = "Agent"
)

type HealthStatus struct {
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time we got an update on a given condition.
	// +optional
	LastHeartbeatTime metav1.Time `json:"lastHeartbeatTime,omitempty"`
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
	// the source of health status
	// +optional
	Source HealthStatusSource `json:"source"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="NodeName",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="IP",type=string,JSONPath=`.spec.ip`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Machine is the Schema for the machine API
type Machine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MachineSpec `json:"spec,omitempty"`
	// status of machine
	// +optional
	Status MachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MachineList contains a list of Machine
type MachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Machine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Machine{}, &MachineList{})
}
