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
	"encoding/json"
	"math"
	"strconv"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	corev1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/controller/unified/resourcesummary/metrics"
	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/util"
	utilclient "github.com/koordinator-sh/koordinator/pkg/util/client"
	utilfeature "github.com/koordinator-sh/koordinator/pkg/util/feature"
)

const (
	AnnotationDryRunStatus = extension.DomainPrefix + "dry-run-status"
)

var (
	priorityClassTypes = []uniext.PriorityClass{
		uniext.PriorityProd, uniext.PriorityMid, uniext.PriorityBatch, uniext.PriorityFree,
	}
)

// Reconciler reconciles a ResourceSummary object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=scheduling.alibabacloud.com,resources=resourcesummaries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.alibabacloud.com,resources=resourcesummaries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.alibabacloud.com,resources=resourcesummaries/finalizers,verbs=update

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var resourceSummary *v1beta1.ResourceSummary
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		resourceSummary = &v1beta1.ResourceSummary{}
		if err := r.Client.Get(ctx, req.NamespacedName, resourceSummary); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}

		status := r.getCurrentStatus(ctx, resourceSummary)
		if utilfeature.DefaultFeatureGate.Enabled(features.ResourceSummaryReportDryRun) {
			statusJSONBytes, err := json.Marshal(status)
			if err != nil {
				return err
			}
			if resourceSummary.Annotations == nil {
				resourceSummary.Annotations = map[string]string{}
			}
			resourceSummary.Annotations[AnnotationDryRunStatus] = string(statusJSONBytes)
			recordStatusToMetric(resourceSummary.Name, false, &resourceSummary.Status)
			recordStatusToMetric(resourceSummary.Name, true, status)
			recordDryRunDiff(resourceSummary.Name, &resourceSummary.Status, status)
			return r.Client.Update(ctx, resourceSummary)
		} else {
			resourceSummary.Status = *status
			recordStatusToMetric(resourceSummary.Name, false, status)
			return r.Client.Status().Update(ctx, resourceSummary)
		}
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if resourceSummary != nil && resourceSummary.Spec.UpdatePeriod != nil && resourceSummary.Spec.UpdatePeriod.Minutes() >= 1 {
		return ctrl.Result{RequeueAfter: resourceSummary.Spec.UpdatePeriod.Duration}, nil
	}
	return ctrl.Result{}, nil
}

func recordStatusToMetric(resourceSummaryName string, isDryRun bool, status *v1beta1.ResourceSummaryStatus) {
	metrics.ResourceSummaryPhase.WithLabelValues(resourceSummaryName, strconv.FormatBool(isDryRun), string(status.Phase)).Set(1)
	metrics.ResourceSummaryNumNodes.WithLabelValues(resourceSummaryName, strconv.FormatBool(isDryRun)).Set(float64(status.NumNodes))
	for _, nodeResourceSummary := range status.Resources {
		for resourceName, quantity := range nodeResourceSummary.Capacity {
			metrics.ResourceSummaryResource.WithLabelValues(resourceSummaryName, strconv.FormatBool(isDryRun),
				string(nodeResourceSummary.PriorityClass), "capacity", string(resourceName)).Set(float64(quantity.MilliValue()))
		}
		for resourceName, quantity := range nodeResourceSummary.Allocated {
			metrics.ResourceSummaryResource.WithLabelValues(resourceSummaryName, strconv.FormatBool(isDryRun),
				string(nodeResourceSummary.PriorityClass), "allocated", string(resourceName)).Set(float64(quantity.MilliValue()))
		}
		for resourceName, quantity := range nodeResourceSummary.Allocatable {
			metrics.ResourceSummaryResource.WithLabelValues(resourceSummaryName, strconv.FormatBool(isDryRun),
				string(nodeResourceSummary.PriorityClass), "allocatable", string(resourceName)).Set(float64(quantity.MilliValue()))
		}
	}
	for _, resourceSpec := range status.ResourceSpecStats {
		for _, allocatable := range resourceSpec.Allocatable {
			metrics.ResourceSummaryResourceSpec.WithLabelValues(resourceSummaryName, strconv.FormatBool(isDryRun), resourceSpec.Name, string(allocatable.PriorityClass)).Set(float64(allocatable.Count))
		}
	}
	for _, podUsed := range status.PodUsedStatistics {
		for _, podPriorityUsed := range podUsed.Allocated {
			for resourceName, quantity := range podPriorityUsed.Allocated {
				metrics.ResourceSummaryPodUsed.WithLabelValues(resourceSummaryName, strconv.FormatBool(isDryRun), podUsed.Name, string(podPriorityUsed.PriorityClass), string(resourceName)).Set(float64(quantity.MilliValue()))
			}
		}
	}
}

