/*
Copyright 2023 The Koordinator Authors.

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

package logicalresourcenode

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	lrnutil "github.com/koordinator-sh/koordinator/pkg/util/logicalresourcenode"
)

type nodeEventHandler struct {
	cache client.Reader
}

func (n *nodeEventHandler) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	n.enqueue(e.Object, q)
}

func (n *nodeEventHandler) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	nodeExpectations.Observe(e.ObjectNew)
	n.enqueue(e.ObjectNew, q)
}

func (n *nodeEventHandler) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	n.enqueue(e.Object, q)
}

func (n *nodeEventHandler) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	n.enqueue(e.Object, q)
}

func (n *nodeEventHandler) enqueue(obj client.Object, q workqueue.RateLimitingInterface) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return
	}

	reservationList := &schedulingv1alpha1.ReservationList{}
	if err := n.cache.List(context.TODO(), reservationList, client.MatchingFields{"status.nodeName": node.Name}); err != nil {
		klog.Errorf("Failed to list reservation with nodeName %s: %v", node.Name, err)
		return
	}

	cfg, err := lrnutil.GetConfig(n.cache)
	if err != nil {
		klog.Errorf("Failed to get lrn config: %v", err)
		return
	}

	lrnNames := sets.NewString()
	for i := range reservationList.Items {
		reservation := &reservationList.Items[i]
		lrnName := lrnutil.GetReservationOwnerLRN(reservation)
		if lrnName == "" {
			continue
		}

		lrn := &schedulingv1alpha1.LogicalResourceNode{}
		if err := n.cache.Get(context.TODO(), types.NamespacedName{Name: lrnName}, lrn); err != nil {
			klog.Warningf("Failed to get LRN %s of Reservation %s for Node %s: %v", lrnName, reservation.Name, node.Name, err)
			continue
		}

		newStatus := schedulingv1alpha1.LogicalResourceNodeStatus{
			Phase:       schedulingv1alpha1.LogicalResourceNodeAvailable,
			NodeName:    node.Name,
			Allocatable: reservation.Status.Allocatable,
		}
		syncNodeStatus(cfg, &newStatus, node)
		if !apiequality.Semantic.DeepEqual(newStatus, lrn.Status) {
			lrnNames.Insert(lrnName)
			continue
		}

		patchBody := generateLRNPatch(cfg, lrn, getGenerationFromLRN(lrn), reservation, node)
		if !patchBody.isConsistent(lrn) {
			lrnNames.Insert(lrnName)
		}
	}

	for _, name := range lrnNames.UnsortedList() {
		q.Add(ctrl.Request{NamespacedName: types.NamespacedName{Name: name}})
	}
}

type qosGroupEventHandler struct{}

func (h *qosGroupEventHandler) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	h.enqueue(e.Object, q)
}

func (h *qosGroupEventHandler) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	h.enqueue(e.ObjectNew, q)
}

func (h *qosGroupEventHandler) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	return
}

func (h *qosGroupEventHandler) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	h.enqueue(e.Object, q)
}

func (h *qosGroupEventHandler) enqueue(obj client.Object, q workqueue.RateLimitingInterface) {
	if lrnName := obj.GetLabels()[labelOwnedByLRN]; lrnName != "" {
		q.Add(ctrl.Request{NamespacedName: types.NamespacedName{Name: lrnName}})
	}
}
