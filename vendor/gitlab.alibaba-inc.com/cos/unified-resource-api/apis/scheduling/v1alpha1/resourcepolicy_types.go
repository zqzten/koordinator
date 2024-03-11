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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ResourcePolicySpec defines the desired state of ResourcePolicy
type ResourcePolicySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	ConsumerRef *v1.ObjectReference `json:"consumerRef,omitempty" protobuf:"bytes,1,opt,name=consumerRef"`
	Strategy    SchedulingStrategy  `json:"strategy,omitempty" protobuf:"bytes,2,opt,name=strategy"`
	Units       []Unit              `json:"units,omitempty" protobuf:"bytes,3,opt,name=units"`
	Selector    map[string]string   `json:"selector,omitempty" protobuf:"bytes,4,rep,name=selector"`
}

// ResourcePolicyStatus defines the observed state of ResourcePolicy
type ResourcePolicyStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ResourcePolicy is the Schema for the resourcepolicies API
type ResourcePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourcePolicySpec   `json:"spec,omitempty"`
	Status ResourcePolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ResourcePolicyList contains a list of ResourcePolicy
type ResourcePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourcePolicy `json:"items"`
}

type SchedulingStrategy string

const (
	Prefer   SchedulingStrategy = "prefer"
	Required SchedulingStrategy = "required"
)

type ResourceType string

const (
	ECI ResourceType = "eci"
	ECS ResourceType = "ecs"
)

type Unit struct {
	Resource     ResourceType      `json:"resource,omitempty" protobuf:"bytes,1,opt,name=resource"`
	Min          *int32            `json:"min,omitempty" protobuf:"varint,2,opt,name=min"`
	Max          *int32            `json:"max,omitempty" protobuf:"varint,3,opt,name=max"`
	NodeSelector map[string]string `json:"nodeSelector,omitempty" protobuf:"bytes,4,rep,name=nodeSelector"`
	SpotInstance bool              `json:"spotInstance,omitempty" protobuf:"varint,5,opt,name=spotInstance"`
}

func init() {
	SchemeBuilder.Register(&ResourcePolicy{}, &ResourcePolicyList{})
}
