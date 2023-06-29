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

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"

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
	Name                = "DrainNodeController"
	defaultRequeueAfter = 3 * time.Second
	defaultMarginTime   = 5 * time.Minute
	defaultWaitDuration = time.Hour
)

type DrainNodeReconciler struct {
	client.Client
	eventRecorder          events.EventRecorder
	podFilter              framework.FilterFunc
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
		podFilter:              handle.Evictor().Filter,
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

// Reconcile reads that state of the cluster for a DrainNode object and makes changes based on the state read
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
	return reconcile.Result{RequeueAfter: defaultRequeueAfter}, err
}

func (r *DrainNodeReconciler) dispatch(dn *v1alpha1.DrainNode) error {
	switch dn.Status.Phase {
	case "":
		return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, &v1alpha1.DrainNodeStatus{
			Phase:               v1alpha1.DrainNodePhasePending,
			PodMigrations:       nil,
			PodMigrationSummary: nil,
			Conditions:          nil,
		}, "Initialized")
	case v1alpha1.DrainNodePhasePending:
		klog.Infof("Deal pending DrainNode: %v, node: %v", dn.Name, dn.Spec.NodeName)
		return r.handlePendingDrainNode(dn)
	case v1alpha1.DrainNodePhaseRunning, v1alpha1.DrainNodePhaseComplete:
		klog.Infof("Deal %v DrainNode: %v, node: %v", dn.Status.Phase, dn.Name, dn.Spec.NodeName)
		return r.handleRunningOrCompleteDrainNode(dn)
	case v1alpha1.DrainNodePhaseAborted:
		klog.Infof("Deal %v DrainNode: %v, node: %v", dn.Status.Phase, dn.Name, dn.Spec.NodeName)
		return r.handleAbortedDrainNode(dn)
	}
	return nil
}

