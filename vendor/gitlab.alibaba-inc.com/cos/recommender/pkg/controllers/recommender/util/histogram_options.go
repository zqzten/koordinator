package util

import (
	"errors"
	"fmt"
	"math"
)

// HistogramOptions define the number and size of buckets of a histogram.
type HistogramOptions interface {
	// NumBuckets return the number of buckets in the histogram.
	NumBuckets() int

	// FindBucket Returns the index of the bucket to which the given value falls.
	// If the value is outside of the range covered by the histogram, it
	// returns the closets bucket(either the first or the last one).
	FindBucket(value float64) int

	// GetBucketStart Returns the start of the bucket with a given index. iF the index is
	// outside the [0 .. NumBucket()-1] range, the result is undefind.
	GetBucketStart(bucket int) float64

	// Epsilon Returs the minimum weight for a bucket to be considered non-empty.
	Epsilon() float64
}

// NewLinearHistogramOptions returns HistogramOptions describing a histogram
// with a given number of fixed-size buckets, with the first bucket start at 0.0
// and the last bucket start larger or equal to maxValue.
// Requires maxValue > 0, bucketSize > 0, epsilon > 0.
func NewLinearHistogramOptions(
	maxValue float64, bucketSize float64, epsilon float64) (HistogramOptions, error) {
	if maxValue <= 0.0 || bucketSize <= 0.0 || epsilon <= 0.0 {
		return nil, errors.New("maxValue and bucketSize must both be positive")
	}
	numBuckets := int(math.Ceil(maxValue/bucketSize)) + 1
	return &linearHistogramOptions{numBuckets: numBuckets, bucketSize: bucketSize, epsilon: epsilon}, nil
}

// NewExponentialHistogramOptions returns HistogramOptions describing a
// histogram with exponentially growing bucket boundaries. The first bucket
// covers the range [0 .. firstBucketSize）.
// Bucket with index n has size equal to firstBucketSize * ratio^n.
// It follows that the bucket with index n >= 1 starts at:
//     firstBucketSize * (1 + ratio + ratio^2 + ... + ratio^(n-1)) =
//     firstBucketSize * (ratio^n - 1) / (ratio - 1).
// The last bucket start is larger or equal to maxValue.
// Requires maxValue > 0, firstBucketSize > 0, ratio > 1, epsilon > 0.

func NewExponentialHistogramOptions(
	maxValue float64, firstBucketSize float64, ratio float64, epsilon float64) (HistogramOptions, error) {
	if maxValue <= 0.0 || firstBucketSize <= 0.0 || ratio <= 1.0 || epsilon <= 0.0 {
		return nil, errors.New("maxValue, firstBucketSize and epsilon must be > 0.0, ratio must be > 1.0")
	}
	numBuckets := int(math.Ceil(log(ratio, maxValue*(ratio-1)/firstBucketSize+1))) + 1
	return &exponentialHistogramOptions{numBuckets: numBuckets, firstBucketSize: firstBucketSize, ratio: ratio, epsilon: epsilon}, nil
}

type linearHistogramOptions struct {
	numBuckets int
	bucketSize float64
	epsilon    float64
}

func (l *linearHistogramOptions) NumBuckets() int {
	return l.numBuckets
}

func (l *linearHistogramOptions) FindBucket(value float64) int {
	bucket := int(value / l.bucketSize)
	if bucket < 0 {
		return 0
	}
	if bucket >= l.numBuckets {
		return l.numBuckets - 1
	}
	return bucket
}

func (l *linearHistogramOptions) GetBucketStart(bucket int) float64 {
	if bucket < 0 || bucket >= l.numBuckets {
		panic(fmt.Sprintf("index %d out of range [0.. %d]", bucket, l.numBuckets-1))
	}
	return float64(bucket) * l.bucketSize
}

func (l *linearHistogramOptions) Epsilon() float64 {
	return l.epsilon
}

type exponentialHistogramOptions struct {
	numBuckets      int
	firstBucketSize float64
	ratio           float64
	epsilon         float64
}

func (e *exponentialHistogramOptions) NumBuckets() int {
	return e.numBuckets
}

func (e *exponentialHistogramOptions) FindBucket(value float64) int {
	if value < e.firstBucketSize {
		return 0
	}
	bucket := int(log(e.ratio, value*(e.ratio-1)/e.firstBucketSize+1))
	if bucket >= e.numBuckets {
		return e.numBuckets - 1
	}
	return bucket
}

func (e *exponentialHistogramOptions) GetBucketStart(bucket int) float64 {
	if bucket < 0 || bucket >= e.numBuckets {
		panic(fmt.Sprintf("index %d out of range [0..%d]", bucket, e.numBuckets-1))
	}
	if bucket == 0 {
		return 0.0
	}
	return e.firstBucketSize * (math.Pow(e.ratio, float64(bucket)) - 1) / (e.ratio - 1)
}

func (e *exponentialHistogramOptions) Epsilon() float64 {
	return e.epsilon
}

func log(base, x float64) float64 {
	return math.Log(x) / math.Log(base)
}
