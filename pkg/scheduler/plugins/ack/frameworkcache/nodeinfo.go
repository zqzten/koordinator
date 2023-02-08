package frameworkcache

import (
	"sync"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/gputopology"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/noderesource"
)

type ExtendNodeInfo struct {
	nodeName            string
	nodeResourceManager *noderesource.NodeResourceManager
	gpuTopologyCache    *gputopology.NodeGPUTopologyCache
	lock                *sync.RWMutex
}

func NewExtendNodeInfo(name string) *ExtendNodeInfo {
	return &ExtendNodeInfo{
		nodeName:            name,
		nodeResourceManager: noderesource.NewNodeResourceManager(),
		gpuTopologyCache:    gputopology.NewNodeGPUTopologyCache(),
		lock:                new(sync.RWMutex),
	}
}

func (e *ExtendNodeInfo) GetNodeResourceManager() *noderesource.NodeResourceManager {
	return e.nodeResourceManager
}

func (e *ExtendNodeInfo) GetGPUTopologyCache() *gputopology.NodeGPUTopologyCache {
	return e.gpuTopologyCache
}

func (e *ExtendNodeInfo) GetName() string {
	return e.nodeName
}

func (e *ExtendNodeInfo) GetLock() *sync.RWMutex {
	return e.lock
}
