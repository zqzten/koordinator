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

package group

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/cache"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/utils"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/options"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/framework"
	resutil "github.com/koordinator-sh/koordinator/pkg/util"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	Name                                              = "DrainNodeGroupController"
	defaultRequeueAfter                               = 5 * time.Minute
	defaultMarginTime                                 = 5 * time.Minute
	DrainNodePhaseNotStarted  v1alpha1.DrainNodePhase = "NotStarted"
	DrainNodePhaseAvailable   v1alpha1.DrainNodePhase = "Available"
	DrainNodePhaseUnavailable v1alpha1.DrainNodePhase = "Unavailable"
	DrainNodePhaseFailed      v1alpha1.DrainNodePhase = "Failed"
)

type DrainNodeGroupReconciler struct {
	client.Client
	eventRecorder events.EventRecorder
	cache         cache.Cache
}

func NewDrainNodeGroupController(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	r, err := newDrainNodeGroupReconciler(handle)
	if err != nil {
		return nil, err
	}

	c, err := controller.New(Name, options.Manager, controller.Options{Reconciler: r, MaxConcurrentReconciles: 1})
	if err != nil {
		return nil, err
	}

	if err = c.Watch(source.Kind(options.Manager.GetCache(), &v1alpha1.DrainNodeGroup{}), &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}

	if err = c.Watch(source.Kind(options.Manager.GetCache(), &v1alpha1.DrainNode{}), handler.EnqueueRequestForOwner(
		options.Manager.GetScheme(), options.Manager.GetRESTMapper(), &v1alpha1.DrainNode{}, handler.OnlyControllerOwner())); err != nil {
		return nil, err
	}
	return r, nil
}

func newDrainNodeGroupReconciler(handle framework.Handle) (*DrainNodeGroupReconciler, error) {
	manager := options.Manager

	r := &DrainNodeGroupReconciler{
		Client:        manager.GetClient(),
		eventRecorder: handle.EventRecorder(),
		cache:         cache.GetCache(),
	}
	err := manager.Add(r)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *DrainNodeGroupReconciler) Name() string {
	return Name
}

func (r *DrainNodeGroupReconciler) Start(ctx context.Context) error {
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

func (r *DrainNodeGroupReconciler) doGC() {
	list := &v1alpha1.DrainNodeGroupList{}
	if err := r.Client.List(context.Background(), list); err != nil {
		klog.Errorf("Failed to list DrainNodeGroup err: %v", err)
		return
	}
	for i := range list.Items {
		dng := &list.Items[i]
		if dng.Spec.TTL == nil || dng.Spec.TTL.Duration == 0 {
			continue
		}
		timeoutDuration := dng.Spec.TTL.Duration + defaultMarginTime
		if time.Since(dng.CreationTimestamp.Time) < timeoutDuration {
			continue
		}
		if err := r.abort(dng); err != nil {
			continue
		}
		if err := r.Client.Delete(context.Background(), dng); err != nil {
			klog.Errorf("Failed to delete DrainNodeGroup %s, err: %v", dng.Name, err)
		} else {
			klog.V(3).Infof("Successfully delete DrainNodeGroup %s", dng.Name)
		}
	}
}

// +kubebuilder:rbac:groups=scheduling.koordinator.sh,resources=drainnodegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.koordinator.sh,resources=drainnodegroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.koordinator.sh,resources=drainnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.koordinator.sh,resources=drainnodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.koordinator.sh,resources=reservations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=pods,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads that state of the cluster for a PodMigrationJob object and makes changes based on the state read
// and what is in the Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
func (r *DrainNodeGroupReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	dng := &v1alpha1.DrainNodeGroup{}
	err := r.Client.Get(ctx, request.NamespacedName, dng)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		klog.Errorf("Failed to Get DrainNodeGroup from %v, err: %v", request, err)
		return reconcile.Result{}, err
	}

	switch dng.Status.Phase {
	case "":
		return reconcile.Result{}, toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng,
			v1alpha1.DrainNodeGroupPhasePending, nil, "Initialized")
	case v1alpha1.DrainNodeGroupPhasePending:
		klog.Infof("Deal pending DrainNodeGroup: %v", dng.Name)
		if running, err := r.nextGroup(); err != nil {
			return reconcile.Result{}, err
		} else if running != dng.Name {
			klog.Infof("DrainNodeGroup %v waiting for %v", dng.Name, running)
			r.eventRecorder.Eventf(dng, nil, v1.EventTypeNormal, string(v1alpha1.DrainNodePhasePending),
				string(v1alpha1.DrainNodePhasePending), fmt.Sprintf("waiting for %v", running))
			return reconcile.Result{RequeueAfter: defaultRequeueAfter}, nil
		}
		return reconcile.Result{}, r.handlePendingGroup(dng)
	case v1alpha1.DrainNodeGroupPhasePlanning:
		klog.Infof("Deal planning DrainNodeGroup: %v", dng.Name)
		return reconcile.Result{}, r.handlePlanningGroup(dng)
	case v1alpha1.DrainNodeGroupPhaseWaiting:
		klog.Infof("Deal waiting DrainNodeGroup: %v", dng.Name)
		return reconcile.Result{}, r.handleWaitingGroup(dng)
	case v1alpha1.DrainNodeGroupPhaseRunning:
		klog.Infof("Deal running DrainNodeGroup: %v", dng.Name)
		return reconcile.Result{}, r.handleRunningGroup(dng)
	default:
		if utils.NeedCleanTaint(dng.Labels) {
			if err := r.cleanTaint(dng); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}
}

