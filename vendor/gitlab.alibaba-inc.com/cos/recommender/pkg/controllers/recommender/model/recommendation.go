package model

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"k8s.io/apimachinery/pkg/runtime"

	"gitlab.alibaba-inc.com/cos/recommender/pkg/common"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"sort"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/klog"

	recv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Map from Recommender annotation key to value.
type recommadationAnnotationsMap map[string]string

// Map from client type to condition.
type recommendationConditionsMap map[recv1alpha1.RecommendationConditionType]recv1alpha1.RecommendationCondition

func (conditionsMap *recommendationConditionsMap) Set(
	conditionType recv1alpha1.RecommendationConditionType,
	status bool, reason string, message string) *recommendationConditionsMap {
	oldCondition, alreadyPresent := (*conditionsMap)[conditionType]
	condition := recv1alpha1.RecommendationCondition{
		Type:    conditionType,
		Reason:  reason,
		Message: message,
	}
	if status {
		condition.Status = apiv1.ConditionTrue
	} else {
		condition.Status = apiv1.ConditionFalse
	}
	if alreadyPresent && oldCondition.Status == condition.Status {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	} else {
		condition.LastTransitionTime = metav1.Now()
	}
	(*conditionsMap)[conditionType] = condition
	return conditionsMap
}

func (conditionsMap *recommendationConditionsMap) AsList() []recv1alpha1.RecommendationCondition {
	conditions := make([]recv1alpha1.RecommendationCondition, 0, len(*conditionsMap))
	for _, condition := range *conditionsMap {
		conditions = append(conditions, condition)
	}

	// Sort conditions by type to avoid elements on the list
	sort.Slice(conditions, func(i, j int) bool {
		return conditions[i].Type < conditions[j].Type
	})
	return conditions
}

func (conditionsMap *recommendationConditionsMap) ConditionActive(conditionType recv1alpha1.RecommendationConditionType) bool {
	condition, found := (*conditionsMap)[conditionType]
	return found && condition.Status == apiv1.ConditionTrue
}

// Recommendation object is responsible for recommending request of pods matching a given label selector.
type Recommendation struct {
	ID RecommendationID
	// Labels selector that determines which Pods are controlled by this Recommendation
	// object. Can be nil, in which case no Pod is matched.
	PodSelector labels.Selector
	// Map of the object annotations (key-value pairs).
	Annotations recommadationAnnotationsMap
	// Map of the object conditions.
	Conditions recommendationConditionsMap
	// Most recently computed client. Can be nil.
	ResourceRecommendation *recv1alpha1.RecommendedPodResources
	// All container aggregations that contribute to this Recommendation.
	aggregateContainerStates aggregateContainerStateMap
	// Initial checkpoints of AggregateContainerStates for containers.
	ContainersInitialAggregateState ContainerNameToAggregateStateMap
	// Created denotes timestamp of the original client object creation.
	Created time.Time
	// CheckpointWritten indicates when last checkpoint for the client was stored.
	CheckpointWritten time.Time
	// TargetRef points to the controller managing the set of pods.
	TargetRef *recv1alpha1.CrossVersionObjectReference
	// PodCount containers number of live Pods matching a given client object.
	PodCount int
}

// NewRecommendation returns a new Recommendation with a given ID and pod selector. Doesn't set the
// links to the matched aggregations.
func NewRecommendation(id RecommendationID, selector labels.Selector, created time.Time) *Recommendation {
	return &Recommendation{
		ID:                              id,
		PodSelector:                     selector,
		aggregateContainerStates:        make(aggregateContainerStateMap),
		ContainersInitialAggregateState: make(ContainerNameToAggregateStateMap),
		Created:                         created,
		Conditions:                      make(recommendationConditionsMap),
		PodCount:                        0,
	}
}

// HasRecommendation returns if the Recommendation object contains any client
func (rec *Recommendation) HasRecommendation() bool {
	return (rec.ResourceRecommendation != nil) && len(rec.ResourceRecommendation.ContainerRecommendations) > 0
}

func (rec *Recommendation) AggregateStateByContainerName() ContainerNameToAggregateStateMap {
	containerNameToAggregateStateMap := AggregateStateByContainerName(rec.aggregateContainerStates)
	rec.MergeCheckpointedState(containerNameToAggregateStateMap)
	return containerNameToAggregateStateMap
}

// MergeCheckpointedState adds checkpointed Recommendation aggregations to the given aggregateStateMap.
func (rec *Recommendation) MergeCheckpointedState(aggregateContainerStateMap ContainerNameToAggregateStateMap) {
	for containerName, aggregation := range rec.ContainersInitialAggregateState {
		aggregateContainerState, found := aggregateContainerStateMap[containerName]
		if !found {
			aggregateContainerState = NewAggregateContainerState()
			aggregateContainerStateMap[containerName] = aggregateContainerState
		}
		aggregateContainerState.MergeContainerState(aggregation)
	}
}

