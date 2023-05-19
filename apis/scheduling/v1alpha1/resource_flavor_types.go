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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResourceFlavorConf struct {
	Name              string               `json:"name,omitempty"`
	NodeNum           int                  `json:"nodeNum,omitempty"`
	ForceAddNodes     []string             `json:"forceAddNodes,omitempty"`
	SameTopologyFirst bool                 `json:"sameTopologyFirst,omitempty"`
	NodeAffinity      *corev1.NodeAffinity `json:"nodeAffinity,omitempty"`
	Toleration        []corev1.Toleration  `json:"toleration,omitempty"`
}

type ResourceFlavorSpec struct {
	Enable bool `json:"enable,omitempty"`
	//key is confName, a quota may be have multiple node partitions.
	Configs map[string]*ResourceFlavorConf `json:"configs,omitempty"`
}

type SelectedNodeMeta struct {
	//this is a user-defined information map, user can put any key/value pair in to it.
	NodeMetaInfo map[string]string `json:"nodeMetaInfo,omitempty"`
}

type ResourceFlavorConfStatus struct {
	SelectedNodeNum int `json:"selectedNodeNum,omitempty"`
	//key is nodeName
	SelectedNodes map[string]*SelectedNodeMeta `json:"SelectedNodes,omitempty"`
}

type ResourceFlavorStatus struct {
	//key is confName, a quota may be have multiple node partitions.
	ConfigStatuses map[string]*ResourceFlavorConfStatus `json:"configStatuses,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster

type ResourceFlavor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ResourceFlavorSpec   `json:"spec,omitempty"`
	Status            ResourceFlavorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type ResourceFlavorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceFlavor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceFlavor{}, &ResourceFlavorList{})
}
