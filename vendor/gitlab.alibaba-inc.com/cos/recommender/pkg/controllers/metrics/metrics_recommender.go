package metrics

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type apiVersion string

const (
	metricsNamespace = TopMetricsNamespace + "recommender"
)

const (
	v1alpha1 apiVersion = "v1alpha1"
	v1       apiVersion = "v1"
)

var (
	recObjectCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "recommendation_objects_count",
			Help:      "Number of Recommendation objects present in the cluster.",
		}, []string{"has_recommendation", "api", "matches_pods", "unsupported_config"},
	)

	recommendationLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "recommendation_latency_seconds",
			Help:      "Time elapsed from creating a valid VPA configuration to the first client.",
			Buckets:   []float64{1.0, 2.0, 5.0, 7.5, 10.0, 20.0, 30.0, 40.00, 50.0, 60.0, 90.0, 120.0, 150.0, 180.0, 240.0, 300.0, 600.0, 900.0, 1800.0},
		},
	)

	functionLatency = CreateExecutionTimeMetric(metricsNamespace,
		"Time spent in various parts of Recommender main loop.")

	aggregateContainerStatesCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "aggregate_container_states_count",
			Help:      "Number of aggregate container states being tracked by the recommender",
		},
	)
)

type objectCounterKey struct {
	has               bool
	matchesPods       bool
	apiVersion        apiVersion
	unsupportedConfig bool
}

// RegisterRecommendation  initializes all metrics for Recommendation
func RegisterRecommendation() {
	prometheus.MustRegister(recObjectCount, recommendationLatency, functionLatency, aggregateContainerStatesCount)
}

// ObjectCounter helps split all VPA objects into buckets
type ObjectCounter struct {
	cnt map[objectCounterKey]int
}

// NewExecTimer provides a timer for Recommender's RunOnce execution
func NewExecTimer() *ExecutionTimer {
	return NewExecutionTimer(functionLatency)
}

// ObserveRecommendationLatency observes the time it took for the first client to appear
func ObserveRecommendationLatency(created time.Time) {
	recommendationLatency.Observe(time.Since(created).Seconds())
}

// RecordAggregateContainerStatesCount records the number of containers being tracked by the recommender
func RecordAggregateContainerStatesCount(statesCount int) {
	aggregateContainerStatesCount.Set(float64(statesCount))
}

// NewObjectCounter creates a new helper to split Recommendation objects into buckets
func NewObjectCounter() *ObjectCounter {
	obj := ObjectCounter{
		cnt: make(map[objectCounterKey]int),
	}

	// initialize with empty data so we can clean stale gauge values in Observe

	for _, h := range []bool{false, true} {
		for _, api := range []apiVersion{v1alpha1, v1} {
			for _, mp := range []bool{false, true} {
				for _, uc := range []bool{false, true} {
					obj.cnt[objectCounterKey{
						has:               h,
						apiVersion:        api,
						matchesPods:       mp,
						unsupportedConfig: uc,
					}] = 0
				}
			}
		}
	}
	return &obj
}

// Observe passes all the computed bucket values to metrics
func (oc *ObjectCounter) Observe() {
	for k, v := range oc.cnt {
		recObjectCount.WithLabelValues(
			fmt.Sprintf("%v", k.has),
			string(k.apiVersion),
			fmt.Sprintf("%v", k.matchesPods),
			fmt.Sprintf("%v", k.unsupportedConfig),
		).Set(float64(v))
	}
}

// Add updates the helper state to include the given VPA object
func (oc *ObjectCounter) Add(hasRec, hasMatchPods, unsupportedConf bool) {

	// TODO: Maybe report v1 version as well.
	api := v1alpha1

	key := objectCounterKey{
		has:               hasRec,
		apiVersion:        api,
		matchesPods:       hasMatchPods,
		unsupportedConfig: unsupportedConf,
	}
	oc.cnt[key]++
}
