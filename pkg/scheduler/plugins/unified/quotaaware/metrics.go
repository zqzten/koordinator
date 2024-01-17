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

package quotaaware

import (
	"k8s.io/component-base/metrics"
	schedulermetrics "k8s.io/kubernetes/pkg/scheduler/metrics"

	koordschedulermetrics "github.com/koordinator-sh/koordinator/pkg/scheduler/metrics"
)

var (
	UnexpectedSchedulingError = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Subsystem: schedulermetrics.SchedulerSubsystem,
			Name:      "unexpected_scheduling_error",
			Help:      "Unexpected scheduling error",
		},
		[]string{"profile", "user", "quotaID", "instanceType", "reason"},
	)
)

func init() {
	koordschedulermetrics.RegisterMetrics(
		UnexpectedSchedulingError,
	)
}
