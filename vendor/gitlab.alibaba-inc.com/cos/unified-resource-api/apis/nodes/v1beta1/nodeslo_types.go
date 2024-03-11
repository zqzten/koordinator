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

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type BlockType string

const (
	// BlockTypeDevice 配置单个块设备，应用于配置磁盘的读写延时
	BlockTypeDevice BlockType = "device"
	// BlockTypeVolumeGroup 配置 VG，应用于 BE 类应用磁盘配置
	BlockTypeVolumeGroup BlockType = "volumegroup"
	// BlockTypePodVolume 配置单个 Pod 存储卷，包含持久卷和临时卷，应用于配置单个 Pod 的存储卷
	BlockTypePodVolume BlockType = "podvolume"
	// BlockTypeRootfs 配置容器 rootfs 可写层，应用于配置 Pod 在 graph disk 上的读写限流
	BlockTypeRootfs BlockType = "rootfs"
	// BlockTypeHostPath 配置容器本地盘 mount point，应用于配置容器在本地盘上的读写限流
	BlockTypeHostPath BlockType = "hostpath"
)

type IOCfg struct {
	// 指定 Device 的 IOPS 上限
	ReadIOPS  *int64 `json:"readIOPS,omitempty"`
	WriteIOPS *int64 `json:"writeIOPS,omitempty"`
	// 指定 Device 的 BPS 上限
	ReadBPS  *int64 `json:"readBPS,omitempty"`
	WriteBPS *int64 `json:"writeBPS,omitempty"`
	// 在离线 IO 权重比例，在线单个 Pod 默认 100，离线总 Pods 权重值 0~100
	// 用于配置 BE组 和 单个Pod
	IOWeightPercent *int64 `json:"ioWeightPercent,omitempty"`
	// 配置磁盘读写延时，即当 95% 的读写延时超过设定值时，认为磁盘已经饱和，则将触发在离线 IO 按权重比例分配特性。
	// 该变量仅用于 RootClass
	// 用于配置整块磁盘
	ReadLatency  *int64 `json:"readLatency,omitempty"`
	WriteLatency *int64 `json:"writeLatency,omitempty"`
}

type BlockCfg struct {
	Name string `json:"name,omitempty"`
	// 对应的device name，单pod的话对应于volume name
	BlockType BlockType `json:"type,omitempty"`
	IOCfg     IOCfg     `json:"ioCfg,omitempty"`
}

type BlkIOQOS struct {
	// Device IO 配置
	Blocks []*BlockCfg `json:"blocks,omitempty"`
}

type BlkIOQOSCfg struct {
	Enable   *bool `json:"enable,omitempty"`
	BlkIOQOS `json:",inline"`
}

type ResourceQOSStrategy struct {
	// whether the strategy is enabled, default = true
	Enable *bool `json:"enable,omitempty"`

	// ResourceQOS for LSR pods.
	LSRClass *ResourceQOS `json:"lsrClass,omitempty"`

	// ResourceQOS for LS pods.
	LSClass *ResourceQOS `json:"lsClass,omitempty"`

	// ResourceQOS for BE pods.
	BEClass *ResourceQOS `json:"beClass,omitempty"`

	// ResourceQOS for system pods
	SystemClass *ResourceQOS `json:"systemClass,omitempty"`

	// ResourceQOS for root cgroup.
	CgroupRoot *ResourceQOS `json:"cgroupRoot,omitempty"`
}

type ResourceQOS struct {
	CPUQOS *CPUQOSCfg `json:"cpuQOS,omitempty"`

	MemoryQOS *MemoryQOSCfg `json:"memoryQOS,omitempty"`

	BlkIOQOS *BlkIOQOSCfg `json:"blkioQOS,omitempty"`

	ResctrlQOS *ResctrlQOSCfg `json:"resctrlQOS,omitempty"`

	NetworkQOS *NetworkQOSCfg `json:"networkQOS,omitempty"`

	// QOSConfig stores the qos config in an old version of ResourceQOS
	QOSConfig `json:",inline"`
}

