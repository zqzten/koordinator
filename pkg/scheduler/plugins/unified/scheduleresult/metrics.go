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

package scheduleresult

import (
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

// VolumeSchedulerSubsystem - subsystem name used by scheduler
const ResultSchedulerSubsystem = "koord_scheduler_result"

var (
	scheduleAttempts = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Subsystem:      ResultSchedulerSubsystem,
			Name:           "schedule_attempts_total",
			Help:           "Number of attempts to schedule pods.",
			StabilityLevel: metrics.ALPHA,
		}, []string{"retry", "result"})
)

// RegisterVolumeSchedulingMetrics is used for scheduler, because the volume binding cache is a library
// used by scheduler process.
func init() {
	legacyregistry.MustRegister(scheduleAttempts)
}
