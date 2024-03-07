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
	"github.com/koordinator-sh/koordinator/apis/configuration"
)

const (
	// ACSSystemExtKey is the key of nodeSLO extend config map
	ACSSystemExtKey = "acsSystemConfig"
	// ACSSystemConfigKey is the key of slo-controller config map
	ACSSystemConfigKey = "acs-system-config"
)

// +k8s:deepcopy-gen=true
type ACSSystem struct {
	// /proc/sys/kernel/sched_schedstats
	SchedSchedStats *int64 `json:"schedSchedStats,omitempty" validate:"omitempty,min=0,max=1"`
	// /proc/sys/kernel/sched_acpu
	SchedAcpu *int64 `json:"schedAcpu,omitempty" validate:"omitempty,min=0,max=1"`
}

// +k8s:deepcopy-gen=true
type ACSSystemStrategy struct {
	Enable    *bool `json:"enable,omitempty"`
	ACSSystem `json:",inline"`
}

// +k8s:deepcopy-gen=true
type NodeACSSystemStrategy struct {
	// an empty label selector matches all objects while a nil label selector matches no objects
	configuration.NodeCfgProfile `json:",inline"`
	*ACSSystemStrategy           `json:",inline"`
}

// ACSSystemCfg defines the configuration for ACS node system configuration.
// +k8s:deepcopy-gen=true
type ACSSystemCfg struct {
	ClusterStrategy *ACSSystemStrategy      `json:"clusterStrategy,omitempty"`
	NodeStrategies  []NodeACSSystemStrategy `json:"nodeStrategies,omitempty"`
}