func recordDryRunDiff(resourceSummaryName string, notDryRunStatus, dryRunStatus *v1beta1.ResourceSummaryStatus) {
	for _, notDryRunResource := range notDryRunStatus.Resources {
		dryRunResource := notDryRunResource
		for _, candidateDryRunResource := range dryRunStatus.Resources {
			if notDryRunResource.PriorityClass == candidateDryRunResource.PriorityClass {
				dryRunResource = candidateDryRunResource
				break
			}
		}
		if dryRunResource == notDryRunResource {
			metrics.ResourceSummaryResource.WithLabelValues(resourceSummaryName, "diff",
				string(notDryRunResource.PriorityClass), "", "").Set(-1.0)
		}
		for resourceName, quantity := range notDryRunResource.Capacity {
			dryRunQuantity, ok := dryRunResource.Capacity[resourceName]
			diff := -1.0
			if ok {
				diff = float64(dryRunQuantity.MilliValue() - quantity.MilliValue())
			}
			metrics.ResourceSummaryResource.WithLabelValues(resourceSummaryName, "diff",
				string(notDryRunResource.PriorityClass), "capacity", string(resourceName)).Set(diff)
		}
		for resourceName, quantity := range notDryRunResource.Allocated {
			dryRunQuantity, ok := dryRunResource.Allocated[resourceName]
			diff := -1.0
			if ok {
				diff = float64(dryRunQuantity.MilliValue() - quantity.MilliValue())
			}
			metrics.ResourceSummaryResource.WithLabelValues(resourceSummaryName, "diff",
				string(notDryRunResource.PriorityClass), "allocated", string(resourceName)).Set(diff)
		}
		for resourceName, quantity := range notDryRunResource.Allocatable {
			dryRunQuantity, ok := dryRunResource.Allocatable[resourceName]
			diff := -1.0
			if ok {
				diff = float64(dryRunQuantity.MilliValue() - quantity.MilliValue())
			}
			metrics.ResourceSummaryResource.WithLabelValues(resourceSummaryName, "diff",
				string(notDryRunResource.PriorityClass), "allocatable", string(resourceName)).Set(diff)
		}
	}
}

