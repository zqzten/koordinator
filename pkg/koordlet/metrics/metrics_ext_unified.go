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

	uniquotav1 "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func init() {
	prometheus.MustRegister(LRNCollectors...)
	prometheus.MustRegister(UnifiedCollectors...)
}

const (
	LRNKey = "lrn"

	AcceleratorResource = "accelerator"
	AcceleratorMinorKey = "accelerator_minor"
	AcceleratorTypeKey  = "type"

	// required by PAI serverless
	// https://aliyuque.antfin.com/obdvnp/apfnvx/cz000wrokt9ziawf?singleDoc# 《逻辑节点（LRN）方案（v1.0）》
	GPUCardModelKey         = "gpu_card_model"
	NodeNameKey             = "node_name"
	SingleNodeAllocationKey = "single_node_allocation"
	ASWIDKey                = "asw_id"
	PointOfDeliveryKey      = "point_of_delivery"
	TenantDLCKey            = "tenant_dlc_alibaba_inc_com"
	MachineGroupKey         = "machinegroup"
	ResourceGroupKey        = "resourcegroup"
	QuotaIDKey              = "quota_id"
	QuotaNameKey            = "quota_name"
)

var (
	NodeResourceAllocatableCPUCores = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "node_resource_allocatable_cpu_cores",
		Help:      "the node allocatable of cpu",
	}, []string{NodeKey})

	NodeResourceAllocatableMemoryTotalBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "node_resource_allocatable_memory_total_bytes",
		Help:      "the node allocatable of memory",
	}, []string{NodeKey})

	NodeResourceAllocatableAcceleratorTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "node_resource_allocatable_accelerator_total",
		Help:      "the node allocatable of accelerator",
	}, []string{NodeKey})

	NodeResourceCapacityCPUCores = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "node_resource_capacity_cpu_cores",
		Help:      "the node capacity of cpu",
	}, []string{NodeKey})

	NodeResourceCapacityMemoryTotalBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "node_resource_capacity_memory_total_bytes",
		Help:      "the node capacity of memory",
	}, []string{NodeKey})

	NodeResourceCapacityAcceleratorTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: KoordletSubsystem,
		Name:      "node_resource_capacity_accelerator_total",
		Help:      "the node capacity of accelerators",
	}, []string{NodeKey})

	UnifiedCollectors = []prometheus.Collector{
		NodeResourceAllocatableCPUCores,
		NodeResourceAllocatableMemoryTotalBytes,
		NodeResourceAllocatableAcceleratorTotal,
		NodeResourceCapacityCPUCores,
		NodeResourceCapacityMemoryTotalBytes,
		NodeResourceCapacityAcceleratorTotal,
	}
)

func RecordNodeResourceAllocatableCPUCores(value float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	NodeResourceAllocatableCPUCores.With(labels).Set(value)
}

func RecordNodeResourceAllocatableMemoryTotalBytes(value float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	NodeResourceAllocatableMemoryTotalBytes.With(labels).Set(value)
}

func RecordNodeResourceAllocatableAcceleratorTotal(value float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	NodeResourceAllocatableAcceleratorTotal.With(labels).Set(value)
}

func RecordNodeResourceCapacityCPUCores(value float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	NodeResourceCapacityCPUCores.With(labels).Set(value)
}

func RecordNodeResourceCapacityMemoryTotalBytes(value float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	NodeResourceCapacityMemoryTotalBytes.With(labels).Set(value)
}

func RecordNodeResourceCapacityAcceleratorTotal(value float64) {
	labels := genNodeLabels()
	if labels == nil {
		return
	}
	NodeResourceCapacityAcceleratorTotal.With(labels).Set(value)
}

func setLRNLabels(labels map[string]string, lrn *schedulingv1alpha1.LogicalResourceNode) {
	labels[LRNKey] = lrn.Name
	//  GPUCardModelKey         = "gpu_card_model"
	//	NodeNameKey             = "node_name"
	//	SingleNodeAllocationKey = "single_node_allocation"
	//	ASWIDKey                = "asw_id"
	//	PointOfDeliveryKey      = "point_of_delivery"
	//	TenantDLCKey            = "tenant_dlc_alibaba_inc_com"
	//	MachineGroupKey         = "machinegroup"
	//	ResourceGroupKey        = "resourcegroup"
	//	QuotaIDKey              = "quota_id"
	//	QuotaNameKey            = "quota_name"
	if lrn.Labels == nil {
		labels[GPUCardModelKey] = ""
		labels[NodeNameKey] = ""
		labels[SingleNodeAllocationKey] = ""
		labels[ASWIDKey] = ""
		labels[PointOfDeliveryKey] = ""
		labels[TenantDLCKey] = ""
		labels[MachineGroupKey] = ""
		labels[ResourceGroupKey] = ""
		labels[QuotaIDKey] = ""
		labels[QuotaNameKey] = ""
	} else {
		labels[GPUCardModelKey] = lrn.Labels[unified.LabelGPUCardModel]
		labels[NodeNameKey] = lrn.Labels[schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode]
		labels[SingleNodeAllocationKey] = lrn.Labels[schedulingv1alpha1.LabelLogicalResourceNodeSingleNodeAllocation]
		labels[ASWIDKey] = lrn.Labels[unified.LabelNodeASWID]
		labels[PointOfDeliveryKey] = lrn.Labels[unified.LabelNodePointOfDelivery]
		labels[TenantDLCKey] = lrn.Labels[unified.LabelTenantDLC]
		labels[MachineGroupKey] = lrn.Labels[unified.LabelTenantDLCMachineGroup]
		labels[ResourceGroupKey] = lrn.Labels[unified.LabelTenantDLCResourceGroup]
		labels[QuotaIDKey] = lrn.Labels[uniquotav1.LabelQuotaID]
		labels[QuotaNameKey] = lrn.Labels[uniquotav1.LabelQuotaName]
	}
}
