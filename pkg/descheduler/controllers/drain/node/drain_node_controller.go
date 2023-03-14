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

package node

import (
	"context"
	"fmt"
	"time"

	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/cache"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/reservation"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/utils"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/options"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/framework"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	Name              = "DrainNodeController"
	defaultMarginTime = 5 * time.Minute
)

type DrainNodeReconciler struct {
	client.Client
	eventRecorder          events.EventRecorder
	cache                  cache.Cache
	reservationInterpreter reservation.Interpreter
}

func NewDrainNodeController(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	r, err := newDrainNodeReconciler(handle)
	if err != nil {
		return nil, err
	}

	c, err := controller.New(Name, options.Manager, controller.Options{Reconciler: r, MaxConcurrentReconciles: 1})
	if err != nil {
		return nil, err
	}

	if err = c.Watch(&source.Kind{Type: &v1alpha1.DrainNode{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}

	if err = c.Watch(&source.Kind{Type: r.reservationInterpreter.GetReservationType()}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &v1alpha1.DrainNode{},
	}); err != nil {
		return nil, err
	}

	if err = c.Watch(&source.Kind{Type: &v1alpha1.PodMigrationJob{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &v1alpha1.DrainNode{},
	}); err != nil {
		return nil, err
	}
	return r, nil
}

func newDrainNodeReconciler(handle framework.Handle) (*DrainNodeReconciler, error) {
	manager := options.Manager
	r := &DrainNodeReconciler{
		Client:                 manager.GetClient(),
		eventRecorder:          handle.EventRecorder(),
		cache:                  cache.GetCache(),
		reservationInterpreter: reservation.NewInterpreter(manager.GetClient(), manager.GetAPIReader()),
	}
	err := manager.Add(r)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *DrainNodeReconciler) Name() string {
	return Name
}

func (r *DrainNodeReconciler) Start(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Minute)
	stopCh := ctx.Done()
	for {
		r.doGC()
		select {
		case <-stopCh:
			return nil
		case <-ticker.C:
		}
	}
}

func (r *DrainNodeReconciler) doGC() {
	list := &v1alpha1.DrainNodeList{}
	if err := r.Client.List(context.Background(), list); err != nil {
		klog.Errorf("Failed to list DrainNode err: %v", err)
		return
	}
	for i := range list.Items {
		dn := &list.Items[i]
		if dn.Spec.TTL == nil || dn.Spec.TTL.Duration == 0 {
			continue
		}
		timeoutDuration := dn.Spec.TTL.Duration + defaultMarginTime
		if time.Since(dn.CreationTimestamp.Time) < timeoutDuration {
			continue
		}
		if err := r.abort(dn); err != nil {
			continue
		}
		if err := r.Client.Delete(context.Background(), dn); err != nil {
			klog.Errorf("Failed to delete DrainNode %s, err: %v", dn.Name, err)
		} else {
			klog.V(3).Infof("Successfully delete DrainNode %s", dn.Name)
		}
	}
}

// +kubebuilder:rbac:groups=scheduling.koordinator.sh,resources=drainnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.koordinator.sh,resources=drainnodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.koordinator.sh,resources=reservations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.koordinator.sh,resources=podmigrationjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=pods,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads that state of the cluster for a PodMigrationJob object and makes changes based on the state read
// and what is in the Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
func (r *DrainNodeReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	dn := &v1alpha1.DrainNode{}
	if err := r.Client.Get(ctx, request.NamespacedName, dn); errors.IsNotFound(err) {
		klog.Errorf("Ignore DrainNode %v, Not Found", request)
		return reconcile.Result{}, nil
	} else if err != nil {
		klog.Errorf("Failed to Get DrainNode from %v, err: %v", request, err)
		return reconcile.Result{}, err
	}

	err := r.dispatch(dn)
	return reconcile.Result{}, err
}

func (r *DrainNodeReconciler) dispatch(dn *v1alpha1.DrainNode) error {
	switch dn.Status.Phase {
	case "":
		return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhasePending, nil, "Initialized")
	case v1alpha1.DrainNodePhasePending:
		klog.Infof("Deal pending DrainNode: %v, node: %v", dn.Name, dn.Spec.NodeName)
		return r.handlePendingDrainNode(dn)
	case v1alpha1.DrainNodePhaseAvailable:
		klog.Infof("Deal available DrainNode: %v, node: %v", dn.Name, dn.Spec.NodeName)
		return r.handleAvailableDrainNode(dn)
	case v1alpha1.DrainNodePhaseWaiting:
		klog.Infof("Deal waiting DrainNode: %v, node: %v", dn.Name, dn.Spec.NodeName)
		return r.handleWaitingDrainNode(dn)
	case v1alpha1.DrainNodePhaseRunning:
		klog.Infof("Deal running DrainNode: %v, node: %v", dn.Name, dn.Spec.NodeName)
		return r.handleRunningDrainNode(dn)
	case v1alpha1.DrainNodePhaseSucceeded:
		if utils.NeedCleanTaint(dn.Labels) {
			return r.cleanTaint(dn)
		}
	case v1alpha1.DrainNodePhaseUnavailable, v1alpha1.DrainNodePhaseFailed, v1alpha1.DrainNodePhaseAborted:
		klog.Infof("Deal %v DrainNode: %v, node: %v", dn.Status.Phase, dn.Name, dn.Spec.NodeName)
		return r.handleAbnormalDrainNode(dn)
	}
	return nil
}

