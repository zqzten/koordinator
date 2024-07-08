package intelligentscheduler

import "k8s.io/kubernetes/pkg/scheduler/framework"

//type VirtualGpuSpecifications struct {
//	VirtualGpuSpecification string
//}

type VirtualGpuPodState struct {
	VGpuCount         int
	VGpuSpecification string
}

func (v VirtualGpuPodState) Clone() framework.StateData {

	// TODO 啥意思
	return v
}
