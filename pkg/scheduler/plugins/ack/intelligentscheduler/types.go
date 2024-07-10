package intelligentscheduler

import (
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
type IntelligentSchedulerGpuInfo struct {
	//lock                 sync.RWMutex
	//available            bool   // 该GPU是否可用
	Index                int    `json:"index"`                    //GPU在该node上的index
	PhysicalType         string `json:"physicalGpuSpecification"` //GPU物理型号，如A100
	TotalMemory          int    `json:"totalMemory"`              //GPU总显存
	UsedMemory           int    `json:"usedMemory"`               //GPU以占用的显存
	UsedUtilization      int    `json:"usedUtilization"`          //GPU以占用的算力
	MemoryIsolation      bool   `json:"memoryIsolation"`          //是否显存隔离
	UtilizationIsolation bool   `json:"utilizationIsolation"`     //是否算力隔离
}

type IntelligentSchedulerNodeInfo struct {
	lock     *sync.RWMutex
	name     string
	GpuInfos map[int]*IntelligentSchedulerGpuInfo
}
