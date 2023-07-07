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

	"github.com/koordinator-sh/koordinator/pkg/util/metrics"
)

var (
	LRNResourceAllocatable = metrics.NewGCGaugeVec("lrn_resource_allocatable", prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "lrn_resource_allocatable",
		Help:      "the resource allocatable of the LRN",
	}, []string{NodeKey, LRNKey, ResourceKey, UnitKey}))

	LRNPods = metrics.NewGCGaugeVec("lrn_pods", prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "lrn_pods",
		Help:      "the pods belonging to the LRN",
	}, []string{NodeKey, LRNKey, PodUID, PodName, PodNamespace}))

	LRNContainers = metrics.NewGCGaugeVec("lrn_containers", prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "lrn_containers",
		Help:      "the containers belonging to the LRN",
	}, []string{NodeKey, LRNKey, PodUID, PodName, PodNamespace, ContainerID, ContainerName}))

	LRNAccelerators = metrics.NewGCGaugeVec("lrn_containers", prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "lrn_accelerators",
		Help:      "the accelerators belonging to the LRN",
	}, []string{NodeKey, LRNKey, AcceleratorMinorKey, AcceleratorTypeKey}))

	LRNCollectors = []prometheus.Collector{
		LRNResourceAllocatable.GetGaugeVec(),
		LRNPods.GetGaugeVec(),
		LRNContainers.GetGaugeVec(),
		LRNAccelerators.GetGaugeVec(),
	}
)

func RecordLRNResourceAllocatable(lrnName string, resourceName string, unit string, value float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	labels[LRNKey] = lrnName
	labels[ResourceKey] = resourceName
	labels[UnitKey] = unit
	LRNResourceAllocatable.WithSet(labels, value)
}

func RecordLRNPods(lrnName string, pod *corev1.Pod) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	labels[LRNKey] = lrnName
	labels[PodUID] = string(pod.UID)
	labels[PodName] = pod.Name
	labels[PodNamespace] = pod.Namespace
	LRNPods.WithSet(labels, 1.0)
}

func RecordLRNContainers(lrnName string, status *corev1.ContainerStatus, pod *corev1.Pod) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	labels[LRNKey] = lrnName
	labels[PodUID] = string(pod.UID)
	labels[PodName] = pod.Name
	labels[PodNamespace] = pod.Namespace
	labels[ContainerID] = status.ContainerID
	labels[ContainerName] = status.Name
	LRNContainers.WithSet(labels, 1.0)
}

func RecordLRNAccelerators(lrnName string, deviceType string, acceleratorMinor string) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	labels[LRNKey] = lrnName
	labels[AcceleratorMinorKey] = acceleratorMinor
	labels[AcceleratorTypeKey] = deviceType
	LRNAccelerators.WithSet(labels, 1.0)
}
