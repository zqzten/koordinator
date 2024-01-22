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
	"fmt"

	"github.com/koordinator-sh/koordinator/apis/configuration"
)

const (
	// CPUStableExtKey is the key of nodeSLO extend config map for the CPUStable strategy.
	CPUStableExtKey = "cpuStable"
	// CPUStableConfigKey is the key of slo-controller config map for the CPUStable strategy.
	CPUStableConfigKey = "cpu-stable-config"

	// AnnotationPodCPUStable is the annotation key of pod-level cpu stable strategy.
	AnnotationPodCPUStable = "koordinator.sh/cpuStable"
)

// +k8s:deepcopy-gen=true
type PodCPUStableStrategy struct {
	CPUStableStrategy `json:",inline"`
}

func GetPodCPUStableStrategy(annotations map[string]string) (*PodCPUStableStrategy, error) {
	if annotations == nil {
		return nil, nil
	}
	data, ok := annotations[AnnotationPodCPUStable]
	if !ok {
		return nil, nil
	}
	strategy := &PodCPUStableStrategy{}
	if err := json.Unmarshal([]byte(data), strategy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pod strategy, err: %w", err)
	}
	return strategy, nil
}

// CPUStablePolicy indicates the policy of applying cpu stable strategy.
type CPUStablePolicy string

var (
	// CPUStablePolicyIgnore indicates the target pod ignores the CPUStable strategy should not update the cgroups.
	CPUStablePolicyIgnore CPUStablePolicy = "ignore"
	// CPUStablePolicyNone indicates the target pod disables the CPUStable strategy.
	CPUStablePolicyNone CPUStablePolicy = "none"
	// CPUStablePolicyAuto indicates the target pod enables the CPUStable strategy and auto set the cgroups.
	CPUStablePolicyAuto CPUStablePolicy = "auto"
)

// +k8s:deepcopy-gen=true
type CPUStablePIDConfig struct {
	// ProportionalPermill is the Proportional gain per-mill of the PID controller.
	// i.e. signalP := (ref - actual) * proportionalRatio.
	ProportionalPermill *int64 `json:"proportionalPermill,omitempty"`
	// IntegralPermill is the Integral gain per-mill of the PID controller.
	// i.e. signalI := lastSignalI + (ref - actual) * integralRatio * sampleInterval.
	IntegralPermill *int64 `json:"integralPermill,omitempty"`
	// DerivativePermill is the Derivative gain per-mill of the PID controller.
	// i.e. signalD := ((ref - actual) - (lastRef - lastActual)) * derivativeRatio / sampleInterval.
	DerivativePermill *int64 `json:"derivativePermill,omitempty"`
	// SignalNormalizedPercent is the normalized percentage of the PID controller which converts the signal of the
	// cpu satisfaction to the adjusted cpu.ht_ratio.
	// i.e. htRatioSignal := (signalP + signalI + signalD) * normalizedRatio.
	SignalNormalizedPercent *int64 `json:"signalNormalizedPercent,omitempty"`
}

