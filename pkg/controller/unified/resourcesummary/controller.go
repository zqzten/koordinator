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
			return r.Client.Update(ctx, resourceSummary)
		} else {
			resourceSummary.Status = *status
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
	podUsedResource, err := statisticsPodUsedResource(candidateNodes, nodeOwnedPods, resourceSummary.Spec.PodStatistics)
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
	capacity, requested, free, allocatablePodNums := statisticsNodeRelated(candidateNodes, nodeOwnedPods, resourceSummary.Spec.ResourceSpecs)
	nodeResourceSummary := convertToNodeResourceSummary(capacity, requested, free)
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
	nodeOwnedPods map[string][]*corev1.Pod, podStatistics []v1beta1.PodStatistics) ([]map[uniext.PriorityClass]corev1.ResourceList, error) {
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
		for i := 0; i < n; i++ {
			for _, ownedPod := range ownedPods {
				if !podSelectors[i].Matches(labels.Set(ownedPod.Labels)) {
					continue
				}
				podPriorityUsed := GetPodPriorityUsed(ownedPod, &node)
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
	nodeOwnedPods map[string][]*corev1.Pod, resourceSpecs []v1beta1.ResourceSpec) (capacity, requested, free map[uniext.PriorityClass]corev1.ResourceList, allocatablePodNum map[string]map[uniext.PriorityClass]int32) {
	capacity = map[uniext.PriorityClass]corev1.ResourceList{}
	requested = map[uniext.PriorityClass]corev1.ResourceList{}
	free = map[uniext.PriorityClass]corev1.ResourceList{}
	//  资源规格名 资源等级 计数
	allocatablePodNum = make(map[string]map[uniext.PriorityClass]int32, len(resourceSpecs))
	for _, resourceSpec := range resourceSpecs {
		allocatablePodNum[resourceSpec.Name] = make(map[uniext.PriorityClass]int32, len(priorityClassTypes))
	}
	for _, node := range candidateNodes.Items {
		ownedPods := nodeOwnedPods[node.Name]
		nodeCapacity, nodeRequested, nodeFree := statisticsNodeResource(&node, ownedPods)
		for _, priorityClassType := range priorityClassTypes {
			capacity[priorityClassType] = quotav1.Add(capacity[priorityClassType], nodeCapacity[priorityClassType])
			requested[priorityClassType] = quotav1.Add(requested[priorityClassType], nodeRequested[priorityClassType])
			free[priorityClassType] = quotav1.Add(free[priorityClassType], nodeFree[priorityClassType])
			for _, resourceSpec := range resourceSpecs {
				allocatablePodNum[resourceSpec.Name][priorityClassType] += calculateAllocatablePodNum(nodeFree[priorityClassType], resourceSpec.Resources)
			}
		}
	}
	return
}

func statisticsNodeResource(node *corev1.Node, ownPods []*corev1.Pod) (capacity, requested, free map[uniext.PriorityClass]corev1.ResourceList) {
	capacity = map[uniext.PriorityClass]corev1.ResourceList{}
	nodeAllocatable := GetAllocatableByOverQuota(node)
	for _, priorityClass := range priorityClassTypes {
		capacity[priorityClass] = quotav1.Add(capacity[priorityClass], GetNodePriorityResource(nodeAllocatable, priorityClass, node))
	}

	requested = map[uniext.PriorityClass]corev1.ResourceList{}
	allRequested := corev1.ResourceList{}
	for _, ownedPod := range ownPods {
		priorityUsed := GetPodPriorityUsed(ownedPod, node)
		priorityUsed.Allocated[corev1.ResourcePods] = *resource.NewQuantity(1, resource.DecimalSI)
		requested[priorityUsed.PriorityClass] = quotav1.Add(requested[priorityUsed.PriorityClass], priorityUsed.Allocated)
		allRequested = quotav1.Add(allRequested, priorityUsed.Allocated)
	}

	for _, priorityClass := range priorityClassTypes {
		if requested[priorityClass] == nil {
			requested[priorityClass] = corev1.ResourceList{}
		} else {
			requested[priorityClass] = quotav1.Mask(requested[priorityClass], quotav1.ResourceNames(capacity[priorityClass]))
		}
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

func convertToNodeResourceSummary(capacity, requested, free map[uniext.PriorityClass]corev1.ResourceList,
) []*v1beta1.NodeResourceSummary {
	rawResourceSummary := map[uniext.PriorityClass]*v1beta1.NodeResourceSummary{}
	for _, priorityClassType := range priorityClassTypes {
		rawResourceSummary[priorityClassType] = &v1beta1.NodeResourceSummary{
			PriorityClass: priorityClassType,
			Capacity:      capacity[priorityClassType],
			Allocated:     requested[priorityClassType],
			Allocatable:   free[priorityClassType],
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
		for priorityClass, count := range podNums {
			allocatableResources = append(allocatableResources, &v1beta1.ResourceSpecStateAllocatable{
				PriorityClass: priorityClass,
				Count:         count,
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
