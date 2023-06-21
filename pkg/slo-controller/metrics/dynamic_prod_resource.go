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

func init() {
	MustRegister(DynamicProdResourceCollector...)
}

var (
	NodeProdResourceEstimatedOvercommitRatio = metrics.NewGCGaugeVec("node_extended_resource_allocatable_internal", prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: SLOControllerSubsystem,
		Name:      "node_prod_resource_estimated_overcommit_ratio",
		Help:      "the prod overcommit ratio estimated by the slo controller",
	}, []string{NodeKey, ResourceKey}))

	NodeProdResourceEstimatedAllocatable = metrics.NewGCGaugeVec("node_prod_resource_estimated_allocatable", prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: SLOControllerSubsystem,
		Name:      "node_prod_resource_estimated_allocatable",
		Help:      "the prod overcommit allocatable estimated by the slo controller",
	}, []string{NodeKey, ResourceKey, UnitKey}))

	NodeProdResourceReclaimable = metrics.NewGCGaugeVec("node_prod_resource_reclaimable", prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: SLOControllerSubsystem,
		Name:      "node_prod_resource_reclaimable",
		Help:      "the prod resource reclaimable",
	}, []string{NodeKey, ResourceKey, UnitKey}))

	DynamicProdResourceCollector = []prometheus.Collector{
		NodeProdResourceEstimatedOvercommitRatio.GetGaugeVec(),
		NodeProdResourceEstimatedAllocatable.GetGaugeVec(),
		NodeProdResourceReclaimable.GetGaugeVec(),
	}
)

func RecordNodeProdResourceEstimatedOvercommitRatio(node *corev1.Node, resourceName string, value float64) {
	labels := genNodeLabels(node)
	labels[ResourceKey] = resourceName
	NodeProdResourceEstimatedOvercommitRatio.WithSet(labels, value)
}

func RecordNodeProdResourceEstimatedAllocatable(node *corev1.Node, resourceName string, unit string, value float64) {
	labels := genNodeLabels(node)
	labels[ResourceKey] = resourceName
	labels[UnitKey] = unit
	NodeProdResourceEstimatedAllocatable.WithSet(labels, value)
}

func RecordNodeProdResourceReclaimable(node *corev1.Node, resourceName string, unit string, value float64) {
	labels := genNodeLabels(node)
	labels[ResourceKey] = resourceName
	labels[UnitKey] = unit
	NodeProdResourceReclaimable.WithSet(labels, value)
}