func (r *Reconciler) getCurrentStatus(ctx context.Context, resourceSummary *v1beta1.ResourceSummary,
) *v1beta1.ResourceSummaryStatus {
	candidateNodes, err := r.selectCandidateNodes(ctx, resourceSummary)
	if err != nil {
		klog.Errorf("[ResourceSummary] Failed to selectCandidateNodes, err:%v", err)
		return &v1beta1.ResourceSummaryStatus{
			Phase:           v1beta1.ResourceSummaryFailed,
			Message:         err.Error(),
			Reason:          "FailedFilter",
			UpdateTimestamp: metav1.Now(),
			Resources:       nil,
		}
	}
	nodeOwnedPods, err := r.getPodForCandidateNodes(ctx, candidateNodes)
	if err != nil {
		klog.Errorf("[ResourceSummary] Failed to getPodForCandidateNodes, err:%v", err)
		return &v1beta1.ResourceSummaryStatus{
			Phase:           v1beta1.ResourceSummaryFailed,
			Message:         err.Error(),
			Reason:          "FailedStatistics",
			UpdateTimestamp: metav1.Now(),
			Resources:       nil,
		}
	}
	nodeGPUCapacity, err := r.getGPUCapacityForCandidateNodes(ctx, candidateNodes)
	if err != nil {
		klog.Errorf("[ResourceSummary] Failed to getGPUCapacityForCandidateNodes, err:%v", err)
		return &v1beta1.ResourceSummaryStatus{
			Phase:           v1beta1.ResourceSummaryFailed,
			Message:         err.Error(),
			Reason:          "FailedStatistics",
			UpdateTimestamp: metav1.Now(),
			Resources:       nil,
		}
	}
	nodeOwnedReservations, err := r.getReservationForCandidateNodes(ctx, candidateNodes)
	if err != nil {
		klog.Errorf("[ResourceSummary] getReservationForCandidateNodes, err:%v", err)
		return &v1beta1.ResourceSummaryStatus{
			Phase:           v1beta1.ResourceSummaryFailed,
			Message:         err.Error(),
			Reason:          "FailedStatistics",
			UpdateTimestamp: metav1.Now(),
			Resources:       nil,
		}
	}
	podUsedResource, err := statisticsPodUsedResource(candidateNodes, nodeOwnedPods, resourceSummary.Spec.PodStatistics, nodeGPUCapacity)
	if err != nil {
		return &v1beta1.ResourceSummaryStatus{
			Phase:             v1beta1.ResourceSummaryFailed,
			Message:           err.Error(),
			Reason:            "FailedStatisticsPodUsed",
			UpdateTimestamp:   metav1.Now(),
			Resources:         nil,
			ResourceSpecStats: nil,
		}
	}
	podUsedStatistics := convertToPodUsedStatistics(resourceSummary.Spec.PodStatistics, podUsedResource)
	capacity, requested, free, reservationCapacity, reservationRequested, reservationFree, allocatablePodNums := statisticsNodeRelated(candidateNodes, nodeOwnedPods, nodeOwnedReservations, resourceSummary.Spec.ResourceSpecs, nodeGPUCapacity)
	nodeResourceSummary := convertToNodeResourceSummary(capacity, requested, free, reservationCapacity, reservationRequested, reservationFree)
	resourceSpecStats := convertToResourceSpecStats(allocatablePodNums)
	return &v1beta1.ResourceSummaryStatus{
		Phase:             v1beta1.ResourceSummarySucceeded,
		UpdateTimestamp:   metav1.Now(),
		NumNodes:          int32(len(candidateNodes.Items)),
		Resources:         nodeResourceSummary,
		ResourceSpecStats: resourceSpecStats,
		PodUsedStatistics: podUsedStatistics,
	}

}

func (r *Reconciler) selectCandidateNodes(ctx context.Context, resourceSummary *v1beta1.ResourceSummary,
) (*corev1.NodeList, error) {
	allNodes := &corev1.NodeList{}
	if err := r.Client.List(ctx, allNodes); err != nil {
		return nil, err
	}
	if len(allNodes.Items) == 0 {
		return allNodes, nil
	}

	candidateNodes := &corev1.NodeList{}
	affinity := GetRequiredNodeAffinity(resourceSummary)
	for _, node := range allNodes.Items {
		if !IsNodeReady(node) {
			continue
		}
		match, err := affinity.Match(&node)
		if err != nil {
			return nil, err
		}
		if !match {
			continue
		}
		isAllTolerated, _ := corev1helper.GetMatchingTolerations(node.Spec.Taints, resourceSummary.Spec.Tolerations)
		if !isAllTolerated {
			continue
		}
		candidateNodes.Items = append(candidateNodes.Items, node)
	}
	return candidateNodes, nil
}

func (r *Reconciler) getPodForCandidateNodes(ctx context.Context, candidateNodes *corev1.NodeList,
) (map[string][]*corev1.Pod, error) {
	nodeOwnedPods := map[string][]*corev1.Pod{}
	for _, node := range candidateNodes.Items {
		podList := &corev1.PodList{}
		if err := r.Client.List(ctx, podList, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("spec.nodeName", node.Name),
		}, utilclient.DisableDeepCopy); err != nil {
			return nil, err
		}
		for i := range podList.Items {
			if util.IsPodTerminated(&podList.Items[i]) {
				continue
			}
			nodeOwnedPods[node.Name] = append(nodeOwnedPods[node.Name], &podList.Items[i])
		}
	}
	return nodeOwnedPods, nil
}

