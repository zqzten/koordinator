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

package cache

import (
	"sync"

	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/reservation"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/options"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/utils"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"
	resourceapi "k8s.io/kubernetes/pkg/api/v1/resource"
)

var lock sync.Mutex
var c *drainNodeCache
var _ Cache = &drainNodeCache{}
var _ CacheEventHandler = &drainNodeCache{}

var resourceName = []v1.ResourceName{v1.ResourceCPU, v1.ResourceMemory}
var reservationScheduler = map[string]struct{}{"ahe-scheduler": {}, "koord-scheduler": {}}

type drainNodeCache struct {
	sync.RWMutex
	nodes                  map[string]*NodeInfo
	reservationInterpreter reservation.Interpreter
}

func GetCache() Cache {
	lock.Lock()
	defer lock.Unlock()
	if c == nil {
		manager := options.Manager
		c = &drainNodeCache{
			nodes:                  map[string]*NodeInfo{},
			reservationInterpreter: reservation.NewInterpreter(manager.GetClient(), manager.GetAPIReader()),
		}
	}
	return c
}

func GetCacheEventHandler() CacheEventHandler {
	lock.Lock()
	defer lock.Unlock()
	if c == nil {
		manager := options.Manager
		c = &drainNodeCache{
			nodes:                  map[string]*NodeInfo{},
			reservationInterpreter: reservation.NewInterpreter(manager.GetClient(), manager.GetAPIReader()),
		}
	}
	return c
}

func (c *drainNodeCache) GetNodeInfo(nodeName string) *NodeInfo {
	c.RLock()
	defer c.RUnlock()
	return c.nodes[nodeName]
}

func (c *drainNodeCache) GetPods(nodeName string) []*PodInfo {
	c.RLock()
	defer c.RUnlock()
	ni := c.nodes[nodeName]
	ret := []*PodInfo{}
	for i := range ni.Pods {
		p := ni.Pods[i]
		ret = append(ret, p)
	}
	return ret
}

func (c *drainNodeCache) getOrCreateNodeInfo(name string) *NodeInfo {
	c.Lock()
	defer c.Unlock()
	var ni *NodeInfo
	var ok bool
	if ni, ok = c.nodes[name]; !ok {
		ni = &NodeInfo{
			Name:        name,
			Pods:        make(map[types.UID]*PodInfo),
			Reservation: make(map[string]struct{}),
		}
		c.nodes[name] = ni
	}
	return ni
}

func (c *drainNodeCache) updateNodeInCache(n *v1.Node) {
	klog.V(3).Infof("update node %v in cache", n.Name)
	ni := c.getOrCreateNodeInfo(n.Name)
	ni.updateStatus(n)
}

func (c *drainNodeCache) deleteNodeFromCache(n *v1.Node) {
	c.Lock()
	defer c.Unlock()
	klog.V(3).Infof("delete node %v from cache", n.Name)
	delete(c.nodes, n.Name)
}

func (c *drainNodeCache) addPodToCache(p *v1.Pod) {
	klog.V(3).Infof("update Pod %v/%v in cache", p.Namespace, p.Name)
	ni := c.getOrCreateNodeInfo(p.Spec.NodeName)
	ni.addPodToCache(p)
	ni.updateStatus(nil)
}

func (c *drainNodeCache) deletePodFromCache(p *v1.Pod) {
	klog.V(3).Infof("delete Pod %v/%v from cache", p.Namespace, p.Name)
	n := c.GetNodeInfo(p.Spec.NodeName)
	if n == nil {
		return
	}
	n.deletePodInfo(p.UID)
	n.updateStatus(nil)
}

func (c *drainNodeCache) addReservationToCache(nName, rName string) {
	klog.V(3).Infof("update reservation %v/%v in node %v", rName, nName)
	ni := c.getOrCreateNodeInfo(nName)
	ni.addReservation(rName)
	ni.updateStatus(nil)
}

func (c *drainNodeCache) deleteReservationFromCache(nName, rName string) {
	klog.V(3).Infof("delete reservation %v from node %v", rName, nName)
	n := c.GetNodeInfo(nName)
	if n == nil {
		return
	}
	n.deleteReservation(rName)
	n.updateStatus(nil)
}

func (ni *NodeInfo) updateStatus(n *v1.Node) {
	ni.Lock()
	defer ni.Unlock()
	if n != nil {
		ni.Allocatable = quotav1.Mask(n.Status.Allocatable, resourceName)
	}
	free := ni.Allocatable
	drainable := true
	// calculate free resource, only support cpu and memory
	for _, pi := range ni.Pods {
		request := quotav1.Mask(pi.Request, resourceName)
		free = quotav1.Subtract(free, request)
		if !pi.Ignore && !pi.Migratable {
			drainable = false
			klog.V(5).Infof("node %v not migratable, because of Pod %v", ni.Name, pi.NamespacedName)
		}
	}
	if len(ni.Reservation) > 0 {
		drainable = false
	}
	ni.Free = free
	ni.Drainable = drainable
	ni.Score = 0

	// calculate node score
	res := quotav1.RemoveZeros(ni.Allocatable)
	w := int64(len(res))
	if w <= 0 {
		return
	}
	var s int64
	for resource, alloc := range res {
		req := free[resource]
		s += 100 * req.MilliValue() / alloc.MilliValue()
	}
	ni.Score = s / w
}

func (ni *NodeInfo) addPodToCache(p *v1.Pod) *PodInfo {
	pi := &PodInfo{}
	pi.Namespace = p.Namespace
	pi.Name = p.Name
	pi.UID = p.UID
	pi.Pod = p
	if pi.Request == nil {
		podRequests, _ := resourceapi.PodRequestsAndLimits(p)
		pi.Request = podRequests
	}
	if utils.IsStaticPod(p) ||
		utils.IsMirrorPod(p) ||
		utils.IsDaemonsetPod(p.OwnerReferences) {
		pi.Ignore = true
	}
	pi.Migratable = true
	for i := range p.Spec.Volumes {
		v := &p.Spec.Volumes[i]
		if v.PersistentVolumeClaim != nil {
			pi.Migratable = false
		}
	}
	if metav1.GetControllerOf(p) == nil {
		pi.Migratable = false
	}
	if _, ok := reservationScheduler[p.Spec.SchedulerName]; !ok {
		pi.Migratable = false
	}

	ni.Lock()
	defer ni.Unlock()
	ni.Pods[pi.UID] = pi
	return pi
}

func (ni *NodeInfo) deletePodInfo(uid types.UID) {
	ni.Lock()
	defer ni.Unlock()
	delete(ni.Pods, uid)
}

func (ni *NodeInfo) addReservation(name string) {
	ni.Lock()
	defer ni.Unlock()
	klog.V(3).Infof("add reservation %v to cache", name)
	ni.Reservation[name] = struct{}{}
}

func (ni *NodeInfo) deleteReservation(name string) {
	ni.Lock()
	defer ni.Unlock()
	klog.V(3).Infof("delete reservation %v from cache", name)
	delete(ni.Reservation, name)
}

// assignedPod selects pods that are assigned (scheduled and running).
func assignedPod(pod *v1.Pod) bool {
	return len(pod.Spec.NodeName) != 0
}

func terminatedPod(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed
}