func (r *DrainNodeReconciler) patchNodeAddLabelAndTaint(dn *v1alpha1.DrainNode) error {
	ni := r.cache.GetNodeInfo(dn.Spec.NodeName)
	if ni == nil {
		return fmt.Errorf("get node %v cache error", dn.Spec.NodeName)
	}
	if !ni.Drainable {
		klog.V(3).Infof("DrainNode %v Node %v is unmigratable ", dn.Name, dn.Spec.NodeName)
		utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhaseUnavailable, nil, fmt.Sprintf("Unavailable, %v node is not migrated", ni.Name))
		return fmt.Errorf("DrainNode %v Node %v is unmigratable ", dn.Name, dn.Spec.NodeName)
	}
	n := &v1.Node{}
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: dn.Spec.NodeName}, n); err != nil {
		klog.Errorf("DrainNode %v failed get Node %v error: %v", dn.Name, dn.Spec.NodeName, err)
		return err
	}

	klog.Infof("DrainNode %v start to patch Node %v", dn.Name, dn.Spec.NodeName)
	if _, err := utils.Patch(r.Client, n, func(o client.Object) client.Object {
		newN := o.(*v1.Node)
		if newN.Labels == nil {
			newN.Labels = map[string]string{}
		}
		newN.Labels[utils.PlanningKey] = ""

		if dn.Spec.DrainNodePolicy != nil && dn.Spec.DrainNodePolicy.Mode == v1alpha1.DrainNodeModeTaint {
			found := false
			for _, t := range newN.Spec.Taints {
				if t.Key == utils.GroupKey {
					found = true
				}
			}
			if !found {
				newN.Spec.Taints = append(newN.Spec.Taints, v1.Taint{
					Key:    utils.GroupKey,
					Effect: v1.TaintEffectNoSchedule,
					Value:  dn.Labels[utils.GroupKey],
				})
			}
		}
		return newN
	}); err != nil {
		klog.Errorf("DrainNode %v Patch Node %v error: %v", dn.Name, dn.Spec.NodeName, err)
		return err
	}
	return nil
}

