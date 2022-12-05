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

package metrics

import (
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	schedulingmetrics "k8s.io/kubernetes/pkg/controller/volume/scheduling/metrics"
)

// VolumeSchedulerSubsystem - subsystem name used by scheduler
const VolumeSchedulerSubsystem = "koord_scheduler_volume"

var (
	// VolumeSchedulingStageLatency tracks the latency of volume scheduling operations.
	VolumeSchedulingStageLatency = metrics.NewHistogramVec(
		&metrics.HistogramOpts{
			Subsystem:         VolumeSchedulerSubsystem,
			Name:              "scheduling_duration_seconds",
			Help:              "Volume scheduling stage latency (Deprecated since 1.19.0)",
			Buckets:           metrics.ExponentialBuckets(0.001, 2, 15),
			StabilityLevel:    metrics.ALPHA,
			DeprecatedVersion: "1.19.0",
		},
		[]string{"operation"},
	)
	// VolumeSchedulingStageFailed tracks the number of failed volume scheduling operations.
	VolumeSchedulingStageFailed = schedulingmetrics.VolumeSchedulingStageFailed
)

// RegisterVolumeSchedulingMetrics is used for scheduler, because the volume binding cache is a library
// used by scheduler process.
func init() {
	legacyregistry.MustRegister(VolumeSchedulingStageLatency)
}
