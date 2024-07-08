package intelligentscheduler

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
