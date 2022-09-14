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

package orchestratingslo

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	unifiedsev1 "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	koordsev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/controllerfinder"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/evictor"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/unified"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/names"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/options"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/framework"
	"github.com/koordinator-sh/koordinator/pkg/util"
	utilclient "github.com/koordinator-sh/koordinator/pkg/util/client"
)

const (
	Name                = names.OrchestratingSLOController
	defaultRequeueAfter = 3 * time.Second
)

var (
	nextScheduleDelta = 100 * time.Millisecond
)

var _ reconcile.Reconciler = &Reconciler{}
var _ framework.DeschedulePlugin = &Reconciler{}

type Reconciler struct {
	client.Client
	handle        framework.Handle
	finder        *controllerfinder.ControllerFinder
	eventRecorder record.EventRecorder
	podCache      PodAssumeCache
	mutex         sync.RWMutex
	rateLimiters  map[reconcile.Request]map[unifiedsev1.OrchestratingSLOOperationType]*RateLimiter
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	r, err := newReconciler(handle)
	if err != nil {
		return nil, err
	}

	// Create a new controller
	c, err := controller.New(Name, options.Manager, controller.Options{Reconciler: r, MaxConcurrentReconciles: 1})
	if err != nil {
		return nil, err
	}

	if err = c.Watch(&source.Kind{Type: &koordsev1alpha1.PodMigrationJob{}}, &handler.Funcs{}); err != nil {
		return nil, err
	}

	err = c.Watch(&source.Kind{Type: &unifiedsev1.OrchestratingSLO{}}, &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			UpdateFunc: func(event event.UpdateEvent) bool {
				oldOrchestratingSLO := event.ObjectOld.(*unifiedsev1.OrchestratingSLO)
				newOrchestratingSLO := event.ObjectNew.(*unifiedsev1.OrchestratingSLO)
				oldStatus := oldOrchestratingSLO.Status.DeepCopy()
				newStatus := newOrchestratingSLO.Status.DeepCopy()
				oldStatus.UpdateTime = nil
				oldStatus.LastScheduleTime = nil
				newStatus.UpdateTime = nil
				newStatus.LastScheduleTime = nil
				return oldOrchestratingSLO.Generation != newOrchestratingSLO.Generation || !reflect.DeepEqual(oldStatus, newStatus)
			},
		},
	)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(handle framework.Handle) (*Reconciler, error) {
	podInformer, err := options.Manager.GetCache().GetInformer(context.TODO(), &v1.Pod{})
	if err != nil {
		return nil, err
	}
	controllerFinder, err := controllerfinder.New(options.Manager)
	if err != nil {
		return nil, err
	}
	podCache := NewPodAssumeCache(podInformer)
	eventRecorder := options.Manager.GetEventRecorderFor("orchestratingSLO")

	r := &Reconciler{
		Client:        options.Manager.GetClient(),
		handle:        handle,
		finder:        controllerFinder,
		eventRecorder: eventRecorder,
		podCache:      podCache,
		rateLimiters:  map[reconcile.Request]map[unifiedsev1.OrchestratingSLOOperationType]*RateLimiter{},
	}
	return r, nil
}

func (r *Reconciler) Name() string {
	return Name
}

func (r *Reconciler) Deschedule(ctx context.Context, nodes []*v1.Node) *framework.Status {
	return &framework.Status{}
}

// Reconcile reads that state of the cluster for a OrchestratingSLO object and makes changes based on the state read
// and what is in the Spec
// +kubebuilder:rbac:groups=scheduling.alibabacloud.com,resources=orchestratingSLO,verbs=get;list;watch;create;update;patch;delete
func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.V(4).Infof("begin reconcile OrchestratingSLO %v", request)
	orchestratingSLO := &unifiedsev1.OrchestratingSLO{}
	err := r.Client.Get(ctx, request.NamespacedName, orchestratingSLO)
	if k8serrors.IsNotFound(err) {
		klog.V(4).Infof("cannot find OrchestratingSLO %v", request)
		r.cleanRateLimiters(request)
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if orchestratingSLO.Spec.Paused {
		klog.V(4).Infof("OrchestratingSLO %v paused", request)
		return reconcile.Result{}, nil
	}

	var sched cron.Schedule
	var scheduledTime time.Time
	if orchestratingSLO.Spec.Operation != nil {
		scheduleWindow := orchestratingSLO.Spec.Operation.ScheduleWindow
		if scheduleWindow != nil {
			sched, err = cron.ParseStandard(scheduleWindow.Schedule)
			if err != nil {
				klog.V(2).Infof(" orchestratingSLO: %v has unparseable schedule, schedule: %v, err: %v", request, scheduleWindow.Schedule, err)
				r.eventRecorder.Eventf(orchestratingSLO, v1.EventTypeWarning, "UnparseableSchedule", "unparseable schedule: %s : %s", scheduleWindow.Schedule, err)
				return reconcile.Result{}, nil
			}
			var nextScheduleTimeDuration *time.Duration
			scheduledTime, nextScheduleTimeDuration = r.satisfiedCronScheduleTime(request, orchestratingSLO, sched)
			if nextScheduleTimeDuration != nil {
				klog.V(4).Infof("OrchestratingSLO %v next schedule time: %v", request, nextScheduleTimeDuration)
				return reconcile.Result{RequeueAfter: *nextScheduleTimeDuration}, nil
			}
		}
	}

	result, err := r.reconcileOne(request, orchestratingSLO)
	if err != nil {
		return result, err
	}

	if sched != nil {
		now := time.Now()
		nextScheduledTime := sched.Next(now).Add(nextScheduleDelta)
		if result.RequeueAfter == 0 || now.Add(result.RequeueAfter).After(nextScheduledTime) {
			result.RequeueAfter = nextScheduledTime.Sub(now)
		}
	} else {
		scheduledTime = time.Now()
	}

	orchestratingSLOStatus := orchestratingSLO.Status.DeepCopy()
	orchestratingSLOStatus.LastScheduleTime = &metav1.Time{Time: scheduledTime}
	err = r.updateStatus(ctx, request, *orchestratingSLOStatus)
	if err != nil {
		return reconcile.Result{}, err
	}

	return result, nil
}

