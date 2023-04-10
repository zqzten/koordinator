/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package custompodaffinity

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

type Cache struct {
	lock  sync.RWMutex
	stats map[string]*serviceUnitStats
}

func newCache() *Cache {
	return &Cache{stats: map[string]*serviceUnitStats{}}
}

func (c *Cache) addPod(nodeName string, pod *corev1.Pod) {
	if pod == nil {
		return
	}
	_, podSpreadInfo := extunified.GetCustomPodAffinity(pod)
	if podSpreadInfo == nil {
		return
	}
	stats, ok := c.stats[nodeName]
	if !ok {
		stats = newServiceUnitStats()
		c.stats[nodeName] = stats
	}
	podKey := getNamespacedName(pod.Namespace, pod.Name)
	if stats.AllocSet.Has(podKey) {
		return
	}
	stats.AllocSet.Insert(podKey)
	stats.incCounter(podSpreadInfo.AppName, podSpreadInfo.ServiceUnit, 1)
}

func getNamespacedName(namespace, name string) string {
	result := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	return result.String()
}

func (c *Cache) deletePod(nodeName string, pod *corev1.Pod) {
	if pod == nil {
		return
	}
	_, podSpreadInfo := extunified.GetCustomPodAffinity(pod)
	if podSpreadInfo == nil {
		return
	}
	stats, ok := c.stats[nodeName]
	if !ok {
		return
	}
	podKey := getNamespacedName(pod.Namespace, pod.Name)
	if !stats.AllocSet.Has(podKey) {
		return
	}
	stats.AllocSet.Delete(podKey)
	c.stats[nodeName].incCounter(podSpreadInfo.AppName, podSpreadInfo.ServiceUnit, -1)
	if c.stats[nodeName].isZero() {
		delete(c.stats, nodeName)
	}
}

func (c *Cache) AddPod(nodeName string, pod *corev1.Pod) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.addPod(nodeName, pod)
}

func (c *Cache) DeletePod(nodeName string, pod *corev1.Pod) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.deletePod(nodeName, pod)
}

func (c *Cache) UpdatePod(nodeName string, oldPod, newPod *corev1.Pod) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.deletePod(nodeName, oldPod)
	c.addPod(nodeName, newPod)
}

func (c *Cache) GetAllocCount(nodeName string, spreadInfo *extunified.PodSpreadInfo, preemptivePods sets.String) int {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if stat, ok := c.stats[nodeName]; ok {
		return stat.GetAllocCount(spreadInfo, preemptivePods)
	}
	return 0
}

func (c *Cache) GetAllocSet(nodeName string) sets.String {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if stat, ok := c.stats[nodeName]; ok {
		return sets.NewString(stat.AllocSet.List()...)
	}
	return nil
}
