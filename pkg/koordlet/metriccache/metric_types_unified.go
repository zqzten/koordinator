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

package metriccache

const (
	// Resource satisfaction
	PodMetricCPUSatisfaction             MetricKind = "pod_cpu_satisfaction"
	PodMetricCPUSatisfactionWithThrottle MetricKind = "pod_cpu_satisfaction_with_throttle"
	PodMetricCPUCfsSatisfaction          MetricKind = "pod_cpu_cfs_satisfaction"
)

var (
	// Resource satisfaction

	// PodCPUCfsSatisfactionMetric calculates the CPU CFS (Completely Fair Scheduler) satisfaction rate,
	// which measures how well CPU scheduling resources meet the requirements.
	PodCPUCfsSatisfactionMetric = defaultMetricFactory.New(PodMetricCPUCfsSatisfaction).withPropertySchema(MetricPropertyPodUID)

	// PodCPUSatisfactionMetric calculates the overall CPU subsystem satisfaction rate,
	// taking into account both scheduling interference and Hyper-Threading (HT) influences.
	PodCPUSatisfactionMetric = defaultMetricFactory.New(PodMetricCPUSatisfaction).withPropertySchema(MetricPropertyPodUID)

	// PodCPUSatisfactionWithThrottleMetric adjusts the PodCPUSatisfactionMetric in the presence of the "ht-quota aware" kernel feature.
	// This feature may introduce additional throttle time (throttledUSecDelta), which can cause the thread to wait.
	// Hence, this throttle time is added to the denominator to reflect the reduced CPU availability.
	// This metric is not meaningful when the "ht-quota aware" feature is disabled, as there is no additional throttle time to consider.
	PodCPUSatisfactionWithThrottleMetric = defaultMetricFactory.New(PodMetricCPUSatisfactionWithThrottle).withPropertySchema(MetricPropertyPodUID)
)