func (r *Reconciler) reconcileOne(request reconcile.Request, orchestratingSLO *unifiedsev1.OrchestratingSLO) (reconcile.Result, error) {
	// - Parse OrchestratingSLOStatus to find the SLOs that do not meet the requirements.
	unhealthySloes, expectSloes, err := parseUnhealthySloes(orchestratingSLO)
	if err != nil {
		klog.Errorf("Failed to parseUnhealthySloes, OrchestratingSLO %v, err: %v", request, err)
		return reconcile.Result{RequeueAfter: defaultRequeueAfter}, err
	}
	if len(unhealthySloes) == 0 {
		klog.V(4).Infof("all Sloes of OrchestratingSLO %v are healthy", request)
		return reconcile.Result{}, nil
	}
	logSloes(request, unhealthySloes, expectSloes)

	// - Get all Pods by selector(with orchestrating-healthy=false label).
	unhealthyPods, err := r.listUnhealthyPods(orchestratingSLO, unhealthySloes)
	if err != nil {
		klog.Errorf("Failed to list unhealthy Pods, OrchestratingSLO %v, err: %v", request, err)
		return reconcile.Result{RequeueAfter: defaultRequeueAfter}, err
	}
	if len(unhealthyPods) == 0 {
		klog.Warningf("OrchestratingSLO %v has unhealthy SLoes but not find unhealthy Pods", request)
		return reconcile.Result{}, nil
	}

	// - Group all Pods by operations
	operationGroups := groupUnhealthyPodsByOperation(orchestratingSLO, unhealthyPods)
	for k, v := range operationGroups {
		klog.V(4).Infof("OrchestratingSLO %v need do operation %s, targetPodsCount: %d", request, k, len(v))
	}

	// - execute Operations
	result, err := r.executeOperations(request, orchestratingSLO, operationGroups)
	if err != nil {
		klog.Errorf("Failed to r.executeOperations, orchestratingSLO %v err: %v", request, err)
	}
	return result, err
}

func (r *Reconciler) satisfiedCronScheduleTime(request reconcile.Request, orchestratingSLO *unifiedsev1.OrchestratingSLO, sched cron.Schedule) (time.Time, *time.Duration) {
	now := time.Now()
	times := getUnmetScheduleTimes(orchestratingSLO, now, sched, r.eventRecorder)
	if len(times) == 0 {
		klog.V(4).Infof("orchestratingSLO: %v has unmet start times", request)
		t := nextScheduledTimeDuration(sched, now)
		return time.Time{}, t
	}

	scheduledTime := times[len(times)-1]
	tooLate := false
	if orchestratingSLO.Spec.Operation != nil {
		scheduleWindow := orchestratingSLO.Spec.Operation.ScheduleWindow
		if scheduleWindow != nil && scheduleWindow.StartingDeadlineSeconds != nil {
			tooLate = scheduledTime.Add(time.Second * time.Duration(*scheduleWindow.StartingDeadlineSeconds)).Before(now)
		}
	}

	if tooLate {
		klog.V(4).Infof("orchestratingSLO: %v missed starting window", request)
		r.eventRecorder.Eventf(orchestratingSLO, v1.EventTypeWarning, "MissSchedule", "Missed scheduled time to start a job: %s", scheduledTime.Format(time.RFC1123Z))
		t := nextScheduledTimeDuration(sched, now)
		return time.Time{}, t
	}
	return scheduledTime, nil
}

func getUnmetScheduleTimes(orchestratingSLO *unifiedsev1.OrchestratingSLO, now time.Time, schedule cron.Schedule, recorder record.EventRecorder) []time.Time {
	var earliestTime time.Time
	if orchestratingSLO.Status.LastScheduleTime != nil {
		earliestTime = orchestratingSLO.Status.LastScheduleTime.Time
	} else {
		earliestTime = orchestratingSLO.ObjectMeta.CreationTimestamp.Time
	}
	if orchestratingSLO.Spec.Operation != nil {
		scheduleWindow := orchestratingSLO.Spec.Operation.ScheduleWindow
		if scheduleWindow != nil && scheduleWindow.StartingDeadlineSeconds != nil {
			// Controller is not going to schedule anything below this point
			schedulingDeadline := now.Add(-time.Second * time.Duration(*scheduleWindow.StartingDeadlineSeconds))

			if schedulingDeadline.After(earliestTime) {
				earliestTime = schedulingDeadline
			}
		}
	}
	if earliestTime.After(now) {
		return []time.Time{}
	}

	// t := schedule.Next(earliestTime)
	// t1 := schedule.Next(t)
	// delta := t1 - t
	// missed := now - earliestTime/delta
	// last missed = earliestTime + delta * (missed - 1)
	var starts []time.Time
	for t := schedule.Next(earliestTime); !t.After(now); t = schedule.Next(t) {
		starts = append(starts, t)
	}
	if len(starts) > 100 {
		recorder.Eventf(orchestratingSLO, v1.EventTypeWarning, "TooManyMissedTimes", "too many missed start times: %d. Set or decrease .spec.operation.scheduleWindow.startingDeadlineSeconds or check clock skew", len(starts))
		klog.Infof("orchestratingSLO: %s/%s has too many missed times %d", orchestratingSLO.Namespace, orchestratingSLO.Name, len(starts))
	}
	return starts
}

func nextScheduledTimeDuration(sched cron.Schedule, now time.Time) *time.Duration {
	t := sched.Next(now).Add(nextScheduleDelta).Sub(now)
	return &t
}