func (r *DrainNodeReconciler) cordonNode(dn *v1alpha1.DrainNode) error {
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

		if dn.Spec.CordonPolicy != nil {
			if dn.Spec.CordonPolicy.Mode == v1alpha1.CordonNodeModeTaint {
				found := false
				for _, t := range newN.Spec.Taints {
					if t.Key == utils.DrainNodeKey {
						found = true
					}
				}
				if !found {
					newN.Spec.Taints = append(newN.Spec.Taints, v1.Taint{
						Key:    utils.DrainNodeKey,
						Effect: v1.TaintEffectNoSchedule,
						Value:  dn.Name,
					})
				}
			}
			if dn.Spec.CordonPolicy.Mode == v1alpha1.CordonNodeModeLabel && len(dn.Spec.CordonPolicy.Labels) != 0 {
				if newN.Labels == nil {
					newN.Labels = map[string]string{}
				}
				for k, v := range dn.Spec.CordonPolicy.Labels {
					newN.Labels[k] = v
				}
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
	if dn.Spec.ConfirmState == v1alpha1.ConfirmStateRejected || dn.Spec.ConfirmState == v1alpha1.ConfirmStateAborted {
		return r.abort(dn)
	}
	if err := r.cordonNode(dn); err != nil {
		return err
	}
	return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, &v1alpha1.DrainNodeStatus{
		Phase:               v1alpha1.DrainNodePhaseRunning,
		PodMigrations:       nil,
		PodMigrationSummary: nil,
		Conditions:          nil,
	}, "Running")
}

func (r *DrainNodeReconciler) handleAbortedDrainNode(dn *v1alpha1.DrainNode) error {
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

func (r *DrainNodeReconciler) handleRunningOrCompleteDrainNode(dn *v1alpha1.DrainNode) error {
	if dn.Spec.ConfirmState == v1alpha1.ConfirmStateRejected || dn.Spec.ConfirmState == v1alpha1.ConfirmStateAborted {
		return r.abort(dn)
	}

	// get current actual podMigrations
	pods := r.cache.GetPods(dn.Spec.NodeName)
	actualPodMigrations, _ := r.getActualPodMigrations(dn, pods)

	// init information
	conditions := make([]v1alpha1.DrainNodeCondition, len(dn.Status.Conditions))
	copy(conditions, dn.Status.Conditions)
	status := &v1alpha1.DrainNodeStatus{
		Phase:      dn.Status.Phase,
		Conditions: conditions,
	}
	podMigrationSummary := map[v1alpha1.PodMigrationPhase]int32{
		v1alpha1.PodMigrationPhaseUnmigratable: 0,
		v1alpha1.PodMigrationPhaseWaiting:      0,
		v1alpha1.PodMigrationPhaseReady:        0,
		v1alpha1.PodMigrationPhaseAvailable:    0,
		v1alpha1.PodMigrationPhaseUnavailable:  0,
		v1alpha1.PodMigrationPhaseMigrating:    0,
		v1alpha1.PodMigrationPhaseSucceed:      0,
		v1alpha1.PodMigrationPhaseFailed:       0,
	}
	var podMigrations []v1alpha1.PodMigration

	// make progress for all podMigrations and gather information
	existingPodInfos := map[types.UID]*cache.PodInfo{}
	for i := range pods {
		existingPodInfo := pods[i]
		existingPodInfos[existingPodInfo.UID] = existingPodInfo
	}
	for i := range actualPodMigrations {
		actualPodMigration := &actualPodMigrations[i]
		podInfo := existingPodInfos[actualPodMigration.PodUID]
		if podInfo == nil || podInfo.Pod == nil || actualPodMigration.Phase == v1alpha1.PodMigrationPhaseSucceed {
			leftForAuditing, err := r.makeMigrationComplete(actualPodMigration)
			if err != nil {
				return err
			}
			if !leftForAuditing {
				continue
			}
		} else {
			err := r.progressMigration(dn, actualPodMigration, podInfo)
			if err != nil {
				return err
			}
		}
		podMigrations = append(podMigrations, *actualPodMigration)
		podMigrationSummary[actualPodMigration.Phase]++
	}
	status.PodMigrations = podMigrations
	status.PodMigrationSummary = podMigrationSummary

	// expose unexpectedRR info by condition, doesn't affect normal flow
	ni := r.cache.GetNodeInfo(dn.Spec.NodeName)
	if ni == nil {
		return fmt.Errorf("get node %v cache error", dn.Spec.NodeName)
	}
	unexpectedRRCount := 0
	unexpectedRR := sets.NewString()
	podMigrationRR := map[string]struct{}{}
	for i := range podMigrations {
		actualPodMigration := &podMigrations[i]
		if actualPodMigration.ReservationName != "" {
			podMigrationRR[actualPodMigration.ReservationName] = struct{}{}
		}
	}
	for reservationName := range ni.Reservation {
		if _, ok := podMigrationRR[reservationName]; !ok {
			unexpectedRRCount++
			unexpectedRR.Insert(reservationName)
		}
	}
	drainNodeCondition := &v1alpha1.DrainNodeCondition{
		Type:    v1alpha1.DrainNodeConditionUnexpectedReservationExists,
		Status:  metav1.ConditionFalse,
		Reason:  fmt.Sprintf("unexpected reservation count: %d,", unexpectedRRCount),
		Message: fmt.Sprintf("unexpectedRR: %v", unexpectedRR.List()),
	}
	if unexpectedRRCount != 0 {
		drainNodeCondition.Status = metav1.ConditionTrue
	}
	utils.UpdateCondition(status, drainNodeCondition)

	// expose unmigratablePodExists by condition, doesn't affect normal flow
	drainNodeCondition = &v1alpha1.DrainNodeCondition{
		Type:    v1alpha1.DrainNodeConditionUnmigratablePodExists,
		Status:  metav1.ConditionFalse,
		Reason:  fmt.Sprintf("unmigratable pod count: %d", podMigrationSummary[v1alpha1.PodMigrationPhaseUnmigratable]),
		Message: fmt.Sprintf("unmigratable pod count: %d", podMigrationSummary[v1alpha1.PodMigrationPhaseUnmigratable]),
	}
	if podMigrationSummary[v1alpha1.PodMigrationPhaseUnmigratable] != 0 {
		drainNodeCondition.Status = metav1.ConditionTrue
	}
	utils.UpdateCondition(status, drainNodeCondition)

	// expose unavailableRR by condition, doesn't affect normal flow
	drainNodeCondition = &v1alpha1.DrainNodeCondition{
		Type:    v1alpha1.DrainNodeConditionUnavailableReservationExists,
		Status:  metav1.ConditionFalse,
		Reason:  fmt.Sprintf("unavailable reservation count: %d", podMigrationSummary[v1alpha1.PodMigrationPhaseUnavailable]),
		Message: fmt.Sprintf("unavailable reservation count: %d", podMigrationSummary[v1alpha1.PodMigrationPhaseUnavailable]),
	}
	if podMigrationSummary[v1alpha1.PodMigrationPhaseUnavailable] != 0 {
		drainNodeCondition.Status = metav1.ConditionTrue
	}
	utils.UpdateCondition(status, drainNodeCondition)

	// expose failedJob by condition, doesn't affect normal flow
	drainNodeCondition = &v1alpha1.DrainNodeCondition{
		Type:    v1alpha1.DrainNodeConditionFailedMigrationJobExists,
		Status:  metav1.ConditionFalse,
		Reason:  fmt.Sprintf("failed job count: %d", podMigrationSummary[v1alpha1.PodMigrationPhaseFailed]),
		Message: fmt.Sprintf("failed job count: %d", podMigrationSummary[v1alpha1.PodMigrationPhaseFailed]),
	}
	if podMigrationSummary[v1alpha1.PodMigrationPhaseFailed] != 0 {
		drainNodeCondition.Status = metav1.ConditionTrue
	}
	utils.UpdateCondition(status, drainNodeCondition)

	// expose OnceAvailableRR by condition, doesn't affect normal flow
	if podMigrationSummary[v1alpha1.PodMigrationPhaseReady]+
		podMigrationSummary[v1alpha1.PodMigrationPhaseUnavailable]+
		podMigrationSummary[v1alpha1.PodMigrationPhaseUnmigratable] == 0 {
		drainNodeCondition = &v1alpha1.DrainNodeCondition{
			Type:    v1alpha1.DrainNodeConditionOnceAvailable,
			Status:  metav1.ConditionTrue,
			Reason:  string(v1alpha1.DrainNodeConditionOnceAvailable),
			Message: "no unmigratable, waiting, ready and unavailable migration",
		}
		utils.UpdateCondition(status, drainNodeCondition)
	}

	// complete drainNode when no progress can be made by drainNode
	if podMigrationSummary[v1alpha1.PodMigrationPhaseWaiting]+
		podMigrationSummary[v1alpha1.PodMigrationPhaseReady]+
		podMigrationSummary[v1alpha1.PodMigrationPhaseAvailable]+
		podMigrationSummary[v1alpha1.PodMigrationPhaseMigrating] == 0 {
		drainNodeCondition = &v1alpha1.DrainNodeCondition{
			Type:    v1alpha1.DrainNodeConditionUnexpectedPodAfterCompleteExists,
			Status:  metav1.ConditionFalse,
			Reason:  "",
			Message: "",
		}
		utils.UpdateCondition(status, drainNodeCondition)
		status.Phase = v1alpha1.DrainNodePhaseComplete
		return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, status, "")
	} else if dn.Status.Phase == v1alpha1.DrainNodePhaseComplete {
		drainNodeCondition = &v1alpha1.DrainNodeCondition{
			Type:    v1alpha1.DrainNodeConditionUnexpectedPodAfterCompleteExists,
			Status:  metav1.ConditionTrue,
			Reason:  string(v1alpha1.DrainNodeConditionUnexpectedPodAfterCompleteExists),
			Message: "some unexpected pod scheduled to this node after drainNode has completed",
		}
		utils.UpdateCondition(status, drainNodeCondition)
		return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, status, "")
	}
	return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, status, "")
}