func statisticsPodUsedResource(candidateNodes *corev1.NodeList,
	nodeOwnedPods map[string][]*corev1.Pod, podStatistics []v1beta1.PodStatistics, nodeGPUCapacity map[string]corev1.ResourceList) ([]map[uniext.PriorityClass]corev1.ResourceList, error) {
	var rawPodUsed []map[uniext.PriorityClass]corev1.ResourceList
	n := len(podStatistics)
	if n == 0 {
		return rawPodUsed, nil
	}
	podSelectors := make([]labels.Selector, n)
	for i := 0; i < n; i++ {
		podSelector, err := metav1.LabelSelectorAsSelector(podStatistics[i].Selector)
		if err != nil {
			klog.Errorf("Fail to convert label selector: %v", err)
			return rawPodUsed, err
		}
		podSelectors[i] = podSelector
	}
	for i := 0; i < n; i++ {
		rawPodUsed = append(rawPodUsed, map[uniext.PriorityClass]corev1.ResourceList{})
	}
	for _, node := range candidateNodes.Items {
		ownedPods := nodeOwnedPods[node.Name]
		gpuCapacity := nodeGPUCapacity[node.Name]
		for i := 0; i < n; i++ {
			for _, ownedPod := range ownedPods {
				if !podSelectors[i].Matches(labels.Set(ownedPod.Labels)) {
					continue
				}
				podPriorityUsed := GetPodPriorityUsed(ownedPod, &node, gpuCapacity)
				rawPodUsed[i][podPriorityUsed.PriorityClass] = quotav1.Add(
					rawPodUsed[i][podPriorityUsed.PriorityClass], podPriorityUsed.Allocated)
			}
		}
	}
	return rawPodUsed, nil
}

func convertToPodUsedStatistics(podStatistics []v1beta1.PodStatistics, rawPodUsed []map[uniext.PriorityClass]corev1.ResourceList) []*v1beta1.PodUsedStatistics {
	var podUsed []*v1beta1.PodUsedStatistics
	for i := 0; i < len(rawPodUsed); i++ {
		var allocated []*v1beta1.PodPriorityUsed
		for _, priorityClassType := range priorityClassTypes {
			podPriorityUsed := &v1beta1.PodPriorityUsed{
				PriorityClass: priorityClassType,
				Allocated:     rawPodUsed[i][priorityClassType],
			}
			if podPriorityUsed.Allocated == nil {
				podPriorityUsed.Allocated = corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("0"),
					corev1.ResourceMemory: resource.MustParse("0Gi"),
				}
			}
			allocated = append(allocated, podPriorityUsed)
		}
		podUsedStatistics := v1beta1.PodUsedStatistics{
			Name:      podStatistics[i].Name,
			Allocated: allocated,
		}
		podUsed = append(podUsed, &podUsedStatistics)
	}
	return podUsed
}

