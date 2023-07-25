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

package limitaware

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/pkg/util"
)

type Cache struct {
	lock           sync.RWMutex
	nodeLimitInfos map[string]*nodeLimitInfo
}

func newCache() *Cache {
	return &Cache{
		nodeLimitInfos: map[string]*nodeLimitInfo{},
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

func (c *Cache) addPod(nodeName string, pod *corev1.Pod) {
	if pod == nil {
		return
	}

	info, ok := c.nodeLimitInfos[nodeName]
	if !ok {
		info = newNodeLimitInfo()
		c.nodeLimitInfos[nodeName] = info
	}
	podKey := util.GetNamespacedName(pod.Namespace, pod.Name)
	if info.allocSet.Has(podKey) {
		return
	}
	info.allocSet.Insert(podKey)
	info.addPod(nodeName, pod)
}

func (c *Cache) deletePod(nodeName string, pod *corev1.Pod) {
	if pod == nil {
		return
	}
	info, ok := c.nodeLimitInfos[nodeName]
	if !ok {
		return
	}
	podKey := util.GetNamespacedName(pod.Namespace, pod.Name)
	if !info.allocSet.Has(podKey) {
		return
	}
	info.allocSet.Delete(podKey)
	info.deletePod(nodeName, pod)

	if info.isZero() {
		delete(c.nodeLimitInfos, nodeName)
	}
}

func (c *Cache) GetAllocated(nodeName string) *framework.Resource {
	c.lock.RLock()
	defer c.lock.RUnlock()
	info, ok := c.nodeLimitInfos[nodeName]
	if !ok {
		return framework.NewResource(nil)
	}
	return info.GetAllocated()
}

func (c *Cache) GetNonZeroAllocated(nodeName string) *framework.Resource {
	c.lock.RLock()
	defer c.lock.RUnlock()
	info, ok := c.nodeLimitInfos[nodeName]
	if !ok {
		return framework.NewResource(nil)
	}
	return info.GetNonZeroAllocated()
}
