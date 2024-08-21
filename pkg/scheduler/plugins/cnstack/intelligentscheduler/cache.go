package intelligentscheduler

import (
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/intelligentscheduler/CRDs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sync"
)

type intelligentCache struct {
	lock                     *sync.RWMutex
	intelligentNodes         map[string]*NodeInfo               // 所有的智算调度node的name
	virtualGpuInstances      map[string]*VirtualGpuInstanceInfo // 所有Virtual GPU Instance
	virtualGpuSpecifications map[string]*VirtualGpuSpecInfo     // 虚拟规格
}

func newIntelligentCache() *intelligentCache {
	return &intelligentCache{
		lock:                     new(sync.RWMutex),
		intelligentNodes:         make(map[string]*NodeInfo), // 所有拥有物理GPU的智算调度node
		virtualGpuInstances:      make(map[string]*VirtualGpuInstanceInfo),
		virtualGpuSpecifications: make(map[string]*VirtualGpuSpecInfo),
	}
}

func (c *intelligentCache) addOrUpdateNode(node *corev1.Node, oversellRate int) {
	c.lock.Lock()
	defer c.lock.Unlock()
	nodeInfo, ok := c.intelligentNodes[node.Name]
	if !ok {
		nodeInfo, err := NewNodeInfo(node, oversellRate)
		if err != nil {
			klog.Errorf("Failed to create new nodeInfo for node %v, err: %v", node.Name, err)
		}
		c.intelligentNodes[node.Name] = nodeInfo
		//klog.Infof("add new intelligent node %v to cache", node.Name)
	} else {
		nodeInfo.Reset(node, oversellRate)
		//klog.Infof("update nodeInfo for node %v", node.Name)
	}
}

func (c *intelligentCache) addOrUpdateVgsInfo(vgs *CRDs.VirtualGpuSpecification) {
	c.lock.Lock()
	defer c.lock.Unlock()
	vs, ok := c.virtualGpuSpecifications[vgs.Name]
	if !ok {
		newVsInfo := NewVirtualGpuSpecInfo(vgs)
		c.virtualGpuSpecifications[vgs.Name] = newVsInfo
		//klog.Infof("add new virtual gpu specification %v to the intelligent scheduler cache", vgs.Name)
	} else {
		vs.Reset(vgs)
		//klog.Infof("update virtual gpu specification %v to the intelligent scheduler cache", vgs.Name)
	}
}

func (c *intelligentCache) addOrUpdateVgiInfo(vgi *CRDs.VirtualGpuInstance) {
	c.lock.Lock()
	defer c.lock.Unlock()
	vi, ok := c.virtualGpuInstances[vgi.Name]
	if !ok {
		newViInfo := NewVirtualGpuInstanceInfo(vgi)
		c.virtualGpuInstances[vgi.Name] = newViInfo
		//klog.Infof("new vgi: [%v]", newViInfo.toString())
		//klog.Infof("add new virtual gpu instance %v in the intelligent scheduler cache", vgi.Name)
	} else {
		vi.Reset(vgi)
		//klog.Infof("updated vgi: [%v]", vi.toString())
		//klog.Infof("update virtual gpu instance %v to the intelligence cache", vgi.Name)
	}
}

func (c *intelligentCache) deleteNode(node *corev1.Node) {
	c.lock.Lock()
	defer c.lock.Unlock()
	_, ok := c.intelligentNodes[node.Name]
	if !ok {
		return
	}
	delete(c.intelligentNodes, node.Name)
	//klog.Infof("delete node %v from cache", node.Name)
}

func (c *intelligentCache) deleteVgsInfo(vgs *CRDs.VirtualGpuSpecification) {
	c.lock.Lock()
	defer c.lock.Unlock()
	_, ok := c.virtualGpuSpecifications[vgs.Name]
	if !ok {
		return
	}
	delete(c.virtualGpuSpecifications, vgs.Name)
}

func (c *intelligentCache) deleteVgiInfo(vgi *CRDs.VirtualGpuInstance) {
	c.lock.Lock()
	defer c.lock.Unlock()
	_, ok := c.virtualGpuInstances[vgi.Name]
	if !ok {
		return
	}
	delete(c.virtualGpuInstances, vgi.Name)
}

// 判断node是否为智算调度node
func (c *intelligentCache) getIntelligentNode(name string) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	_, ok := c.intelligentNodes[name]
	if !ok {
		return false
	} else {
		return true
	}
}

func (c *intelligentCache) getNodeInfo(name string) *NodeInfo {
	c.lock.RLock()
	defer c.lock.RUnlock()
	node, ok := c.intelligentNodes[name]
	if !ok {
		return nil
	} else {
		return node
	}
}

func (c *intelligentCache) getVgsInfo(name string) *VirtualGpuSpecInfo {
	c.lock.Lock()
	defer c.lock.Unlock()
	vi, ok := c.virtualGpuSpecifications[name]
	if ok {
		return vi
	} else {
		return nil
	}
}

func (c *intelligentCache) getVgiInfo(name string) *VirtualGpuInstanceInfo {
	c.lock.RLock()
	defer c.lock.RUnlock()
	vi, ok := c.virtualGpuInstances[name]
	if ok {
		return vi
	} else {
		return nil
	}
}

func (c *intelligentCache) getVgiInfoNamesByPod(pod *corev1.Pod) []string {
	c.lock.RLock()
	defer c.lock.RUnlock()
	var viNames []string
	podNameNamespace := pod.Name + "/" + pod.Namespace
	for name, vi := range c.virtualGpuInstances {
		if vi.Pod == podNameNamespace {
			viNames = append(viNames, name)
		}
	}
	return viNames
}

func (c *intelligentCache) getAllVgiInfo() map[string]*VirtualGpuInstanceInfo {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.virtualGpuInstances
}
