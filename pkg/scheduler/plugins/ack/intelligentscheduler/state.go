package intelligentscheduler

import (
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sync"
)

//type VirtualGpuSpecifications struct {
//	VirtualGpuSpecification string
//}

type VirtualGpuPodState struct {
	lock              sync.RWMutex
	VGpuCount         int
	VGpuSpecification string
}

func (v *VirtualGpuPodState) Clone() framework.StateData {
	v.lock.RLock()
	defer v.lock.RUnlock()
	newState := &VirtualGpuPodState{
		VGpuCount:         v.VGpuCount,
		VGpuSpecification: v.VGpuSpecification,
	}
	return newState
}

type VirtualGpuInstanceState struct {
	lock     sync.RWMutex
	vgiNames map[int]string // {idx: name}
}

func (v *VirtualGpuInstanceState) Clone() framework.StateData {
	v.lock.RLock()
	defer v.lock.RUnlock()
	newState := &VirtualGpuInstanceState{
		vgiNames: make(map[int]string, len(v.vgiNames)),
	}
	for k, v := range v.vgiNames {
		newState.vgiNames[k] = v
	}
	return newState
}

type NodeState struct {
	lock     sync.RWMutex
	nodeInfo NodeInfo
}

func (v *NodeState) Clone() framework.StateData {
	v.lock.RLock()
	defer v.lock.RUnlock()
	newState := &NodeState{
		nodeInfo: *v.nodeInfo.Clone(),
	}
	return newState
}
