/*
Copyright 2021.

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

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// RecommendationCheckpointSpec is the specification of the checkpoint object.
type RecommendationCheckpointSpec struct {
	// Name of the Recommendation object that stored VerticalPodAutoscalerCheckpoint object.
	RecommendationObjectName string `json:"recommendationObjectName,omitempty" protobuf:"bytes,1,opt,name=recommendationObjectName"`

	// Name of the checkpointed container.
	ContainerName string `json:"containerName,omitempty" protobuf:"bytes,2,opt,name=containerName"`
}

// RecommendationCheckpointStatus contains data of the checkpoint.
type RecommendationCheckpointStatus struct {
	// The time when the status was last refreshed.
	// +nullable
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty" protobuf:"bytes,1,opt,name=lastUpdateTime"`

	// Version of the format of the stored data.
	Version string `json:"version,omitempty" protobuf:"bytes,2,opt,name=version"`

	// Checkpoint of histogram for consumption of CPU.
	CPUHistogram HistogramCheckpoint `json:"cpuHistogram,omitempty" protobuf:"bytes,3,rep,name=cpuHistogram"`

	// Checkpoint of histogram for the target cpu.
	// We can calculate the target cpu confidence interval base on this histogram
	// so that we can estimate the reliability of the recommended target cpu.
	CPUConfidenceHistogram HistogramCheckpoint `json:"cpuConfidenceHistogram,omitempty" protobuf:"bytes,4,rep,name=cpuConfidenceHistogram"`

	// Checkpoint of histogram for consumption of memory.
	MemoryHistogram HistogramCheckpoint `json:"memoryHistogram,omitempty" protobuf:"bytes,5,rep,name=memoryHistogram"`

	// Checkpoint of histogram for the target memory.
	// We can calculate the target memory confidence interval base on this histogram
	// so that we can estimate the relibaility of the recommended target memory.
	MemoryConfidenceHistogram HistogramCheckpoint `json:"memoryConfidenceHistogram,omitempty" protobuf:"bytes,6,rep,name=cpuConfidenceHistogram"`

	// Timestamp of the fist sample from the histograms.
	// +nullable
	FirstSampleStart metav1.Time `json:"firstSampleStart,omitempty" protobuf:"bytes,7,opt,name=firstSampleStart"`

	// Timestamp of the last sample from the histograms.
	// +nullable
	LastSampleStart metav1.Time `json:"lastSampleStart,omitempty" protobuf:"bytes,8,opt,name=lastSampleStart"`

	// Total number of samples in the histograms.
	TotalSamplesCount int `json:"totalSamplesCount,omitempty" protobuf:"bytes,9,opt,name=totalSamplesCount"`
}

// HistogramCheckpoint contains data needed to reconstruct the histogram.
type HistogramCheckpoint struct {
	// Reference timestamp for samples collected within this histogram.
	// +nullable
	ReferenceTimestamp metav1.Time `json:"referenceTimestamp,omitempty" protobuf:"bytes,1,opt,name=referenceTimestamp"`

	// Map from bucket index to bucket weight.
	// +kubebuilder:validation:Type=object
	// +kubebuilder:validation:XPreserveUnknownFields
	BucketWeights map[int]uint32 `json:"bucketWeights,omitempty" protobuf:"bytes,2,opt,name=bucketWeights"`

	// Sum of samples to be used as denominator for weights from BucketWeights.
	TotalWeight float64 `json:"totalWeight,omitempty" protobuf:"bytes,3,opt,name=totalWeight"`
}

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=redcheckpoint

// RecommendationCheckpoint is the checkpoint of the internal state of client that
// is used for recovery after recommender's restart.
type RecommendationCheckpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Specification of the checkpoint.
	// +optional
	Spec RecommendationCheckpointSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Data of the checkpoint.
	// +optional
	Status RecommendationCheckpointStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RecommendationCheckpointList is a list of RecommendationCheckpoint objects.
type RecommendationCheckpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []RecommendationCheckpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RecommendationCheckpoint{}, &RecommendationCheckpointList{})
}
