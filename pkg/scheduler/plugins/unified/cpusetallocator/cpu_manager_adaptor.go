package cpusetallocator

import (
	"k8s.io/apimachinery/pkg/types"

	schedulingconfig "github.com/koordinator-sh/koordinator/apis/scheduling/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
)

type cpuManagerAdapter struct {
	nodenumaresource.CPUManager
	updater *cpuSharePoolUpdater
}

func newCPUManagerAdapter(cpuManager nodenumaresource.CPUManager, updater *cpuSharePoolUpdater) nodenumaresource.CPUManager {
	return &cpuManagerAdapter{
		CPUManager: cpuManager,
		updater:    updater,
	}
}

func (m *cpuManagerAdapter) UpdateAllocatedCPUSet(nodeName string, podUID types.UID, cpuset nodenumaresource.CPUSet, cpuExclusivePolicy schedulingconfig.CPUExclusivePolicy) {
	m.CPUManager.UpdateAllocatedCPUSet(nodeName, podUID, cpuset, cpuExclusivePolicy)
	m.updater.asyncUpdate(nodeName)
}

func (m *cpuManagerAdapter) Free(nodeName string, podUID types.UID) {
	m.CPUManager.Free(nodeName, podUID)
	m.updater.asyncUpdate(nodeName)
}
