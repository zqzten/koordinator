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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LabelNodeASWID           = "alibabacloud.com/asw-id"            //  Access switch, or ASW, is the "first-hop" switch that connects nodes to the network.
	LabelNodePointOfDelivery = "alibabacloud.com/point-of-delivery" // A point of delivery, or PoD, is a hierarchical network containing multiple ASWs.
	// Commucating with nodes in a different PoD is subject to network contention and potentially unstable.
	// For more info of node network topology, please refer to https://yuque.antfin.com/docs/share/b5abbb26-6d06-47b2-9aa4-e71ac3e1f146#rtobf
)

// DeviceSpec defines the desired state of Device
type DeviceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	NodeName       string          `json:"nodeName"`
	Devices        []DeviceInfo    `json:"devices"`
	DeviceTopology *DeviceTopology `json:"deviceTopology,omitempty"`
}

// DeviceTopology presents device topology info on a node
type DeviceTopology struct {
	CPUSockets      []CPUSocketInfo `json:"cpuSockets,omitempty"`
	ASWIDs          []string        `json:"aswIDs,omitempty"`          // a node may connect to multiple ASWs
	PointOfDelivery string          `json:"pointOfDelivery,omitempty"` // point of delivery
}

type CPUSocketInfo struct {
	Index     uint32         `json:"index"`
	NUMANodes []NUMANodeInfo `json:"numaNodes,omitempty"`
}

type NUMANodeInfo struct {
	Index        uint32           `json:"index"`
	PCIeSwitches []PCIeSwitchInfo `json:"pcieSwitches,omitempty"`
}

type PCIeSwitchInfo struct {
	Index       uint32          `json:"index"`
	GPUs        []int32         `json:"gpus,omitempty"`        // GPU minors
	RDMADevices RDMADeviceInfos `json:"rdmaDevices,omitempty"` // RDMA devices info
}

type DeviceInfo struct {
	DeviceType   DeviceType        `json:"deviceType"`
	ExtendInfos  map[string]string `json:"extendInfos,omitempty"`
	DeviceSource `json:",inline"`
}

type DeviceType string

const (
	GPU  DeviceType = "gpu-device"
	NPU  DeviceType = "npu-device"
	FPGA DeviceType = "fpga-device"
	RDMA DeviceType = "rdma-device"
	POV  DeviceType = "pov-device"
)

type DeviceSource struct {
	GPU *GPUDeviceSource `json:"gpu,omitempty"`
	// add your device here
	Common *CommonDeviceSource `json:"common,omitempty"`
}

// DeviceStatus defines the observed state of Device
type DeviceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	DeviceAllocations []DeviceAllocation `json:"deviceAllocations,omitempty"`
}

type DeviceAllocation struct {
	DeviceType DeviceType `json:"deviceType"` // device-name
	Allocation Allocation `json:"allocation"`
}

type Allocation struct {
	Entries []AllocationEntry `json:"entries"`
}

type AllocationEntry struct {
	Kind      string `json:"kind"` // pod or task
	Name      string `json:"name"` // pod or task name
	Namespace string `json:"namespace"`
	AllocSpec string `json:"allocSpec"` // json format. record the detailed gpu allocation of containers across gpus.
}

type CommonDeviceSource struct {
	List []CommonDeviceSpec `json:"list"`
}

type CommonDeviceSpec struct {
	ID        string                       `json:"id"`       // Device UUID
	Resources map[string]resource.Quantity `json:"resource"` // map of sub-resources
	Health    bool                         `json:"health"`
	Minor     int32                        `json:"minor"` // index starting from 0
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster

// Device is the Schema for the devices API
type Device struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceSpec   `json:"spec,omitempty"`
	Status DeviceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DeviceList contains a list of Device
type DeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Device `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Device{}, &DeviceList{})
}
