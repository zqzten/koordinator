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

package util

import (
	"time"

	"github.com/stretchr/testify/mock"

	recv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
)

// MockHistogram is a mock implementation of Histogram interface.
type MockHistogram struct {
	mock.Mock
}

// Average is a mock implementation of Histogram.Average
func (m *MockHistogram) Average() float64 {
	args := m.Called()
	return args.Get(0).(float64)
}

// Percentile is a mock implementation of Histogram.Percentile.
func (m *MockHistogram) Percentile(percentile float64) float64 {
	args := m.Called(percentile)
	return args.Get(0).(float64)
}

// AddSample is a mock implementation of Histogram.AddSample.
func (m *MockHistogram) AddSample(value float64, weight float64, time time.Time) {
	m.Called(value, weight, time)
}

// SubtractSample is a mock implementation of Histogram.SubtractSample.
func (m *MockHistogram) SubtractSample(value float64, weight float64, time time.Time) {
	m.Called(value, weight, time)
}

// IsEmpty is a mock implementation of Histogram.IsEmpty.
func (m *MockHistogram) IsEmpty() bool {
	args := m.Called()
	return args.Bool(0)
}

// Equals is a mock implementation of Histogram.Equals.
func (m *MockHistogram) Equals(other Histogram) bool {
	args := m.Called()
	return args.Bool(0)
}

// Merge is a mock implementation of Histogram.Merge.
func (m *MockHistogram) Merge(other Histogram) {
	m.Called(other)
}

// String is a mock implementation of Histogram.String.
func (m *MockHistogram) String() string {
	args := m.Called()
	return args.String(0)
}

// SaveToCheckpoint is a mock implementation of Histogram.SaveToChekpoint.
func (m *MockHistogram) SaveToCheckpoint() (*recv1alpha1.HistogramCheckpoint, error) {
	return &recv1alpha1.HistogramCheckpoint{}, nil
}

// LoadFromCheckpoint is a mock implementation of Histogram.LoadFromCheckpoint.
func (m *MockHistogram) LoadFromCheckpoint(checkpoint *recv1alpha1.HistogramCheckpoint) error {
	return nil
}
