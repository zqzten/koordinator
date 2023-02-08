package gputopology

import (
	"sync"

	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/frameworkcache"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/gputopology"
)

type GPUTopologyCache struct {
	podToNode      map[apitypes.UID]string
	frameworkCache *frameworkcache.FrameworkCache
	lock           *sync.RWMutex
}

func NewTopologyCache(c *frameworkcache.FrameworkCache) *GPUTopologyCache {
	return &GPUTopologyCache{
		podToNode:      map[apitypes.UID]string{},
		frameworkCache: c,
		lock:           new(sync.RWMutex),
	}
}

func (gtc *GPUTopologyCache) AddPod(podAllocation *gputopology.PodTopologyAllocation) {
	gtc.lock.Lock()
	defer gtc.lock.Unlock()
	if podAllocation.NodeName == "" {
		return
	}
	oldNodeName, ok := gtc.podToNode[podAllocation.PodUID]
	if ok {
		gtc.frameworkCache.GetExtendNodeInfo(oldNodeName).GetGPUTopologyCache().RemovePod(podAllocation.PodUID)
	}
	gtc.podToNode[podAllocation.PodUID] = podAllocation.NodeName
	gtc.frameworkCache.GetExtendNodeInfo(podAllocation.NodeName).GetGPUTopologyCache().AddPod(podAllocation)
}

func (gtc *GPUTopologyCache) RemovePod(podUID apitypes.UID) bool {
	gtc.lock.Lock()
	defer gtc.lock.Unlock()
	nodeName := gtc.podToNode[podUID]
	if nodeName != "" {
		topologyCache := gtc.frameworkCache.GetExtendNodeInfo(nodeName).GetGPUTopologyCache()
		topologyCache.RemovePod(podUID)
		klog.V(5).Infof("Node: %v,GPUTopologyCacheAfterRemovePod(%v): %v", nodeName, podUID, topologyCache.String(false))
		delete(gtc.podToNode, podUID)
		return true
	}
	return false
}

func (gtc *GPUTopologyCache) SetPhase(podUID apitypes.UID, phase gputopology.SchedulePhase) {
	gtc.lock.Lock()
	defer gtc.lock.Unlock()
	nodeName := gtc.podToNode[podUID]
	if nodeName == "" {
		return
	}
	gtc.frameworkCache.GetExtendNodeInfo(nodeName).GetGPUTopologyCache().SetPodPhase(podUID, phase)
}

func (gtc *GPUTopologyCache) GetTopologyPodAllocations(podUIDs ...apitypes.UID) map[apitypes.UID]*gputopology.PodTopologyAllocation {
	gtc.lock.RLock()
	defer gtc.lock.RUnlock()
	result := map[apitypes.UID]*gputopology.PodTopologyAllocation{}
	for _, podUID := range podUIDs {
		nodeName := gtc.podToNode[podUID]
		if nodeName == "" {
			continue
		}
		allocation := gtc.frameworkCache.GetExtendNodeInfo(nodeName).GetGPUTopologyCache().GetPod(podUID)
		if allocation == nil {
			continue
		}
		result[podUID] = allocation
	}
	return result
}

func (gtc *GPUTopologyCache) GetNodeTopologyCache(nodeName string) *gputopology.NodeGPUTopologyCache {
	gtc.lock.RLock()
	defer gtc.lock.RUnlock()
	return gtc.frameworkCache.GetExtendNodeInfo(nodeName).GetGPUTopologyCache().Clone()
}