func (r *DrainNodeReconciler) getActualPodMigrations(dn *v1alpha1.DrainNode, pods []*cache.PodInfo) (actualPodMigrations []v1alpha1.PodMigration, deltaPodMigrations []v1alpha1.PodMigration) {
	recordedPodMigrations := map[types.UID]struct{}{}
	for i := range dn.Status.PodMigrations {
		podMigration := dn.Status.PodMigrations[i]
		recordedPodMigrations[podMigration.PodUID] = struct{}{}
		actualPodMigrations = append(actualPodMigrations, podMigration)
	}

	for _, p := range pods {
		if p.Ignore {
			continue
		}
		if _, ok := recordedPodMigrations[p.UID]; ok {
			continue
		}
		actualPodMigration := v1alpha1.PodMigration{
			PodUID:         p.UID,
			Namespace:      p.Namespace,
			PodName:        p.Name,
			Phase:          v1alpha1.PodMigrationPhaseWaiting,
			StartTimestamp: metav1.Now(),
		}
		if !r.podFilter(p.Pod) {
			actualPodMigration.Phase = v1alpha1.PodMigrationPhaseUnmigratable
		}
		if dn.Spec.ConfirmState == v1alpha1.ConfirmStateConfirmed {
			actualPodMigration.ChartedAfterDrainNodeConfirmed = true
		}
		actualPodMigrations = append(actualPodMigrations, actualPodMigration)
		deltaPodMigrations = append(deltaPodMigrations, actualPodMigration)
	}
	return actualPodMigrations, deltaPodMigrations
}

