/*
Copyright 2017 The Kubernetes Authors.

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

package logic

import (
	"flag"

	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/model"
)

var (
	safetyMarginFraction = flag.Float64("client-margin-fraction", 0.15, `Fraction of usage added as the safety margin to the recommended request`)
	podMinCPUMillicores  = flag.Float64("pod-client-min-cpu-millicores", 25, `Minimum CPU client for a pod`)
	podMinMemoryMb       = flag.Float64("pod-client-min-memory-mb", 250, `Minimum memory client for a pod`)
)

// PodResourceRecommender computes resource client for a Vpa object.
type PodResourceRecommender interface {
	GetRecommendedPodResources(containerNameToAggregateStateMap model.ContainerNameToAggregateStateMap) RecommendedPodResources
}

// RecommendedPodResources is a Map from container name to recommended resources.
type RecommendedPodResources map[string]RecommendedContainerResources

// RecommendedContainerResources is the client of resources for a
// container.
type RecommendedContainerResources struct {
	// Recommended optimal amount of resources.
	Target model.Resources
	P60    model.Resources
	P95    model.Resources
	P99    model.Resources
	Max    model.Resources
	Avg    model.Resources
}

type podResourceRecommender struct {
	targetEstimator ResourceEstimator
	p60Estimator    ResourceEstimator
	p95Estimator    ResourceEstimator
	p99Estimator    ResourceEstimator
	maxEstimator    ResourceEstimator
	avgEstimator    ResourceEstimator
}

func (r *podResourceRecommender) GetRecommendedPodResources(containerNameToAggregateStateMap model.ContainerNameToAggregateStateMap) RecommendedPodResources {
	var recommendation = make(RecommendedPodResources)
	if len(containerNameToAggregateStateMap) == 0 {
		return recommendation
	}

	fraction := 1.0 / float64(len(containerNameToAggregateStateMap))
	minResources := model.Resources{
		model.ResourceCPU:    model.ScaleResource(model.CPUAmountFromCores(*podMinCPUMillicores*0.001), fraction),
		model.ResourceMemory: model.ScaleResource(model.MemoryAmountFromBytes(*podMinMemoryMb*1024*1024), fraction),
	}

	recommender := &podResourceRecommender{
		WithMinResources(minResources, r.targetEstimator),
		WithMinResources(minResources, r.p60Estimator),
		WithMinResources(minResources, r.p95Estimator),
		WithMinResources(minResources, r.p99Estimator),
		WithMinResources(minResources, r.maxEstimator),
		WithMinResources(minResources, r.avgEstimator),
	}

	for containerName, aggregatedContainerState := range containerNameToAggregateStateMap {
		recommendation[containerName] = recommender.estimateContainerResources(aggregatedContainerState)
	}
	return recommendation
}

// Takes AggregateContainerState and returns a container client.
func (r *podResourceRecommender) estimateContainerResources(s *model.AggregateContainerState) RecommendedContainerResources {
	return RecommendedContainerResources{
		FilterControlledResources(r.targetEstimator.GetResourceEstimation(s), s.GetControlledResources()),
		FilterControlledResources(r.p60Estimator.GetResourceEstimation(s), s.GetControlledResources()),
		FilterControlledResources(r.p95Estimator.GetResourceEstimation(s), s.GetControlledResources()),
		FilterControlledResources(r.p99Estimator.GetResourceEstimation(s), s.GetControlledResources()),
		FilterControlledResources(r.maxEstimator.GetResourceEstimation(s), s.GetControlledResources()),
		FilterControlledResources(r.avgEstimator.GetResourceEstimation(s), s.GetControlledResources()),
	}
}

// FilterControlledResources returns estimations from 'estimation' only for resources present in 'controlledResources'.
func FilterControlledResources(estimation model.Resources, controlledResources []model.ResourceName) model.Resources {
	result := make(model.Resources)
	for _, resource := range controlledResources {
		if value, ok := estimation[resource]; ok {
			result[resource] = value
		}
	}
	return result
}

// CreatePodResourceRecommender returns the primary recommender.
func CreatePodResourceRecommender() PodResourceRecommender {

	targetCPUPercentile := 0.95
	uncappedCPUPercentile60 := 0.60
	uncappedCPUPercentile95 := 0.95
	uncappedCPUPercentile99 := 0.99
	uncappedCPUMax := 1.00

	targetMemoryPercentile := 0.99
	uncappedMemoryPercentile60 := 0.60
	uncappedMemoryPercentile95 := 0.95
	uncappedMemoryPercentile99 := 0.99
	uncappedMemoryMax := 1.00

	targetEstimator := NewPercentileEstimator(targetCPUPercentile, targetMemoryPercentile)
	p60Estimator := NewPercentileEstimator(uncappedCPUPercentile60, uncappedMemoryPercentile60)
	p95Estimator := NewPercentileEstimator(uncappedCPUPercentile95, uncappedMemoryPercentile95)
	p99Estimator := NewPercentileEstimator(uncappedCPUPercentile99, uncappedMemoryPercentile99)
	maxEstimator := NewPercentileEstimator(uncappedCPUMax, uncappedMemoryMax)
	averageEstimator := NewAverageEstimator()

	// add safe margin only for target estimator
	targetEstimator = WithMargin(*safetyMarginFraction, targetEstimator)

	// Apply confidence multiplier to the upper bound estimator. This means
	// that the updater will be less eager to evict pods with short history
	// in order to reclaim unused resources.
	// Using the confidence multiplier 1 with exponent +1 means that
	// the upper bound is multiplied by (1 + 1/history-length-in-days).
	// See estimator.go to see how the history length and the confidence
	// multiplier are determined. The formula yields the following multipliers:
	// No history     : *INF  (do not force pod eviction)
	// 12h history    : *3    (force pod eviction if the request is > 3 * upper bound)
	// 24h history    : *2
	// 1 week history : *1.14
	// p60_Estimator = WithConfidenceMultiplier(1.0, 1.0, p60_Estimator)

	// Apply confidence multiplier to the lower bound estimator. This means
	// that the updater will be less eager to evict pods with short history
	// in order to provision them with more resources.
	// Using the confidence multiplier 0.001 with exponent -2 means that
	// the lower bound is multiplied by the factor (1 + 0.001/history-length-in-days)^-2
	// (which is very rapidly converging to 1.0).
	// See estimator.go to see how the history length and the confidence
	// multiplier are determined. The formula yields the following multipliers:
	// No history   : *0   (do not force pod eviction)
	// 5m history   : *0.6 (force pod eviction if the request is < 0.6 * lower bound)
	// 30m history  : *0.9
	// 60m history  : *0.95
	// lowerBoundEstimator = WithConfidenceMultiplier(0.001, -2.0, lowerBoundEstimator)

	return &podResourceRecommender{
		targetEstimator,
		p60Estimator,
		p95Estimator,
		p99Estimator,
		maxEstimator,
		averageEstimator,
	}
}