func (r *DrainNodeGroupReconciler) nextGroup() (string, error) {
	list := v1alpha1.DrainNodeGroupList{}
	if err := r.Client.List(context.Background(), &list); err != nil {
		return "", err
	}
	sort.SliceStable(list.Items, func(i, j int) bool {
		t1 := list.Items[i].CreationTimestamp
		t2 := list.Items[j].CreationTimestamp
		if t1.Equal(&t2) {
			return list.Items[i].Name < list.Items[j].Name
		}
		return t1.Before(&t2)
	})
	running := ""
	for i := 0; i < len(list.Items) && len(running) == 0; i++ {
		d := &list.Items[i]
		switch d.Status.Phase {
		case "", v1alpha1.DrainNodeGroupPhasePending:
			running = d.Name
		case v1alpha1.DrainNodeGroupPhasePlanning, v1alpha1.DrainNodeGroupPhaseWaiting, v1alpha1.DrainNodeGroupPhaseRunning:
			running = d.Name
		}
	}
	return running, nil
}

func (r *DrainNodeGroupReconciler) handlePendingGroup(dng *v1alpha1.DrainNodeGroup) error {
	if utils.IsAbort(dng.Labels) {
		return r.abort(dng)
	}

	klog.Infof("DrainNodeGroup %v clean labels", dng.Name)
	selector, _ := labels.Parse(fmt.Sprintf(utils.GroupKey))
	nodes := &v1.NodeList{}
	if err := r.Client.List(context.Background(), nodes, &client.ListOptions{LabelSelector: selector}); err != nil {
		return err
	}
	for i := range nodes.Items {
		n := &nodes.Items[i]
		if _, err := utils.Patch(r.Client, n, func(o client.Object) client.Object {
			newNode := o.(*v1.Node)
			delete(newNode.Labels, utils.GroupKey)
			delete(newNode.Labels, utils.PlanningKey)
			return newNode
		}); err != nil {
			return err
		}
	}

	klog.Infof("DrainNodeGroup %v patch labels", dng.Name)
	var err error
	if dng.Spec.NodeSelector == nil {
		selector = labels.Everything()
	} else {
		selector, err = metav1.LabelSelectorAsSelector(dng.Spec.NodeSelector)
		if err != nil {
			return err
		}
	}
	nodes = &v1.NodeList{}
	if err := r.Client.List(context.Background(), nodes, &client.ListOptions{LabelSelector: selector}); err != nil {
		return err
	}
	for i := range nodes.Items {
		n := &nodes.Items[i]
		if _, err := utils.Patch(r.Client, n, func(o client.Object) client.Object {
			newNode := o.(*v1.Node)
			newNode.Labels[utils.GroupKey] = dng.Name
			return newNode
		}); err != nil {
			return err
		}
	}
	klog.V(3).Infof("DrainNodeGroup %v start", dng.Name)
	return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhasePlanning, nil, "Start Planning")
}