func (r *DrainNodeReconciler) handlePendingDrainNode(dn *v1alpha1.DrainNode) error {
	if utils.IsAbort(dn.Labels) {
		return r.abort(dn)
	}

	if err := r.patchNodeAddLabelAndTaint(dn); err != nil {
		return err
	}

	// create reservations and toggle status
	pods := r.cache.GetPods(dn.Spec.NodeName)
	status := []v1alpha1.MigrationJobStatus{}
	pendingCount := 0
	for _, p := range pods {
		if p.Ignore {
			continue
		}
		if !p.Migratable {
			klog.V(3).Infof("DrainNode %v Pod %v/%v id unmigratable", dn.Name, p.Namespace, p.Name)
			return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhaseUnavailable, status,
				fmt.Sprintf("Unavailable, %v/%v pod on node %v is not migrated", p.Namespace, p.Name, dn.Spec.NodeName))
		}
		rr, err := r.reservationInterpreter.GetReservation(context.Background(), dn.Name, p.UID)
		if err != nil {
			return err
		}

		if rr == nil {
			pod := &v1.Pod{}
			err := r.Client.Get(context.Background(), p.NamespacedName, pod)
			if err != nil {
				return err
			}
			rr, err = r.reservationInterpreter.CreateReservation(context.Background(), dn, pod)
			if err != nil {
				klog.Errorf("DrainNode %v Pod %v/%v create Reservation error: %v", dn.Name, pod.Namespace, pod.Name, err)
				return err
			}
			klog.V(3).Infof("DrainNode %v Pod %v/%v create Reservation %v", dn.Name, pod.Namespace, pod.Name, rr.GetName())
		}

		status = append(status, v1alpha1.MigrationJobStatus{
			Namespace:        p.Namespace,
			PodName:          p.Name,
			ReservationName:  rr.GetName(),
			ReservationPhase: v1alpha1.ReservationPhase(rr.GetPhase()),
			TargetNode:       rr.GetScheduledNodeName(),
		})

		if rr.GetUnschedulableCondition() != nil {
			klog.V(3).Infof("DrainNode %v Pod %v/%v Reservation %v scheduled fail", dn.Name, p.Namespace, p.Name, rr.GetName())
			return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhaseUnavailable, status,
				fmt.Sprintf("Unavailable, Pod %v/%v Reservation %v scheduled fail", p.Namespace, p.Name, rr.GetName()))
		}

		if rr.IsPending() {
			pendingCount++
		}
	}

	if pendingCount == 0 {
		klog.V(3).Infof("DrainNode %v Node %v pending reservation == 0 toggle to available", dn.Name, dn.Spec.NodeName)
		return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhaseAvailable, status, "Available")
	}

	return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhasePending, status, "Pending")
}

func (r *DrainNodeReconciler) handleAbnormalDrainNode(dn *v1alpha1.DrainNode) error {
	if err := r.cleanTaint(dn); err != nil {
		return err
	}

	klog.V(3).Infof("Abnormal DrainNode %v, status %v, delete reservations", dn.Name, dn.Status.Phase)
	if err := r.reservationInterpreter.DeleteReservations(context.Background(), dn.Name); err != nil {
		return err
	}

	klog.V(3).Infof("Abnormal DrainNode %v, status %v, delete migrations", dn.Name, dn.Status.Phase)
	jobs := &v1alpha1.PodMigrationJobList{}
	selector, _ := labels.Parse(fmt.Sprintf("%v=%v", utils.DrainNodeKey, dn.Name))
	if err := r.Client.List(context.Background(), jobs, &client.ListOptions{LabelSelector: selector}); err != nil {
		return err
	}
	for i := range jobs.Items {
		job := &jobs.Items[i]
		if err := r.Client.Delete(context.Background(), job); err != nil {
			return err
		}
	}
	return nil
}

func (r *DrainNodeReconciler) handleAvailableDrainNode(dn *v1alpha1.DrainNode) error {
	if utils.IsAbort(dn.Labels) {
		return r.abort(dn)
	}
	if newPod, err := r.checkNewPodScheduled(dn); err != nil {
		return err
	} else if len(newPod) > 0 {
		klog.V(3).Infof("New pod %v scheduled to node %v, DrainNode %v failed", newPod, dn.Spec.NodeName, dn.Name)
		return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhaseFailed, nil,
			fmt.Sprintf("new pod %v scheduled to node %v", newPod, dn.Spec.NodeName))
	}

	if allocatedRR, err := r.dealReservations(dn); err != nil {
		return err
	} else if len(allocatedRR) > 0 {
		klog.V(3).Infof("Reservation %v is allocated when migrations not started, DrainNode %v failed", allocatedRR, dn.Name)
		return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhaseFailed, nil,
			fmt.Sprintf("reservation %v is used", allocatedRR))
	}
	return nil
}

