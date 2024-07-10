package intelligentscheduler

import (
	//"code.alipay.com/cnstack/intelligent-operator"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	"sync"
)

type intelligentCache struct {
	lock                     *sync.RWMutex
	nodeInfos                map[string]*IntelligentSchedulerNodeInfo // 所有node的初始资源
	virtualGpuInstances      map[string]*VirtualGpuInstance           // 所有已经分配出去的虚拟GPU
	virtualGpuSpecifications map[string]*VirtualGpuSpecification      // 虚拟规格的cr
}

func newIntelligentCache() *intelligentCache {
	return &intelligentCache{
		lock:                     new(sync.RWMutex),
		nodeInfos:                make(map[string]*IntelligentSchedulerNodeInfo),
		virtualGpuInstances:      make(map[string]*VirtualGpuInstance),
		virtualGpuSpecifications: make(map[string]*VirtualGpuSpecification),
	}
}

func (c *intelligentCache) addNode(node *IntelligentSchedulerNodeInfo) {}

func (c *intelligentCache) getNode(name string) *IntelligentSchedulerNodeInfo {
	c.lock.Lock()
	defer c.lock.Unlock()
	ni, ok := c.nodeInfos[name]
	if ok {
		return ni
	}
	ni = NewIntelligentSchedulerNodeInfo(name)
	return ni
}

func (info *IntelligentSchedulerNodeInfo) Reset(node *corev1.Node) {
	if !isIntelligentNode(node) {
		klog.V(6).Infof("node %v is not intelligent scheduled node, failed to reset it", node.Name)
	}
	info.lock.Lock()
	defer info.lock.Unlock()
	// TODO 根据node初始化info
}

func NewIntelligentSchedulerNodeInfo(name string) *IntelligentSchedulerNodeInfo {
	return &IntelligentSchedulerNodeInfo{
		lock:     new(sync.RWMutex),
		name:     name,
		GpuInfos: make(map[int]*IntelligentSchedulerGpuInfo),
	}
}
