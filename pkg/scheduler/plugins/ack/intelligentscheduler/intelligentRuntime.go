package intelligentscheduler

import v1 "k8s.io/api/core/v1"

const (
	IntelligentSchedulerNodeLabel = "alipay/intelligent-schedule"
)

// score的计算可以放在这里
type IntelligentSchedulerRuntime struct {
	name            string
	nodeScorePolicy string
	gpuScorePolicy  string
}

func NewIntelligentSchedulerRuntime(name string, nodeScorePolicy string, gpuScorePolicy string) *IntelligentSchedulerRuntime {
	return &IntelligentSchedulerRuntime{
		name:            name,
		nodeScorePolicy: nodeScorePolicy,
		gpuScorePolicy:  gpuScorePolicy,
	}
}
func (r *IntelligentSchedulerRuntime) Name() string {
	return r.name
}

func (r *IntelligentSchedulerRuntime) getNodeScorePolicy() string {
	return r.nodeScorePolicy
}

func (r *IntelligentSchedulerRuntime) getGPUScorePolicy() string {
	return r.gpuScorePolicy
}

func (r *IntelligentSchedulerRuntime) Init() error {
	//TODO
	return nil
}

func isIntelligentNode(node *v1.Node) bool {
	// TODO 这个label需要在设置node gpu调度方式时设置
	if node.Labels[IntelligentSchedulerNodeLabel] == "true" {
		return true
	}
	return false
}