func (r *Reconciler) updateStatus(ctx context.Context, request reconcile.Request, orchestratingSLOStatus unifiedsev1.OrchestratingSLOStatus) error {
	klog.V(2).Infof("Update OrchestratingSLO %v status", request)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		orchestratingSLO := &unifiedsev1.OrchestratingSLO{}
		err := r.Client.Get(ctx, request.NamespacedName, orchestratingSLO)
		if err != nil {
			klog.Errorf("Failed to Get orchestratingSLO %v, err: %v", request, err)
			return err
		}
		orchestratingSLO.Status = orchestratingSLOStatus
		err = r.Client.Status().Update(ctx, orchestratingSLO)
		if err != nil {
			klog.Errorf("Failed to Update orchestratingSLO %v, err: %v", request, err)
		}
		return err
	})
	if err != nil {
		klog.Errorf("Failed to Update orchestratingSLO %v, err: %v", request, err)
	} else {
		klog.V(2).Infof("Successfully update orchestratingSLO %v", request)
	}
	return err
}

func logSloes(request reconcile.Request, unhealthySloes []unifiedsev1.OrchestratingSLODefinitionStatus, expectSloes map[unifiedsev1.OrchestratingSLOType]float64) {
	for sloType, slo := range expectSloes {
		sli := 100.0
		for _, sloDefinitionStatus := range unhealthySloes {
			if sloDefinitionStatus.Type == sloType {
				sli, _ = parseSLOValue(sloDefinitionStatus.Value)
				break
			}
		}
		klog.V(4).Infof("OrchestratingSLO %v has problems, SLOType: %v expectSLO: %v%%, currentSLI: %v%%", request, sloType, slo, sli)
	}
}

func parseSLOValue(val string) (float64, error) {
	val = strings.Replace(val, "%", "", -1)
	return strconv.ParseFloat(val, 64)
}

func parseUnhealthySloes(orchestratingSLO *unifiedsev1.OrchestratingSLO) ([]unifiedsev1.OrchestratingSLODefinitionStatus, map[unifiedsev1.OrchestratingSLOType]float64, error) {
	if len(orchestratingSLO.Status.Sloes) == 0 {
		return nil, nil, nil
	}
	var unhealthySloes []unifiedsev1.OrchestratingSLODefinitionStatus
	expectSloes := make(map[unifiedsev1.OrchestratingSLOType]float64)
	for _, sloDefinition := range orchestratingSLO.Spec.Sloes {
		slo, err := parseSLOValue(sloDefinition.Value)
		if err == nil {
			expectSloes[sloDefinition.Type] = slo
		}
	}
	for _, sloDefinitionStatus := range orchestratingSLO.Status.Sloes {
		if sloDefinitionStatus.UnhealthyReplicas == 0 {
			continue
		}

		sli, err := parseSLOValue(sloDefinitionStatus.Value)
		if err != nil {
			return nil, nil, err
		}
		slo := expectSloes[sloDefinitionStatus.Type]
		if slo == 0 || sli >= slo {
			continue
		}
		unhealthySloes = append(unhealthySloes, sloDefinitionStatus)
	}
	return unhealthySloes, expectSloes, nil
}

func (r *Reconciler) listUnhealthyPods(orchestratingSLO *unifiedsev1.OrchestratingSLO, unhealthySloes []unifiedsev1.OrchestratingSLODefinitionStatus) (map[unifiedsev1.OrchestratingSLOType][]*v1.Pod, error) {
	pods, err := r.getRelatedPods(orchestratingSLO, &metav1.LabelSelector{
		MatchLabels: map[string]string{
			LabelOrchestratingSLOHealthy: "false",
		},
	})
	if err != nil {
		klog.Errorf("Failed to getRelatedPods, orchestratingSLO %s/%s, err: %v", orchestratingSLO.Namespace, orchestratingSLO.Name, err)
		return nil, err
	}
	klog.V(4).Infof("orchestratingSLO %s/%s has %d unhealthy Pod(s)", orchestratingSLO.Namespace, orchestratingSLO.Name, len(pods))

	unhealthyPods := map[unifiedsev1.OrchestratingSLOType][]*v1.Pod{}
	for _, pod := range pods {
		if !kubecontroller.IsPodActive(pod) || (!IsRunningAndReady(pod) && !IsWaitOnlinePod(pod)) {
			continue
		}
		val := pod.Annotations[AnnotationOrchestratingSLOProblems]
		if val == "" {
			continue
		}
		var problemCtx ProblemContext
		err = json.Unmarshal([]byte(val), &problemCtx)
		if err != nil {
			klog.Errorf("Failed to unmarshal AnnotationOrchestratingSLOProblems in Pod %s/%s, err: %v", pod.Namespace, pod.Name, err)
			continue
		}
		for _, problem := range problemCtx.Problems {
			sloType := unifiedsev1.OrchestratingSLOType(problem.SLOType)
			satisfied := false
			for _, sloDefinitionStatus := range unhealthySloes {
				if sloType != sloDefinitionStatus.Type {
					continue
				}
				satisfied = true
				break
			}
			if satisfied {
				items := unhealthyPods[sloType]
				items = append(items, pod)
				unhealthyPods[sloType] = items
			}
		}
	}
	return unhealthyPods, nil
}

