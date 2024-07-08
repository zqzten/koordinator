/*
Copyright 2019 The Kubernetes Authors.

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
	"math"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// Buckets between 0.01 and 655.36 cores
	cpuBuckets = prometheus.ExponentialBuckets(0.01, 2., 17)
	// Buckets between 1MB and 65.5 GB
	memoryBuckets = prometheus.ExponentialBuckets(1e6, 2., 17)
	// Buckets for relative comparisons, from -100% to x100
	relativeBuckets = []float64{-1., -.75, -.5, -.25, -.1, -.05, -0.025, -.01, -.005, -0.0025, -.001,
		0., .001, .0025, .005, .01, .025, .05, .1, .25, .5, .75, 1., 2.5, 5., 10., 25., 50., 100.}
)

var (
	usageRecommendationRelativeDiff = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "usage_recommendation_relative_diffs",
			Help:      "Diffs between client and usage, normalized by client value",
			Buckets:   relativeBuckets,
		}, []string{"resource", "is_oom"},
	)
	usageMissingRecommendationCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "usage_sample_missing_recommendation_count",
			Help:      "Count of usage samples when a client should be present but is missing",
		}, []string{"resource", "is_oom"},
	)
	cpuRecommendationOverUsageDiff = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "cpu_recommendation_over_usage_diffs_cores",
			Help:      "Absolute diffs between client and usage for CPU when client > usage",
			Buckets:   cpuBuckets,
		}, []string{"recommendation_missing"},
	)
	memoryRecommendationOverUsageDiff = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "mem_recommendation_over_usage_diffs_bytes",
			Help:      "Absolute diffs between client and usage for memory when client > usage",
			Buckets:   memoryBuckets,
		}, []string{"recommendation_missing", "is_oom"},
	)
	cpuRecommendationLowerOrEqualUsageDiff = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "cpu_recommendation_lower_equal_usage_diffs_cores",
			Help:      "Absolute diffs between client and usage for CPU when client <= usage",
			Buckets:   cpuBuckets,
		}, []string{"recommendation_missing"},
	)
	memoryRecommendationLowerOrEqualUsageDiff = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "mem_recommendation_lower_equal_usage_diffs_bytes",
			Help:      "Absolute diffs between client and usage for memory when client <= usage",
			Buckets:   memoryBuckets,
		}, []string{"recommendation_missing", "is_oom"},
	)
	cpuRecommendations = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "cpu_recommendations_cores",
			Help:      "CPU client values as observed on recorded usage sample",
			Buckets:   cpuBuckets,
		}, []string{},
	)
	memoryRecommendations = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "mem_recommendations_bytes",
			Help:      "Memory client values as observed on recorded usage sample",
			Buckets:   memoryBuckets,
		}, []string{"is_oom"},
	)
	relativeRecommendationChange = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "relative_recommendation_changes",
			Help:      "Changes between consecutive client values, normalized by old value",
			Buckets:   relativeBuckets,
		}, []string{"resource", "vpa_size_log2"},
	)
)

// Register initializes all Recommendation quality metrics
func Register() {
	prometheus.MustRegister(usageRecommendationRelativeDiff)
	prometheus.MustRegister(usageMissingRecommendationCounter)
	prometheus.MustRegister(cpuRecommendationOverUsageDiff)
	prometheus.MustRegister(memoryRecommendationOverUsageDiff)
	prometheus.MustRegister(cpuRecommendationLowerOrEqualUsageDiff)
	prometheus.MustRegister(memoryRecommendationLowerOrEqualUsageDiff)
	prometheus.MustRegister(cpuRecommendations)
	prometheus.MustRegister(memoryRecommendations)
	prometheus.MustRegister(relativeRecommendationChange)
}

// observeUsageRecommendationRelativeDiff records relative diff between usage and
// client if client has a positive value.
func observeUsageRecommendationRelativeDiff(usage, recommendation float64, isOOM bool, resource corev1.ResourceName) {
	if recommendation > 0 {
		usageRecommendationRelativeDiff.WithLabelValues(string(resource), strconv.FormatBool(isOOM)).Observe((usage - recommendation) / recommendation)
	}
}

// observeMissingRecommendation counts usage samples with missing recommendations.
func observeMissingRecommendation(isOOM bool, resource corev1.ResourceName) {
	usageMissingRecommendationCounter.WithLabelValues(string(resource), strconv.FormatBool(isOOM)).Inc()
}

// observeUsageRecommendationDiff records absolute diff between usage and
// client.
func observeUsageRecommendationDiff(usage, recommendation float64, isRecommendationMissing, isOOM bool, resource corev1.ResourceName) {
	recommendationOverUsage := recommendation > usage
	diff := math.Abs(usage - recommendation)
	switch resource {
	case corev1.ResourceCPU:
		if recommendationOverUsage {
			cpuRecommendationOverUsageDiff.WithLabelValues(
				strconv.FormatBool(isRecommendationMissing)).Observe(diff)
		} else {
			cpuRecommendationLowerOrEqualUsageDiff.WithLabelValues(
				strconv.FormatBool(isRecommendationMissing)).Observe(diff)
		}
	case corev1.ResourceMemory:
		if recommendationOverUsage {
			memoryRecommendationOverUsageDiff.WithLabelValues(
				strconv.FormatBool(isRecommendationMissing), strconv.FormatBool(isOOM)).Observe(diff)
		} else {
			memoryRecommendationLowerOrEqualUsageDiff.WithLabelValues(
				strconv.FormatBool(isRecommendationMissing), strconv.FormatBool(isOOM)).Observe(diff)
		}
	default:
		klog.Warningf("Unknown resource: %v", resource)
	}
}

// observeRecommendation records the value of client.
func observeRecommendation(recommendation float64, isOOM bool, resource corev1.ResourceName) {
	switch resource {
	case corev1.ResourceCPU:
		cpuRecommendations.WithLabelValues().Observe(recommendation)
	case corev1.ResourceMemory:
		memoryRecommendations.WithLabelValues(strconv.FormatBool(isOOM)).Observe(recommendation)
	default:
		klog.Warningf("Unknown resource: %v", resource)
	}
}

// ObserveQualityMetrics records all quality metrics that we can derive from client and usage.
func ObserveQualityMetrics(usage, recommendation float64, isOOM bool, resource corev1.ResourceName) {
	observeRecommendation(recommendation, isOOM, resource)
	observeUsageRecommendationDiff(usage, recommendation, false, isOOM, resource)
	observeUsageRecommendationRelativeDiff(usage, recommendation, isOOM, resource)
}

// ObserveQualityMetricsRecommendationMissing records all quality metrics that we can derive from usage when client is missing.
func ObserveQualityMetricsRecommendationMissing(usage float64, isOOM bool, resource corev1.ResourceName) {
	observeMissingRecommendation(isOOM, resource)
	observeUsageRecommendationDiff(usage, 0, true, isOOM, resource)
}

// ObserveRecommendationChange records relative_recommendation_changes metric.
func ObserveRecommendationChange(previous, current corev1.ResourceList, recSize int) {
	// This will happen if there is no previous client, we don't want to emit anything then.
	if previous == nil {
		return
	}
	// This is not really expected thus a warning.
	if current == nil {
		klog.Warningf("Cannot compare with current client being nil. size: %v", recSize)
		return
	}

	for resource, amount := range current {
		newValue := quantityAsFloat(resource, amount)
		oldValue := quantityAsFloat(resource, previous[resource])

		if oldValue > 0.0 {
			diff := newValue/oldValue - 1.0 // -1.0 to report decreases as negative values and keep 0.0 neutral
			log2 := GetVpaSizeLog2(recSize)
			relativeRecommendationChange.WithLabelValues(string(resource), strconv.Itoa(log2)).Observe(diff)
		} else {
			klog.Warningf("Cannot compare as old client for %v is 0. , size: %v", resource, recSize)
		}
	}
}

// quantityAsFloat converts resource.Quantity to a float64 value, in some scale (constant per resource but unspecified)
func quantityAsFloat(resource corev1.ResourceName, quantity resource.Quantity) float64 {
	switch resource {
	case corev1.ResourceCPU:
		return float64(quantity.MilliValue())
	case corev1.ResourceMemory:
		return float64(quantity.Value())
	default:
		klog.Warningf("Unknown resource: %v", resource)
		return 0.0
	}
}
