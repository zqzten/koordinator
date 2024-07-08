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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ReserveResourceSpec defines the desired state of ReserveResource
type ReserveResourceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Template is the object that describes the pod that will be created.
	Template *corev1.PodTemplateSpec `json:"template,omitempty"`

	// 预留资源信息，如果为空以Template统计为准，如果不为空会覆盖Template的信息
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// 预留资源作用在哪个节点，如果不写，可以让调度器选择
	NodeName string `json:"nodeName,omitempty"`

	// 碎片整理，选择被整理的对象。或者抢占场景下被抢占的对象
	ResourceFrom []AllocMeta `json:"resourceFrom,omitempty"`

	// 预留资源的使用权
	ResourceOwners []ResourceOwner `json:"resourceOwners,omitempty"`

	// 预留资源的有效时长
	TimeToLiveDuration *metav1.Duration `json:"timeToLiveDuration,omitempty"`

	// 指定时间被删除
	Deadline *metav1.Time `json:"deadline,omitempty"`

	// 优先级
	Priority *int32 `json:"priority,omitempty"`

	// 指定调度器名来处理
	SchedulerName string `json:"schedulerName,omitempty"`

	// 是否跳过校验节点的free+ResourceFrom >= Resources
	SkipValidateFreeResource bool `json:"skipCheckFreeResource,omitempty"`
}

// 描述这个资源应该让谁来使用
type ResourceOwner struct {
	AllocMeta *AllocMeta `json:"allocMeta,omitempty"`

	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
	// metav1.OwnerReference (Namespace +'/' + Name + '/' + 'Kind')
	ControllerKey string `json:"controllerKey,omitempty"`

	Namespaces []string `json:"namespaces,omitempty"`
}

type AllocMeta struct {
	UID       types.UID `json:"uid,omitempty"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
}

// ReserveResourceStatus defines the observed state of ReserveResource
type ReserveResourceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	State ReserveResourcePhase `json:"state,omitempty"`

	Allocs []AllocMeta `json:"allocs,omitempty"`

	// 预留失败的信息
	Conditions []ReserveResourceCondition `json:"conditions,omitempty"`
}

type ReserveResourceCondition struct {
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime"`
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	// +optional
	Reason string `json:"reason"`
	// +optional
	Message string `json:"message"`
}

type ReserveResourcePhase string

const (
	ReserveResourcePhasePending   ReserveResourcePhase = "Pending"   // wait for process
	ReserveResourcePhaseAvailable ReserveResourcePhase = "Available" // 预留生效
	ReserveResourcePhaseExpired   ReserveResourcePhase = "Expired"   // 设置为失效，过期失效
	ReserveResourcePhaseFailed    ReserveResourcePhase = "Failed"    // 预留失败
	ReserveResourcePhaseCompleted ReserveResourcePhase = "Completed" // 预留资源被消费完成
)

// +genclient
// +kubebuilder:object:root=true

// ReserveResource is the Schema for the reserveresources API
type ReserveResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReserveResourceSpec   `json:"spec,omitempty"`
	Status ReserveResourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ReserveResourceList contains a list of ReserveResource
type ReserveResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReserveResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ReserveResource{}, &ReserveResourceList{})
}
