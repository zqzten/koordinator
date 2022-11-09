package custompodaffinity

import (
	"sync"

	corev1 "k8s.io/api/core/v1"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

type Cache struct {
	lock  sync.RWMutex
	stats map[string]*serviceUnitStats
}

func newCache() *Cache {
	return &Cache{stats: map[string]*serviceUnitStats{}}
}

func (c *Cache) addPod(pod *corev1.Pod) {
	if pod == nil {
		return
	}
	_, podSpreadInfo := extunified.GetCustomPodAffinity(pod)
	if podSpreadInfo == nil {
		return
	}

	if _, ok := c.stats[pod.Spec.NodeName]; !ok {
		c.stats[pod.Spec.NodeName] = newServiceUnitStats()
	}
	c.stats[pod.Spec.NodeName].incCounter(podSpreadInfo.AppName, podSpreadInfo.ServiceUnit, 1)
}

func (c *Cache) deletePod(pod *corev1.Pod) {
	if pod == nil {
		return
	}
	_, podSpreadInfo := extunified.GetCustomPodAffinity(pod)
	if podSpreadInfo == nil {
		return
	}
	if _, ok := c.stats[pod.Spec.NodeName]; !ok {
		return
	}
	c.stats[pod.Spec.NodeName].incCounter(podSpreadInfo.AppName, podSpreadInfo.ServiceUnit, -1)
	if c.stats[pod.Spec.NodeName].isZero() {
		delete(c.stats, pod.Spec.NodeName)
	}
}

func (c *Cache) AddPod(pod *corev1.Pod) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.addPod(pod)
}

func (c *Cache) DeletePod(pod *corev1.Pod) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.deletePod(pod)
}

func (c *Cache) UpdatePod(oldPod, newPod *corev1.Pod) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.deletePod(oldPod)
	c.addPod(newPod)
}

func (c *Cache) GetAllocCount(nodeName string, spreadInfo *extunified.PodSpreadInfo) int {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if stat, ok := c.stats[nodeName]; ok {
		return stat.GetAllocCount(spreadInfo)
	}
	return 0
}
