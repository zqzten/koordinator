package config

import (
	"encoding/json"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"

	"gitlab.alibaba-inc.com/cos/recommender/pkg/common"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/util"
)

const (
	// MinSampleWeight is the minimal weight of any sample (prior to including decaying factor)
	MinSampleWeight = 0.1
	// epsilon is the minimal weight kept in histograms, it should be small enough that old samples
	// (just inside MemoryAggregationWindowLength) added with MinSampleWeight are still kept
	epsilon = 0.001 * MinSampleWeight
	// DefaultMemoryAggregationIntervalCount is the default value for MemoryAggregationIntervalCount.
	DefaultMemoryAggregationIntervalCount = 8
	// DefaultMemoryAggregationInterval is the default value for MemoryAggregationInterval.
	// which the peak memory usage is computed.
	DefaultMemoryAggregationInterval = time.Hour * 24
	// DefaultHistogramBucketSizeGrowth is the default value for histogramBucketSizeGrowth.
	DefaultHistogramBucketSizeGrowth = 0.05
	// DefaultMemoryHistogramDecayHalfLife is the default value for MemoryHistogramDecayHalfLife.
	DefaultMemoryHistogramDecayHalfLife = time.Hour * 24
	// DefaultCPUHistogramDecayHalfLife is the default value for CPUHistogramDecayHalfLife.
	DefaultCPUHistogramDecayHalfLife = time.Hour * 12
)

var (
	recommenderConfig   = NewDefaultRecommenderCfg()
	recommenderConfLock = sync.RWMutex{}
)

type RecommenderCfg struct {
	// MemoryAggregationInterval is the length of a single interval, for
	// which the peak memory usage is computed.
	// Memory usage peaks are aggregated in multiples of the interval.In other words
	// there is one memory usage sample per interval (the maximum usage over that interval)
	MemoryAggregationIntervalHour *int64 `json:"memoryAggregationIntervalHour,omitempty"`
	// MemoryAggregationIntervalCount is the number of consecutive MemoryAggregationIntervals
	// which make up the MemoryAggregationWindowLength which in turn is the period for memory
	// usage aggregation by Recommendation.
	MemoryAggregationIntervalCount *int64 `json:"memoryAggregationIntervalCount,omitempty"`
	// MemoryHistogramDecayHalfLife is the amount of time it takes a historical
	// memory usage sample to lose half of its weight. In other words, a fresh
	// usage sample is twice as 'important' as one with age equal to the half
	// life period.
	MemoryHistorygramDecayHalfLifeHour *int64 `json:"memoryHistorygramDecayHalfLifeHour,omitempty"`
	// CPUHistogramDecayHalfLife is the amount of time it takes a historical
	// CPU usage sample to lose half of its weight.
	CPUHistorgramDecayHalfLifeHour *int64 `json:"cpuHistorgramDecayHalfLifeHour,omitempty"`
	// Expose recommendation target value as prometheus metric.
	EnableRecommendationTargetMetric *bool `json:"enableRecommendationTargetMetric,omitempty"`
	// AggregateContainerStateGCInterval defines how often expired AggregateContainerStates are garbage collected.
	AggregateContainerStateGCIntervalSecond *int64 `json:"AggregateContainerStateGCIntervalSecond,omitempty"`
}

func NewDefaultRecommenderCfg() *RecommenderCfg {
	return &RecommenderCfg{
		MemoryAggregationIntervalHour:           common.Int64Ptr(24),
		MemoryAggregationIntervalCount:          common.Int64Ptr(8),
		MemoryHistorygramDecayHalfLifeHour:      common.Int64Ptr(24),
		CPUHistorgramDecayHalfLifeHour:          common.Int64Ptr(12),
		EnableRecommendationTargetMetric:        common.BoolPtr(true),
		AggregateContainerStateGCIntervalSecond: common.Int64Ptr(3600),
	}
}