func statisticsNodeRelated(candidateNodes *corev1.NodeList,
	nodeOwnedPods map[string][]*corev1.Pod, nodeOwnedReservations map[string][]*schedulingv1alpha1.Reservation, resourceSpecs []v1beta1.ResourceSpec, nodeGPUCapacity map[string]corev1.ResourceList) (capacity, requested, free, reservationCapacity, reservationRequested, reservationFree map[uniext.PriorityClass]corev1.ResourceList, allocatablePodNum map[string]map[uniext.PriorityClass]int32) {
	capacity = map[uniext.PriorityClass]corev1.ResourceList{}
	requested = map[uniext.PriorityClass]corev1.ResourceList{}
	free = map[uniext.PriorityClass]corev1.ResourceList{}
	reservationCapacity = map[uniext.PriorityClass]corev1.ResourceList{}
	reservationRequested = map[uniext.PriorityClass]corev1.ResourceList{}
	reservationFree = map[uniext.PriorityClass]corev1.ResourceList{}

	//  资源规格名 资源等级 计数
	allocatablePodNum = make(map[string]map[uniext.PriorityClass]int32, len(resourceSpecs))
	for _, resourceSpec := range resourceSpecs {
		allocatablePodNum[resourceSpec.Name] = make(map[uniext.PriorityClass]int32, len(priorityClassTypes))
	}
	for _, node := range candidateNodes.Items {
		ownedPods := nodeOwnedPods[node.Name]
		ownedReservations := nodeOwnedReservations[node.Name]
		gpuCapacity := nodeGPUCapacity[node.Name]
		nodeCapacity, nodeRequested, nodeFree, nodeReservationCapacity, nodeReservationRequested, nodeReservationFree := statisticsNodeResource(&node, ownedPods, ownedReservations, gpuCapacity)

		for _, priorityClassType := range priorityClassTypes {
			capacity[priorityClassType] = quotav1.Add(capacity[priorityClassType], nodeCapacity[priorityClassType])
			requested[priorityClassType] = quotav1.Add(requested[priorityClassType], nodeRequested[priorityClassType])
			free[priorityClassType] = quotav1.Add(free[priorityClassType], nodeFree[priorityClassType])
			reservationCapacity[priorityClassType] = quotav1.Add(reservationCapacity[priorityClassType], nodeReservationCapacity[priorityClassType])
			reservationRequested[priorityClassType] = quotav1.Add(reservationRequested[priorityClassType], nodeReservationRequested[priorityClassType])
			reservationFree[priorityClassType] = quotav1.Add(reservationFree[priorityClassType], nodeReservationFree[priorityClassType])
			for _, resourceSpec := range resourceSpecs {
				allocatablePodNum[resourceSpec.Name][priorityClassType] += calculateAllocatablePodNum(nodeFree[priorityClassType], resourceSpec.Resources)
			}
		}
	}
	return
}

func statisticsNodeResource(node *corev1.Node, ownPods []*corev1.Pod, ownedReservations []*schedulingv1alpha1.Reservation, gpuCapacity corev1.ResourceList) (capacity, requested, free, reservationCapacity, reservationRequested, reservationFree map[uniext.PriorityClass]corev1.ResourceList) {
	capacity = map[uniext.PriorityClass]corev1.ResourceList{}
	nodeAllocatable := unified.GetAllocatableByOverQuota(node)
	addGPUCapacityToNodeAllocatable(nodeAllocatable, gpuCapacity)
	for _, priorityClass := range priorityClassTypes {
		capacity[priorityClass] = quotav1.Add(capacity[priorityClass], GetNodePriorityResource(nodeAllocatable, priorityClass, node))
	}

	requested = map[uniext.PriorityClass]corev1.ResourceList{}
	allRequested := corev1.ResourceList{}
	for _, ownedPod := range ownPods {
		priorityUsed := GetPodPriorityUsed(ownedPod, node, gpuCapacity)
		priorityUsed.Allocated[corev1.ResourcePods] = *resource.NewQuantity(1, resource.DecimalSI)

		requested[priorityUsed.PriorityClass] = quotav1.Add(requested[priorityUsed.PriorityClass], priorityUsed.Allocated)
		allRequested = quotav1.Add(allRequested, priorityUsed.Allocated)
	}

	reservationCapacity = map[uniext.PriorityClass]corev1.ResourceList{}
	reservationRequested = map[uniext.PriorityClass]corev1.ResourceList{}
	reservationFree = map[uniext.PriorityClass]corev1.ResourceList{}
	for _, ownedReservation := range ownedReservations {
		priorityUsed, priorityCapacity, priorityFree := GetReservationPriorityResource(ownedReservation, node, gpuCapacity)
		priorityFree.Allocated[corev1.ResourcePods] = *resource.NewQuantity(1, resource.DecimalSI)
		priorityUsed.Allocated[corev1.ResourcePods] = *resource.NewQuantity(int64(len(ownedReservation.Status.CurrentOwners)), resource.DecimalSI)
		priorityCapacity.Allocated[corev1.ResourcePods] = *resource.NewQuantity(int64(len(ownedReservation.Status.CurrentOwners))+1, resource.DecimalSI)

		requested[priorityFree.PriorityClass] = quotav1.Add(requested[priorityFree.PriorityClass], priorityFree.Allocated)
		allRequested = quotav1.Add(allRequested, priorityFree.Allocated)

		reservationCapacity[priorityCapacity.PriorityClass] = quotav1.Add(reservationCapacity[priorityCapacity.PriorityClass], priorityCapacity.Allocated)
		reservationRequested[priorityUsed.PriorityClass] = quotav1.Add(reservationRequested[priorityUsed.PriorityClass], priorityUsed.Allocated)
		reservationFree[priorityFree.PriorityClass] = quotav1.Add(reservationFree[priorityFree.PriorityClass], priorityFree.Allocated)
	}

	for _, priorityClass := range priorityClassTypes {
		if requested[priorityClass] == nil {
			requested[priorityClass] = corev1.ResourceList{}
		} else {
			requested[priorityClass] = quotav1.Mask(requested[priorityClass], quotav1.ResourceNames(capacity[priorityClass]))
		}
		reservationCapacity[priorityClass] = quotav1.Mask(reservationCapacity[priorityClass], quotav1.ResourceNames(capacity[priorityClass]))
		reservationRequested[priorityClass] = quotav1.Mask(reservationRequested[priorityClass], quotav1.ResourceNames(capacity[priorityClass]))
		reservationFree[priorityClass] = quotav1.Mask(reservationFree[priorityClass], quotav1.ResourceNames(capacity[priorityClass]))
	}
	free = CalculateFree(capacity, requested, allRequested)
	return
}