// CPUQOSCfg stores node-level config of cpu qos
type CPUQOSCfg struct {
	// Enable indicates whether the cpu qos is enabled.
	Enable *bool `json:"enable,omitempty"`
	CPUQOS `json:",inline"`
}

// MemoryQOSCfg stores node-level config of memory qos
type MemoryQOSCfg struct {
	// Enable indicates whether the memory qos is enabled (Cluster default: false).
	// This field is used for node-level control, while pod-level configuration is done with MemoryQOS and `Policy`
	// instead of an `Enable` option. Please view the differences between MemoryQOSCfg and PodMemoryQOSConfig structs.
	Enable    *bool `json:"enable,omitempty"`
	MemoryQOS `json:",inline"`
}

// ResctrlQOSCfg stores node-level config of resctrl qos
type ResctrlQOSCfg struct {
	// Enable indicates whether the resctrl qos is enabled.
	Enable     *bool `json:"enable,omitempty"`
	ResctrlQOS `json:",inline"`
}

// NetworkQOSCfg stores node-level config of network qos
type NetworkQOSCfg struct {
	// ref: https://yuque.antfin.com/docs/share/ae07d965-46ce-4143-ac8d-48ff2bd789a2
	// Enable indicates whether the network qos is enabled.
	Enable     *bool `json:"enable,omitempty"`
	NetworkQOS `json:",inline"`
}

type CPUQOS struct {
	// group identity value for pods, default = 0
	GroupIdentity *int64 `json:"groupIdentity,omitempty"`
}

