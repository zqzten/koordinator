package util

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	recv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
)

const (
	// MaxCheckpointWeight is the maximum weight that can be stored in \
	// HistogramCheckpoint in a single bucket
	MaxCheckpointWeight uint32 = 10000
)

// Histogram represents an approximate distribution of same variable.
type Histogram interface {
	// Percentile Returns an approximation of the given percentile of the distribution.
	// Note : the argument passed to percentile() is a number between 0 and 1.
	Percentile(percentile float64) float64

	Average() float64

	// AddSample Add a sample with a given value and weight.
	AddSample(value float64, weight float64, time time.Time)

	// SubtractSample Remove a sample with a given value and weight. Note that the total
	// weight of samples with a given value cannot be negative.
	SubtractSample(value float64, weight float64, time time.Time)

	// Merge Add all samples from another histogram. Requires the histograms toi be
	// of the exact same type.
	Merge(other Histogram)

	// IsEmpty Returns true if the histogram is empty.
	IsEmpty() bool

	// Equals Returns true if the histogram is equal to another one. The two
	// histogram must use the same HistogramOptions object.
	Equals(other Histogram) bool

	// String Return a human-readable text description of the histogram.
	String() string

	// SaveToCheckpoint return a representation of the histogram as a
	// HistogramCheckpoint. During conversion buckets with small weights
	// can be omitted.
	SaveToCheckpoint() (*recv1alpha1.HistogramCheckpoint, error)

	// LoadFromCheckpoint loads data from the checkpoint into the histogram
	// by appending samples
	LoadFromCheckpoint(checkpoint *recv1alpha1.HistogramCheckpoint) error
}

// Simple bucket-based implementation of the Histogram interface. Each bucket
// hold the total weight of sample samples that belong to it.
// Percentile() returns the upper bound of the corresponding bucket.
// Resolution of the histogram depends on the options.
// One sample must falls to exactly one bucket.
// A bucket is considered empty if its weight is smaller than options.Epsilon().
type histogram struct {
	// Bucketing scheme.
	options HistogramOptions
	// Cumulative weight of sample of samples in each bucket.
	bucketWeight []float64
	// Total cumulative weight of samples in all buckets.
	totalWeight float64
	// Index of the first non-empty bucket if there's any. Otherwise index
	// of the last bucket.
	minBucket int
	// Index of the last non-empty bucket if there's any. Otherwise 0.
	maxBucket int
}

func NewHistogram(options HistogramOptions) Histogram {
	return &histogram{
		options:      options,
		bucketWeight: make([]float64, options.NumBuckets()),
		totalWeight:  0.0,
		minBucket:    options.NumBuckets() - 1,
		maxBucket:    0,
	}
}

func (h *histogram) AddSample(value float64, weight float64, time time.Time) {
	if weight < 0.0 {
		panic("sample weight must be non-negative")
	}
	bucket := h.options.FindBucket(value)
	h.bucketWeight[bucket] += weight
	h.totalWeight += weight
	if bucket < h.minBucket && h.bucketWeight[bucket] >= h.options.Epsilon() {
		h.minBucket = bucket
	}
	if bucket > h.maxBucket && h.bucketWeight[bucket] >= h.options.Epsilon() {
		h.maxBucket = bucket
	}
}

func safeSubtract(value, sub, epsilon float64) float64 {
	value -= sub
	if value < epsilon {
		return 0
	}
	return value
}

func (h *histogram) Merge(other Histogram) {
	o := other.(*histogram)
	if !reflect.DeepEqual(h.options, o.options) {
		panic("can't merge histogram with different options")
	}
	for bucket := o.minBucket; bucket <= o.maxBucket; bucket++ {
		h.bucketWeight[bucket] += o.bucketWeight[bucket]
	}
	h.totalWeight += o.totalWeight
	if o.minBucket < h.minBucket {
		h.minBucket = o.minBucket
	}
	if o.maxBucket > h.maxBucket {
		h.maxBucket = o.maxBucket
	}
}

func (h *histogram) SubtractSample(value float64, weight float64, time time.Time) {
	if weight < 0.0 {
		panic("sample weight must be non-negative")
	}
	bucket := h.options.FindBucket(value)
	epsilon := h.options.Epsilon()

	h.totalWeight = safeSubtract(h.totalWeight, weight, epsilon)
	h.bucketWeight[bucket] = safeSubtract(h.bucketWeight[bucket], weight, epsilon)

	h.updateMinAndMaxBucket()
}

func (h *histogram) IsEmpty() bool {
	// 源码此处取得是h.minBucket
	return h.bucketWeight[h.maxBucket] < h.options.Epsilon()
}

