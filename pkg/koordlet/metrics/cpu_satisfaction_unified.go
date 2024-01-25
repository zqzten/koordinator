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
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
)

var (
	PodCPUCfsSatisfaction = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "pod_cpu_cfs_satisfaction",
		Help:      "Pod cpu cfs satisfaction collected by koordlet",
	}, []string{NodeKey, PodUID, PodName, PodNamespace})

	PodCPUSatisfaction = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "pod_cpu_satisfaction",
		Help:      "Pod cpu subsystem satisfaction collected by koordlet",
	}, []string{NodeKey, PodUID, PodName, PodNamespace})

	PodCPUSatisfactionWithThrottle = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "pod_cpu_satisfaction_with_throttle",
		Help:      "Pod cpu subsystem satisfaction with ht-quota aware feature enabled collected by koordlet",
	}, []string{NodeKey, PodUID, PodName, PodNamespace})

	CPUSatisfactionCollectors = []prometheus.Collector{
		PodCPUCfsSatisfaction,
		PodCPUSatisfaction,
		PodCPUSatisfactionWithThrottle,
	}
)

func ResetPodCPUCfsSatisfaction() {
	PodCPUCfsSatisfaction.Reset()
}

func RecordPodCPUCfsSatisfaction(pod *corev1.Pod, s float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	labels[PodUID] = string(pod.UID)
	labels[PodName] = pod.Name
	labels[PodNamespace] = pod.Namespace

	PodCPUCfsSatisfaction.With(labels).Set(s)
}

func ResetPodCPUSatisfaction() {
	PodCPUSatisfaction.Reset()
}

func RecordPodCPUSatisfaction(pod *corev1.Pod, s float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	labels[PodUID] = string(pod.UID)
	labels[PodName] = pod.Name
	labels[PodNamespace] = pod.Namespace

	PodCPUSatisfaction.With(labels).Set(s)
}

func ResetPodCPUSatisfactionWithThrottle() {
	PodCPUSatisfactionWithThrottle.Reset()
}

func RecordPodCPUSatisfactionWithThrottle(pod *corev1.Pod, s float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	labels[PodUID] = string(pod.UID)
	labels[PodName] = pod.Name
	labels[PodNamespace] = pod.Namespace

	PodCPUSatisfactionWithThrottle.With(labels).Set(s)
}
