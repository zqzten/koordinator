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

package v1beta3

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
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
	metav1.TypeMeta `json:",inline"`

	Network    string `json:"network,omitempty"`
	Address    string `json:"address,omitempty"`
	ServerCert []byte `json:"serverCert,omitempty"`
	ServerKey  []byte `json:"serverKey,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LimitAwareArgs defines the parameters for LimitAware plugin.
type LimitAwareArgs struct {
	metav1.TypeMeta

	// ScoringStrategy selects the node resource scoring strategy.
	ScoringResourceWeights map[corev1.ResourceName]int64 `json:"scoringResourceWeights,omitempty"`

	// DefaultLimitToAllocatable cluster-level limitToAllocatable
	DefaultLimitToAllocatable extension.LimitToAllocatable `json:"defaultLimitToAllocatable,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ASIQuotaAdaptorArgs struct {
	metav1.TypeMeta

	PriorityRangeConfig map[string]*PriorityRangeConfig `json:"priorityRangeConfig,omitempty"`
}

// PriorityRangeConfig 用来配置priority的一些抢占属性
type PriorityRangeConfig struct {
	// 开始的优先级，包含等于
	PriorityStart int `json:"priorityStart"`
	// 结束的优先级，包含等于
	PriorityEnd int `json:"priorityEnd"`
	// 是否触发抢占
	EnablePreempt bool `json:"enablePreempt"`
	// 是否允许被抢占
	EnableBePreempted bool `json:"enableBePreempted"`
	// 是否允许跨段抢占
	EnableCrossBandPreempt bool `json:"enableCrossBandPreempt"`
	// 默认的驱逐策略
	DefaultEvictAction string `json:"defaultEvictAction"`

	// PriorityLevel
	PriorityLevel int `json:"priorityLevel"`
}