// MemoryQOS enables memory qos features based on alibaba cloud linux 2, including memcg qos, wmark_ratio, wmark_min_adj.
// ref: alikernel memory qos: https://yuque.antfin-inc.com/docs/share/54390a3e-1e77-427c-9f45-fb167a0ed02f#Pr3QH
// ref: slo-manager memory qos: https://yuque.antfin.com/docs/share/0baad621-16ba-40fa-84f9-2a2fad5c4a8e?#
type MemoryQOS struct {
	// memcg qos
	// If enabled, memcg qos will be set by the agent, where some fields are implicitly calculated from pod spec.
	// 1. `memory.min` := spec.requests.memory * minLimitFactor / 100 (use 0 if requests.memory is not set)
	// 2. `memory.low` := spec.requests.memory * lowLimitFactor / 100 (use 0 if requests.memory is not set)
	// 3. `memory.limit_in_bytes` := spec.limits.memory (set $node.allocatable.memory if limits.memory is not set)
	// 4. `memory.high` := memory.limit_in_bytes * throttlingFactor / 100 (use "max" if memory.high <= memory.min)
	// MinLimitPercent specifies the minLimitFactor percentage to calculate `memory.min`, which protects memory
	// from global reclamation when memory usage does not exceed the min limit.
	// Cluster default: 100; Close: 0.
	MinLimitPercent *int64 `json:"minLimitPercent,omitempty"`
	// LowLimitPercent specifies the lowLimitFactor percentage to calculate `memory.low`, which TRIES BEST
	// protecting memory from global reclamation when memory usage does not exceed the low limit unless no unprotected
	// memcg can be reclaimed.
	// NOTE: `memory.low` should be larger than `memory.min`. If spec.requests.memory == spec.limits.memory,
	// pod `memory.low` and `memory.high` become invalid, while `memory.wmark_ratio` is still in effect.
	// Cluster default: 0; Close: 0.
	LowLimitPercent *int64 `json:"lowLimitPercent,omitempty"`
	// ThrottlingPercent specifies the throttlingFactor percentage to calculate `memory.high` with pod
	// memory.limits or node allocatable memory, which triggers memcg direct reclamation when memory usage exceeds.
	// Lower the factor brings more heavier reclaim pressure.
	// Cluster default: 80 (compatible with default of upstream memory qos); Close: 0.
	ThrottlingPercent *int64 `json:"throttlingPercent,omitempty"`

	// wmark_ratio
	// Async memory reclamation is triggered when cgroup memory usage exceeds `memory.wmark_high` and the reclamation
	// stops when usage is below `memory.wmark_low`. Basically,
	// `memory.wmark_high` := min(memory.high, memory.limit_in_bytes) * memory.wmark_scale_factor
	// `memory.wmark_low` := min(memory.high, memory.limit_in_bytes) * (memory.wmark_ratio - memory.wmark_scale_factor)
	// WmarkRatio specifies `memory.wmark_ratio` that help calculate `memory.wmark_high`, which triggers async
	// memory reclamation when memory usage exceeds.
	// Cluster default: 95; Close: 0.
	WmarkRatio *int64 `json:"wmarkRatio,omitempty"`
	// WmarkScalePermill specifies `memory.wmark_scale_factor` that helps calculate `memory.wmark_low`, which
	// stops async memory reclamation when memory usage belows.
	// Cluster default: 20; Close: 50.
	WmarkScalePermill *int64 `json:"wmarkScalePermill,omitempty"`

	// wmark_min_adj
	// WmarkMinAdj specifies `memory.wmark_min_adj` which adjusts per-memcg threshold for global memory
	// reclamation. Lower the factor brings later reclamation.
	// The adjustment uses different formula for different value range.
	// [-25, 0)：global_wmark_min' = global_wmark_min + (global_wmark_min - 0) * wmarkMinAdj
	// (0, 50]：global_wmark_min' = global_wmark_min + (global_wmark_low - global_wmark_min) * wmarkMinAdj
	// Cluster default: [LSR:-25, LS:-25, BE:50]; Close: [LSR:0, LS:0, BE:0].
	WmarkMinAdj *int64 `json:"wmarkMinAdj,omitempty"`

	// TODO: improve usages of oom priority and oom kill group
	PriorityEnable *int64 `json:"priorityEnable,omitempty"`
	Priority       *int64 `json:"priority,omitempty"`
	OomKillGroup   *int64 `json:"oomKillGroup,omitempty"`
}

type ResctrlQOS struct {
	// LLC available range start for pods by percentage, default 0
	CATRangeStartPercent *int64 `json:"catRangeStartPercent,omitempty"`
	// LLC available range end for pods by percentage, default 100
	CATRangeEndPercent *int64 `json:"catRangeEndPercent,omitempty"`
	// MBA percent, default 100
	MBAPercent *int64 `json:"mbaPercent,omitempty"`
}

type NetworkQOS struct {
	// ref: https://yuque.antfin.com/docs/share/ae07d965-46ce-4143-ac8d-48ff2bd789a2
	BandwidthRequestPercent *int64      `json:"bandwidthRequestPercent,omitempty"`
	BandwidthLimitPercent   *int64      `json:"bandwidthLimitPercent,omitempty"`
	NetQOS                  NetQOSLevel `json:"netQOS,omitempty"`
	PacketDSCP              *int64      `json:"packetDSCP,omitempty"`
}

type PodMemoryQOSPolicy string

const (
	// PodMemoryQOSPolicyDefault indicates pod inherits node-level config
	PodMemoryQOSPolicyDefault PodMemoryQOSPolicy = "default"
	// PodMemoryQOSPolicyNone indicates pod disables memory qos
	PodMemoryQOSPolicyNone PodMemoryQOSPolicy = "none"
	// PodMemoryQOSPolicyAuto indicates pod uses a recommended config
	PodMemoryQOSPolicyAuto PodMemoryQOSPolicy = "auto"
)

