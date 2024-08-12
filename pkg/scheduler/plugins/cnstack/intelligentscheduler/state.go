package intelligentscheduler

import (
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sync"
)

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

func (v *VirtualGpuPodState) getCount() int {
	v.lock.RLock()
	defer v.lock.RUnlock()
	return v.VGpuCount
}

func (v *VirtualGpuPodState) getSpec() string {
	v.lock.RLock()
	defer v.lock.RUnlock()
	return v.VGpuSpecification
}

type VirtualGpuInstanceState struct {
	lock     sync.RWMutex
	vgiNames []string
}

func (v *VirtualGpuInstanceState) Clone() framework.StateData {
	v.lock.RLock()
	defer v.lock.RUnlock()
	newState := &VirtualGpuInstanceState{
		vgiNames: make([]string, len(v.vgiNames)),
	}
	for k, v := range v.vgiNames {
		newState.vgiNames[k] = v
	}
	return newState
}

func (v *VirtualGpuInstanceState) getVgiNames() []string {
	v.lock.RLock()
	defer v.lock.RUnlock()
	return v.vgiNames
}

type NodeState struct {
	lock     sync.RWMutex
	nodeInfo *NodeInfo
}

func (v *NodeState) Clone() framework.StateData {
	v.lock.RLock()
	defer v.lock.RUnlock()
	newState := &NodeState{
		nodeInfo: v.nodeInfo.Clone(),
	}
	return newState
}

func (v *NodeState) getNodeInfo() *NodeInfo {
	v.lock.RLock()
	defer v.lock.RUnlock()
	return v.nodeInfo.Clone()
}