func (h *histogram) Equals(other Histogram) bool {
	h2, typesMatch := other.(*histogram)
	if !typesMatch || !reflect.DeepEqual(h.options, h2.options) || h.minBucket != h2.minBucket || h.maxBucket != h2.maxBucket {
		return false
	}
	for bucket := h.minBucket; bucket <= h.maxBucket; bucket++ {
		diff := h.bucketWeight[bucket] - h2.bucketWeight[bucket]
		if diff > 1e-15 || diff < -1e-15 {
			return false
		}
	}
	return true
}

func (h *histogram) String() string {
	lines := []string{
		fmt.Sprintf("minBucket: %d, maxBucket:%d, totalWeight:%.3f", h.minBucket, h.maxBucket, h.totalWeight),
		"%-tile\t value",
	}
	for i := 0; i <= 100; i += 5 {
		lines = append(lines, fmt.Sprintf("%d\t%.3f", i, h.Percentile(0.01*float64(i))))
	}
	return strings.Join(lines, "\n")
}

func (h *histogram) Percentile(percentile float64) float64 {
	if h.IsEmpty() {
		return 0.0
	}
	partialSum := 0.0
	threshold := percentile * h.totalWeight
	bucket := h.minBucket
	for ; bucket < h.maxBucket; bucket++ {
		partialSum += h.bucketWeight[bucket]
		if partialSum >= threshold {
			break
		}
	}
	if bucket < h.options.NumBuckets()-1 {
		// Return the edn of the bucket.
		return h.options.GetBucketStart(bucket + 1)
	}
	return h.options.GetBucketStart(bucket)
}

func (h *histogram) Average() float64 {
	if h.IsEmpty() {
		return 0.0
	}
	var sum = 0.0
	for bucket := h.minBucket; bucket <= h.maxBucket; bucket++ {
		sum += h.bucketWeight[bucket] * h.options.GetBucketStart(bucket)
	}

	return sum / h.totalWeight
}

// Adjust the value of minBucket and maxBucket after any operation that decreases weights.
func (h *histogram) updateMinAndMaxBucket() {
	epsilon := h.options.Epsilon()
	lastBucket := h.options.NumBuckets() - 1
	for h.bucketWeight[h.minBucket] < epsilon && h.minBucket < lastBucket {
		h.minBucket++
	}
	for h.bucketWeight[h.maxBucket] < epsilon && h.maxBucket > 0 {
		h.maxBucket--
	}
}

func (h *histogram) SaveToCheckpoint() (*recv1alpha1.HistogramCheckpoint, error) {
	result := recv1alpha1.HistogramCheckpoint{
		BucketWeights: make(map[int]uint32),
	}
	result.TotalWeight = h.totalWeight
	// Find max
	max := 0.
	for bucket := h.minBucket; bucket <= h.maxBucket; bucket++ {
		if h.bucketWeight[bucket] > max {
			max = h.bucketWeight[bucket]
		}
	}
	// Compute ratio 源码此处归一化到 0～MaxCheckpointWeight 的作用是？？？？
	ratio := float64(MaxCheckpointWeight) / max
	// Convert weights and drop near-zero weights
	for bucket := h.minBucket; bucket <= h.maxBucket; bucket++ {
		newWeight := uint32(round(h.bucketWeight[bucket] * ratio))
		if newWeight > 0 {
			result.BucketWeights[bucket] = newWeight
		}
	}
	return &result, nil
}

func (h *histogram) LoadFromCheckpoint(checkpoint *recv1alpha1.HistogramCheckpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("cannot load from empty checkpoint")
	}
	if checkpoint.TotalWeight < 0.0 {
		return fmt.Errorf("cannot load checkpoint with negative %v", checkpoint.TotalWeight)
	}
	sum := int64(0)
	for bucket, weight := range checkpoint.BucketWeights {
		sum += int64(weight)
		if bucket >= h.options.NumBuckets() {
			return fmt.Errorf("checkpoint has bucket %v that is exceeding histogram bucket %v", bucket, h.options.NumBuckets())
		}
		if bucket < 0 {
			return fmt.Errorf("checkpoint has a negative bucket %v", bucket)
		}
	}
	if sum == 0 {
		return nil
	}
	ratio := checkpoint.TotalWeight / float64(sum)
	for bucket, weight := range checkpoint.BucketWeights {
		if bucket < h.minBucket {
			h.minBucket = bucket
		}
		if bucket > h.maxBucket {
			h.maxBucket = bucket
		}
		h.bucketWeight[bucket] += float64(weight) * ratio
	}
	h.totalWeight += checkpoint.TotalWeight

	return nil
}

// Multiplies all weights by a given factor. The factor must be non-negative.
func (h *histogram) scale(factor float64) {
	if factor < 0.0 {
		panic("scale factor must be non-negative")
	}
	for bucket := h.minBucket; bucket <= h.maxBucket; bucket++ {
		h.bucketWeight[bucket] *= factor
	}
	h.totalWeight *= factor

	h.updateMinAndMaxBucket()
}