func (r *DrainNodeReconciler) makeMigrationComplete(actualPodMigration *v1alpha1.PodMigration) (bool, error) {
	leftForAuditing := false
	if actualPodMigration.ReservationName != "" && actualPodMigration.PodMigrationJobName == "" {
		leftForAuditing = true
		klog.V(3).Infof("Pod %s/%s finished, delete reservation %s", actualPodMigration.Namespace, actualPodMigration.PodName, actualPodMigration.ReservationName)
		if err := r.reservationInterpreter.DeleteReservation(context.Background(), actualPodMigration.ReservationName); err != nil && !errors.IsNotFound(err) {
			return true, err
		}
	}
	if actualPodMigration.PodMigrationJobName != "" {
		leftForAuditing = true
		job := &v1alpha1.PodMigrationJob{}
		if err := r.Client.Get(context.Background(), types.NamespacedName{Name: actualPodMigration.PodMigrationJobName}, job); err != nil {
			return true, err
		}
		actualPodMigration.PodMigrationJobPhase = job.Status.Phase
		actualPodMigration.PodMigrationJobPaused = pointer.Bool(job.Spec.Paused)
	}
	if !leftForAuditing {
		return false, nil
	}
	actualPodMigration.Phase = v1alpha1.PodMigrationPhaseSucceed
	return true, nil
}

func readyForMigration(migrationPolicy *v1alpha1.MigrationPolicy, startTime time.Time) (readyByMode, readyByDuration, valid bool) {
	valid = migrationPolicy != nil && (migrationPolicy.Mode == v1alpha1.MigrationPodModeMigrateDirectly || migrationPolicy.Mode == v1alpha1.MigrationPodModeWaitFirst)
	now := metav1.Now()
	waitDeadline := defaultWaitDuration
	if migrationPolicy != nil && migrationPolicy.WaitDuration != nil {
		waitDeadline = migrationPolicy.WaitDuration.Duration
	}
	readyByDuration = now.Sub(startTime) > waitDeadline
	readyByMode = migrationPolicy != nil && migrationPolicy.Mode == v1alpha1.MigrationPodModeMigrateDirectly
	return
}