// +k8s:deepcopy-gen=true
type CPUStableScaleModel struct {
	// TargetCPUSatisfactionPercent is the target CPU satisfaction percentage of a pod which we consider it "stable".
	// To be more specific, we check if the current CPU satisfaction of the pod is in the target range
	// [target - epsilon, target + epsilon]. If the current is less than the target range, a relax operation will be
	// triggered. If the current is greater than the target range, a throttle operation will be triggered.
	TargetCPUSatisfactionPercent *int64 `json:"targetCPUSatisfactionPercent,omitempty"`
	// CPUSatisfactionEpsilonPermill is the epsilon in per-mill of target CPU satisfaction.
	CPUSatisfactionEpsilonPermill *int64 `json:"cpuSatisfactionEpsilonPermill,omitempty"`
	// HTRatioUpperBound is the upper bound of the HT ratio for a pod.
	HTRatioUpperBound *int64 `json:"htRatioUpperBound,omitempty"`
	// HTRatioLowerBound is the lower bound of the HT ratio for a pod.
	HTRatioLowerBound *int64 `json:"htRatioLowerBound,omitempty"`
	// CPUUtilMinPercent is the minimum pod CPU utilization percentage of a pod below which we should abort the
	// scale operation since the pod should not be affected by the HT ratio.
	CPUUtilMinPercent *int64 `json:"cpuUtilMinPercent,omitempty"`

	// Configurations of the AIMD controller.
	// https://en.wikipedia.org/wiki/Additive_increase/multiplicative_decrease
	// AIMD should be used with the CPUSatisfactionEpsilonPermill to achieve convergence of final HT ratio when the
	// CPU satisfaction keep fluctuating.
	// HTRatioIncreasePercent is the Additive-Increasing ratio of HT ratio for throttling a pod.
	// i.e. nextHTRatio = currentHTRatio + baseRatio * HTRatioIncreasePercent / 100.
	HTRatioIncreasePercent *int64 `json:"htRatioIncreasePercent,omitempty"`
	// HTRatioIncreasePercent is the Multiplicative-Decreasing ratio of HT ratio for relaxing a pod.
	// i.e. nextHTRatio = currentHTRatio * HTRatioZoomOutPercent / 100.
	HTRatioZoomOutPercent *int64 `json:"htRatioZoomOutPercent,omitempty"`

	// PIDConfig is the configurations of the PID controller.
	// https://en.wikipedia.org/wiki/Proportional%E2%80%93integral%E2%80%93derivative_controller
	PIDConfig CPUStablePIDConfig `json:"pidConfig,omitempty"`
}

// +k8s:deepcopy-gen=true
type CPUStableWarmupEntry struct {
	// SharePoolUtilPercent is the lower bound of the share pool util percentage of the warmup entry.
	// If the current share pool util is no less than this value and less than the next entry's value, the htRatio
	// in this entry is picked as the warm state.
	SharePoolUtilPercent *int64 `json:"sharePoolUtilPercent,omitempty"`
	HTRatio              *int64 `json:"htRatio,omitempty"`
}

// +k8s:deepcopy-gen=true
type CPUStableWarmupModel struct {
	// Entries are the warm-up configurations defined according to share pool utilization in ascending order.
	Entries []CPUStableWarmupEntry `json:"entries,omitempty"`
}

// +k8s:deepcopy-gen=true
type CPUStableStrategy struct {
	// Policy indicates which policy to apply for the cpu stable strategy.
	// If no set, use "ignore", which skips setting related cgroups like the ht_ratio for the pod.
	Policy *CPUStablePolicy `json:"policy,omitempty"`
	// PodInitializeWindowSeconds is the time window size to determine whether the pod is initializing and can use the
	// warm strategy when metric points are not enough.
	PodInitializeWindowSeconds *int64 `json:"podInitializeWindowSeconds,omitempty"`
	// PodMetricWindowSeconds is the time window size by which the strategy query for a pod from the metric cache to
	// determine the scale operation.
	PodMetricWindowSeconds *int64 `json:"podMetricWindowSeconds,omitempty"`
	// ValidMetricPointsPercent is the percentage of valid metric points of a pod no less than which a throttle or relax
	// operation can be triggered.
	ValidMetricPointsPercent *int64 `json:"validMetricPointsPercent,omitempty"`
	// PodDegradeWindowSeconds is the time window size for which a pod keeps no valid metrics is considered as degraded.
	PodDegradeWindowSeconds *int64 `json:"podDegradeWindowSeconds,omitempty"`
	// CPUStableScaleModel is the configuration about scaling the pod ht_ratio for a target CPU satisfaction.
	CPUStableScaleModel `json:",inline"`
	// WarmupConfig is the configuration about warming up the pod ht_ratio according to share pool utilization.
	WarmupConfig CPUStableWarmupModel `json:"warmupConfig,omitempty"`
}

// +k8s:deepcopy-gen=true
type NodeCPUStableStrategy struct {
	// an empty label selector matches all objects while a nil label selector matches no objects
	configuration.NodeCfgProfile `json:",inline"`
	*CPUStableStrategy           `json:",inline"`
}

// CPUStableCfg defines the configuration for cpu stable strategy for LS pods.
// +k8s:deepcopy-gen=true
type CPUStableCfg struct {
	ClusterStrategy *CPUStableStrategy      `json:"clusterStrategy,omitempty"`
	NodeStrategies  []NodeCPUStableStrategy `json:"nodeStrategies,omitempty"`
}
