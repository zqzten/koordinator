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
	"encoding/json"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/koordinator-sh/koordinator/apis/configuration"
)

const (
	// DeadlineEvictExtKey is the key of nodeSLO extend config map
	DeadlineEvictExtKey = "deadlineEvict"
	// DeadlineEvictConfigKey is the key of slo-controller config map
	DeadlineEvictConfigKey = "deadline-evict-config"
	// AnnotationPodDeadlineEvictKey is key of pod for deadline evict
	// TODO not used yet until webhook forbid this from user
	AnnotationPodDeadlineEvictKey = "alibabacloud.com/" + DeadlineEvictExtKey
	// AnnotationPodDurationBeforeEvictionKey is the key of duration when sending event before eviction
	AnnotationPodDurationBeforeEvictionKey = "alibabacloud.com/duration-before-eviction"
	// DefaultPodDurationBeforeEviction is the default duration before sending eviction
	DefaultPodDurationBeforeEviction = time.Minute * 10

	// followings are fore evicition controller
	AnnotationEvictionTypeKey              = "alibabacloud.com/eviction-type"
	AnnotationEvictionTypeInvoluntary      = "involuntary"
	AnnotationSkipNotReadyFlowControlKey   = "alibabacloud.com/skip-not-ready-flow-control" // true or false
	AnnotationSkipNotReadyFlowControlValue = "true"
	AnnotationEvictionMessageKey           = "alibabacloud.com/eviction-message"
	AnnotationEvictionConditionKey         = "alibabacloud.com/eviction-condition"
	AnnotationEvictionConditionValue       = "true"
	LabelEvictionKey                       = "alibabacloud.com/eviction"
	LabelEvictionValue                     = "true"
)

// +k8s:deepcopy-gen=true
type DeadlineEvictConfig struct {
	DeadlineDuration *metav1.Duration `json:"deadlineDuration,omitempty"`
}

// +k8s:deepcopy-gen=true
type DeadlineEvictStrategy struct {
	Enable              *bool `json:"enable,omitempty"`
	DeadlineEvictConfig `json:",inline"`
}

// +k8s:deepcopy-gen=true
type NodeDeadlineEvictStrategy struct {
	// an empty label selector matches all objects while a nil label selector matches no objects
	configuration.NodeCfgProfile `json:",inline"`
	*DeadlineEvictStrategy       `json:",inline"`
}

// DeadlineEvictCfg defines the configuration for evict BE type pods with deadline.
// +k8s:deepcopy-gen=true
type DeadlineEvictCfg struct {
	ClusterStrategy *DeadlineEvictStrategy      `json:"clusterStrategy,omitempty"`
	NodeStrategies  []NodeDeadlineEvictStrategy `json:"nodeStrategies,omitempty"`
}

func GetPodDeadlineEvictStrategy(pod *corev1.Pod) (*DeadlineEvictStrategy, error) {
	if pod == nil || pod.Annotations == nil {
		return nil, nil
	}
	deadlineEvictStr, exist := pod.Annotations[AnnotationPodDeadlineEvictKey]
	if !exist {
		return nil, nil
	}
	deadlineEvictStrategy := &DeadlineEvictStrategy{}

	err := json.Unmarshal([]byte(deadlineEvictStr), deadlineEvictStrategy)
	if err != nil {
		return nil, err
	}
	return deadlineEvictStrategy, nil
}

func GetPodDurationBeforeEviction(pod *corev1.Pod) (*time.Duration, error) {
	if pod == nil || pod.Annotations == nil {
		return nil, nil
	}
	durationBeforeEvictionStr, exist := pod.Annotations[AnnotationPodDurationBeforeEvictionKey]
	if !exist {
		return nil, nil
	}
	durationBeforeEviction, err := time.ParseDuration(durationBeforeEvictionStr)
	if err != nil {
		return nil, err
	}
	return &durationBeforeEviction, nil
}
