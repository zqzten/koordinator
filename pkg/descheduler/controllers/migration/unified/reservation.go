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

package unified

import (
	"fmt"
	"strings"

	sev1 "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	koordsev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/reservation"
)

var _ reservation.Object = &Reservation{}

type Reservation struct {
	*sev1.ReserveResource
}

func (r *Reservation) String() string {
	return fmt.Sprintf("%s/%s", r.Namespace, r.Name)
}

func (r *Reservation) OriginObject() client.Object {
	return r.ReserveResource
}

func (r *Reservation) GetReservationConditions() []koordsev1alpha1.ReservationCondition {
	failedSchedulingCondIndex := -1
	conditions := make([]koordsev1alpha1.ReservationCondition, 0, len(r.Status.Conditions))
	for i := range r.Status.Conditions {
		cond := &r.Status.Conditions[i]
		koordReservationCond := koordsev1alpha1.ReservationCondition{
			Reason:             cond.Reason,
			Message:            cond.Message,
			LastProbeTime:      cond.LastProbeTime,
			LastTransitionTime: cond.LastTransitionTime,
		}
		if cond.Reason == "FailedScheduling" {
			koordReservationCond.Type = koordsev1alpha1.ReservationConditionScheduled
			koordReservationCond.Reason = koordsev1alpha1.ReasonReservationUnschedulable
			koordReservationCond.Status = koordsev1alpha1.ConditionStatusFalse
		} else if cond.Reason == "ProcessResourceFrom" {
			koordReservationCond.Type = "ProcessResourceFrom"
			if r.Status.ResourceFromStatus == sev1.ProcessResourceFromStatusDeleting {
				koordReservationCond.Reason = string(sev1.ProcessResourceFromStatusDeleting)
				koordReservationCond.Status = koordsev1alpha1.ConditionStatusFalse
			} else if r.Status.ResourceFromStatus == sev1.ProcessResourceFromStatusDone {
				koordReservationCond.Reason = string(sev1.ProcessResourceFromStatusDone)
				koordReservationCond.Status = koordsev1alpha1.ConditionStatusTrue
			}
		} else {
			koordReservationCond.Type = koordsev1alpha1.ReservationConditionType(koordReservationCond.Reason)
		}

		conditions = append(conditions, koordReservationCond)
		if cond.Reason == "FailedScheduling" {
			failedSchedulingCondIndex = len(conditions) - 1
		}
	}

	phase := r.GetPhase()
	if phase == koordsev1alpha1.ReservationAvailable || phase == koordsev1alpha1.ReservationSucceeded {
		if failedSchedulingCondIndex >= 0 {
			cond := &conditions[failedSchedulingCondIndex]
			cond.Reason = koordsev1alpha1.ReasonReservationScheduled
			cond.Status = koordsev1alpha1.ConditionStatusTrue
		} else {
			conditions = append(conditions, koordsev1alpha1.ReservationCondition{
				Type:   koordsev1alpha1.ReservationConditionScheduled,
				Reason: koordsev1alpha1.ReasonReservationScheduled,
				Status: koordsev1alpha1.ConditionStatusTrue,
			})
		}
		conditions = append(conditions, koordsev1alpha1.ReservationCondition{
			Type:   koordsev1alpha1.ReservationConditionReady,
			Reason: koordsev1alpha1.ReasonReservationSucceeded,
			Status: koordsev1alpha1.ConditionStatusFalse,
		})
	} else if r.Status.State == sev1.ReserveResourcePhaseExpired {
		if len(r.Status.Allocs) > 0 {
			conditions = append(conditions, koordsev1alpha1.ReservationCondition{
				Type:   koordsev1alpha1.ReservationConditionScheduled,
				Reason: koordsev1alpha1.ReasonReservationScheduled,
				Status: koordsev1alpha1.ConditionStatusTrue,
			})
		} else {
			conditions = append(conditions, koordsev1alpha1.ReservationCondition{
				Type:   koordsev1alpha1.ReservationConditionReady,
				Reason: koordsev1alpha1.ReasonReservationExpired,
				Status: koordsev1alpha1.ConditionStatusFalse,
			})
		}
	}

	return conditions
}

func (r *Reservation) QueryPreemptedPodsRefs() []corev1.ObjectReference {
	var podsRefs []corev1.ObjectReference
	for _, v := range r.Spec.ResourceFrom {
		podsRefs = append(podsRefs, corev1.ObjectReference{
			Kind:      v.Kind,
			Namespace: v.Namespace,
			Name:      v.Name,
			UID:       v.UID,
		})
	}
	return podsRefs
}

func (r *Reservation) GetBoundPod() *corev1.ObjectReference {
	if len(r.Status.Allocs) == 0 {
		return nil
	}
	alloc := &r.Status.Allocs[0]
	return &corev1.ObjectReference{
		Kind:      alloc.Kind,
		Namespace: alloc.Namespace,
		Name:      alloc.Name,
		UID:       alloc.UID,
	}
}

func (r *Reservation) GetReservationOwners() []koordsev1alpha1.ReservationOwner {
	var reservationOwners []koordsev1alpha1.ReservationOwner
	for _, v := range r.Spec.ResourceOwners {
		var owner koordsev1alpha1.ReservationOwner
		owner.LabelSelector = v.LabelSelector
		if v.AllocMeta != nil {
			owner.Object = &corev1.ObjectReference{
				Kind:      v.AllocMeta.Kind,
				Namespace: v.AllocMeta.Namespace,
				Name:      v.AllocMeta.Name,
				UID:       v.AllocMeta.UID,
			}
		}
		if v.ControllerKey != "" {
			parts := strings.Split(v.ControllerKey, "/")
			if len(parts) != 3 {
				continue
			}
			owner.Controller = &koordsev1alpha1.ReservationControllerReference{
				Namespace: parts[0],
				OwnerReference: metav1.OwnerReference{
					Name: parts[1],
					Kind: parts[2],
				},
			}
		}
		reservationOwners = append(reservationOwners, owner)
	}
	return reservationOwners
}

func (r *Reservation) GetScheduledNodeName() string {
	return r.Spec.NodeName
}

func (r *Reservation) GetPhase() koordsev1alpha1.ReservationPhase {
	if r.Status.State == "" || r.Status.State == sev1.ReserveResourcePhasePending {
		return koordsev1alpha1.ReservationPending
	}

	if r.Status.State == sev1.ReserveResourcePhaseFailed {
		return koordsev1alpha1.ReservationFailed
	}

	if r.Status.State == sev1.ReserveResourcePhaseExpired {
		if len(r.Status.Allocs) > 0 && r.Spec.DeleteAfterOwnerUse {
			return koordsev1alpha1.ReservationSucceeded
		}
		return koordsev1alpha1.ReservationFailed
	}

	if r.Status.State == sev1.ReserveResourcePhaseAvailable {
		if len(r.Spec.ResourceFrom) == 0 && r.Spec.NodeName != "" {
			return koordsev1alpha1.ReservationAvailable
		}

		resourceFromCond := reservation.GetReservationCondition(r, "ProcessResourceFrom", string(sev1.ProcessResourceFromStatusDone))
		if resourceFromCond != nil && resourceFromCond.Status == koordsev1alpha1.ConditionStatusTrue {
			return koordsev1alpha1.ReservationAvailable
		}
		return koordsev1alpha1.ReservationPending
	}

	return "Unknown"
}

func (r *Reservation) NeedPreemption() bool {
	return r.Labels[LabelEnableMigrate] == "true"
}