func (r *Reconciler) getRelatedPods(orchestratingSLO *unifiedsev1.OrchestratingSLO, labelSelector *metav1.LabelSelector) ([]*v1.Pod, error) {
	if orchestratingSLO.Spec.TargetReference != nil {
		targetRef := orchestratingSLO.Spec.TargetReference
		pods, _, err := r.finder.GetPodsForRef(targetRef.APIVersion, targetRef.Kind, targetRef.Name, orchestratingSLO.Namespace, labelSelector, false)
		return pods, err
	} else if labelSelector != nil && orchestratingSLO.Spec.Selector != nil {
		innerLabelSelector := orchestratingSLO.Spec.Selector.DeepCopy()
		for k, v := range labelSelector.MatchLabels {
			if innerLabelSelector.MatchLabels == nil {
				innerLabelSelector.MatchLabels = make(map[string]string)
			}
			innerLabelSelector.MatchLabels[k] = v
		}
		labelSelector = innerLabelSelector
	} else if orchestratingSLO.Spec.Selector != nil {
		labelSelector = orchestratingSLO.Spec.Selector.DeepCopy()
	}

	if labelSelector != nil {
		selector, err := util.GetFastLabelSelector(labelSelector)
		if err != nil {
			return nil, err
		}
		podList := &v1.PodList{}
		listOpts := &client.ListOptions{
			Namespace:     orchestratingSLO.Namespace,
			LabelSelector: selector,
		}
		err = r.Client.List(context.TODO(), podList, listOpts, utilclient.DisableDeepCopy)
		if err != nil {
			return nil, err
		}
		pods := make([]*v1.Pod, 0, len(podList.Items))
		for i := range podList.Items {
			pods = append(pods, &podList.Items[i])
		}
		return pods, nil
	}
	return nil, fmt.Errorf("must specify targetReference or selector in orchestratingSLO")
}

func groupUnhealthyPodsByOperation(
	orchestratingSLO *unifiedsev1.OrchestratingSLO,
	unhealthyPods map[unifiedsev1.OrchestratingSLOType][]*v1.Pod,
) map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod {
	operationPodsFilters := make(map[unifiedsev1.OrchestratingSLOOperationType]sets.String)
	operationGroups := map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod{}
	for sloType, pods := range unhealthyPods {
		operations := getOperations(orchestratingSLO, sloType)
		if len(operations) == 0 {
			continue
		}
		for _, ops := range operations {
			filter := operationPodsFilters[ops]
			if filter == nil {
				filter = sets.NewString()
				operationPodsFilters[ops] = filter
			}
			for _, pod := range pods {
				if filter.Has(string(pod.UID)) {
					continue
				}
				filter.Insert(string(pod.UID))
				operationGroups[ops] = append(operationGroups[ops], pod)
			}
		}
	}
	return operationGroups
}

func getOperations(orchestratingSLO *unifiedsev1.OrchestratingSLO, sloType unifiedsev1.OrchestratingSLOType) []unifiedsev1.OrchestratingSLOOperationType {
	for _, v := range orchestratingSLO.Spec.Sloes {
		if v.Type == sloType {
			return v.Operations
		}
	}
	return nil
}

func (r *Reconciler) cleanRateLimiters(request reconcile.Request) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.rateLimiters, request)
}

