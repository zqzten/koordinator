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
	"sort"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// UncheckedCollector is a collector which is allowed to register the same metric name with different label names.
type UncheckedCollector struct {
	prometheus.Collector
}

// NewUncheckedCollector creates a UncheckedCollector wrapped from the given Collector.
func NewUncheckedCollector(collector prometheus.Collector) *UncheckedCollector {
	return &UncheckedCollector{
		Collector: collector,
	}
}

// Describe return nothing to mark as unchecked Collector and skip validation during Register.
func (v *UncheckedCollector) Describe(_ chan<- *prometheus.Desc) {
}

func (v *UncheckedCollector) GetCollector() prometheus.Collector {
	return v.Collector
}

type NewCollectorFunc func(labelNames []string) prometheus.Collector

// LooseLabelsCollector is a collector which can register the same metric name with different label names.
// LooseLabelsCollector is to expose a metric of same name with different labels names. The default metric vectors
// have the following constraints:
// 1. Default registry does not allow a metric of same name registered with different label names.
// 2. Default gauge vector does not allow a metric of same name record with different label names.
// https://github.com/prometheus/client_java/issues/696
// The LooseLabelsCollector achieve to refresh its label names via re-register an unchecked collector.
type LooseLabelsCollector struct {
	RWMutex         sync.RWMutex
	UncheckedVec    *UncheckedCollector
	KnownLabelNames map[string]struct{}
	NewCollector    NewCollectorFunc
	Registry        prometheus.Registerer
}

func NewLooseLabelsGaugeVec(opts prometheus.GaugeOpts, labels []string, registerer prometheus.Registerer) *LooseLabelsCollector {
	return NewLooseLabelsCollector(func(labels []string) prometheus.Collector {
		return prometheus.NewGaugeVec(opts, labels)
	}, labels, registerer)
}

func NewLooseLabelsCounterVec(opts prometheus.CounterOpts, labels []string, registerer prometheus.Registerer) *LooseLabelsCollector {
	return NewLooseLabelsCollector(func(labels []string) prometheus.Collector {
		return prometheus.NewCounterVec(opts, labels)
	}, labels, registerer)
}

func NewLooseLabelsCollector(newCollectorFn NewCollectorFunc, labels []string, registerer prometheus.Registerer) *LooseLabelsCollector {
	knownLabels := make(map[string]struct{})
	for _, labelName := range labels {
		knownLabels[labelName] = struct{}{}
	}
	sort.Strings(labels)
	c := NewUncheckedCollector(newCollectorFn(labels))
	registerer.MustRegister(c)
	return &LooseLabelsCollector{
		KnownLabelNames: knownLabels,
		UncheckedVec:    c,
		NewCollector:    newCollectorFn,
		Registry:        registerer,
	}
}

func (m *LooseLabelsCollector) GetMetricVec() prometheus.Collector {
	m.RWMutex.RLock()
	defer m.RWMutex.RUnlock()
	return m.UncheckedVec.GetCollector()
}

func (m *LooseLabelsCollector) FillKnownLabels(labels prometheus.Labels) {
	m.RWMutex.RLock()
	defer m.RWMutex.RUnlock()
	for labelName := range m.KnownLabelNames {
		if _, ok := labels[labelName]; !ok {
			labels[labelName] = ""
		}
	}
}

// RefreshLabelsIfNeed checks if the given labels are already registered (including the current labels is a superset of
// the given labels). If not, it will re-register the collector with the given labels.
func (m *LooseLabelsCollector) RefreshLabelsIfNeed(labels prometheus.Labels) bool {
	if m.HasLabels(labels) {
		return false
	}
	m.RefreshLabels(labels)
	return true
}

// HasLabels checks if the given labels are already registered (including the current labels is a superset of
// the given labels).
func (m *LooseLabelsCollector) HasLabels(labels prometheus.Labels) bool {
	m.RWMutex.RLock()
	defer m.RWMutex.RUnlock()
	for labelName := range labels {
		if _, ok := m.KnownLabelNames[labelName]; !ok {
			return false
		}
	}
	return true
}

// RefreshLabels re-registers the collector with the given labels.
func (m *LooseLabelsCollector) RefreshLabels(labels prometheus.Labels) {
	var labelNames []string
	knownLabelMap := make(map[string]struct{})
	for labelName := range labels {
		knownLabelMap[labelName] = struct{}{}
		labelNames = append(labelNames, labelName)
	}
	sort.Strings(labelNames)

	m.RWMutex.Lock()
	defer m.RWMutex.Unlock()
	m.KnownLabelNames = knownLabelMap
	m.Registry.Unregister(m.UncheckedVec)
	m.UncheckedVec = NewUncheckedCollector(m.NewCollector(labelNames))
	m.Registry.MustRegister(m.UncheckedVec)
}
