package intelligentscheduler

import (
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/intelligentscheduler/CRDs"
	"sync"
)

type IntelligentSchedulerArgs struct {
	// GPUMemoryScoreWeight is used to define the gpu memory score weight in score phase
	GPUMemoryScoreWeight *uint `json:"gpuMemoryScoreWeight"`
	// GPUUtilizationScoreWeight is used to define the gpu memory score weight in score phase
	GPUUtilizationScoreWeight *uint `json:"gpuUtilizationScoreWeight"`
	// NodeSelectorPolicy define the policy used to select a node in intelligent scheduler
	NodeSelectorPolicy string `json:"nodeSelectorPolicy"`
	// GpuSelectorPolicy define the policy used to select Gpus in a node in intelligent scheduler
	GpuSelectorPolicy string `json:"gpuSelectorPolicy"`
}

// IntelligentSchedulerGpuInfo反映node上每个GPU的状态信息
type PhysicalGpuInfo struct {
	lock                 *sync.RWMutex
	Node                 string `json:"node"`                     // 该GPU所在的node
	Index                int    `json:"index"`                    //GPU在该node上的index
	PhysicalType         string `json:"physicalGpuSpecification"` //GPU物理型号，如A100
	TotalMemory          int    `json:"totalMemory"`              //GPU总显存
	UsedMemory           int    `json:"usedMemory"`               //GPU以占用的显存
	UsedUtilization      int    `json:"usedUtilization"`          //GPU以占用的算力
	MemoryIsolation      bool   `json:"memoryIsolation"`          //是否显存隔离
	UtilizationIsolation bool   `json:"utilizationIsolation"`     //是否算力隔离
}

type NodeInfo struct {
	lock     *sync.RWMutex
	name     string
	GpuInfos map[int]*PhysicalGpuInfo // {idx: PhysicalGpuInfo}
}

func NewNodeInfo(name string) *NodeInfo {
	return &NodeInfo{
		lock: new(sync.RWMutex),
		name: name,
	}
}

func (info *NodeInfo) Clone() *NodeInfo {
	return &NodeInfo{
		lock: info.lock,
		name: info.name,
	}
}

func (info *NodeInfo) Reset(pgi *CRDs.PhysicalGpuInstance) {
	info.lock.Lock()
	defer info.lock.Unlock()
	physicalGpuInfo, ok := info.GpuInfos[pgi.Spec.Index]
	if !ok {
		info.GpuInfos[pgi.Spec.Index] = &PhysicalGpuInfo{
			lock:                 new(sync.RWMutex),
			Node:                 pgi.Spec.Node,
			Index:                pgi.Spec.Index,
			PhysicalType:         pgi.Spec.PhysicalGpuSpecification,
			TotalMemory:          pgi.Spec.TotalMemory,
			UsedMemory:           pgi.Status.UsedMemory,
			UsedUtilization:      pgi.Status.UsedUtilization,
			MemoryIsolation:      pgi.Status.MemoryIsolation,
			UtilizationIsolation: pgi.Status.UtilizationIsolation,
		}
	} else {
		physicalGpuInfo.lock.Lock()
		defer physicalGpuInfo.lock.Unlock()
		physicalGpuInfo.UsedMemory = pgi.Status.UsedMemory
		physicalGpuInfo.UsedUtilization = pgi.Status.UsedUtilization
		physicalGpuInfo.MemoryIsolation = pgi.Status.MemoryIsolation
		physicalGpuInfo.UtilizationIsolation = pgi.Status.UtilizationIsolation
	}
}

type VirtualGpuSpecInfo struct {
	lock                      *sync.RWMutex
	nickName                  string
	physicalGpuSpecifications []string
	description               string
	gpuMemory                 int
	gpuMemoryIsolation        bool
	gpuUtilization            int
	gpuUtilizationIsolation   bool
	isActive                  bool
}

func NewVirtualGpuSpecInfo(vgs *CRDs.VirtualGpuSpecification) *VirtualGpuSpecInfo {
	return &VirtualGpuSpecInfo{
		lock:                      new(sync.RWMutex),
		nickName:                  vgs.Spec.NickName,
		physicalGpuSpecifications: vgs.Spec.PhysicalGpuSpecifications,
		description:               vgs.Spec.Description,
		gpuMemory:                 vgs.Spec.GPUMemory,
		gpuMemoryIsolation:        vgs.Spec.GPUMemoryIsolation,
		gpuUtilization:            vgs.Spec.GPUUtilization,
		gpuUtilizationIsolation:   vgs.Spec.GPUUtilizationIsolation,
		isActive:                  vgs.Status.IsActive,
	}
}

func (info *VirtualGpuSpecInfo) Reset(vgs *CRDs.VirtualGpuSpecification) {
	info.lock.Lock()
	defer info.lock.Unlock()
	info.nickName = vgs.Spec.NickName
	info.physicalGpuSpecifications = vgs.Spec.PhysicalGpuSpecifications
	info.description = vgs.Spec.Description
	info.gpuMemory = vgs.Spec.GPUMemory
	info.gpuMemoryIsolation = vgs.Spec.GPUMemoryIsolation
	info.gpuUtilization = vgs.Spec.GPUUtilization
	info.gpuUtilizationIsolation = vgs.Spec.GPUUtilizationIsolation
	info.isActive = vgs.Status.IsActive
}

type VirtualGpuInstanceInfo struct {
	lock                     *sync.RWMutex
	Name                     string // 虚拟GPU实例对应vgi crd的name
	VirtualGpuSpecification  string // 虚拟GPU实例的规格名称
	Pod                      string // 虚拟GPU实例所属的pod
	Node                     string // 虚拟GPU实例所在节点, 只有Running的时候有值
	Status                   string // 状态信息, NoQuota/Pending/Allocated/Running/Releasing
	GPUIndex                 int    // 使用哪张物理卡
	GPUDeviceId              string // 唯一标识某张物理卡
	PhysicalGpuSpecification string // 使用的物理卡型号
}

func NewVirtualGpuInstanceInfo(vgi *CRDs.VirtualGpuInstance) *VirtualGpuInstanceInfo {
	return &VirtualGpuInstanceInfo{
		lock:                     new(sync.RWMutex),
		Name:                     vgi.Name,
		VirtualGpuSpecification:  vgi.Spec.VirtualGpuSpecification,
		Pod:                      vgi.Status.Pod,
		Node:                     vgi.Status.Node,
		GPUIndex:                 vgi.Status.GPUIndex,
		GPUDeviceId:              vgi.Status.GPUDeviceId,
		PhysicalGpuSpecification: vgi.Status.PhysicalGpuSpecification,
		Status:                   vgi.Status.Status,
	}
}

func (info *VirtualGpuInstanceInfo) Reset(vgi *CRDs.VirtualGpuInstance) {
	info.lock.Lock()
	defer info.lock.Unlock()
	info.Name = vgi.Name
	info.VirtualGpuSpecification = vgi.Spec.VirtualGpuSpecification
	info.Pod = vgi.Status.Pod
	info.Node = vgi.Status.Node
	info.GPUIndex = vgi.Status.GPUIndex
	info.GPUDeviceId = vgi.Status.GPUDeviceId
	info.PhysicalGpuSpecification = vgi.Status.PhysicalGpuSpecification
	info.Status = vgi.Status.Status
}
