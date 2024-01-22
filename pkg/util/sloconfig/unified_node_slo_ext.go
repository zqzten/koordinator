package sloconfig

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

var (
	defaultDeadlineDuration = metav1.Duration{
		Duration: time.Hour * 24,
	}
)

func init() {
	RegisterDefaultExtensionsMap(unified.DeadlineEvictExtKey, &unified.DeadlineEvictStrategy{
		Enable: pointer.Bool(false),
		DeadlineEvictConfig: unified.DeadlineEvictConfig{
			DeadlineDuration: &defaultDeadlineDuration,
		},
	})
	RegisterDefaultExtensionsMap(unified.ACSSystemExtKey, DefaultACSSystemStrategy())
	RegisterDefaultExtensionsMap(unified.CPUStableExtKey, DefaultCPUStableStrategy())
}

func NoneACSSystemStrategy() *unified.ACSSystemStrategy {
	return &unified.ACSSystemStrategy{
		Enable: pointer.Bool(false),
		ACSSystem: unified.ACSSystem{
			SchedSchedStats: pointer.Int64(1),
			SchedAcpu:       pointer.Int64(0),
		},
	}
}

func DefaultCPUStableStrategy() *unified.CPUStableStrategy {
	return &unified.CPUStableStrategy{
		Policy:                     CPUStablePolicyPtr(unified.CPUStablePolicyIgnore),
		PodMetricWindowSeconds:     pointer.Int64(3),
		ValidMetricPointsPercent:   pointer.Int64(80),
		PodDegradeWindowSeconds:    pointer.Int64(1800),
		PodInitializeWindowSeconds: pointer.Int64(10),
		CPUStableScaleModel: unified.CPUStableScaleModel{
			TargetCPUSatisfactionPercent:  pointer.Int64(75),
			CPUSatisfactionEpsilonPermill: pointer.Int64(20),
			HTRatioUpperBound:             pointer.Int64(200),
			HTRatioLowerBound:             pointer.Int64(100),
			CPUUtilMinPercent:             pointer.Int64(40),
			// AIMD
			HTRatioIncreasePercent: pointer.Int64(10),
			HTRatioZoomOutPercent:  pointer.Int64(90),
			// PID
			PIDConfig: unified.CPUStablePIDConfig{
				ProportionalPermill:     pointer.Int64(800),
				IntegralPermill:         pointer.Int64(40),
				DerivativePermill:       pointer.Int64(100),
				SignalNormalizedPercent: pointer.Int64(-10000),
			},
		},
		WarmupConfig: unified.CPUStableWarmupModel{
			Entries: []unified.CPUStableWarmupEntry{
				{
					SharePoolUtilPercent: pointer.Int64(0),
					HTRatio:              pointer.Int64(150),
				},
				{
					SharePoolUtilPercent: pointer.Int64(6),
					HTRatio:              pointer.Int64(140),
				},
				{
					SharePoolUtilPercent: pointer.Int64(12),
					HTRatio:              pointer.Int64(130),
				},
				{
					SharePoolUtilPercent: pointer.Int64(18),
					HTRatio:              pointer.Int64(120),
				},
				{
					SharePoolUtilPercent: pointer.Int64(24),
					HTRatio:              pointer.Int64(100),
				},
			},
		},
	}
}

func DefaultACSSystemStrategy() *unified.ACSSystemStrategy {
	return &unified.ACSSystemStrategy{
		Enable: pointer.Bool(false),
		ACSSystem: unified.ACSSystem{
			SchedSchedStats: pointer.Int64(1),
			SchedAcpu:       pointer.Int64(1),
		},
	}
}

func CPUStablePolicyPtr(policy unified.CPUStablePolicy) *unified.CPUStablePolicy {
	return &policy
}