func (r *DrainNodeGroupReconciler) handlePlanningGroup(dng *v1alpha1.DrainNodeGroup) error {
	if utils.IsAbort(dng.Labels) {
		return r.abort(dng)
	}
	nodes, err := r.getNodeInfos(dng)
	if err != nil {
		return err
	}
	// Determine whether the group has reached the termination condition
	if dng.Spec.TerminationPolicy != nil && len(nodes[v1alpha1.DrainNodePhasePending]) == 0 {
		if dng.Spec.TerminationPolicy.MaxNodeCount != nil &&
			len(nodes[DrainNodePhaseAvailable]) >= int(*dng.Spec.TerminationPolicy.MaxNodeCount) {
			klog.Infof("DrainNodeGroup %v meet MaxNodeCount TerminationPolicy", dng.Name)
			return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhaseWaiting, nodes, "meet MaxNodeCount TerminationPolicy")
		}
		if dng.Spec.TerminationPolicy.PercentageOfResourceReserved != nil &&
			*dng.Spec.TerminationPolicy.PercentageOfResourceReserved > 0 {
			total := v1.ResourceList{}
			free := v1.ResourceList{}
			for key, list := range nodes {
				for i, n := range list {
					total = quotav1.Add(total, n.Allocatable)
					if key == DrainNodePhaseUnavailable ||
						(key == DrainNodePhaseNotStarted && i > 0) {
						free = quotav1.Add(free, n.Free)
					}
				}
			}
			reserved := resutil.MultiplyResourceList(total, float64(*dng.Spec.TerminationPolicy.PercentageOfResourceReserved)/100)
			klog.V(3).Infof("total %v , free %v, reserved %v, percentage %v", total, free, reserved, *dng.Spec.TerminationPolicy.PercentageOfResourceReserved)
			if len(quotav1.IsNegative(quotav1.Subtract(free, reserved))) > 0 {
				klog.Infof("DrainNodeGroup %v meet PercentageOfResourceReserved TerminationPolicy", dng.Name)
				var newPhase v1alpha1.DrainNodeGroupPhase
				if len(nodes[DrainNodePhaseAvailable]) > 0 {
					newPhase = v1alpha1.DrainNodeGroupPhaseWaiting
				} else {
					newPhase = v1alpha1.DrainNodeGroupPhaseFailed
				}
				return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, newPhase, nodes, "meet PercentageOfResourceReserved TerminationPolicy")
			}
		}
	}

	switch {
	case len(nodes[v1alpha1.DrainNodePhasePending]) > 0:
		return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhasePlanning, nodes, "Planning")
	case len(nodes[DrainNodePhaseNotStarted]) == 0 && len(nodes[DrainNodePhaseAvailable]) > 0:
		return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhaseWaiting, nodes, "Waiting for confirm")
	case len(nodes[DrainNodePhaseNotStarted]) == 0 && len(nodes[DrainNodePhaseAvailable]) == 0:
		return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhaseFailed, nodes, "Failed, all nodes are not migrated")
	default:
		ni := nodes[DrainNodePhaseNotStarted][0]
		if err := r.createDrainNode(dng, ni); err != nil {
			return err
		}
	}
	return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhasePlanning, nodes, "Planning")
}

