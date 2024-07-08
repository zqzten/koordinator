/*
Copyright 2021 Alibaba Cloud.

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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type MetricCollectPolicy struct {
	// Metric aggregate duration by seconds
	AggregateDurationSeconds *int64 `json:"aggregateDurationSeconds,omitempty"`
}

type NodeMetricInfo struct {
	NodeUsed ResourceMap `json:"nodeResourceUsed,omitempty"`
}

type PodMetricInfo struct {
	PodNamespace string      `json:"podNamespace,omitempty"`
	PodName      string      `json:"podName,omitempty"`
	PodUsed      ResourceMap `json:"podResourceUsed,omitempty"`
}

// NodeMetricSpec defines the desired state of NodeMetric
type NodeMetricSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// CollectPolicy determines metric type of agent reporter, e.g. average of 5 minutes
	CollectPolicy *MetricCollectPolicy `json:"metricCollectPolicy,omitempty"`
}

// NodeMetricStatus defines the observed state of NodeMetric
type NodeMetricStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	UpdateTimestamp *metav1.Time     `json:"updateTime,omitempty"`
	NodeMetric      *NodeMetricInfo  `json:"nodeMetric,omitempty"`
	PodMetric       []*PodMetricInfo `json:"podsMetric,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status

// NodeMetric is the Schema for the nodemetrics API
type NodeMetric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeMetricSpec   `json:"spec,omitempty"`
	Status NodeMetricStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeMetricList contains a list of NodeMetric
type NodeMetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeMetric `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeMetric{}, &NodeMetricList{})
}
