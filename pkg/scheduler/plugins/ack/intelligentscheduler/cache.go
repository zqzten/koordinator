package intelligentscheduler

import (
	//CRDs "code.alipay.com/cnstack/intelligent-operator/api/v1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/intelligentscheduler/CRDs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
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

func (c *intelligentCache) addOrUpdateNode(node *corev1.Node) {
	c.lock.Lock()
	defer c.lock.Unlock()
	nodeOversellInfo, ok := c.intelligentNodes[node.Name]
	if !ok {
		nodeInfo, err := NewNodeInfo(node)
		if err != nil {
			klog.Errorf("Failed to create new oversellInfo for node %v, err: %v", node.Name, err)
		}
		c.intelligentNodes[node.Name] = nodeInfo
	} else {
		nodeOversellInfo.Reset(node)
		klog.Infof("update oversellInfo for node %v", node.Name)
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

func (c *intelligentCache) deleteNode(node *corev1.Node) {
	c.lock.Lock()
	defer c.lock.Unlock()
	_, ok := c.intelligentNodes[node.Name]
	if !ok {
		return
	}
	delete(c.intelligentNodes, node.Name)
}

func (c *intelligentCache) deleteVgsInfo(vgs *CRDs.VirtualGpuSpecification) {
	c.lock.Lock()
	defer c.lock.Unlock()
	_, ok := c.virtualGpuSpecifications[vgs.Spec.NickName]
	if !ok {
		return
	}
	delete(c.virtualGpuSpecifications, vgs.Spec.NickName)
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
	podUid := string(pod.UID)
	for name, vi := range c.virtualGpuInstances {
		if vi.Pod == podUid {
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