func (r *DrainNodeReconciler) handleWaitingDrainNode(dn *v1alpha1.DrainNode) error {
	if utils.IsAbort(dn.Labels) {
		return r.abort(dn)
	}
	if newPod, err := r.checkNewPodScheduled(dn); err != nil {
		return err
	} else if len(newPod) > 0 {
		klog.V(3).Infof("New pod %v scheduled to node %v, DrainNode %v failed", newPod, dn.Spec.NodeName, dn.Name)
		return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhaseFailed, nil,
			fmt.Sprintf("new pod %v scheduled to node %v", newPod, dn.Spec.NodeName))
	}

	if allocatedRR, err := r.dealReservations(dn); err != nil {
		return err
	} else if len(allocatedRR) > 0 {
		klog.V(3).Infof("Reservation %v is allocated when migrations not started, DrainNode %v failed", allocatedRR, dn.Name)
		return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhaseFailed, nil,
			fmt.Sprintf("reservation %v is used", allocatedRR))
	}

	var newPhase v1alpha1.DrainNodePhase
	switch dn.Spec.ConfirmState {
	case v1alpha1.ConfirmStateConfirmed:
		newPhase = v1alpha1.DrainNodePhaseRunning
	case v1alpha1.ConfirmStateRejected:
		newPhase = v1alpha1.DrainNodePhaseAborted
	default:
		return nil
	}
	klog.V(3).Infof("DrainNode %v ConfirmState %v, new phase", dn.Name, dn.Spec.ConfirmState, newPhase)
	return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, newPhase, nil, "ConfirmState Updated")
}

func (r *DrainNodeReconciler) handleRunningDrainNode(dn *v1alpha1.DrainNode) error {
	if utils.IsAbort(dn.Labels) {
		return r.abort(dn)
	}
	rrs, err := r.reservationInterpreter.ListReservation(context.Background(), dn.Name)
	if err != nil {
		return err
	}

	runningCount := 0
	failCount := 0
	status := []v1alpha1.MigrationJobStatus{}
	for _, rr := range rrs {
		job := &v1alpha1.PodMigrationJob{}
		if err := r.Client.Get(context.Background(), types.NamespacedName{Name: rr.GetName()}, job); err != nil && errors.IsNotFound(err) {
			if job, err = r.createMigration(dn, rr); err != nil {
				klog.Errorf("DrainNode %v Reservation %v create PodMigrationJob error: %v", dn.Name, rr.GetName(), err)
				return err
			}
			klog.V(3).Infof("DrainNode %v Reservation %v create PodMigrationJob", dn.Name, rr.GetName())
		} else if err != nil {
			return err
		}
		if job.Status.Phase == v1alpha1.PodMigrationJobFailed || job.Status.Phase == v1alpha1.PodMigrationJobAborted {
			failCount++
		}

		if job.Status.Phase == "" || job.Status.Phase == v1alpha1.PodMigrationJobPending || job.Status.Phase == v1alpha1.PodMigrationJobRunning {
			runningCount++
		}

		status = append(status, v1alpha1.MigrationJobStatus{
			Namespace:            rr.GetLabels()[utils.PodNamespaceKey],
			PodName:              rr.GetLabels()[utils.PodNameKey],
			ReservationName:      rr.GetName(),
			ReservationPhase:     v1alpha1.ReservationPhase(rr.GetPhase()),
			TargetNode:           rr.GetScheduledNodeName(),
			PodMigrationJobName:  job.Name,
			PodMigrationJobPhase: job.Status.Phase,
		})
	}
	if runningCount > 0 {
		return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhaseRunning, status, "Running")
	}

	if newPod, err := r.checkNewPodScheduled(dn); err != nil {
		return err
	} else if len(newPod) > 0 {
		klog.Errorf("New pod %v scheduled to node %v, DrainNode %v failed", newPod, dn.Spec.NodeName, dn.Name)
		return fmt.Errorf("new pod %v scheduled to node %v", newPod, dn.Spec.NodeName)
	}

	if failCount > 0 {
		klog.V(3).Infof("DrainNode %v Node %v PodMigrationJob failed", dn.Name, dn.Spec.NodeName)
		return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhaseFailed, status,
			fmt.Sprintf("Failed, %d PodMigrationJob failed", failCount))
	}
	klog.V(3).Infof("DrainNode %v Node %v PodMigrationJob success", dn.Name, dn.Spec.NodeName)
	return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhaseSucceeded, status, "Succeeded")
}