func (r *DrainNodeGroupReconciler) handleWaitingGroup(dng *v1alpha1.DrainNodeGroup) error {
	if utils.IsAbort(dng.Labels) {
		return r.abort(dng)
	}
	nodes, err := r.getNodeInfos(dng)
	if err != nil {
		return err
	}
	partition := int32(0)
	if dng.Spec.ExecutionPolicy != nil && dng.Spec.ExecutionPolicy.Partition != nil {
		partition = *dng.Spec.ExecutionPolicy.Partition
	} else {
		partition = math.MaxInt32
	}

	running := len(nodes[v1alpha1.DrainNodePhaseRunning])
	Succeeded := len(nodes[v1alpha1.DrainNodePhaseComplete])
	failed := len(nodes[DrainNodePhaseFailed])
	aborted := len(nodes[v1alpha1.DrainNodePhaseAborted])
	remaining := partition - int32(running+Succeeded+failed+aborted)
	for i := 0; int32(i) < remaining && i < len(nodes[DrainNodePhaseAvailable]); i++ {
		ni := nodes[DrainNodePhaseAvailable][i]
		dn := &v1alpha1.DrainNode{}
		if err := r.Client.Get(context.Background(), types.NamespacedName{Name: fmt.Sprintf("%v-%v", dng.Name, ni.Name)}, dn); err != nil {
			return err
		}
		if _, err := utils.Patch(r.Client, dn, func(o client.Object) client.Object {
			newDn := o.(*v1alpha1.DrainNode)
			newDn.Spec.ConfirmState = v1alpha1.ConfirmStateConfirmed
			return newDn
		}); err != nil {
			return err
		}
	}

	if len(nodes[v1alpha1.DrainNodePhaseRunning]) > 0 {
		return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhaseRunning, nodes,
			fmt.Sprintf("Running, %d DrainNode start running", len(nodes[v1alpha1.DrainNodePhaseRunning])))
	}
	return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhaseWaiting, nodes, "Waiting")
}

func (r *DrainNodeGroupReconciler) handleRunningGroup(dng *v1alpha1.DrainNodeGroup) error {
	if utils.IsAbort(dng.Labels) {
		return r.abort(dng)
	}
	nodes, err := r.getNodeInfos(dng)
	if err != nil {
		return err
	}

	if len(nodes[v1alpha1.DrainNodePhaseRunning]) > 0 {
		return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhaseRunning, nodes, "Running")
	}

	switch {
	case len(nodes[DrainNodePhaseAvailable]) > 0:
		return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhaseWaiting, nodes, "Waiting")
	case len(nodes[v1alpha1.DrainNodePhaseComplete]) > 0:
		return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhaseSucceeded, nodes,
			fmt.Sprintf("Succeeded, %d DrainNode Succeeded", len(nodes[v1alpha1.DrainNodePhaseComplete])))
	case len(nodes[v1alpha1.DrainNodePhaseComplete]) == 0:
		return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhaseFailed, nodes, "Failed, no successful node")
	}
	return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhaseRunning, nodes, "Running")
}