// UsesAggregation return true iff an aggregation with the given key contributes to the Recommendation.
func (rec *Recommendation) UsesAggregation(aggregationKey AggregateStateKey) (exists bool) {
	_, exists = rec.aggregateContainerStates[aggregationKey]
	return
}

// UpdateRecommendationForContainerNotExist deletes aggregation used by this container and recommendation is related to it.
func (rec *Recommendation) UpdateRecommendationForContainerNotExist(aggregationKey AggregateStateKey, clientIf client.Client) {
	state, ok := rec.aggregateContainerStates[aggregationKey]
	if !ok {
		return
	}
	state.MarkNotRecommended()
	namespace := rec.ID.Namespace
	name := rec.ID.RecommendationName
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	recommendation := &recv1alpha1.Recommendation{}
	err := clientIf.Get(context.TODO(), key, recommendation)
	if err != nil {
		klog.Errorf("Fail to get recommendation %s/%s from ETCD, err: %v", namespace, name, err)
		return
	}

	// NOTE: when workload rolling updates, pods with old labels(e.g. template-hash) might be treated as inactive
	// the container status should not be deleted directly. Make a quick fix here until the gc logic refactor.
	related := &apiv1.PodList{}
	err = clientIf.List(context.TODO(), related, &client.ListOptions{
		Namespace:     rec.ID.Namespace,
		LabelSelector: rec.PodSelector,
	})
	if err != nil {
		klog.Warningf("get relative pods by selector %v for recommendation %v failed, err: %v",
			rec.PodSelector.String(), rec.ID, err)
		return
	}
	if len(related.Items) != 0 {
		podSample := related.Items[0]
		for _, container := range podSample.Spec.Containers {
			if container.Name == aggregationKey.ContainerName() {
				klog.V(4).Infof("no need to gc container %v for recommendation %v since there is still relatvie pods",
					container.Name, rec.ID)
				return
			}
		}
	}

	duplicate := recommendation.DeepCopy()
	deleteRecommendationConatinerStatus(recommendation, aggregationKey)
	if !reflect.DeepEqual(duplicate, recommendation) {
		if err = clientIf.Status().Update(context.TODO(), recommendation); err != nil {
			klog.Errorf("Fail to update the container %v of recommendation %s/%s from ETCD, err: %v", aggregationKey.ContainerName(), namespace, name, err)
			return
		}
	}
	delete(rec.aggregateContainerStates, aggregationKey)
}

func (rec *Recommendation) UpdateRecommendationForContainerExpired(aggregationKey AggregateStateKey, client client.Client) {
	state, ok := rec.aggregateContainerStates[aggregationKey]
	if !ok {
		return
	}
	state.MarkNotRecommended()
	namespace := rec.ID.Namespace
	name := rec.ID.RecommendationName
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	recommendation := &recv1alpha1.Recommendation{}
	err := client.Get(context.TODO(), key, recommendation)
	if err != nil {
		klog.Errorf("Fail to get recommendation %s/%s from ETCD, err: %v", namespace, name, err)
		return
	}
	duplicate := recommendation.DeepCopy()

	msg := ""
	if condition, exist := rec.Conditions[recv1alpha1.MetricIsExpired]; exist {
		if !strings.Contains(condition.Message, aggregationKey.ContainerName()) {
			msg = condition.Message + fmt.Sprintf(",%s", aggregationKey.ContainerName())
		}
	} else {
		msg = fmt.Sprintf("%s", aggregationKey.ContainerName())
	}
	rec.Conditions.Set(recv1alpha1.MetricIsExpired, true, "MetricOFContainerIsExpired", msg)
	recommendation.Status.Conditions = rec.Conditions.AsList()

	deleteRecommendationConatinerStatus(recommendation, aggregationKey)
	if !reflect.DeepEqual(duplicate, recommendation) {
		if err = client.Status().Update(context.TODO(), recommendation); err != nil {
			klog.Errorf("Fail to update the container %v of recommendation %s/%s from ETCD, err: %v", aggregationKey.ContainerName(), namespace, name, err)
			return
		}
	}
	delete(rec.aggregateContainerStates, aggregationKey)
}

func (rec *Recommendation) UpdateRecommendationForActiveContainer(aggregationKey AggregateStateKey, client client.Client) {
	condition, exist := rec.Conditions[recv1alpha1.MetricIsExpired]
	if !exist {
		return
	}
	namespace := rec.ID.Namespace
	name := rec.ID.RecommendationName
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	recommendation := &recv1alpha1.Recommendation{}
	err := client.Get(context.TODO(), key, recommendation)
	if err != nil {
		klog.Errorf("Fail to get recommendation %s/%s from ETCD, err: %v", namespace, name, err)
		return
	}
	containers := strings.Split(condition.Message, ",")
	for index, container := range containers {
		if container == aggregationKey.ContainerName() {
			containers = append(containers[:index], containers[index+1:]...)
			if len(containers) == 0 {
				delete(rec.Conditions, recv1alpha1.MetricIsExpired)
			} else {
				rec.Conditions.Set(recv1alpha1.MetricIsExpired, true, "MetricOFContainerIsExpired", strings.Join(containers, ","))
			}
			recommendation.Status.Conditions = rec.Conditions.AsList()

			if err = client.Status().Update(context.TODO(), recommendation); err != nil {
				klog.Errorf("Fail to delete the container %v of recommendation %s/%s from ETCD, err: %v", aggregationKey.ContainerName(), namespace, name, err)
				return
			}
			break
		}
	}
}