type PodMemoryQOSConfig struct {
	// Policy indicates the qos plan; use "default" if empty
	Policy    PodMemoryQOSPolicy `json:"policy,omitempty"`
	MemoryQOS `json:",inline"`
	// ContainerCfg []MemQOS `json:"containerCfg,omitempty"`
}

type NetQOSLevel string

const (
	GoldNetQOS    NetQOSLevel = "gold"
	SilverNetQOS  NetQOSLevel = "silver"
	CopperNetQOS  NetQOSLevel = "copper"
	UnknownNetQOS NetQOSLevel = ""
)

// QOSConfig is the qos config fields used in slo-manager v0.2.0.
// DEPRECATED: Use ResourceQOS instead.
type QOSConfig struct {
	// CPUQOS
	GroupIdentity *int64 `json:"groupIdentity,omitempty"`

	// MemoryQOS
	MemoryQOSEnable         *bool  `json:"memoryQOSEnable,omitempty"`
	MemoryMinLimitPercent   *int64 `json:"memoryMinLimitPercent,omitempty"`
	MemoryLowLimitPercent   *int64 `json:"memoryLowLimitPercent,omitempty"`
	MemoryThrottlingPercent *int64 `json:"memoryThrottlingPercent,omitempty"`
	MemoryWmarkRatio        *int64 `json:"memoryWmarkRatio,omitempty"`
	MemoryWmarkScalePermill *int64 `json:"memoryWmarkScalePermill,omitempty"`
	MemoryWmarkMinAdj       *int64 `json:"memoryWmarkMinAdj,omitempty"`
	MemoryPriorityEnable    *int64 `json:"memoryPriorityEnable,omitempty"`
	MemoryPriority          *int64 `json:"memoryPriority,omitempty"`
	MemoryOomKillGroup      *int64 `json:"memoryOomKillGroup,omitempty"`

	// ResctrlQOS
	CATRangeStartPercent *int64 `json:"catRangeStartPercent,omitempty"`
	CATRangeEndPercent   *int64 `json:"catRangeEndPercent,omitempty"`
	MBAPercent           *int64 `json:"mbaPercent,omitempty"`

	// NetQOS
	BandwidthRequestPercent *int64      `json:"bandwidthRequestPercent,omitempty"`
	BandwidthLimitPercent   *int64      `json:"bandwidthLimitPercent,omitempty"`
	NetQOS                  NetQOSLevel `json:"netQOS,omitempty"`
	PacketDSCP              *int64      `json:"packetDSCP,omitempty"`
}

type CPUSuppressPolicy string

const (
	CPUSetPolicy      CPUSuppressPolicy = "cpuset"
	CPUCfsQuotaPolicy CPUSuppressPolicy = "cfsQuota"
)

type ResourceThresholdStrategy struct {
	// whether the strategy is enabled, default = true
	Enable *bool `json:"enable,omitempty"`
	// cpu suppress threshold percentage (0,100), default = 65
	CPUSuppressThresholdPercent *int64 `json:"cpuSuppressThresholdPercent,omitempty"`
	//CPUSuppressPolicy
	CPUSuppressPolicy CPUSuppressPolicy `json:"cpuSuppressPolicy,omitempty"`
	// cpu evict threshold percentage (0,100), default = 70
	CPUEvictThresholdPercent *int64 `json:"cpuEvictThresholdPercent,omitempty"`
	//if be CPU RealLimit/allocatedLimit > CPUEvictBESatisfactionUpperPercent, then stop evict bepods
	CPUEvictBESatisfactionUpperPercent *int64 `json:"cpuEvictBESatisfactionUpperPercent,omitempty"`
	//if be CPU (RealLimit/allocatedLimit < CPUEvictBESatisfactionLowerPercent and usage nearly 100%) continue CPUEvictTimeWindowSeconds,then start evict
	CPUEvictBESatisfactionLowerPercent *int64 `json:"cpuEvictBESatisfactionLowerPercent,omitempty"`
	// cpu evict start after continue avg(cpuusage) > CPUEvictThresholdPercent in seconds
	CPUEvictTimeWindowSeconds *int64 `json:"cpuEvictTimeWindowSeconds,omitempty"`
	// upper: memory evict threshold percentage (0,100), default = 70
	MemoryEvictThresholdPercent *int64 `json:"memoryEvictThresholdPercent,omitempty"`
	// lower: memory release util usage under MemoryEvictLowerPercent,default : $MemoryEvictThresholdPercent - 2
	MemoryEvictLowerPercent *int64 `json:"memoryEvictLowerPercent,omitempty"`
}

