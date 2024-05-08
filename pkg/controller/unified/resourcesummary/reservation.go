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

package resourcesummary

import (
	"context"

	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	utilclient "github.com/koordinator-sh/koordinator/pkg/util/client"
	reservationutil "github.com/koordinator-sh/koordinator/pkg/util/reservation"
)

func (r *Reconciler) getReservationForCandidateNodes(ctx context.Context, candidateNodes *corev1.NodeList,
) (map[string][]*schedulingv1alpha1.Reservation, error) {
	nodeOwnedReservations := map[string][]*schedulingv1alpha1.Reservation{}
	for _, node := range candidateNodes.Items {
		reservationList := &schedulingv1alpha1.ReservationList{}
		if err := r.Client.List(ctx, reservationList, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("status.nodeName", node.Name),
		}, utilclient.DisableDeepCopy); err != nil {
			return nil, err
		}
		for i := range reservationList.Items {
			if reservationList.Items[i].Status.Phase == schedulingv1alpha1.ReservationAvailable {
				nodeOwnedReservations[node.Name] = append(nodeOwnedReservations[node.Name], &reservationList.Items[i])
			}
		}
	}
	return nodeOwnedReservations, nil
}

func GetReservationPriorityResource(reservation *schedulingv1alpha1.Reservation, node *corev1.Node, gpuCapacity corev1.ResourceList, resourceNames ...corev1.ResourceName) (used, capacity, free v1beta1.PodPriorityUsed) {
	pod := reservationutil.NewReservePod(reservation)
	unifiedPriority := extunified.GetUnifiedPriorityClass(pod)

	allocatable := priorityPodRequestedToNormal(reservation.Status.Allocatable, unifiedPriority)
	if allocatable == nil {
		allocatable = corev1.ResourceList{}
	}
	allocated := priorityPodRequestedToNormal(reservation.Status.Allocated, unifiedPriority)
	if allocated == nil {
		allocated = corev1.ResourceList{}
	}

	scaleCPUAndACU(pod, node, allocatable)
	scaleCPUAndACU(pod, node, allocated)
	fillGPUResource(allocatable, gpuCapacity)
	fillGPUResource(allocated, gpuCapacity)
	if len(resourceNames) > 0 {
		allocatable = quotav1.Mask(allocatable, resourceNames)
		allocated = quotav1.Mask(allocated, resourceNames)
	}
	return v1beta1.PodPriorityUsed{PriorityClass: unifiedPriority, Allocated: allocated}, v1beta1.PodPriorityUsed{PriorityClass: unifiedPriority, Allocated: allocatable}, v1beta1.PodPriorityUsed{PriorityClass: unifiedPriority, Allocated: quotav1.SubtractWithNonNegativeResult(allocatable, allocated)}
}
