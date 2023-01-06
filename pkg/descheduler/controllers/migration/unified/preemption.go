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
	"context"
	"time"

	sev1 "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	koordsev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/reservation"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/util"
)

const (
	defaultRequeueAfter = 3 * time.Second
)

const (
	ReasonWaitForConfirmPreemption = "WaitForConfirmPreemption"
	ReasonRejectedPreemption       = "RejectedPreemption"
)

func (p *Interpreter) Preempt(ctx context.Context, job *koordsev1alpha1.PodMigrationJob, reservationObj reservation.Object) (bool, reconcile.Result, error) {
	preemptedPodsRefs := reservationObj.QueryPreemptedPodsRefs()
	if len(preemptedPodsRefs) == 0 {
		return false, reconcile.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	waiting, err := p.waitForConfirmPreemption(ctx, job, reservationObj)
	if err != nil {
		return false, reconcile.Result{}, err
	} else if waiting {
		return false, reconcile.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	rejected, err := p.rejectPreemptionIfConfirmed(ctx, job, reservationObj)
	if err != nil {
		return false, reconcile.Result{}, err
	} else if rejected {
		return true, reconcile.Result{}, nil
	}

	err = p.triggerEvictPreemptedPods(ctx, job, reservationObj)
	if err != nil {
		return false, reconcile.Result{}, err
	}

	if p.isPreemptComplete(job) {
		return true, reconcile.Result{}, nil
	}

	klog.V(4).Infof("MigrationJob %s checks whether preempts successfully", job.Name)
	rr := reservationObj.(*Reservation).ReserveResource
	if rr.Status.ResourceFromStatus == "" || rr.Status.ResourceFromStatus == sev1.ProcessResourceFromStatusDeleting {
		klog.V(4).Infof("MigrationJob %s is preempting other Pods", job.Name)
		return false, reconcile.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	klog.V(4).Infof("MigrationJob %s preempts successfully", job.Name)
	cond := &koordsev1alpha1.PodMigrationJobCondition{
		Type:   koordsev1alpha1.PodMigrationJobConditionPreemption,
		Status: koordsev1alpha1.PodMigrationJobConditionStatusTrue,
		Reason: koordsev1alpha1.PodMigrationJobReasonPreemptComplete,
	}
	err = p.updateCondition(ctx, job, cond)
	return false, reconcile.Result{RequeueAfter: defaultRequeueAfter}, err
}

func (p *Interpreter) waitForConfirmPreemption(ctx context.Context, job *koordsev1alpha1.PodMigrationJob, reservationObj reservation.Object) (bool, error) {
	confirmState := job.Labels[LabelMigrationConfirmState]
	if confirmState != string(sev1.ReserveResourceConfirmStateWait) {
		return false, nil
	}
	_, cond := util.GetCondition(&job.Status, koordsev1alpha1.PodMigrationJobConditionPreemption)
	if cond == nil || cond.Status == koordsev1alpha1.PodMigrationJobConditionStatusFalse {
		childReservations, err := p.queryChildReservations(ctx, job.Spec.ReservationOptions.ReservationRef)
		if err != nil {
			klog.Errorf("Failed to queryChildReserveResource, MigrationJob: %s, err: %v", job.Name, err)
			return true, err
		}
		klog.V(4).Infof("MigrationJob %s preempts %d Pods, wait for confirm preemption", job.Name, len(childReservations))
		job.Status.PreemptedPodsReservations = childReservations
		job.Status.PreemptedPodsRef = reservationObj.QueryPreemptedPodsRefs()
		job.Status.Status = string(koordsev1alpha1.PodMigrationJobConditionPreemption)
		cond = &koordsev1alpha1.PodMigrationJobCondition{
			Type:   koordsev1alpha1.PodMigrationJobConditionPreemption,
			Status: koordsev1alpha1.PodMigrationJobConditionStatusFalse,
			Reason: ReasonWaitForConfirmPreemption,
		}
		err = p.updateCondition(ctx, job, cond)
		return true, err
	}
	klog.V(4).Infof("MigrationJob %s is waiting for confirm preemption", job.Name)
	return true, nil
}

func (p *Interpreter) rejectPreemptionIfConfirmed(ctx context.Context, job *koordsev1alpha1.PodMigrationJob, reservationObj client.Object) (bool, error) {
	confirmState := job.Labels[LabelMigrationConfirmState]
	if confirmState != string(sev1.ReserveResourceConfirmStateRejected) {
		return false, nil
	}

	klog.V(4).Infof("MigrationJob %s rejected preemption", job.Name)
	// update rr spec to rejected
	rr := reservationObj.(*sev1.ReserveResource)
	rr = rr.DeepCopy()
	rr.Spec.ConfirmState = sev1.ReserveResourceConfirmStateRejected
	err := p.Client.Update(ctx, rr)
	if err != nil {
		klog.Errorf("Failed to update reserveResource %s/%s, err: %v", rr.Namespace, rr.Name, err)
		return false, err
	}

	job.Status.Phase = koordsev1alpha1.PodMigrationJobFailed
	job.Status.Reason = ReasonRejectedPreemption
	job.Status.Message = "Abort job caused by reject preemption"
	util.UpdateCondition(&job.Status, &koordsev1alpha1.PodMigrationJobCondition{
		Type:    koordsev1alpha1.PodMigrationJobConditionPreemption,
		Status:  koordsev1alpha1.PodMigrationJobConditionStatusFalse,
		Reason:  ReasonRejectedPreemption,
		Message: job.Status.Message,
	})
	return true, p.Client.Update(ctx, job)
}

func (p *Interpreter) triggerEvictPreemptedPods(ctx context.Context, job *koordsev1alpha1.PodMigrationJob, reservationObj client.Object) error {
	_, cond := util.GetCondition(&job.Status, koordsev1alpha1.PodMigrationJobConditionPreemption)
	if cond != nil && cond.Reason == koordsev1alpha1.PodMigrationJobReasonPreempting {
		return nil
	}

	rr := reservationObj.(*sev1.ReserveResource)
	if rr.Spec.ConfirmState == sev1.ReserveResourceConfirmStateWait {
		klog.V(4).Infof("MigrationJob %s try to trigger preempt Pods", job.Name)
		rr = rr.DeepCopy()
		rr.Spec.ConfirmState = sev1.ReserveResourceConfirmStateConfirmed
		err := p.Client.Update(ctx, rr)
		if err != nil {
			klog.Errorf("Failed to update ReserveResource, err: %v", err)
			return err
		}
	}

	return p.updateCondition(ctx, job, &koordsev1alpha1.PodMigrationJobCondition{
		Type:   koordsev1alpha1.PodMigrationJobConditionPreemption,
		Status: koordsev1alpha1.PodMigrationJobConditionStatusFalse,
		Reason: koordsev1alpha1.PodMigrationJobReasonPreempting,
	})
}

func (p *Interpreter) isPreemptComplete(job *koordsev1alpha1.PodMigrationJob) bool {
	_, cond := util.GetCondition(&job.Status, koordsev1alpha1.PodMigrationJobConditionPreemption)
	if cond != nil && (cond.Reason == koordsev1alpha1.PodMigrationJobReasonPreemptComplete && cond.Status == koordsev1alpha1.PodMigrationJobConditionStatusTrue) {
		return true
	}
	return false
}

func (p *Interpreter) updateCondition(ctx context.Context, job *koordsev1alpha1.PodMigrationJob, cond *koordsev1alpha1.PodMigrationJobCondition) error {
	updated := util.UpdateCondition(&job.Status, cond)
	if updated {
		job.Status.Status = string(cond.Type)
		job.Status.Reason = cond.Reason
		job.Status.Message = cond.Message
		return p.Client.Status().Update(ctx, job)
	}
	return nil
}