func (r *Reconciler) createOrGetRateLimiter(request reconcile.Request, orchestratingSLO *unifiedsev1.OrchestratingSLO, operationType unifiedsev1.OrchestratingSLOOperationType) *RateLimiter {
	var qps float32
	var burst int
	var flowControlPolicy *unifiedsev1.OrchestratingSLOFlowControlPolicy
	switch operationType {
	case unifiedsev1.RebindCPUOrchestratingSLOOperation:
		qps, burst = defaultRebindCPUQPS, defaultRebindCPUBurst
		if orchestratingSLO.Spec.Operation != nil && orchestratingSLO.Spec.Operation.RebindCPU != nil {
			rebindCPUParams := orchestratingSLO.Spec.Operation.RebindCPU
			if rebindCPUParams.FlowControlPolicy != nil {
				flowControlPolicy = rebindCPUParams.FlowControlPolicy
			}
		}
	case unifiedsev1.MigrateOrchestratingSLOOperation:
		qps, burst = defaultMigrateQPS, defaultMigrateBurst
		if orchestratingSLO.Spec.Operation != nil && orchestratingSLO.Spec.Operation.Migrate != nil {
			migrateParams := orchestratingSLO.Spec.Operation.Migrate
			if migrateParams.FlowControlPolicy != nil {
				flowControlPolicy = migrateParams.FlowControlPolicy
			}
		}
	default:
		return nil
	}

	if flowControlPolicy != nil {
		if val, err := strconv.ParseFloat(flowControlPolicy.QPS, 64); err == nil && val > 0 {
			qps = float32(val)
		}
		burst = int(flowControlPolicy.Burst)
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()
	rateLimiters := r.rateLimiters[request]
	if rateLimiters == nil {
		rateLimiters = map[unifiedsev1.OrchestratingSLOOperationType]*RateLimiter{}
		r.rateLimiters[request] = rateLimiters
	}
	rateLimiter := rateLimiters[operationType]
	if rateLimiter == nil || rateLimiter.QPS() != qps || rateLimiter.burst != burst {
		rateLimiter = &RateLimiter{
			RateLimiter: flowcontrol.NewTokenBucketRateLimiter(qps, burst),
			burst:       burst,
		}
		rateLimiters[operationType] = rateLimiter
	}
	return rateLimiter
}

type operationFn func(request reconcile.Request, orchestratingSLO *unifiedsev1.OrchestratingSLO, unhealthyPods []*v1.Pod) (migrated []*v1.Pod, skipped []*v1.Pod, incurable map[types.NamespacedName]string, err error)

func (r *Reconciler) executeOperations(
	request reconcile.Request,
	orchestratingSLO *unifiedsev1.OrchestratingSLO,
	operationGroups map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod,
) (reconcile.Result, error) {
	if len(operationGroups) == 0 {
		return reconcile.Result{}, nil
	}

	operationFns := map[unifiedsev1.OrchestratingSLOOperationType]operationFn{
		unifiedsev1.RebindCPUOrchestratingSLOOperation: r.rebindCPU,
		unifiedsev1.MigrateOrchestratingSLOOperation:   r.migrate,
	}

	for i, operationType := range operationOrders {
		unhealthyPods := operationGroups[operationType]
		if len(unhealthyPods) == 0 {
			continue
		}
		fn, ok := operationFns[operationType]
		if !ok {
			klog.Warningf("OrchestratingSLO %v unsupported operation type: %v", request, operationType)
			continue
		}

		klog.V(4).Infof("OrchestratingSLO %v try to %v", request, operationType)
		processed, skipped, incurable, err := fn(request, orchestratingSLO, unhealthyPods)
		if err != nil {
			klog.Errorf("Failed to do %v for OrchestratingSLO %v, err: %v", request, operationType, err)
			return reconcile.Result{RequeueAfter: defaultRequeueAfter}, err
		}
		klog.V(4).Infof("OrchestratingSLO %v executed %v, processed %d, skipped %d, incurable %d",
			request, operationType, len(processed), len(skipped), len(incurable))

		nextOperations := operationOrders[i+1:]
		removePodsInNextOperationGroups(nextOperations, operationGroups, processed, skipped)
	}
	return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
}

func removePodsInNextOperationGroups(
	operations []unifiedsev1.OrchestratingSLOOperationType,
	operationGroups map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod,
	pods ...[]*v1.Pod,
) {
	for _, ops := range operations {
		unhealthyPods := operationGroups[ops]
		if len(unhealthyPods) == 0 {
			continue
		}

		unhealthyPodMap := make(map[types.UID]*v1.Pod)
		for _, pod := range unhealthyPods {
			unhealthyPodMap[pod.UID] = pod
		}

		removedCount := 0
		for _, skippedPods := range pods {
			for _, v := range skippedPods {
				pod := unhealthyPodMap[v.UID]
				if pod == nil {
					continue
				}
				delete(unhealthyPodMap, v.UID)
				removedCount++
			}
		}

		if len(unhealthyPodMap) == 0 {
			delete(operationGroups, ops)
		}

		if removedCount > 0 {
			unhealthyPods = unhealthyPods[:0]
			for _, v := range unhealthyPodMap {
				unhealthyPods = append(unhealthyPods, v)
			}
			operationGroups[ops] = unhealthyPods
		}
	}
}

func (r *Reconciler) filterNominatedPods(
	request reconcile.Request,
	unhealthyPods []*v1.Pod,
	retryPeriod time.Duration,
	stabilizationWindow time.Duration,
	maxRetryCount int,
	operationType unifiedsev1.OrchestratingSLOOperationType,
) (nominated, skipped []*v1.Pod, incurable map[types.NamespacedName]string, nextRetryCount map[types.UID]int, err error) {
	nextRetryCount = make(map[types.UID]int)
	for _, v := range unhealthyPods {
		pod := &v1.Pod{}
		namespacedName := types.NamespacedName{
			Namespace: v.Namespace,
			Name:      v.Name,
		}
		err = r.Get(context.TODO(), namespacedName, pod)
		if err != nil {
			klog.Errorf("Failed to Get Pod %v for orchestratingSLO %v, err: %v", namespacedName, request, err)
			skipped = append(skipped, pod)
			continue
		}

		if !kubecontroller.IsPodActive(pod) || (!IsRunningAndReady(pod) && !IsWaitOnlinePod(pod)) {
			klog.V(4).Infof("Pod %v is unavailable, so skip it, orchestratingSLO %v", namespacedName, request)
			skipped = append(skipped, pod)
			continue
		}

		var operationStatus SLOOperationStatus
		operationStatus, err = r.getPodOperationStatus(pod)
		if err != nil {
			return
		}
		if operationStatus == SLOOperationStatusPending {
			klog.V(4).Infof("Pod %v is SLO Operation Pending status, orchestratingSLO %v", namespacedName, request)
			skipped = append(skipped, pod)
			continue
		}

		var operationCtx *OperationContext
		operationCtx, err = GetOperationContext(pod, operationType)
		if err != nil {
			klog.Errorf("Failed to GetOperationContext, Pod %v, err: %v", namespacedName, err)
			skipped = append(skipped, pod)
			continue
		}

		if operationCtx.LastUpdateTime != nil && time.Since(operationCtx.LastUpdateTime.Time) < retryPeriod {
			klog.V(4).Infof("Pod %v do not need to %v caused by be initiated within the last %s, lastUpdateTime: %v, orchestratingSLO %v",
				namespacedName, operationType, retryPeriod, operationCtx.LastUpdateTime, request)
			skipped = append(skipped, pod)
			continue
		}

		retryCounts := operationCtx.Count
		if retryCounts >= maxRetryCount {
			if operationCtx.LastUpdateTime != nil && time.Since(operationCtx.LastUpdateTime.Time) < stabilizationWindow {
				if incurable == nil {
					incurable = make(map[types.NamespacedName]string)
				}
				incurable[namespacedName] = fmt.Sprintf("the Pod has try to %v %d counts, it's incurable in stabilizationWindow(%v)",
					operationType, retryCounts, stabilizationWindow)
				klog.V(3).Infof("Pod %v has try to %v %d counts(max %d), it's incurable in stabilizationWindow(%v), orchestratingSLO %v",
					namespacedName, operationType, retryCounts, maxRetryCount, stabilizationWindow, request)
				continue
			}
			nextRetryCount[pod.UID] = 1
		} else {
			nextRetryCount[pod.UID] = retryCounts + 1
		}
		nominated = append(nominated, pod)
	}
	return
}

func (r *Reconciler) rebindCPU(request reconcile.Request, orchestratingSLO *unifiedsev1.OrchestratingSLO, unhealthyPods []*v1.Pod) (rebound []*v1.Pod, skipped []*v1.Pod, incurable map[types.NamespacedName]string, err error) {
	rateLimiter := r.createOrGetRateLimiter(request, orchestratingSLO, unifiedsev1.RebindCPUOrchestratingSLOOperation)
	param := NewRebindCPUParam(orchestratingSLO, rateLimiter)

	var nominatedPods []*v1.Pod
	var nextRetryCounts map[types.UID]int
	nominatedPods, skipped, incurable, nextRetryCounts, err = r.filterNominatedPods(request, unhealthyPods, param.RetryPeriod, param.StabilizationWindow, param.MaxRetryCount, unifiedsev1.RebindCPUOrchestratingSLOOperation)
	if len(nominatedPods) == 0 {
		return
	}

	nodesToPods := make(map[string][]*v1.Pod)
	for _, v := range nominatedPods {
		nodesToPods[v.Spec.NodeName] = append(nodesToPods[v.Spec.NodeName], v)
	}

	processedNodes := sets.NewString()
rebindLoop:
	for nodeName, pods := range nodesToPods {
		if !param.RateLimiter.TryAccept() {
			klog.Warningf("stopped rebindCPU caused by rate limiter, node %s, orchestratingSLo %v", nodeName, request)
			for nodeName, pods := range nodesToPods {
				if !processedNodes.Has(nodeName) {
					skipped = append(skipped, pods...)
				}
			}
			return
		}

		for _, v := range pods {
			err = r.updateOperationContext(v, unifiedsev1.RebindCPUOrchestratingSLOOperation, nextRetryCounts[v.UID])
			if err != nil {
				klog.Errorf("Failed to updateOperationContext with RebindCPU, Pod %s/%s, err: %v", v.Namespace, v.Name, err)
				continue rebindLoop
			}
			klog.V(4).Infof("Successfully updateOperationContext with RebindCPU, Pod %s/%s", v.Namespace, v.Name)
		}

		if param.DryRun {
			klog.V(4).Infof("OrchestratingSLo %v skip rebindCPU on node %s caused by dryRun mode", request, nodeName)
			r.eventRecorder.Eventf(orchestratingSLO, v1.EventTypeNormal, "RebindCPU", "Skip rebindCPU on node %q caused by dryRun mode", nodeName)
			rebound = append(rebound, pods...)
			processedNodes.Insert(nodeName)
			continue
		}

		err = r.doRebindCPU(orchestratingSLO, nodeName)
		if err != nil {
			klog.Errorf("Failed to doRebindCPU with Node %s, err: %v", nodeName, err)
			return
		}
		rebound = append(rebound, pods...)
		processedNodes.Insert(nodeName)
	}
	return
}

func (r *Reconciler) doRebindCPU(orchestratingSLO *unifiedsev1.OrchestratingSLO, nodeName string) error {
	rebindResourceCtx := &extension.RebindResourceContext{
		ID:      string(migration.UUIDGenerateFn()),
		Reason:  "Violating the SLO of CPU Orchestration",
		Trigger: "descheduler",
	}
	rebindResourceCtxBytes, err := json.Marshal(rebindResourceCtx)
	if err != nil {
		return err
	}

	deadlineSeconds := int64(180)
	rebindResource := &unifiedsev1.RebindResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: string(migration.UUIDGenerateFn()),
			Annotations: map[string]string{
				extension.AnnotationRebindResourceContext: string(rebindResourceCtxBytes),
			},
			Labels: map[string]string{
				"sigma.ali/node-sn": nodeName,
			},
		},
		Spec: unifiedsev1.RebindResourceSpec{
			NodeName:              nodeName,
			RebindAction:          unifiedsev1.RebindCPU,
			ActiveDeadlineSeconds: &deadlineSeconds,
		},
	}
	err = retry.OnError(retry.DefaultBackoff, k8serrors.IsTooManyRequests, func() error {
		err = r.Client.Create(context.TODO(), rebindResource)
		if err != nil {
			klog.Errorf("Failed to Create RebindResource %s with Node %s, err: %v", rebindResource.Name, nodeName, err)
		}
		return err
	})
	if err != nil {
		klog.Errorf("Failed to create rebindResource %s for node %s, err: %v", rebindResource.Name, nodeName, err)
		r.eventRecorder.Eventf(orchestratingSLO, v1.EventTypeWarning, "RebindCPU", "Failed to create RebindResource for node %q caused by %v", nodeName, err)

	} else {
		klog.V(4).Infof("Successfully to create rebindResource %s for node %s, err: %v", rebindResource.Name, nodeName, err)
		r.eventRecorder.Eventf(orchestratingSLO, v1.EventTypeNormal, "RebindCPU", "Start rebinding node %q, rebindResource %q", nodeName, rebindResource.Name)
	}
	return err
}