func (r *DrainNodeGroupReconciler) cleanTaint(dng *v1alpha1.DrainNodeGroup) error {
	drainNodes, err := r.getDrainNodesMap(dng.Name)
	if err != nil {
		return err
	}
	for _, dn := range drainNodes {
		if _, err := utils.Patch(r.Client, dn, func(o client.Object) client.Object {
			newDn := o.(*v1alpha1.DrainNode)
			newDn.Spec.ConfirmState = v1alpha1.ConfirmStateAborted
			return newDn
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *DrainNodeGroupReconciler) abort(dng *v1alpha1.DrainNodeGroup) error {
	drainNodes, err := r.getDrainNodesMap(dng.Name)
	if err != nil {
		return err
	}
	finished := true
	for _, dn := range drainNodes {
		if _, err := utils.Patch(r.Client, dn, func(o client.Object) client.Object {
			newDn := o.(*v1alpha1.DrainNode)
			newDn.Spec.ConfirmState = v1alpha1.ConfirmStateAborted
			return newDn
		}); err != nil {
			return err
		}
		if dn.Status.Phase != v1alpha1.DrainNodePhaseComplete &&
			dn.Status.Phase != v1alpha1.DrainNodePhaseAborted {
			finished = false
		}
	}
	if !finished {
		return fmt.Errorf("DrainNodeGroup wait for DrainNode aborted")
	}
	return toggleDrainNodeGroupState(r.Client, r.eventRecorder, dng, v1alpha1.DrainNodeGroupPhaseAborted, nil, "DrainNodeGroup Aborted")
}

func drainNodeSatisfyCondition(dn *v1alpha1.DrainNode, conditionType v1alpha1.DrainNodeConditionType) bool {
	conditionIndex, condition := utils.GetCondition(&dn.Status, conditionType)
	if conditionIndex != -1 && condition.Status == metav1.ConditionTrue {
		return true
	}
	return false
}

func (r *DrainNodeGroupReconciler) createDrainNode(dng *v1alpha1.DrainNodeGroup, ni *cache.NodeInfo) error {
	dn := &v1alpha1.DrainNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%v-%v", dng.Name, ni.Name),
			Labels: map[string]string{
				utils.GroupKey:    dng.Name,
				utils.NodeNameKey: ni.Name,
			},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(dng, utils.DrainNodeGroupKind)},
		},
		Spec: v1alpha1.DrainNodeSpec{
			NodeName:     ni.Name,
			TTL:          dng.Spec.TTL,
			CordonPolicy: dng.Spec.CordonNodePolicy,
			MigrationPolicy: v1alpha1.MigrationPolicy{
				Mode: v1alpha1.MigrationPodModeMigrateDirectly,
			},
			ConfirmState: v1alpha1.ConfirmStateWait,
		},
	}

	klog.Infof("create DrainNode %v from DrainNodeGroup %v", dn.Name, dng.Name)
	if err := r.Client.Create(context.Background(), dn); err != nil && errors.IsAlreadyExists(err) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func (r *DrainNodeGroupReconciler) getNodeInfos(dng *v1alpha1.DrainNodeGroup) (cache.NodeInfoMap, error) {
	var selector labels.Selector
	var err error
	ret := cache.NodeInfoMap{}
	if dng.Spec.NodeSelector == nil {
		selector = labels.Everything()
	} else {
		selector, err = metav1.LabelSelectorAsSelector(dng.Spec.NodeSelector)
		if err != nil {
			return ret, err
		}
	}
	nodes, err := r.getNodesMap(selector)
	if err != nil {
		return ret, err
	}
	drainNodes, err := r.getDrainNodesMap(dng.Name)
	if err != nil {
		return ret, err
	}
	for k, n := range nodes {
		ni := r.cache.GetNodeInfo(n.Name)
		if ni == nil {
			return ret, fmt.Errorf("get node %v cache error", n.Name)
		}
		if dn, ok := drainNodes[fmt.Sprintf("%v-%v", dng.Name, k)]; ok {
			if dn.Status.Phase == "" {
				ret[v1alpha1.DrainNodePhasePending] = append(ret[v1alpha1.DrainNodePhasePending], ni)
			} else {
				if drainNodeSatisfyCondition(dn, v1alpha1.DrainNodeConditionUnavailableReservationExists) {
					ret[DrainNodePhaseUnavailable] = append(ret[DrainNodePhaseUnavailable], ni)
				} else if drainNodeSatisfyCondition(dn, v1alpha1.DrainNodeConditionUnmigratablePodExists) &&
					drainNodeSatisfyCondition(dn, v1alpha1.DrainNodeConditionFailedMigrationJobExists) &&
					drainNodeSatisfyCondition(dn, v1alpha1.DrainNodeConditionUnexpectedReservationExists) &&
					drainNodeSatisfyCondition(dn, v1alpha1.DrainNodeConditionUnexpectedPodAfterCompleteExists) {
					ret[DrainNodePhaseFailed] = append(ret[DrainNodePhaseFailed], ni)
				} else if drainNodeSatisfyCondition(dn, v1alpha1.DrainNodeConditionOnceAvailable) {
					ret[DrainNodePhaseAvailable] = append(ret[DrainNodePhaseAvailable], ni)
				} else {
					ret[dn.Status.Phase] = append(ret[dn.Status.Phase], ni)
				}
			}
		} else {
			ret[DrainNodePhaseNotStarted] = append(ret[DrainNodePhaseNotStarted], ni)
		}
	}

	list := ret[DrainNodePhaseNotStarted]
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Score > list[j].Score
	})
	list = ret[DrainNodePhaseAvailable]
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})

	return ret, nil
}

