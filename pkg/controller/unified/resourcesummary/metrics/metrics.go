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
	"github.com/prometheus/client_golang/prometheus/promauto"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var ResourceSummaryNumNodes = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "resource_summary_num_nodes",
		Help: "resource_summary_num_nodes",
	},
	[]string{"resource_summary_name", "is_dry_run"},
)

var ResourceSummaryPhase = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "resource_summary_phase",
		Help: "resource_summary_phase",
	},
	[]string{"resource_summary_name", "is_dry_run", "phase"},
)

var ResourceSummaryResource = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "resource_summary_resource",
		Help: "resource_summary_resource",
	},
	[]string{"resource_summary_name", "is_dry_run", "priority_class", "resource_dimension", "resource_name"},
)

var ResourceSummaryResourceSpec = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "resource_summary_resource_spec",
		Help: "resource_summary_resource_spec",
	},
	[]string{"resource_summary_name", "is_dry_run", "resource_spec_name", "priority_class"},
)

var ResourceSummaryPodUsed = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "resource_summary_pod_used",
		Help: "resource_summary_pod_used",
	},
	[]string{"resource_summary_name", "is_dry_run", "pod_used_name", "priority_class", "resource_name"},
)

// RegisterVolumeSchedulingMetrics is used for scheduler, because the volume binding cache is a library
// used by scheduler process.
func init() {
	metrics.Registry.MustRegister(ResourceSummaryNumNodes)
	metrics.Registry.MustRegister(ResourceSummaryPhase)
	metrics.Registry.MustRegister(ResourceSummaryResource)
	metrics.Registry.MustRegister(ResourceSummaryResourceSpec)
	metrics.Registry.MustRegister(ResourceSummaryPodUsed)
}