func (r *Reconciler) migrate(request reconcile.Request, orchestratingSLO *unifiedsev1.OrchestratingSLO, unhealthyPods []*v1.Pod) (migrated []*v1.Pod, skipped []*v1.Pod, incurable map[types.NamespacedName]string, err error) {
	rateLimiter := r.createOrGetRateLimiter(request, orchestratingSLO, unifiedsev1.MigrateOrchestratingSLOOperation)
	param := NewMigrateParam(orchestratingSLO, rateLimiter)

	var nominatedPods []*v1.Pod
	var nextRetryCounts map[types.UID]int
	nominatedPods, skipped, incurable, nextRetryCounts, err = r.filterNominatedPods(request, unhealthyPods, param.RetryPeriod, param.StabilizationWindow, param.MaxRetryCount, unifiedsev1.MigrateOrchestratingSLOOperation)
	if len(nominatedPods) == 0 {
		return
	}

	var totalReplicas int
	var unavailablePods map[types.NamespacedName]struct{}
	totalReplicas, unavailablePods, err = r.getUnavailablePods(orchestratingSLO)
	if err != nil {
		return
	}
	if totalReplicas == 0 {
		klog.V(4).Infof("OrchestratingSLO %v has no replicas, skip it", request)
		return
	}

	var migratingPods map[types.NamespacedName]struct{}
	migratingPods, err = r.getMigratingPods(map[string]string{LabelOrchestratingSLOName: orchestratingSLO.Name})
	if err != nil {
		return
	}

	exceed := false
	maxMigrating := 0
	exceed, maxMigrating, err = r.exceedMaxMigratingReplicas(totalReplicas, len(migratingPods), param)
	if err != nil {
		return
	}
	if exceed {
		klog.Errorf("OrchestratingSLO %v has %d migrateJobs exceeded maxMigrating %d", request, len(migratingPods), maxMigrating)
		skipped = append(skipped, nominatedPods...)
		return
	}

	mergeUnavailableAndMigratingPods(unavailablePods, migratingPods)
	maxUnavailable := 0
	exceed, maxUnavailable, err = r.exceedMaxUnavailableReplicas(totalReplicas, len(unavailablePods), param)
	if err != nil {
		return
	}
	if exceed {
		klog.Errorf("OrchestratingSLO %v has %d unavailable Pods exceeded maxUnavailable %d", request, len(unavailablePods), maxUnavailable)
		skipped = append(skipped, nominatedPods...)
		return
	}

	if maxUnavailable > maxMigrating {
		maxUnavailable = maxMigrating
	}
	allowedMigrateReplicas := maxUnavailable - len(unavailablePods)

	for i, pod := range nominatedPods {
		if len(migrated) >= allowedMigrateReplicas {
			klog.Infof("Migration stopped early because the replicas allowed to migrate was exceeded, maxUnavailable: %d, unavailablePods: %d, orchestratingSLo %v", maxUnavailable, len(unavailablePods), request)
			skipped = append(skipped, nominatedPods[i:]...)
			return
		}
		podNamespacedName := types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		}
		_, migrating := migratingPods[podNamespacedName]
		if migrating {
			klog.V(4).Infof("Pod %v is migrating, orchestratingSLO: %v", podNamespacedName, request)
			migrated = append(migrated, pod)
			continue
		}

		exceed, err = r.exceedMaxMigratingPerNode(pod.Spec.NodeName, param.MaxMigratingPerNode)
		if exceed {
			klog.Warningf("skipped migrate caused by exceed MaxMigratingPerNode(%d), Pod %v, orchestratingSLO %v",
				param.MaxMigratingPerNode, podNamespacedName, request)
			skipped = append(skipped, pod)
			continue
		}

		if !param.RateLimiter.TryAccept() {
			klog.Warningf("stopped migrate caused by rate limiter, Pod %v, orchestratingSLO %v", podNamespacedName, request)
			skipped = append(skipped, nominatedPods[i:]...)
			return
		}

		err = r.updateOperationContext(pod, unifiedsev1.MigrateOrchestratingSLOOperation, nextRetryCounts[pod.UID])
		if err != nil {
			klog.Errorf("Failed to updateOperationContext with Migrate, Pod %v, orchestratingSLO: %v, err: %v", podNamespacedName, request, err)
			continue
		}
		klog.V(4).Infof("Successfully updateOperationContext with Migrate, Pod %v, orchestratingSLO: %v", podNamespacedName, request)

		if param.DryRun {
			klog.V(4).Infof("OrchestratingSLo %v skip migrate for Pod %v caused by dryRun mode", request, podNamespacedName)
			r.eventRecorder.Eventf(orchestratingSLO, v1.EventTypeNormal, "MigratePod", "Skip migrate for Pod %q caused by dryRun mode", podNamespacedName)
			migrated = append(migrated, pod)
			continue
		}

		if !r.doMigrate(orchestratingSLO, pod, param) {
			klog.Errorf("Failed to doMigrate with Pod %v", podNamespacedName)
			return
		}
		migrated = append(migrated, pod)
	}
	return
}

