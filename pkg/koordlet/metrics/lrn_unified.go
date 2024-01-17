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
	LRNAllocatableCPUCores = metrics.NewGCGaugeVec("lrn_allocatable_cpu_cores", prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "lrn_allocatable_cpu_cores",
		Help:      "the cpu resource allocatable of the LRN",
	}, []string{NodeKey, LRNKey}))

	LRNAllocatableMemoryTotalBytes = metrics.NewGCGaugeVec("lrn_allocatable_memory_total_bytes", prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "lrn_allocatable_memory_total_bytes",
		Help:      "the memory resource allocatable of the LRN",
	}, []string{NodeKey, LRNKey}))

	LRNAllocatableAcceleratorTotal = metrics.NewGCGaugeVec("lrn_allocatable_accelerator_total", prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "lrn_allocatable_accelerator_total",
		Help:      "the accelerator resource allocatable of the LRN",
	}, []string{NodeKey, LRNKey}))

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

	LRNAccelerators = metrics.NewGCGaugeVec("lrn_accelerators", prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "lrn_accelerators",
		Help:      "the accelerators belonging to the LRN",
	}, []string{NodeKey, LRNKey, AcceleratorMinorKey, AcceleratorTypeKey}))

	// NodeLRNs is a loose-labels collector for the metric node_lrns.
	// To invoke RefreshNodeLRNsLabels() before recording, it can refresh label names dynamically.
	// e.g.
	//   1. node_lrns{node="node-0", lrn="lrn-0", label_aaa="xxx"}, lrnLabels={"label_aaa": "yyy", "label_bbb": "zzz"}
	//   2. RefreshNodeLRNsLabels($lrnLabels)
	//   3. RecordNodeLRNs("lrn-0", $lrnLabels)
	//   4. node_lrns{node="node-0", lrn="lrn-0", label_aaa="yyy", label_bbb="zzz"}
	NodeLRNs = metrics.NewLooseLabelsGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "node_lrns",
		Help:      "the LRNs belonging to the node",
	}, []string{NodeKey, LRNKey, GPUCardModelKey, NodeNameKey, ASWIDKey, PointOfDeliveryKey, TenantDLCKey,
		MachineGroupKey, ResourceGroupKey, QuotaIDKey, QuotaNameKey}, prometheus.DefaultRegisterer)

	LRNCollectors = []prometheus.Collector{
		LRNAllocatableCPUCores.GetGaugeVec(),
		LRNAllocatableMemoryTotalBytes.GetGaugeVec(),
		LRNAllocatableAcceleratorTotal.GetGaugeVec(),
		LRNPods.GetGaugeVec(),
		LRNContainers.GetGaugeVec(),
		LRNAccelerators.GetGaugeVec(),
	}
)

func RecordLRNAllocatableCPUCores(lrnName string, value float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	labels[LRNKey] = lrnName
	LRNAllocatableCPUCores.WithSet(labels, value)
}

func RecordLRNAllocatableMemoryTotalBytes(lrnName string, value float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	labels[LRNKey] = lrnName
	LRNAllocatableMemoryTotalBytes.WithSet(labels, value)
}

func RecordLRNAllocatableAcceleratorTotal(lrnName string, value float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	labels[LRNKey] = lrnName
	LRNAllocatableAcceleratorTotal.WithSet(labels, value)
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

// RefreshNodeLRNsLabels refreshes the label names of NodeLRNs.
func RefreshNodeLRNsLabels(lrnLabels map[string]string) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	setLRNLabels(labels, "", lrnLabels)
	NodeLRNs.RefreshLabelsIfNeed(labels)
}

func RecordNodeLRNs(lrnName string, lrnLabels map[string]string) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	setLRNLabels(labels, lrnName, lrnLabels)
	NodeLRNs.FillKnownLabels(labels)
	NodeLRNs.GetMetricVec().(*prometheus.GaugeVec).With(labels).Set(1.0)
}

func ResetNodeLRNs() {
	NodeLRNs.GetMetricVec().(*prometheus.GaugeVec).Reset()
}