type AggregationsConfig struct {
	// MemoryAggregationInterval is the length of a single interval, for
	// which the peak memory usage is computed.
	// Memory usage peaks are aggregated in multiples of the interval.In other words
	// there is one memory usage sample per interval (the maximum usage over that interval)
	MemoryAggregationInterval time.Duration
	// MemoryAggregationIntervalCount is the number of consecutive MemoryAggregationIntervals
	// which make up the MemoryAggregationWindowLength which in turn is the period for memory
	// usage aggregation by Recommendation.
	MemoryAggregationIntervalCount int64
	// CPUHistogramOptions are options to be used by histograms that store
	// CPU measures expressed in cores.
	CPUHistogramOptions util.HistogramOptions
	// MemoryHistogramOptions are options to be used by histograms that
	// store memory measures expressed in bytes.
	MemoryHistogramOptions util.HistogramOptions
	// HistogramBucketSizeGrowth defines the growth rate of the histogram buckets.
	// Each bucket is wider than the previous one by this fraction.
	HistogramBucketSizeGrowth float64
	// MemoryHistogramDecayHalfLife is the amount of time it takes a historical
	// memory usage sample to lose half of its weight. In other words, a fresh
	// usage sample is twice as 'important' as one with age equal to the half
	// life period.
	MemoryHistogramDecayHalfLife time.Duration
	// CPUHistogramDecayHalfLife is the amount of time it takes a historical
	// CPU usage sample to lose half of its weight.
	CPUHistogramDecayHalfLife time.Duration
}

// GetMemoryAggregationWindowLength returns the total length of the memory usage history
func (a *AggregationsConfig) GetMemoryAggregationWindowLength() time.Duration {
	return a.MemoryAggregationInterval * time.Duration(a.MemoryAggregationIntervalCount)
}

func (a *AggregationsConfig) cpuHistogramOptions() util.HistogramOptions {
	// CPU histogram use exponential bucketing scheme with the smallest bucket
	// size of 0.01 core, max of 1000.0 cores and the relative error of HistogramRelativeError.
	// When parameters below are changed SupportedCheckpointVersion has to be bumped.
	options, err := util.NewExponentialHistogramOptions(1000, 0.01, 1.+a.HistogramBucketSizeGrowth, epsilon)
	if err != nil {
		panic("Invalid memory histogram options")
	}
	return options
}

func (a *AggregationsConfig) memoryHistogramOptions() util.HistogramOptions {
	// Memory histogram use exponential bucketing scheme with the smallest
	// bucket size of 10MB, max of 1TB and the relative error of HistogramRelativeError.
	options, err := util.NewExponentialHistogramOptions(1e12, 1e7, 1.+a.HistogramBucketSizeGrowth, epsilon)
	if err != nil {
		panic("Invalid memory histogram options")
	}
	return options
}

func NewDefaultAggregationsConfig() *AggregationsConfig {
	a := &AggregationsConfig{
		MemoryAggregationInterval:      DefaultMemoryAggregationInterval,
		MemoryAggregationIntervalCount: DefaultMemoryAggregationIntervalCount,
		HistogramBucketSizeGrowth:      DefaultHistogramBucketSizeGrowth,
		MemoryHistogramDecayHalfLife:   DefaultMemoryHistogramDecayHalfLife,
		CPUHistogramDecayHalfLife:      DefaultCPUHistogramDecayHalfLife,
	}
	a.CPUHistogramOptions = a.cpuHistogramOptions()
	a.MemoryHistogramOptions = a.memoryHistogramOptions()
	return a
}