func (r *DrainNodeReconciler) progressMigration(dn *v1alpha1.DrainNode, actualPodMigration *v1alpha1.PodMigration, podInfo *cache.PodInfo) error {
	if dn.Status.Phase == v1alpha1.DrainNodePhaseComplete {
		return nil
	}
	pod := podInfo.Pod
	switch actualPodMigration.Phase {
	case v1alpha1.PodMigrationPhaseWaiting:
		migrationPolicy, err := utils.GetMigrationPolicy(pod)
		if err != nil {
			return err
		}
		ready := false
		readyByPodMode, readyByPodDuration, valid := readyForMigration(migrationPolicy, actualPodMigration.StartTimestamp.Time)
		readyByDrainNodeMode, readyByDrainNodeDuration, _ := readyForMigration(&dn.Spec.MigrationPolicy, dn.CreationTimestamp.Time)
		ready = (valid && (readyByPodMode || (readyByPodDuration || readyByDrainNodeDuration))) || (!valid && (readyByDrainNodeMode || readyByDrainNodeDuration))
		if ready {
			actualPodMigration.Phase = v1alpha1.PodMigrationPhaseReady
			return r.progressMigration(dn, actualPodMigration, podInfo)
		}
		return nil
	case v1alpha1.PodMigrationPhaseReady:
		rr, err := r.reservationInterpreter.GetReservation(context.Background(), dn.Name, actualPodMigration.PodUID)
		if err != nil {
			return err
		}
		if rr == nil {
			rr, err = r.reservationInterpreter.CreateReservation(context.Background(), dn, pod)
			if err != nil {
				klog.Errorf("DrainNode %v Pod %v/%v create Reservation error: %v", dn.Name, actualPodMigration.Namespace, actualPodMigration.PodName, err)
				return err
			}
			klog.V(3).Infof("DrainNode %v Pod %v/%v create Reservation %v", dn.Name, actualPodMigration.Namespace, actualPodMigration.PodName, rr.GetName())
		}
		actualPodMigration.ReservationName = rr.GetName()
		actualPodMigration.ReservationPhase = v1alpha1.ReservationPhase(rr.GetPhase())
		actualPodMigration.TargetNode = rr.GetScheduledNodeName()
		// next migration phase
		if (actualPodMigration.ReservationPhase == v1alpha1.ReservationSucceeded && actualPodMigration.PodMigrationJobName == "") ||
			actualPodMigration.ReservationPhase == v1alpha1.ReservationFailed {
			actualPodMigration.Phase = v1alpha1.PodMigrationPhaseUnavailable
			return nil
		}
		if rr.GetPhase() == string(v1alpha1.ReservationAvailable) {
			actualPodMigration.Phase = v1alpha1.PodMigrationPhaseAvailable
			return r.progressMigration(dn, actualPodMigration, podInfo)
		}
		return nil
	case v1alpha1.PodMigrationPhaseAvailable:
		if dn.Spec.ConfirmState == v1alpha1.ConfirmStateConfirmed {
			actualPodMigration.Phase = v1alpha1.PodMigrationPhaseMigrating
			return r.progressMigration(dn, actualPodMigration, podInfo)
		}
		return nil
	case v1alpha1.PodMigrationPhaseMigrating:
		job := &v1alpha1.PodMigrationJob{}
		migrationJobName := getReservationOrMigrationJobName(dn.Name, actualPodMigration.PodUID)
		if err := r.Client.Get(context.Background(), types.NamespacedName{Name: migrationJobName}, job); err != nil && errors.IsNotFound(err) {
			if job, err = r.createMigration(dn, actualPodMigration); err != nil {
				klog.Errorf("DrainNode %v create PodMigrationJob %v error: %v", dn.Name, migrationJobName, err)
				return err
			}
			klog.V(3).Infof("DrainNode %v create PodMigrationJob %v", dn.Name, migrationJobName)
		} else if err != nil {
			return err
		}
		actualPodMigration.PodMigrationJobName = job.Name
		actualPodMigration.PodMigrationJobPhase = job.Status.Phase
		actualPodMigration.PodMigrationJobPaused = pointer.Bool(job.Spec.Paused)
		if actualPodMigration.PodMigrationJobPhase == v1alpha1.PodMigrationJobFailed || actualPodMigration.PodMigrationJobPhase == v1alpha1.PodMigrationJobAborted {
			actualPodMigration.Phase = v1alpha1.PodMigrationPhaseFailed
		}
		if actualPodMigration.PodMigrationJobPhase == v1alpha1.PodMigrationJobSucceeded {
			actualPodMigration.Phase = v1alpha1.PodMigrationPhaseSucceed
		}
		return nil
	}
	return nil
}

