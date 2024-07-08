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

package cgroups

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

var _ handler.EventHandler = &cgroupsEventHandler{}

type cgroupsEventHandler struct {
	client.Client
	recycler *CgroupsInfoRecycler
}

func (c *cgroupsEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	newCgroup := evt.Object.(*resourcesv1alpha1.Cgroups)
	if newCgroup == nil {
		klog.Warningf("cgroup create event received no object")
		return
	}
	if !isCgroupsSpecEmpty(newCgroup) {
		// new cgroups crd is not empty, add it to reconcile
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: newCgroup.Namespace,
				Name:      newCgroup.Name,
			},
		})
	}
}

func (c *cgroupsEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newCgroup := evt.ObjectNew.(*resourcesv1alpha1.Cgroups)
	oldCgroup := evt.ObjectOld.(*resourcesv1alpha1.Cgroups)
	if newCgroup == nil || oldCgroup == nil {
		klog.Warningf("cgroup update event received no object")
		return
	}
	if reflect.DeepEqual(newCgroup.Spec, oldCgroup.Spec) {
		// cgroups crd spec not has not change, no need to reconcile
		return
	}
	if !isCgroupsSpecEmpty(newCgroup) {
		// newCgroup is not empty, add it to reconcile
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: newCgroup.Namespace,
				Name:      newCgroup.Name,
			},
		})
	}
	if oldSubNew := cgroupsSub(oldCgroup, newCgroup); !isCgroupsSpecEmpty(oldSubNew) {
		// old-new is not empty, add to recycler for gc
		klog.V(5).Infof("cgroup %v/%v updated, old %v, new %v, need recycle detail %v",
			oldSubNew.Namespace, oldSubNew.Name, oldCgroup.Spec, newCgroup.Spec, oldSubNew.Spec)
		c.recycler.AddCgroups(oldSubNew)
	}
}

func (c *cgroupsEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	oldCgroup := evt.Object.(*resourcesv1alpha1.Cgroups)
	if oldCgroup == nil {
		klog.Warningf("cgroup delete event received no object")
		return
	}
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: oldCgroup.Namespace,
			Name:      oldCgroup.Name,
		},
	})
}

func (c *cgroupsEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {

}

var _ handler.EventHandler = &podEventHandler{}

type podEventHandler struct {
	client.Client
	recycler *CgroupsInfoRecycler
}

func (p *podEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	newPod := evt.Object.(*corev1.Pod)
	if newPod == nil {
		klog.Warningf("pod create event received no object")
		return
	}
	if newPod.Spec.NodeName != "" {
		// add new pod to reconcile and update nodeSLO if necessary
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: newPod.Namespace,
				Name:      newPod.Name,
			},
		})
		klog.V(5).Infof("receive new pod %v/%v with node %v", newPod.Namespace, newPod.Name,
			newPod.Spec.NodeName)
	}
}

func (p *podEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newPod := evt.ObjectNew.(*corev1.Pod)
	oldPod := evt.ObjectOld.(*corev1.Pod)
	if newPod == nil || oldPod == nil {
		klog.Warningf("cgroup update event received no object")
		return
	}
	if newPod.Spec.NodeName == oldPod.Spec.NodeName {
		// pod's node has not changed, no need to update nodeSLO
		return
	}
	if newPod.Spec.NodeName != "" {
		// pod's node changed, reconcile cgroups info on new nodeSLO
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: newPod.Namespace,
				Name:      newPod.Name,
			},
		})
		klog.V(5).Infof("receive update pod %v/%v with new node %v", newPod.Namespace, newPod.Name,
			newPod.Spec.NodeName)
	}
	if oldPod.Spec.NodeName != "" {
		// pod's node changed, clean cgroups info on old nodeSLO
		p.recycler.AddPod(oldPod.Namespace, oldPod.Name, oldPod.Spec.NodeName)
		klog.V(5).Infof("receive update pod %v/%v with old node %v", newPod.Namespace, newPod.Name,
			oldPod.Spec.NodeName)
	}
}

func (p *podEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	oldPod := evt.Object.(*corev1.Pod)
	if oldPod == nil {
		klog.Warningf("cgroup delete event received no object")
		return
	}
	if oldPod.Spec.NodeName != "" {
		// pod deleted, clean cgroups info on nodeSLO
		p.recycler.AddPod(oldPod.Namespace, oldPod.Name, oldPod.Spec.NodeName)
		klog.V(5).Infof("receive delete pod %v/%v with node %v", oldPod.Namespace, oldPod.Name,
			oldPod.Spec.NodeName)
	}
}

func (p *podEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {

}