func (r *Reconciler) getMigratingPods(selectLabels map[string]string) (map[types.NamespacedName]struct{}, error) {
	jobList := &koordsev1alpha1.PodMigrationJobList{}
	listOpts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(selectLabels),
	}
	err := r.Client.List(context.TODO(), jobList, listOpts)
	if err != nil {
		return nil, err
	}

	pods := map[types.NamespacedName]struct{}{}
	for i := range jobList.Items {
		job := &jobList.Items[i]
		if job.Status.Phase == koordsev1alpha1.PodMigrationJobPending || job.Status.Phase == koordsev1alpha1.PodMigrationJobRunning {
			if job.Spec.PodRef != nil && job.Spec.PodRef.Namespace != "" && job.Spec.PodRef.Name != "" {
				k := types.NamespacedName{
					Namespace: job.Spec.PodRef.Namespace,
					Name:      job.Spec.PodRef.Name,
				}
				pods[k] = struct{}{}
			}
		}
	}
	return pods, nil
}

func (r *Reconciler) getUnavailablePods(orchestratingSLO *unifiedsev1.OrchestratingSLO) (int, map[types.NamespacedName]struct{}, error) {
	pods, err := r.getRelatedPods(orchestratingSLO, nil)
	if err != nil {
		klog.Errorf("Failed to getRelatedPods, orchestratingSLO %s/%s, err: %v", orchestratingSLO.Namespace, orchestratingSLO.Name, err)
		return 0, nil, err
	}

	unavailablePods := make(map[types.NamespacedName]struct{})
	for _, pod := range pods {
		if kubecontroller.IsPodActive(pod) &&
			(IsRunningAndReady(pod) || IsWaitOnlinePod(pod)) &&
			!unified.HasEvictionLabel(pod) {
			continue
		}
		k := types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		}
		unavailablePods[k] = struct{}{}
	}
	return len(pods), unavailablePods, nil
}

func mergeUnavailableAndMigratingPods(unavailablePods, migratingPods map[types.NamespacedName]struct{}) {
	for k, v := range migratingPods {
		unavailablePods[k] = v
	}
}

func (r *Reconciler) exceedMaxMigratingReplicas(totalReplicas int, migratingReplicas int, param *MigrateParam) (bool, int, error) {
	maxMigratingCount, err := GetMaxMigrating(totalReplicas, param.MaxMigrating)
	if err != nil {
		return false, 0, err
	}
	if maxMigratingCount <= 0 {
		return true, 0, nil // don't allow unset MaxMigrating
	}
	// TODO(joseph.lt): validate maxMigrateCount >= totalReplicas
	exceeded := false
	if migratingReplicas >= maxMigratingCount {
		exceeded = true
	}
	return exceeded, maxMigratingCount, nil
}