func (r *DrainNodeReconciler) uncordonNode(dn *v1alpha1.DrainNode) error {
	n := &v1.Node{}
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: dn.Spec.NodeName}, n); err != nil {
		return err
	}
	klog.Infof("clean node taint DrainNode %v, status %v, patch node %v", dn.Name, dn.Status.Phase, dn.Spec.NodeName)
	if _, err := utils.Patch(r.Client, n, func(o client.Object) client.Object {
		newN := o.(*v1.Node)
		delete(newN.Labels, utils.PlanningKey)
		if dn.Spec.CordonPolicy != nil {
			if dn.Spec.CordonPolicy.Mode == v1alpha1.CordonNodeModeTaint {
				var newTaints []v1.Taint
				for _, t := range newN.Spec.Taints {
					if t.Key == utils.DrainNodeKey && t.Value == dn.Name {
						continue
					}
					newTaints = append(newTaints, t)
				}
				newN.Spec.Taints = newTaints
			}
			if dn.Spec.CordonPolicy.Mode == v1alpha1.CordonNodeModeLabel && len(dn.Spec.CordonPolicy.Labels) != 0 {
				for k := range dn.Spec.CordonPolicy.Labels {
					delete(newN.Labels, k)
				}
			}
		}
		return newN
	}); err != nil {
		return err
	}
	return nil
}

func (r *DrainNodeReconciler) abort(dn *v1alpha1.DrainNode) error {
	if err := r.uncordonNode(dn); err != nil {
		return err
	}
	if dn.Status.Phase == v1alpha1.DrainNodePhaseComplete ||
		dn.Status.Phase == v1alpha1.DrainNodePhaseAborted {
		return nil
	}
	klog.V(3).Infof("DrainNode %v Node %v abort", dn.Name, dn.Spec.NodeName)
	return utils.ToggleDrainNodeState(r.Client, r.eventRecorder, dn, &v1alpha1.DrainNodeStatus{
		Phase:               v1alpha1.DrainNodePhaseAborted,
		PodMigrations:       dn.Status.PodMigrations,
		PodMigrationSummary: dn.Status.PodMigrationSummary,
		Conditions:          dn.Status.Conditions,
	}, "")
}

func (r *DrainNodeReconciler) createMigration(dn *v1alpha1.DrainNode, actualPodMigration *v1alpha1.PodMigration) (*v1alpha1.PodMigrationJob, error) {
	rr, err := r.reservationInterpreter.GetReservation(context.Background(), dn.Name, actualPodMigration.PodUID)
	if err != nil {
		return nil, err
	}
	if rr == nil {
		return nil, fmt.Errorf("DrainNode %v Reservation %v NotFound", dn.Name, getReservationOrMigrationJobName(dn.Name, actualPodMigration.PodUID))
	}

	job := &v1alpha1.PodMigrationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: rr.GetName(),
			Labels: map[string]string{
				utils.DrainNodeKey: dn.Name,
			},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(dn, utils.DrainNodeKind)},
		},
		Spec: v1alpha1.PodMigrationJobSpec{
			Paused: utils.PausedForPodAfterConfirmed(dn) && actualPodMigration.ChartedAfterDrainNodeConfirmed,
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
	err = r.Client.Create(context.Background(), job)
	return job, err
}

func getReservationOrMigrationJobName(drainNodeName string, podUID types.UID) string {
	return fmt.Sprintf("%s-%.8v", drainNodeName, podUID)
}
