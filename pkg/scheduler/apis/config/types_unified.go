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

package config

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UnifiedPodConstraintArgs holds arguments used to configure the UnifiedPodConstraint plugin.
type UnifiedPodConstraintArgs struct {
	metav1.TypeMeta

	// EnableDefaultPodConstraint indicates whether to enable defaultPodConstraint for unifiedPodConstraint.
	EnableDefaultPodConstraint *bool `json:"enableDefaultPodConstraint,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CachedPodArgs holds arguments used to configure the CachedPodArgs plugin.
type CachedPodArgs struct {
	metav1.TypeMeta

	Network    string
	Address    string
	ServerCert []byte
	ServerKey  []byte
}