func (r *DrainNodeGroupReconciler) getNodesMap(selector labels.Selector) (map[string]*v1.Node, error) {
	list := &v1.NodeList{}
	if err := r.Client.List(context.Background(), list, &client.ListOptions{LabelSelector: selector}); err != nil {
		return nil, err
	}
	ret := map[string]*v1.Node{}
	for i := range list.Items {
		n := &list.Items[i]
		ret[n.Name] = n
	}
	return ret, nil
}

func (r *DrainNodeGroupReconciler) getDrainNodesMap(name string) (map[string]*v1alpha1.DrainNode, error) {
	selector, _ := labels.Parse(fmt.Sprintf("%v=%v", utils.GroupKey, name))
	list := &v1alpha1.DrainNodeList{}
	if err := r.Client.List(context.Background(), list, &client.ListOptions{LabelSelector: selector}); err != nil {
		return nil, err
	}
	ret := map[string]*v1alpha1.DrainNode{}
	for i := range list.Items {
		n := &list.Items[i]
		ret[n.Name] = n
	}
	return ret, nil
}

func toggleDrainNodeGroupState(c client.Client,
	eventRecorder events.EventRecorder,
	dng *v1alpha1.DrainNodeGroup,
	phase v1alpha1.DrainNodeGroupPhase,
	nodes cache.NodeInfoMap,
	msg string) error {

	ok, err := utils.PatchStatus(c, dng, func(o client.Object) client.Object {
		newDng := o.(*v1alpha1.DrainNodeGroup)

		newDng.Status.Phase = phase
		if len(nodes) > 0 {
			total := int32(0)
			for k, list := range nodes {
				if k == DrainNodePhaseNotStarted {
					continue
				}
				total += int32(len(list))
			}
			newDng.Status.TotalCount = total
			newDng.Status.UnavailableCount = int32(len(nodes[DrainNodePhaseUnavailable]))
			newDng.Status.AvailableCount = total - newDng.Status.UnavailableCount
			newDng.Status.RunningCount = int32(len(nodes[v1alpha1.DrainNodePhaseRunning]))
			newDng.Status.SucceededCount = int32(len(nodes[v1alpha1.DrainNodePhaseComplete]))
			newDng.Status.FailedCount = int32(len(nodes[DrainNodePhaseFailed]))
		}
		if newDng.Status.Conditions == nil {
			newDng.Status.Conditions = []metav1.Condition{}
		}
		cond := metav1.Condition{
			Type:    string(phase),
			Status:  metav1.ConditionTrue,
			Reason:  string(phase),
			Message: msg,
		}
		meta.SetStatusCondition(&newDng.Status.Conditions, cond)
		return newDng
	})

	if ok {
		klog.Infof("Update DrainNodeGroup %v status %v, msg %v", dng.Name, phase, msg)
		eventRecorder.Eventf(dng, nil, v1.EventTypeNormal, string(phase), string(phase), msg)
		return nil
	}

	return err
}
