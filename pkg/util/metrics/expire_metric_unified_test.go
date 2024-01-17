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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestUncheckedCollector(t *testing.T) {
	t.Run("test", func(t *testing.T) {
		// verify a normal (checked) collector cannot register and de-register multiple times with a same metric name
		// while label names are different
		testRegistry := prometheus.NewRegistry()
		testVector := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "test_gauge_collector",
		}, []string{"test"})
		err := testRegistry.Register(testVector)
		assert.NoError(t, err)
		got := testRegistry.Unregister(testVector)
		assert.True(t, got)
		testVector1 := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "test_gauge_collector",
		}, []string{"test", "test1"})
		err = testRegistry.Register(testVector1)
		assert.Error(t, err)

		// verify an unchecked collector can register and de-register multiple times with a same metric name while
		// label names are different
		testUncheckedVector := NewUncheckedCollector(testVector)
		err = testRegistry.Register(testUncheckedVector)
		assert.NoError(t, err)
		got = testRegistry.Unregister(testUncheckedVector)
		assert.False(t, got) // false for an unchecked collector
		testUncheckedVector1 := NewUncheckedCollector(testVector1)
		err = testRegistry.Register(testUncheckedVector1)
		assert.NoError(t, err)
		got = testRegistry.Unregister(testUncheckedVector1)
		assert.False(t, got)
	})
}

func TestLooseLabelsCollector(t *testing.T) {
	t.Run("test", func(t *testing.T) {
		testRegistry := prometheus.NewRegistry()
		testGaugeVec := NewLooseLabelsGaugeVec(prometheus.GaugeOpts{
			Name: "test_gauge_collector",
		}, []string{"test"}, testRegistry)
		testGaugeVec.GetMetricVec().(*prometheus.GaugeVec).Reset()
		testGaugeVec.GetMetricVec().(*prometheus.GaugeVec).With(prometheus.Labels{"test": "value"}).Set(1.0)
		// expand a label
		labels := prometheus.Labels{"test": "value", "test1": "value1"}
		got := testGaugeVec.RefreshLabelsIfNeed(labels)
		assert.True(t, got)
		testGaugeVec.FillKnownLabels(labels)
		testGaugeVec.GetMetricVec().(*prometheus.GaugeVec).With(labels).Set(2.0)
		// include a label
		labels = prometheus.Labels{"test1": "value"}
		got = testGaugeVec.RefreshLabelsIfNeed(labels)
		assert.False(t, got)
		testGaugeVec.FillKnownLabels(labels)
		testGaugeVec.GetMetricVec().(*prometheus.GaugeVec).With(labels).Set(2.0)
		// force shrink a label
		labels = prometheus.Labels{"test1": "value"}
		testGaugeVec.RefreshLabels(labels)
		testGaugeVec.FillKnownLabels(labels)
		testGaugeVec.GetMetricVec().(*prometheus.GaugeVec).With(labels).Set(2.0)
		// keep the same labels
		labels = prometheus.Labels{"test1": "value1"}
		got = testGaugeVec.RefreshLabelsIfNeed(labels)
		assert.False(t, got)
		testGaugeVec.FillKnownLabels(labels)
		testGaugeVec.GetMetricVec().(*prometheus.GaugeVec).With(labels).Set(2.0)
		// clean up
		testGaugeVec.GetMetricVec().(*prometheus.GaugeVec).Reset()
		got = testRegistry.Unregister(testGaugeVec.GetMetricVec())
		assert.False(t, got)

		testCounterVec := NewLooseLabelsCounterVec(prometheus.CounterOpts{
			Name: "test_counter_collector",
		}, []string{"test"}, testRegistry)
		testCounterVec.GetMetricVec().(*prometheus.CounterVec).Reset()
		testCounterVec.GetMetricVec().(*prometheus.CounterVec).With(prometheus.Labels{"test": "value"}).Inc()
		// expand a label
		labels = prometheus.Labels{"test": "value", "test1": "value1"}
		got = testCounterVec.RefreshLabelsIfNeed(labels)
		assert.True(t, got)
		testCounterVec.FillKnownLabels(labels)
		testCounterVec.GetMetricVec().(*prometheus.CounterVec).With(labels).Inc()
		// include a label
		labels = prometheus.Labels{"test1": "value"}
		got = testCounterVec.RefreshLabelsIfNeed(labels)
		assert.False(t, got)
		testCounterVec.FillKnownLabels(labels)
		testCounterVec.GetMetricVec().(*prometheus.CounterVec).With(labels).Inc()
		// force shrink a label
		labels = prometheus.Labels{"test1": "value"}
		testCounterVec.RefreshLabels(labels)
		testCounterVec.FillKnownLabels(labels)
		testCounterVec.GetMetricVec().(*prometheus.CounterVec).With(labels).Inc()
		// keep the same labels
		labels = prometheus.Labels{"test1": "value1"}
		got = testCounterVec.RefreshLabelsIfNeed(labels)
		assert.False(t, got)
		testCounterVec.FillKnownLabels(labels)
		testCounterVec.GetMetricVec().(*prometheus.CounterVec).With(labels).Inc()
		// clean up
		testCounterVec.GetMetricVec().(*prometheus.CounterVec).Reset()
		got = testRegistry.Unregister(testCounterVec.GetMetricVec())
		assert.False(t, got)
	})
}