type SystemStrategy struct {
	// for /proc/sys/vm/min_free_kbytes, min_free_kbytes = minFreeKbytesFactor * nodeTotalMemory /10000
	MinFreeKbytesFactor *int64 `json:"minFreeKbytesFactor,omitempty"`
	// /proc/sys/vm/watermark_scale_factor
	WatermarkScaleFactor *int64 `json:"watermarkScaleFactor,omitempty"`
}

type CPUBurstPolicy string

const (
	// disable cpu burst policy
	CPUBurstNone = "none"
	// only enable cpu burst policy by setting cpu.cfs_burst_us
	CPUBurstOnly = "cpuBurstOnly"
	// only enable cfs quota burst policy by scale up cpu.cfs_quota_us if pod throttled
	CFSQuotaBurstOnly = "cfsQuotaBurstOnly"
	// enable both
	CPUBurstAuto = "auto"
)

type CPUBurstConfig struct {
	Policy CPUBurstPolicy `json:"policy,omitempty"`
	// cpu burst percentage for setting cpu.cfs_burst_us, legal range: [0, 10000], default as 1000 (1000%)
	CPUBurstPercent *int64 `json:"cpuBurstPercent,omitempty"`
	// pod cfs quota scale up ceil percentage, default = 300 (300%)
	CFSQuotaBurstPercent *int64 `json:"cfsQuotaBurstPercent,omitempty"`
	// specifies a period of time for pod can use at burst, default = -1 (unlimited)
	CFSQuotaBurstPeriodSeconds *int64 `json:"cfsQuotaBurstPeriodSeconds,omitempty"`
}

// ref https://yuque.antfin-inc.com/docs/share/84f8e2fd-272f-42c5-99fa-2c987406a4bd
type CPUBurstStrategy struct {
	CPUBurstConfig `json:",inline"`
	// scale down cfs quota if node cpu overload, default = 50
	SharePoolThresholdPercent *int64 `json:"sharePoolThresholdPercent,omitempty"`
}

// NodeSLOSpec defines the desired state of NodeSLO
type NodeSLOSpec struct {
	//node global system config
	SystemStrategy *SystemStrategy `json:"systemStrategy,omitempty"`
	// BE pods will be limited if node resource usage overload
	ResourceUsedThresholdWithBE *ResourceThresholdStrategy `json:"resourceUsedThresholdWithBE,omitempty"`
	// QoS config strategy for pods of different qos-class
	ResourceQOSStrategy *ResourceQOSStrategy `json:"resourceQOSStrategy,omitempty"`
	// CPU Burst Strategy
	CPUBurstStrategy *CPUBurstStrategy `json:"cpuBurstStrategy,omitempty"`
	// Compatible for Cgroups CRD from ack-cgroup-controller, includes pod cgroup args to modify
	// DO NOT reuse this for new features
	Cgroups []*resourcesv1alpha1.NodeCgroupsInfo `json:"cgroups,omitempty"`
}

// NodeSLOStatus defines the observed state of NodeSLO
type NodeSLOStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status

// NodeSLO is the Schema for the nodeslos API
type NodeSLO struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeSLOSpec   `json:"spec,omitempty"`
	Status NodeSLOStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeSLOList contains a list of NodeSLO
type NodeSLOList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeSLO `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeSLO{}, &NodeSLOList{})
}