func GetAggregationsConfig() *AggregationsConfig {
	aggregationsConfig := NewDefaultAggregationsConfig()
	recommenderConfLock.RLock()
	defer recommenderConfLock.RUnlock()
	if recommenderConfig.MemoryAggregationIntervalHour != nil {
		aggregationsConfig.MemoryAggregationInterval = time.Duration(*recommenderConfig.MemoryAggregationIntervalHour) * time.Hour
	}
	if recommenderConfig.MemoryAggregationIntervalCount != nil {
		aggregationsConfig.MemoryAggregationIntervalCount = *recommenderConfig.MemoryAggregationIntervalCount
	}
	aggregationsConfig.HistogramBucketSizeGrowth = DefaultHistogramBucketSizeGrowth
	if recommenderConfig.MemoryHistorygramDecayHalfLifeHour != nil {
		aggregationsConfig.MemoryHistogramDecayHalfLife = time.Duration(*recommenderConfig.MemoryHistorygramDecayHalfLifeHour) * time.Hour
	}
	if recommenderConfig.CPUHistorgramDecayHalfLifeHour != nil {
		aggregationsConfig.CPUHistogramDecayHalfLife = time.Duration(*recommenderConfig.CPUHistorgramDecayHalfLifeHour) * time.Hour
	}
	aggregationsConfig.CPUHistogramOptions = aggregationsConfig.cpuHistogramOptions()
	aggregationsConfig.MemoryHistogramOptions = aggregationsConfig.memoryHistogramOptions()
	return aggregationsConfig
}

type MetricConfig struct {
	// Expose recommendation target value as prometheus metric.
	EnableRecommendationTargetMetric bool
}

func NewDefaultMetricConfig() *MetricConfig {
	return &MetricConfig{
		EnableRecommendationTargetMetric: false,
	}
}

func GetMetricConfig() *MetricConfig {
	metricConfig := NewDefaultMetricConfig()
	recommenderConfLock.Lock()
	defer recommenderConfLock.Unlock()
	if recommenderConfig.EnableRecommendationTargetMetric != nil {
		metricConfig.EnableRecommendationTargetMetric = *recommenderConfig.EnableRecommendationTargetMetric
	}
	return metricConfig
}

type GCConfig struct {
	// AggregateContainerStateGCInterval defines how often expired AggregateContainerStates are garbage collected.
	AggregateContainerStateGCInterval time.Duration
}

func NewDefaultGCConfig() *GCConfig {
	return &GCConfig{
		AggregateContainerStateGCInterval: time.Hour,
	}
}

func GetGCConfig() *GCConfig {
	gcConfig := NewDefaultGCConfig()
	recommenderConfLock.Lock()
	defer recommenderConfLock.Unlock()
	if recommenderConfig.AggregateContainerStateGCIntervalSecond != nil {
		gcConfig.AggregateContainerStateGCInterval = time.Duration(*recommenderConfig.AggregateContainerStateGCIntervalSecond) * time.Second
	}
	return gcConfig
}

// InitializeConfig initializes the global aggregation configuration. Not thread-safe.
func InitializeConfig(client listerv1.ConfigMapNamespaceLister, configMapName string) {
	go wait.Forever(func() {
		updateRecommenderConfig(client, configMapName)
	}, 1*time.Minute)
}

func updateRecommenderConfig(client listerv1.ConfigMapNamespaceLister, configMapName string) {
	// query configmap
	var err error
	var configmap *corev1.ConfigMap
	if configmap, err = client.Get(configMapName); err != nil && !errors.IsNotFound(err) {
		klog.Errorf("Failed to load configmap %s, err:%s", configMapName, err)
		return
	} else if err != nil || configmap == nil {
		klog.V(5).Infof("config map %v not exist, use default value", configMapName)
		return
	}

	// update aggregationsConfig
	recommenderConfLock.Lock()
	defer recommenderConfLock.Unlock()
	configStr, isOk := configmap.Data[common.RecommenderConfigKey]
	if isOk {
		if err := json.Unmarshal([]byte(configStr), &recommenderConfig); err != nil {
			klog.Errorf("Failed to parse recommender configmap: %v, error: %v", configStr, err)
		}
	}
	configFmtedStr, _ := json.Marshal(recommenderConfig)
	klog.Infof("sync config map finished %v, origin %v, config %v", configMapName, configStr, string(configFmtedStr))
}