func calculateAllocatablePodNum(free, request corev1.ResourceList) int32 {
	var min int32 = math.MaxInt32
	for resourceName, quantity := range request {
		if quantity.IsZero() {
			continue
		}
		freeQuantity, found := free[resourceName]
		if !found {
			return 0
		}
		min = int32(math.Floor(math.Min(float64(freeQuantity.MilliValue()/quantity.MilliValue()), float64(min))))
	}
	if resourcePodsNum, ok := free[corev1.ResourcePods]; ok && int32(resourcePodsNum.Value()) < min {
		min = int32(resourcePodsNum.Value())
	}
	return min
}

func convertToNodeResourceSummary(capacity, requested, free, reservationCapacity, reservationRequested, reservationFree map[uniext.PriorityClass]corev1.ResourceList,
) []*v1beta1.NodeResourceSummary {
	rawResourceSummary := map[uniext.PriorityClass]*v1beta1.NodeResourceSummary{}
	for _, priorityClassType := range priorityClassTypes {
		rawResourceSummary[priorityClassType] = &v1beta1.NodeResourceSummary{
			PriorityClass:              priorityClassType,
			Capacity:                   capacity[priorityClassType],
			Allocated:                  requested[priorityClassType],
			Allocatable:                free[priorityClassType],
			ReserveResourceCapacity:    reservationCapacity[priorityClassType],
			ReserveResourceAllocated:   reservationRequested[priorityClassType],
			ReserveResourceAllocatable: reservationFree[priorityClassType],
		}
	}
	var resourceSummaries []*v1beta1.NodeResourceSummary
	for _, resourceSummary := range rawResourceSummary {
		resourceSummaries = append(resourceSummaries, resourceSummary)
	}
	return resourceSummaries
}

func convertToResourceSpecStats(allocatablePodNums map[string]map[uniext.PriorityClass]int32) []*v1beta1.ResourceSpecStat {
	resourceSpecStats := make([]*v1beta1.ResourceSpecStat, 0)
	for name, podNums := range allocatablePodNums {
		allocatableResources := make([]*v1beta1.ResourceSpecStateAllocatable, 0)
		for _, priorityClassType := range priorityClassTypes {
			allocatableResources = append(allocatableResources, &v1beta1.ResourceSpecStateAllocatable{
				PriorityClass: priorityClassType,
				Count:         podNums[priorityClassType],
			})
		}
		resourceSpecStats = append(resourceSpecStats, &v1beta1.ResourceSpecStat{
			Name:        name,
			Allocatable: allocatableResources,
		})
	}
	return resourceSpecStats
}

func Add(mgr ctrl.Manager) error {
	if !utilfeature.DefaultFeatureGate.Enabled(features.ResourceSummaryReport) {
		return nil
	}
	reconciler := &Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	return reconciler.SetupWithManager(mgr)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.ResourceSummary{}).
		Complete(r)
}
