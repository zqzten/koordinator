package intelligentscheduler

import v1 "k8s.io/api/core/v1"

const (
	GPUResourceCountName = "aliyun.com/gpu-count"
)

func getNodeGPUCount(node *v1.Node) int {
	val, ok := node.Status.Allocatable[v1.ResourceName(GPUResourceCountName)]
	if !ok {
		return int(0)
	}
	return int(val.Value())
}