func deleteRecommendationConatinerStatus(recommendation *recv1alpha1.Recommendation, aggregationKey AggregateStateKey) {
	if recommendation.Status.Recommendation == nil {
		klog.Errorf("There isn't any resource recommended in recommendation %s/%s", recommendation.Namespace, recommendation.Name)
		return
	}
	containers := recommendation.Status.Recommendation.ContainerRecommendations
	duplicate := make([]recv1alpha1.RecommendedContainerResources, 0)
	for _, container := range containers {
		if container.ContainerName != aggregationKey.ContainerName() {
			duplicate = append(duplicate, container)
		}
	}
	recommendation.Status.Recommendation.ContainerRecommendations = duplicate
}

// returns true iff the Recommendation matches the given aggregation key.
func (rec *Recommendation) matchesAggregation(aggregationKey AggregateStateKey) bool {
	if rec.ID.Namespace != aggregationKey.Namespace() {
		return false
	}
	return rec.PodSelector != nil && rec.PodSelector.Matches(aggregationKey.Labels())
}

// UseAggregationIfMatching  checks if the given aggregation matches this Recommendation
// and adds it to the set of Recommendation's aggregations if that is the case.
func (rec *Recommendation) UseAggregationIfMatching(aggregationKey AggregateStateKey, aggregation *AggregateContainerState) {
	if rec.UsesAggregation(aggregationKey) {
		// Already linked
		return
	}
	if rec.matchesAggregation(aggregationKey) {
		rec.aggregateContainerStates[aggregationKey] = aggregation
		aggregation.IsUnderRecommendation = true
	}
}

func (rec *Recommendation) UpdateRecommendation(recommendation *recv1alpha1.RecommendedPodResources) {
	for _, containerRecommendation := range recommendation.ContainerRecommendations {
		for container, state := range rec.aggregateContainerStates {
			if container.ContainerName() == containerRecommendation.ContainerName {
				state.LastRecommendation = containerRecommendation.Target
			}
		}
	}
	rec.ResourceRecommendation = recommendation
}

// UpdateConditions Updates the conditions of Recommendation objects based on it's state,
// PodMatched is passed to indicate if there are currently active pods in the cluster
//	matching this Recommendation.
func (rec *Recommendation) UpdateConditions(podsMatched bool) {
	reason := ""
	msg := ""
	if podsMatched {
		delete(rec.Conditions, recv1alpha1.NoPodsMatched)
	} else {
		reason = "NoPodsMatched"
		msg = "No pods match this Recommendation object"
		rec.Conditions.Set(recv1alpha1.NoPodsMatched, true, reason, msg)
	}
	if rec.HasRecommendation() {
		rec.Conditions.Set(recv1alpha1.RecommendationProvided, true, "", "")
	} else {
		rec.Conditions.Set(recv1alpha1.RecommendationProvided, false, reason, msg)
	}

}

// HasMatchedPods returns true if there are currently active pods in the
// cluster matching this Recommendation, based on conditions. UpdateConditions should be
// called first.
func (rec *Recommendation) HasMatchedPods() bool {
	noPodsMatched, found := rec.Conditions[recv1alpha1.NoPodsMatched]
	if found && noPodsMatched.Status == apiv1.ConditionTrue {
		return false
	}
	return true
}

// AsStatus returns this objects equivalent of VPA Status. UpdateConditions
// should be called first.
func (rec *Recommendation) AsStatus() *recv1alpha1.RecommendationStatus {
	status := &recv1alpha1.RecommendationStatus{
		Conditions: rec.Conditions.AsList(),
	}
	if rec.ResourceRecommendation != nil {
		status.Recommendation = rec.ResourceRecommendation
	}
	return status
}

func (rec *Recommendation) GetTargetRefObject(kubeClient client.Client, scheme *runtime.Scheme) error {
	if rec.TargetRef == nil {
		return fmt.Errorf("the target shouldn't be nil")
	}
	kind := schema.FromAPIVersionAndKind(rec.TargetRef.APIVersion, rec.TargetRef.Kind)
	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(kind)

	err := kubeClient.Get(context.TODO(), client.ObjectKey{Namespace: rec.ID.Namespace, Name: rec.TargetRef.Name}, target)
	if err != nil && errors.IsNotFound(err) {
		return &common.NotExists{}
	}
	return err
}
