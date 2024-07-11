package intelligentscheduler

import (
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/intelligentscheduler/CRDs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"

	"sync"
)

type intelligentCache struct {
	lock                     *sync.RWMutex
	intelligentNodes         map[string]struct{}                // 所有的智算调度node的name
	nodeInfos                map[string]*NodeInfo               // 所有node的GPU资源
	virtualGpuInstances      map[string]*VirtualGpuInstanceInfo // 所有已经分配出去的虚拟GPU
	virtualGpuSpecifications map[string]*VirtualGpuSpecInfo     // 虚拟规格
}

func newIntelligentCache() *intelligentCache {
	return &intelligentCache{
		lock:                     new(sync.RWMutex),
		intelligentNodes:         make(map[string]struct{}),  // 所有拥有物理GPU的智算调度node
		nodeInfos:                make(map[string]*NodeInfo), // 所有智算调度node上的所有GPU资源
		virtualGpuInstances:      make(map[string]*VirtualGpuInstanceInfo),
		virtualGpuSpecifications: make(map[string]*VirtualGpuSpecInfo),
	}
}

func (c *intelligentCache) addOrUpdateNode(node *corev1.Node) {
	c.lock.Lock()
	defer c.lock.Unlock()
	_, ok := c.intelligentNodes[node.Name]
	if !ok {
		c.intelligentNodes[node.Name] = struct{}{}
	}
}

func (c *intelligentCache) addOrUpdateVgsInfo(vgs *CRDs.VirtualGpuSpecification) {
	c.lock.Lock()
	defer c.lock.Unlock()
	vs, ok := c.virtualGpuSpecifications[vgs.Spec.NickName]
	if !ok {
		newVsInfo := NewVirtualGpuSpecInfo(vgs)
		c.virtualGpuSpecifications[vgs.Spec.NickName] = newVsInfo
		klog.Info("add new virtual gpu specification to the intelligent scheduler cache")
	} else {
		vs.Reset(vgs)
		klog.Infof("update virtual gpu specification %s to the intelligent scheduler cache", vgs.Spec.NickName)
	}
}

func (c *intelligentCache) addOrUpdateVgiInfo(vgi *CRDs.VirtualGpuInstance) {
	c.lock.Lock()
	defer c.lock.Unlock()
	vi, ok := c.virtualGpuInstances[vgi.Name]
	if !ok {
		newViInfo := NewVirtualGpuInstanceInfo(vgi)
		c.virtualGpuInstances[vgi.Name] = newViInfo
	} else {
		vi.Reset(vgi)
		klog.Infof("update virtual gpu instance %s to the intelligence cache", vgi.Name)
	}
}

func (c *intelligentCache) addOrUpdatePgiInfo(pgi *CRDs.PhysicalGpuInstance) {
	c.lock.Lock()
	defer c.lock.Unlock()
	node := pgi.Spec.Node
	_, ok := c.intelligentNodes[node]
	if !ok {
		return
	}
	nodeInfo, ok := c.nodeInfos[node]
	if !ok {
		newNodeInfo := NewNodeInfo(node)
		newNodeInfo.Reset(pgi)
		c.nodeInfos[node] = newNodeInfo
	} else {
		nodeInfo.Reset(pgi)
		klog.Infof("update physical gpu instance %s of the node %s to the intelligence cache", pgi.Name, node)
	}
}

func (c *intelligentCache) getVgsInfo(nickName string) *VirtualGpuSpecInfo {
	c.lock.Lock()
	defer c.lock.Unlock()
	vi, ok := c.virtualGpuSpecifications[nickName]
	if ok {
		return vi
	} else {
		return nil
	}
}

func (c *intelligentCache) getNodeInfo(name string) *NodeInfo {
	c.lock.Lock()
	defer c.lock.Unlock()
	ni, ok := c.nodeInfos[name]
	if ok {
		return ni
	}
	ni = NewIntelligentSchedulerNodeInfo(name)
	return ni
}

func NewIntelligentSchedulerNodeInfo(name string) *NodeInfo {
	return &NodeInfo{
		lock:     new(sync.RWMutex),
		name:     name,
		GpuInfos: make(map[int]*PhysicalGpuInfo),
	}
}
