/*
Copyright 2022 The Koordinator Authors.

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

package unified

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

const (
	InternalSchedulingDomainPrefix = "internal.scheduling.koordinator.sh"

	AnnotationNVIDIADriverVersions = InternalSchedulingDomainPrefix + "/nvidia-driver-versions"
	AnnotationNVIDIAGPUSelector    = InternalSchedulingDomainPrefix + "/nvidia-gpu-selector"
	AnnotationDeviceTopology       = InternalSchedulingDomainPrefix + "/device-topology"
	AnnotationRDMATopology         = InternalSchedulingDomainPrefix + "/rdma-topology"
	AnnotationDevicePCIInfos       = InternalSchedulingDomainPrefix + "/device-pci-infos"
	// AnnotationGPUPartitions describes the GPU partition tables, the corresponding value type is GPUPartitionTable
	AnnotationGPUPartitions = InternalSchedulingDomainPrefix + "/gpu-partitions"

	AnnotationNetworkingVFMeta        = "networking.alibaba.com/vf-meta"
	AnnotationRundPassthoughPCI       = "io.alibaba.pouch.vm.passthru.pci"
	AnnotationRundNVSwitchOrder       = "io.katacontainers.prestart.gpu.nvswitch"
	AnnotationRundNvidiaDriverVersion = "io.katacontainers.prestart.gpu.nvidia-driver-version"
	AnnotationRundGPUDriverVersion    = "io.katacontainers.prestart.gpu.driver-version"

	NVSwitchDeviceType                     = schedulingv1alpha1.DeviceType("nvswitch")
	NVSwitchResource                       = "koordinator.sh/nvswitch"
	ResourcePPU        corev1.ResourceName = "alibabacloud.com/ppu"

	LabelGPUModelSeries       string = extension.NodeDomainPrefix + "/gpu-model-series"
	LabelEnableSharedNVSwitch string = extension.NodeDomainPrefix + "/enable-shared-nvswitch"
)

type NVIDIADriverVersions []string

type GPUSelector struct {
	DriverVersions []string `json:"driverVersions,omitempty"`
}

// DeviceTopology presents device topology info on a node
type DeviceTopology struct {
	NUMASockets []NUMASocketInfo `json:"numaSockets,omitempty"`
}

type NUMASocketInfo struct {
	// Index represents the NUMA Socket ID
	Index int32 `json:"index"`
	// NUMANodes contains all NUMA nodes belonging to the NUMA Socket
	NUMANodes []NUMANodeInfo `json:"numaNodes,omitempty"`
}

type NUMANodeInfo struct {
	// Index represents the NUMA Node ID
	Index int32 `json:"index"`
	// PCIESwitches contains all PCIE Switches belonging to the NUMA Node
	PCIESwitches []PCIESwitchInfo `json:"pcieSwitches,omitempty"`
}

type PCIESwitchInfo struct {
	// Index represents the PCIE Switch ID
	Index int32 `json:"index"`
	// GPUs contains all GPUs's minor id belonging to the PCI-E Switch
	GPUs []int32 `json:"gpus,omitempty"`
	// RDMAs contains all RDMAs's device information belonging to the PCI-E Switch
	RDMAs []*RDMADeviceInfo `json:"rdmas,omitempty"` // RDMA devices info
}

type RDMADeviceInfo struct {
	// Type represents the PF type
	Type RDMADeviceType `json:"type,omitempty"`
	// Name represents the NIC Name, e.g. "mlx5_bond_0"
	Name string `json:"name,omitempty"`
	// Minor represents the device minor id, e.g. 1
	Minor int32 `json:"minor"`
	// UVerbs represents the device uverbs path, e.g. "/dev/infiniband/uverbs1"
	UVerbs string `json:"uVerbs,omitempty"`
	// Bond represents the bond device id, e.g. "bond0"
	Bond uint32 `json:"bond,omitempty"`
	// BondSlaves contains all physical NICs, e.g. ["eth1","eth2"]
	BondSlaves []string `json:"bondSlaves,omitempty"`
	// Bandwidth represents the network bandwidth, the unit is Gbps
	Bandwidth int `json:"bw,omitempty"`
}

type RDMADeviceType string

const (
	RDMADeviceTypeSystem RDMADeviceType = "pf_system"
	RDMADeviceTypeWorker RDMADeviceType = "pf_worker"
)

type VFDeviceType string

const (
	VFDeviceTypeGeneral VFDeviceType = "vf_general"
	VFDeviceTypeGPU     VFDeviceType = "vf_gpu"
	VFDeviceTypeCPU     VFDeviceType = "vf_cpu"
	VFDeviceTypeStorage VFDeviceType = "vf_storage"
)

// RDMATopology describes VFs information per RDMA device
type RDMATopology struct {
	// VFs contains all VFs information, the key is RDMA device minor id.
	VFs map[int32][]VF `json:"vfs,omitempty"`
}

type VF struct {
	Name     string       `json:"name,omitempty"`
	BusID    string       `json:"busID,omitempty"`
	Minor    int32        `json:"minor"`
	Priority VFPriority   `json:"priority,omitempty"`
	Type     VFDeviceType `json:"type,omitempty"`
}

type VFPriority string

const (
	VFPriorityHigh VFPriority = "VFPriorityHigh"
	VFPriorityLow  VFPriority = "VFPriorityLow"
)

type DeviceAllocationExtension struct {
	*RDMAAllocatedExtension `json:",inline"`
}

type RDMAAllocatedExtension struct {
	VFs []*VF `json:"vfs,omitempty"`
}

type VFMeta struct {
	BondName   string   `json:"bond-name,omitempty"`
	BondSlaves []string `json:"bond-slaves,omitempty"`
	VFIndex    int      `json:"vf-index"`
	PCIAddress string   `json:"pci-address,omitempty"`
}

type DevicePCIInfo struct {
	Type  schedulingv1alpha1.DeviceType `json:"type,omitempty"`
	Minor int32                         `json:"minor,omitempty"`
	BusID string                        `json:"busID,omitempty"`
}

func GetDeviceTopology(annotations map[string]string) (*DeviceTopology, error) {
	var deviceTopology DeviceTopology
	if s, ok := annotations[AnnotationDeviceTopology]; ok {
		if err := json.Unmarshal([]byte(s), &deviceTopology); err != nil {
			return nil, err
		}
	}
	return &deviceTopology, nil
}

func GetRDMATopology(annotations map[string]string) (*RDMATopology, error) {
	var rdmaTopology RDMATopology
	if s, ok := annotations[AnnotationRDMATopology]; ok {
		if err := json.Unmarshal([]byte(s), &rdmaTopology); err != nil {
			return nil, err
		}
	}
	return &rdmaTopology, nil
}

func GetDriverVersions(annotations map[string]string) (NVIDIADriverVersions, error) {
	var versions NVIDIADriverVersions
	if s, ok := annotations[AnnotationNVIDIADriverVersions]; ok {
		if err := json.Unmarshal([]byte(s), &versions); err != nil {
			return nil, err
		}
	}
	return versions, nil
}

func GetGPUSelector(annotations map[string]string) (*GPUSelector, error) {
	var selector GPUSelector
	if s, ok := annotations[AnnotationNVIDIAGPUSelector]; ok {
		if err := json.Unmarshal([]byte(s), &selector); err != nil {
			return nil, err
		}
	}
	return &selector, nil
}

func SetVFMeta(pod *corev1.Pod, vfMetas []VFMeta) error {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	data, err := json.Marshal(vfMetas)
	if err != nil {
		return err
	}
	pod.Annotations[AnnotationNetworkingVFMeta] = string(data)
	return nil
}

func GetDevicePCIInfos(annotations map[string]string) ([]DevicePCIInfo, error) {
	var infos []DevicePCIInfo
	if s, ok := annotations[AnnotationDevicePCIInfos]; ok {
		if err := json.Unmarshal([]byte(s), &infos); err != nil {
			return nil, err
		}
	}
	return infos, nil
}

// 一些高级的 NVIDIA GPU 型号要求按照一定的组合关系才能获得最佳的 NVLINK 联通带宽。而这种组合关系称为 Partition。
// 分配时也要按照这个 Partition 分配，Partition 之外的组合关系是无效的，调度器会拒绝调度。

type GPUPartition struct {
	ID           int
	NumberOfGPUs int
	ModuleIDs    []int
}

type GPUPartitionTable map[int][]GPUPartition

// H800PartitionTables represents the default partition tables
var H800PartitionTables = map[int][]GPUPartition{
	8: {
		{ID: 0, NumberOfGPUs: 8, ModuleIDs: []int{1, 2, 3, 4, 5, 6, 7, 8}},
	},
	4: {
		{ID: 1, NumberOfGPUs: 4, ModuleIDs: []int{1, 2, 3, 4}},
		{ID: 2, NumberOfGPUs: 4, ModuleIDs: []int{5, 6, 7, 8}},
	},
	2: {
		{ID: 3, NumberOfGPUs: 2, ModuleIDs: []int{1, 3}},
		{ID: 4, NumberOfGPUs: 2, ModuleIDs: []int{2, 4}},
		{ID: 5, NumberOfGPUs: 2, ModuleIDs: []int{5, 7}},
		{ID: 6, NumberOfGPUs: 2, ModuleIDs: []int{6, 8}},
	},
	1: {
		// keep the following order to reduce fragments
		{ID: 7, NumberOfGPUs: 1, ModuleIDs: []int{1}},
		{ID: 9, NumberOfGPUs: 1, ModuleIDs: []int{3}},
		{ID: 8, NumberOfGPUs: 1, ModuleIDs: []int{2}},
		{ID: 10, NumberOfGPUs: 1, ModuleIDs: []int{4}},
		{ID: 11, NumberOfGPUs: 1, ModuleIDs: []int{5}},
		{ID: 13, NumberOfGPUs: 1, ModuleIDs: []int{7}},
		{ID: 12, NumberOfGPUs: 1, ModuleIDs: []int{6}},
		{ID: 14, NumberOfGPUs: 1, ModuleIDs: []int{8}},
	},
}

var H100PartitionTables = GPUPartitionTable{
	8: {
		{ID: 0, NumberOfGPUs: 8, ModuleIDs: []int{1, 2, 3, 4, 5, 6, 7, 8}},
	},
	4: {
		{ID: 1, NumberOfGPUs: 4, ModuleIDs: []int{1, 2, 3, 4}},
		{ID: 2, NumberOfGPUs: 4, ModuleIDs: []int{5, 6, 7, 8}},
	},
	2: {
		{ID: 3, NumberOfGPUs: 2, ModuleIDs: []int{1, 3}},
		{ID: 4, NumberOfGPUs: 2, ModuleIDs: []int{2, 4}},
		{ID: 5, NumberOfGPUs: 2, ModuleIDs: []int{5, 7}},
		{ID: 6, NumberOfGPUs: 2, ModuleIDs: []int{6, 8}},
	},
	1: {
		// keep the following order to reduce fragments
		{ID: 7, NumberOfGPUs: 1, ModuleIDs: []int{1}},
		{ID: 9, NumberOfGPUs: 1, ModuleIDs: []int{3}},
		{ID: 8, NumberOfGPUs: 1, ModuleIDs: []int{2}},
		{ID: 10, NumberOfGPUs: 1, ModuleIDs: []int{4}},
		{ID: 11, NumberOfGPUs: 1, ModuleIDs: []int{5}},
		{ID: 13, NumberOfGPUs: 1, ModuleIDs: []int{7}},
		{ID: 12, NumberOfGPUs: 1, ModuleIDs: []int{6}},
		{ID: 14, NumberOfGPUs: 1, ModuleIDs: []int{8}},
	},
}

var PartitionTables = map[string]map[int][]GPUPartition{
	"H800": H800PartitionTables,
	"H100": H100PartitionTables,
}

func MustAllocateGPUByPartition(node *corev1.Node) bool {
	if fabricManagerOperatingMode, ok := node.Labels[LabelEnableSharedNVSwitch]; ok {
		return fabricManagerOperatingMode == "true"
	}
	// TODO 这里为了兼容存量没有加该 Label 的 Rund 场景，仍然先保留按照卡型判断的逻辑，即对于 PartitionTable 中记录的卡型 default=true，none 或者 bareMetal 或者 fullPassThrough 需要显式申明 label 说明
	model := node.Labels[LabelGPUModelSeries]
	_, ok := PartitionTables[model]
	return ok
}

func GetGPUPartitionTableFromDevice(device *schedulingv1alpha1.Device) (GPUPartitionTable, error) {
	if rawGPUPartitionTable, ok := device.Annotations[AnnotationGPUPartitions]; ok && rawGPUPartitionTable != "" {
		gpuPartitionTable := GPUPartitionTable{}
		err := json.Unmarshal([]byte(rawGPUPartitionTable), &gpuPartitionTable)
		if err != nil {
			return nil, err
		}
		if gpuPartitionTable == nil {
			return nil, fmt.Errorf("invalid gpu partitions in device cr: %s", rawGPUPartitionTable)
		}
		return gpuPartitionTable, nil
	}
	return nil, nil
}

func GetGPUPartitionTable(node *corev1.Node) map[int][]GPUPartition {
	model := node.Labels[LabelGPUModelSeries]
	return PartitionTables[model]
}