func (r *DrainNodeReconciler) cleanTaint(dn *v1alpha1.DrainNode) error {
	if dn.Spec.DrainNodePolicy == nil || dn.Spec.DrainNodePolicy.Mode != v1alpha1.DrainNodeModeTaint {
		return nil
	}
	n := &v1.Node{}
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: dn.Spec.NodeName}, n); err != nil {
		return err
	}

	klog.Infof("clean node taint DrainNode %v, status %v, patch node %v", dn.Name, dn.Status.Phase, dn.Spec.NodeName)
	if _, err := utils.Patch(r.Client, n, func(o client.Object) client.Object {
		newN := o.(*v1.Node)

		newTaints := []v1.Taint{}
		for _, t := range newN.Spec.Taints {
			if t.Key == utils.GroupKey && t.Value == dn.Labels[utils.GroupKey] {
				continue
			}
			newTaints = append(newTaints, t)
		}
		newN.Spec.Taints = newTaints
		return newN
	}); err != nil {
		return err
	}
	return nil
}

func (r *DrainNodeReconciler) abort(dn *v1alpha1.DrainNode) error {
	if err := r.cleanTaint(dn); err != nil {
		return err
	}
	if dn.Status.Phase == v1alpha1.DrainNodePhaseSucceeded ||
		dn.Status.Phase == v1alpha1.DrainNodePhaseFailed ||
		dn.Status.Phase == v1alpha1.DrainNodePhaseAborted {
		return nil
	}
	klog.V(3).Infof("DrainNode %v Node %v abort", dn.Name, dn.Spec.NodeName)
	return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, v1alpha1.DrainNodePhaseAborted, nil, "DrainNode Aborted")
}

func (r *DrainNodeReconciler) checkNewPodScheduled(dn *v1alpha1.DrainNode) (string, error) {
	pods := r.cache.GetPods(dn.Spec.NodeName)

	for _, p := range pods {
		if p.Ignore {
			continue
		}
		rr, err := r.reservationInterpreter.GetReservation(context.Background(), dn.Name, p.UID)
		if err != nil {
			return "", err
		}
		if rr == nil {
			return getKey(p.Namespace, p.Name), nil
		}
	}
	return "", nil
}

func (r *DrainNodeReconciler) dealReservations(dn *v1alpha1.DrainNode) (string, error) {
	rrs, err := r.reservationInterpreter.ListReservation(context.Background(), dn.Name)
	if err != nil {
		return "", err
	}

	pods := r.cache.GetPods(dn.Spec.NodeName)
	podMap := make(map[string]*cache.PodInfo, len(pods))
	for _, p := range pods {
		podMap[getKey(p.Namespace, p.Name)] = p
	}

	allocatedRR := ""
	for _, rr := range rrs {
		labels := rr.GetLabels()
		// reservation exists, pod terminated, delete reservation
		if _, ok := podMap[getKey(labels[utils.PodNamespaceKey], labels[utils.PodNameKey])]; !ok {
			err := r.reservationInterpreter.DeleteReservation(context.Background(), rr.GetName())
			if err != nil {
				return "", err
			}
		}
		if rr.IsAllocated() {
			allocatedRR = rr.GetName()
		}
	}
	return allocatedRR, nil
}

func (r *DrainNodeReconciler) createMigration(dn *v1alpha1.DrainNode, rr reservation.Object) (*v1alpha1.PodMigrationJob, error) {
	job := &v1alpha1.PodMigrationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: rr.GetName(),
			Labels: map[string]string{
				utils.DrainNodeKey: dn.Name,
			},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(dn, utils.DrainNodeKind)},
		},
		Spec: v1alpha1.PodMigrationJobSpec{
			PodRef: &v1.ObjectReference{
				Namespace: rr.GetLabels()[utils.PodNamespaceKey],
				Name:      rr.GetLabels()[utils.PodNameKey],
			},
			ReservationOptions: &v1alpha1.PodMigrateReservationOptions{
				ReservationRef: &v1.ObjectReference{
					Kind:       rr.GetObjectKind().GroupVersionKind().Kind,
					APIVersion: rr.GetObjectKind().GroupVersionKind().Version,
					Namespace:  rr.GetNamespace(),
					Name:       rr.GetName(),
					UID:        rr.GetUID(),
				},
			},
		},
	}
	err := r.Client.Create(context.Background(), job)
	return job, err
}

func getKey(ns, name string) string {
	return fmt.Sprintf("%v/%v", ns, name)
}
