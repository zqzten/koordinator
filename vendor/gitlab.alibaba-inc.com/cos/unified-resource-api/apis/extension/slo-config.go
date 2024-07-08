package extension

import (
	nodesv1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/nodes/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CalculatePolicy string

const (
	CalculateByPodRequest CalculatePolicy = "request"
	CalculateByPodUsage   CalculatePolicy = "usage"
)

// +k8s:deepcopy-gen=true
type SystemConfig struct {
	ClusterStrategy *nodesv1beta1.SystemStrategy `json:"clusterStrategy,omitempty"`
	NodeStrategies  []NodeSystemStrategy         `json:"nodeStrategies,omitempty"`
}

// +k8s:deepcopy-gen=true
type NodeSystemStrategy struct {
	NodeCfgProfile `json:",inline"`
	*nodesv1beta1.SystemStrategy
}

// +k8s:deepcopy-gen=true
type ResourceThresholdCfg struct {
	ClusterStrategy *nodesv1beta1.ResourceThresholdStrategy `json:"clusterStrategy,omitempty"`
	NodeStrategies  []NodeResourceThresholdStrategy         `json:"nodeStrategies,omitempty"`
}

// +k8s:deepcopy-gen=true
type NodeResourceThresholdStrategy struct {
	NodeCfgProfile `json:",inline"`
	*nodesv1beta1.ResourceThresholdStrategy
}

// +k8s:deepcopy-gen=true
type ResourceQOSCfg struct {
	ClusterStrategy *nodesv1beta1.ResourceQOSStrategy `json:"clusterStrategy,omitempty"`
	NodeStrategies  []NodeResourceQOSStrategy         `json:"nodeStrategies,omitempty"`
}

// +k8s:deepcopy-gen=true
type NodeResourceQOSStrategy struct {
	NodeCfgProfile `json:",inline"`
	*nodesv1beta1.ResourceQOSStrategy
}

// +k8s:deepcopy-gen=true
type NodeCPUBurstCfg struct {
	NodeCfgProfile `json:",inline"`
	*nodesv1beta1.CPUBurstStrategy
}

// +k8s:deepcopy-gen=true
type CPUBurstCfg struct {
	ClusterStrategy *nodesv1beta1.CPUBurstStrategy `json:"clusterStrategy,omitempty"`
	NodeStrategies  []NodeCPUBurstCfg              `json:"nodeStrategies,omitempty"`
}

// +k8s:deepcopy-gen=true
type ColocationStrategy struct {
	Enable                         *bool            `json:"enable,omitempty"`
	MetricAggregateDurationSeconds *int64           `json:"metricAggregateDurationSeconds,omitempty"`
	CPUReclaimThresholdPercent     *int64           `json:"cpuReclaimThresholdPercent,omitempty"`
	MemoryReclaimThresholdPercent  *int64           `json:"memoryReclaimThresholdPercent,omitempty"`
	MemoryCalculatePolicy          *CalculatePolicy `json:"memoryCalculatePolicy,omitempty"` // default by usage
	DegradeTimeMinutes             *int64           `json:"degradeTimeMinutes,omitempty"`
	UpdateTimeThresholdSeconds     *int64           `json:"updateTimeThresholdSeconds,omitempty"`
	ResourceDiffThreshold          *float64         `json:"resourceDiffThreshold,omitempty"`
}

// +k8s:deepcopy-gen=true
// ColocationCfg is the slo colocation global config
type ColocationCfg struct {
	// true : colocation disabled. false : colocation enabled.
	ColocationFreeze *bool `json:"colocationFreeze,omitempty"`

	ColocationStrategy

	NodeConfigs []NodeColocationCfg `json:"nodeConfigs,omitempty"`
}

// +k8s:deepcopy-gen=true
type NodeColocationCfg struct {
	NodeCfgProfile `json:",inline"`
	ColocationStrategy
}

// +k8s:deepcopy-gen=true
type NodeCfgProfile struct {
	Name string `json:"name,omitempty"`
	// an empty label selector matches all objects while a nil label selector matches no objects
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
}
