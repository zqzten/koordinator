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
	"reflect"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

func (c *drainNodeCache) OnNodeAdd(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if !ok {
		return
	}
	c.updateNodeInCache(node)
}

func (c *drainNodeCache) OnNodeUpdate(oldObj, newObj interface{}) {
	oldNode, ok := oldObj.(*v1.Node)
	if !ok {
		return
	}
	newNode, ok := newObj.(*v1.Node)
	if !ok {
		return
	}

	if !reflect.DeepEqual(oldNode.Status.Allocatable, newNode.Status.Allocatable) {
		c.updateNodeInCache(newNode)
	}
}

func (c *drainNodeCache) OnNodeDelete(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			klog.V(3).Infof("Couldn't get object from tombstone %#v", obj)
			return
		}
		node, ok = tombstone.Obj.(*v1.Node)
		if !ok {
			klog.V(3).Infof("Tombstone contained object that is not a Node: %#v", obj)
			return
		}
	}
	c.deleteNodeFromCache(node)
}

func (c *drainNodeCache) OnPodAdd(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok || !assignedPod(pod) {
		return
	}
	if terminatedPod(pod) {
		c.deletePodFromCache(pod)
		return
	}
	c.addPodToCache(pod)
}

func (c *drainNodeCache) OnPodUpdate(oldObj, newObj interface{}) {
	newPod, ok := newObj.(*v1.Pod)
	if !ok || !assignedPod(newPod) {
		return
	}
	if terminatedPod(newPod) {
		c.deletePodFromCache(newPod)
		return
	}
	c.addPodToCache(newPod)
}

func (c *drainNodeCache) OnPodDelete(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			klog.V(3).Infof("Couldn't get object from tombstone %#v", obj)
			return
		}
		pod, ok = tombstone.Obj.(*v1.Pod)
		if !ok {
			klog.V(3).Infof("Tombstone contained object that is not a Pod: %#v", obj)
			return
		}
	}
	if !assignedPod(pod) {
		return
	}

	c.deletePodFromCache(pod)
}

func (c *drainNodeCache) OnReservationAdd(obj interface{}) {
	r := c.reservationInterpreter.ToReservation(obj)
	if r == nil || r.IsPending() || r.IsAllocated() {
		return
	}

	if r.IsScheduled() {
		c.addReservationToCache(r.GetScheduledNodeName(), r.String())
	}
}

func (c *drainNodeCache) OnReservationUpdate(oldObj, newObj interface{}) {
	oldR := c.reservationInterpreter.ToReservation(oldObj)
	if oldR == nil || oldR.IsAllocated() {
		return
	}

	newR := c.reservationInterpreter.ToReservation(newObj)
	if newR == nil || newR.IsPending() {
		return
	}

	if newR.IsScheduled() && !newR.IsAllocated() {
		c.addReservationToCache(newR.GetScheduledNodeName(), newR.String())
		return
	}

	if newR.IsScheduled() && newR.IsAllocated() {
		c.deleteReservationFromCache(newR.GetScheduledNodeName(), newR.String())
		return
	}
}

func (c *drainNodeCache) OnReservationDelete(obj interface{}) {
	r := c.reservationInterpreter.ToReservation(obj)
	if r == nil || r.IsPending() || r.IsAllocated() {
		return
	}
	if r.IsScheduled() {
		c.deleteReservationFromCache(r.GetScheduledNodeName(), r.String())
	}
}