func (r *Reconciler) exceedMaxUnavailableReplicas(totalReplicas, unavailableReplicas int, param *MigrateParam) (bool, int, error) {
	maxUnavailable, err := GetMaxUnavailable(totalReplicas, param.MaxUnavailable)
	if err != nil {
		return false, 0, err
	}
	if maxUnavailable <= 0 {
		return true, 0, nil // don't allow unset MaxMigrating
	}
	exceeded := false
	if unavailableReplicas >= maxUnavailable {
		exceeded = true
	}
	return exceeded, maxUnavailable, nil
}

func (r *Reconciler) exceedMaxMigratingPerNode(nodeName string, maxMigratingPerNode int) (bool, error) {
	migratingPods, err := r.getMigratingPods(map[string]string{
		unified.LabelPodMigrateFromNode: nodeName,
	})
	if err != nil {
		return false, err
	}
	return len(migratingPods) >= maxMigratingPerNode, nil
}

func (r *Reconciler) doMigrate(orchestratingSLO *unifiedsev1.OrchestratingSLO, pod *v1.Pod, param *MigrateParam) bool {
	ticketID := string(migration.UUIDGenerateFn())
	reason := "Violating the SLO of Orchestration"

	jobCtx := &migration.JobContext{
		Labels: map[string]string{
			unified.LabelPodMigrateFromNode: pod.Spec.NodeName,
			unified.LabelPodMigrateTicket:   ticketID,
			unified.LabelPodMigrateTrigger:  "descheduler",
			LabelOrchestratingSLONamespace:  orchestratingSLO.Namespace,
			LabelOrchestratingSLOName:       orchestratingSLO.Name,
		},
		Annotations: map[string]string{
			unified.AnnotationPodMigrateReason: reason,
		},
		Timeout: &param.Timeout,
	}
	if param.Defragmentation && param.NeedReserveResource {
		jobCtx.Labels[unified.LabelEnableMigrate] = "true"
		jobCtx.Labels[unified.LabelMigrationConfirmState] = string(unifiedsev1.ReserveResourceConfirmStateConfirmed)
	}
	var evictPolicy string
	switch param.EvictType {
	case unifiedsev1.PodMigrateNativeEvict:
		evictPolicy = evictor.NativeEvictorName
	case unifiedsev1.PodMigrateEvictJustDelete:
		evictPolicy = evictor.DeleteEvictorName
	case unifiedsev1.PodMigrateEvictByLabel:
		evictPolicy = unified.EvictorName
	}
	if evictPolicy != "" {
		jobCtx.Labels[evictor.LabelEvictPolicy] = evictPolicy
	}
	ctx := migration.WithContext(context.Background(), jobCtx)

	evictSucceeded := r.handle.Evictor().Evict(ctx, pod, framework.EvictOptions{
		PluginName: Name,
		Reason:     reason,
	})
	if evictSucceeded {
		r.eventRecorder.Eventf(orchestratingSLO, v1.EventTypeNormal, "MigratePod", "Start migrating Pod %s/%s", pod.Namespace, pod.Name)
	} else {
		r.eventRecorder.Eventf(orchestratingSLO, v1.EventTypeWarning, "FailedMigratePod", "Failed to create PodMigrateJob for Pod %s/%s", pod.Namespace, pod.Name)
	}
	return evictSucceeded
}

func (r *Reconciler) getPodOperationStatus(pod *v1.Pod) (SLOOperationStatus, error) {
	name, err := cache.MetaNamespaceKeyFunc(pod)
	if err != nil {
		return "", err
	}

	pod, err = r.podCache.GetPod(name)
	if err != nil {
		return "", err
	}
	return ParseSLOOperationStatus(pod.Labels[LabelOrchestratingSLOOperationStatus]), nil
}

func (r *Reconciler) updateOperationContext(pod *v1.Pod, operation unifiedsev1.OrchestratingSLOOperationType, count int) error {
	podClone := pod.DeepCopy()
	if podClone.Labels == nil {
		podClone.Labels = make(map[string]string)
	}
	podClone.Labels[LabelOrchestratingSLOOperationStatus] = string(SLOOperationStatusPending)
	if err := r.podCache.Assume(podClone); err != nil {
		return err
	}

	err := retryDo(func() error {
		err := r.doPatchOperationContext(pod.Namespace, pod.Name, operation, count)
		if err != nil {
			klog.Errorf("Failed to updateOperationContext, Pod %s/%s, err: %v", pod.Namespace, pod.Name, err)
		}
		return err
	})
	if err != nil {
		name, err := cache.MetaNamespaceKeyFunc(podClone)
		if err != nil {
			return err
		}
		r.podCache.Restore(name)
	}
	return err
}

func (r *Reconciler) doPatchOperationContext(namespace, name string, operation unifiedsev1.OrchestratingSLOOperationType, count int) error {
	pod := &v1.Pod{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, pod)
	if err != nil {
		return err
	}

	ctx := &OperationContext{
		LastUpdateTime: &metav1.Time{Time: time.Now()},
		Operation:      string(operation),
		Count:          count,
	}
	contexts, err := UpdateOperationContexts(pod, ctx)
	if err != nil {
		return err
	}
	data, err := json.Marshal(contexts)
	if err != nil {
		klog.Errorf("Failed to Marshal operationContext, err: %v", err)
		return err
	}
	patch := unified.NewStrategicPatch()
	patch.InsertLabel(LabelOrchestratingSLOOperationStatus, string(SLOOperationStatusRunning))
	patch.InsertAnnotation(AnnotationOrchestratingSLOOperationContext, string(data))
	return r.Client.Patch(context.Background(), pod, patch, &client.PatchOptions{})
}

func retryDo(fn func() error) error {
	return retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return k8serrors.IsConflict(err) || k8serrors.IsTooManyRequests(err)
	}, fn)
}
