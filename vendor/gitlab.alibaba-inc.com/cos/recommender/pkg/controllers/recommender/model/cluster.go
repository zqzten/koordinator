package model

import (
	"context"
	"fmt"
	"time"

	"gitlab.alibaba-inc.com/cos/recommender/pkg/common"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/util"
	recv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
)

const (
	RecommendationMissingMaxDuration = 30 * time.Minute
)

// AggregateStateKey determines the set of containers for which the usage samples
// are kept aggregated in the model.
type AggregateStateKey interface {
	Namespace() string
	ContainerName() string
	Labels() labels.Labels
}

// Implementation of the AggregateStateKey interface. It can be used as a map key.
type aggregateStateKey struct {
	namespace     string
	containerName string
	labelSetKey   labelSetKey
	// Pointer to the global map from labelSetKey to labels.Set.
	labelSetMap *labelSetMap
}

// Namespace Labels returns the namespace for the aggregateStateKey.
func (k aggregateStateKey) Namespace() string {
	return k.namespace
}

func (k aggregateStateKey) ContainerName() string {
	return k.containerName
}

func (k aggregateStateKey) Labels() labels.Labels {
	if k.labelSetMap == nil {
		return labels.Set{}
	}
	return (*k.labelSetMap)[k.labelSetKey]
}

// Sting representation of the labels.LabelSet. This is the value returned by
// labelSet.String().As opposed to the LabelSet object, it can be used as a map key.
type labelSetKey string

// Map of label sets keyed by their string representation.
type labelSetMap map[labelSetKey]labels.Set

// AggregateContainerStateMap is a map from AggregateStateKey to AggregateContainerState.
type aggregateContainerStateMap map[AggregateStateKey]*AggregateContainerState

type PodState struct {
	// Unique id of the Pod.
	ID PodID
	// Set of labels attached to the Pod.
	labelSetKey labelSetKey
	// Containers that belongs to the pod, keyed by the container name.
	Containers map[string]*ContainerState
	// PodPhase describing current life cycle phase of the pod.
	Phase v1.PodPhase
}

// ClusterState holds all runtime information about the cluster required for the
// recommend operations , i.e. configuration of resource(pods, containers, Recommenadtion objects),
// aggregated utilization of compute resource (CPU\ memory) and events(container OOMs).
// All clientset to the recommender algorithm lives in this structure.
type ClusterState struct {
	// Pods in the cluster.
	Pods map[PodID]*PodState
	// Recommendation objects in the cluster.
	Recommendations map[RecommendationID]*Recommendation
	// EmptyRecommendations objects in the cluster that have no client mapped to the first
	// time we've noticed the client missing or last time we logged
	// a warning about it.
	EmptyRecommendations map[RecommendationID]time.Time
	// Observedrecommendations Used to check if there are updates needed.
	ObservedRecommendations []*recv1alpha1.Recommendation
	// All container aggregations where the usage samples are stored.
	aggregateStateMap aggregateContainerStateMap
	// Map with all label sets used by the aggregations. It serveres as a cache
	// that allows to quickly access label.Set corresponding to a labelSetKey.
	labelSetMap labelSetMap
	// map of Recommendation ID with the origin workload
	OriginWorkloads map[RecommendationID]*WorkloadInfo
}

// NewClusterState returns a new ClusterState with no pods.
func NewClusterState() *ClusterState {
	return &ClusterState{
		Pods:                 make(map[PodID]*PodState),
		Recommendations:      make(map[RecommendationID]*Recommendation),
		EmptyRecommendations: make(map[RecommendationID]time.Time),
		aggregateStateMap:    make(aggregateContainerStateMap),
		labelSetMap:          make(labelSetMap),
		OriginWorkloads:      make(map[RecommendationID]*WorkloadInfo),
	}
}

// StateMapSize  is the number of pods being tracked by the Recommendations
func (cluster *ClusterState) StateMapSize() int {
	return len(cluster.aggregateStateMap)
}

func newPod(id PodID) *PodState {
	return &PodState{
		ID:         id,
		Containers: make(map[string]*ContainerState),
	}
}

// AddOrUpdatePod updates the state of the pod with a given PodID, if it is
// present in the cluster object. Otherwise a new pod is created and added to
// the Cluster object.
// If the labels of the pod have change, it updates the links between the containers
// and the aggregations.
func (cluster *ClusterState) AddOrUpdatePod(podID PodID, newLabels labels.Set, phase v1.PodPhase) {
	pod, podExists := cluster.Pods[podID]
	if !podExists {
		pod = newPod(podID)
		cluster.Pods[podID] = pod
	}
	newlabelSetKey := cluster.getLabelSetKey(newLabels)
	if podExists && pod.labelSetKey != newlabelSetKey {
		// This Pod is already counted in the old Recommendation, remove the link.
		cluster.removePodFromItsRecommendation(pod)
	}
	if !podExists || pod.labelSetKey != newlabelSetKey {
		pod.labelSetKey = newlabelSetKey
		// Set the links between the containers and aggregations based on the current pod labels.
		for containerName, container := range pod.Containers {
			containerID := ContainerID{PodID: podID, ContainerName: containerName}
			container.aggregator = cluster.findOrCreateAggregateContainerState(containerID)
		}
		cluster.addPodToItsRecommendation(pod)
	}
	pod.Phase = phase
}

// addPodToItsRecommendation increases the count of Pods associated with a Recommendation object.
// Dose a scan similar to findOrCreateAggregateContainerState so cloud be optimized if needed.
func (cluster *ClusterState) addPodToItsRecommendation(pod *PodState) {
	for _, rec := range cluster.Recommendations {
		if util.PodLabelsMatchRecommendation(pod.ID.Namespace, cluster.labelSetMap[pod.labelSetKey], rec.ID.Namespace, rec.PodSelector) {
			rec.PodCount++
		}
	}
}

func (cluster *ClusterState) AddOrUpdateRecommendation(apiObject *recv1alpha1.Recommendation, selector labels.Selector) error {
	recID := RecommendationID{Namespace: apiObject.Namespace, RecommendationName: apiObject.Name}
	annotationMap := apiObject.Annotations
	conditionsMap := make(recommendationConditionsMap)
	for _, condition := range apiObject.Status.Conditions {
		conditionsMap[condition.Type] = condition
	}
	var currentRecommendation *recv1alpha1.RecommendedPodResources
	if conditionsMap[recv1alpha1.RecommendationProvided].Status == v1.ConditionTrue {
		currentRecommendation = apiObject.Status.Recommendation
	}

	rec, recExists := cluster.Recommendations[recID]
	if recExists && (rec.PodSelector.String() != selector.String()) {
		// Pod selector was changed. Deleted the client and recreate
		// it with the new selector.
		if err := cluster.DeleteRecommendation(recID); err != nil {
			return err
		}
		recExists = false
	}
	if !recExists {
		rec = NewRecommendation(recID, selector, apiObject.CreationTimestamp.Time)
		cluster.Recommendations[recID] = rec
		for aggregationKey, aggregation := range cluster.aggregateStateMap {
			rec.UseAggregationIfMatching(aggregationKey, aggregation)
		}
		rec.PodCount = len(cluster.GetMatchingPods(rec))
	}
	if apiObject.Spec.WorkloadRef != nil {
		rec.TargetRef = apiObject.Spec.WorkloadRef
	}
	rec.Annotations = annotationMap
	rec.Conditions = conditionsMap
	rec.ResourceRecommendation = currentRecommendation
	return nil
}

// DeletePod removes an existing pod from the cluster.
func (cluster *ClusterState) DeletePod(podID PodID) {
	pod, found := cluster.Pods[podID]
	if found {
		cluster.removePodFromItsRecommendation(pod)
	}
	delete(cluster.Pods, podID)
}

// GetMatchingPods returns a list of currently active pods that match the
// given Recommendation. Traverses through all pods in the cluster - use sparingly.
func (cluster *ClusterState) GetMatchingPods(rec *Recommendation) []PodID {
	matchingPods := make([]PodID, 0)
	for podID, pod := range cluster.Pods {
		if util.PodLabelsMatchRecommendation(podID.Namespace, cluster.labelSetMap[pod.labelSetKey],
			rec.ID.Namespace, rec.PodSelector) {
			matchingPods = append(matchingPods, podID)
		}
	}
	return matchingPods
}

// AddOrUpdateContainer creates a new container with the given ContainerID and
// adds it to the parent pod in the ClusterState object, if not yet present.
// Requires the pod to be added to the ClusterState first. Otherwise an error is
// returned.
func (cluster *ClusterState) AddOrUpdateContainer(containerID ContainerID, request Resources) error {
	pod, podExists := cluster.Pods[containerID.PodID]
	if !podExists {
		return NewKeyError(containerID.PodID)
	}
	if container, containerExists := pod.Containers[containerID.ContainerName]; !containerExists {
		cluster.findOrCreateAggregateContainerState(containerID)
		pod.Containers[containerID.ContainerName] = NewContainerState(request, NewContainerStateAggregatorProxy(cluster, containerID))
	} else {
		// Container aleady exists. Possibly update the request.
		container.Request = request
	}
	return nil
}

// DeleteRecommendation removes a Recommendation with the given ID from the ClusterState.
func (cluster *ClusterState) DeleteRecommendation(recID RecommendationID) error {
	rec, recExists := cluster.Recommendations[recID]
	if !recExists {
		return NewKeyError(recID)
	}
	for _, state := range rec.aggregateContainerStates {
		state.MarkNotRecommended()
	}
	delete(cluster.Recommendations, recID)
	delete(cluster.EmptyRecommendations, recID)
	delete(cluster.OriginWorkloads, recID)
	return nil
}

// removePodFromItsRecommendation decreases the count of Pods associated with a client object.
func (cluster *ClusterState) removePodFromItsRecommendation(pod *PodState) {
	for _, rec := range cluster.Recommendations {
		if PodLabelsMatchRecommendation(pod.ID.Namespace, cluster.labelSetMap[pod.labelSetKey], rec.ID.Namespace, rec.PodSelector) {
			rec.PodCount--
		}
	}
}

// PodLabelsMatchRecommendation returns true iff the recommendationWithSelector matches the pod labels.
func PodLabelsMatchRecommendation(podNamespace string, labels labels.Set, recNamespace string, recSelector labels.Selector) bool {
	if podNamespace != recNamespace {
		return false
	}
	return recSelector.Matches(labels)
}

// getLabelSetKey puts the given labelSet in the global labelSet map and return a
// corresponding labelSetKey.
func (cluster *ClusterState) getLabelSetKey(labelSet labels.Set) labelSetKey {
	labelSetKey := labelSetKey(labelSet.String())
	cluster.labelSetMap[labelSetKey] = labelSet
	return labelSetKey
}

// AddSample adds a new usage to the proper container in the ClusterState
// object. Requires the container as well as the parent pod to be added to the
// ClusterState first. Otherwises an error is returned.
func (cluster *ClusterState) AddSample(sample *ContainerUsageSampleWithKey) error {
	pod, podExists := cluster.Pods[sample.Container.PodID]
	if !podExists {
		return NewKeyError(sample.Container.PodID)
	}
	containerState, containerExists := pod.Containers[sample.Container.ContainerName]
	if !containerExists {
		return NewKeyError(sample.Container)
	}
	if !containerState.AddSample(&sample.ContainerUsageSample) {
		return fmt.Errorf("sample discarded (invalid or out of order)")
	}
	return nil
}

// RecordOOM adds info regarding OOM event in the model as an artificial memory sample.
func (cluster *ClusterState) RecordOOM(containerID ContainerID, timestamp time.Time, requestedMemory ResourceAmount) error {
	pod, podExists := cluster.Pods[containerID.PodID]
	if !podExists {
		return NewKeyError(containerID.PodID)
	}
	ContainerState, containerExists := pod.Containers[containerID.ContainerName]
	if !containerExists {
		return NewKeyError(containerID.ContainerName)
	}
	err := ContainerState.RecordOOM(timestamp, requestedMemory)
	if err != nil {
		return fmt.Errorf("error while recording OOM for %v, Reason: %v", containerID, err)
	}
	return nil
}

// ContainerUsageSampleWithKey  holds a ContainerUsageSample together with the
// ID of the container it belongs to.
type ContainerUsageSampleWithKey struct {
	ContainerUsageSample
	Container ContainerID
}

// MakeAggregateStateKey returns the AggregateStateKey that should be used
// to aggregate usage samples from a container with the given name in a given pod.
func (cluster *ClusterState) MakeAggregateStateKey(pod *PodState, containerName string) AggregateStateKey {
	return aggregateStateKey{
		namespace:     pod.ID.Namespace,
		containerName: containerName,
		labelSetKey:   pod.labelSetKey,
		labelSetMap:   &cluster.labelSetMap,
	}
}

// aggregateStateKeyForContainerID returns the AggregateStateKey for the ContainerID.
// The pod with the corresponding PodID must already be present in the ClusterState.
func (cluster *ClusterState) aggregateStateKeyForContainerID(containerID ContainerID) AggregateStateKey {
	pod, podExists := cluster.Pods[containerID.PodID]
	if !podExists {
		panic(fmt.Sprintf("Pod not present in the ClusterState: %v", containerID.PodID))
	}
	return cluster.MakeAggregateStateKey(pod, containerID.ContainerName)
}

func (cluster *ClusterState) findOrCreateAggregateContainerState(containerID ContainerID) *AggregateContainerState {
	aggregateStateKey := cluster.aggregateStateKeyForContainerID(containerID)
	aggregateContainerState, aggregateStateExists := cluster.aggregateStateMap[aggregateStateKey]
	if !aggregateStateExists {
		aggregateContainerState = NewAggregateContainerState()
		cluster.aggregateStateMap[aggregateStateKey] = aggregateContainerState
		// Link the new aggregation to the existing recommendations.
		for _, rec := range cluster.Recommendations {
			rec.UseAggregationIfMatching(aggregateStateKey, aggregateContainerState)
		}
	}
	return aggregateContainerState
}

// RecordRecommendation marks the state of client in the cluster. We
// keep track of empty recommendations and log information about them
// periodically.
func (cluster *ClusterState) RecordRecommendation(rec *Recommendation, now time.Time) error {
	if rec.ResourceRecommendation != nil && len(rec.ResourceRecommendation.ContainerRecommendations) > 0 {
		delete(cluster.EmptyRecommendations, rec.ID)
		return nil
	}
	lastLogged, ok := cluster.EmptyRecommendations[rec.ID]
	if !ok {
		cluster.EmptyRecommendations[rec.ID] = now
	} else {
		if lastLogged.Add(RecommendationMissingMaxDuration).Before(now) {
			cluster.EmptyRecommendations[rec.ID] = now
			return fmt.Errorf("recommender %v/%v is missing client for more than %v", rec.ID.Namespace, rec.ID.RecommendationName, RecommendationMissingMaxDuration)
		}
	}
	return nil
}

// GarbageRecommendation removes obsolete AggregateCollectionStates from the ClusterState
// as well as remove the invalid recommendation from ETCD.
// AggregateCollectionState is obsolete in following situations:
// 1) There are no more active pods that can contribute,
// 2) The last sample is too old to give meaningful client (>8 days),
// 3) There are no samples and the aggregate state was created >8 days ago.
func (cluster *ClusterState) GarbageRecommendation(now time.Time, kubeClient client.Client) {
	klog.V(1).Info("Garbage collection of AggregateCollectionStates triggered")
	keysToDeleteForNotExist := make([]AggregateStateKey, 0)
	keysToDeleteForExpired := make([]AggregateStateKey, 0)
	keysIsActive := make([]AggregateStateKey, 0)
	activeKeys := cluster.getActiveAggregateStateKeys()
	for key, aggregateContainerState := range cluster.aggregateStateMap {
		isKeyActive := activeKeys[key]
		if !isKeyActive {
			keysToDeleteForNotExist = append(keysToDeleteForNotExist, key)
			klog.V(1).Infof("Removing empty and inactive AggregateCollectionState for %+v", key)
			continue
		}
		if aggregateContainerState.isExpired(now) {
			keysToDeleteForExpired = append(keysToDeleteForExpired, key)
			klog.V(1).Infof("Removing expired AggregateCollectionState for %+v", key)
		} else {
			keysIsActive = append(keysIsActive, key)
		}
	}

	// TODO: refactor this gc logic, which is not final-state oriented now.
	// Update status of recommendation whose pods exist in cluster but have been inactive.
	// Update status of recommendation whose container has been deleted.
	for _, key := range keysToDeleteForNotExist {
		delete(cluster.aggregateStateMap, key)
		for _, rec := range cluster.Recommendations {
			rec.UpdateRecommendationForContainerNotExist(key, kubeClient)
		}
	}

	// Update status of recommendation whose container's metric is expired.
	for _, key := range keysToDeleteForExpired {
		delete(cluster.aggregateStateMap, key)
		for _, rec := range cluster.Recommendations {
			rec.UpdateRecommendationForContainerExpired(key, kubeClient)
		}
	}

	// Update status of recommendation whose container is active.
	for _, key := range keysIsActive {
		for _, rec := range cluster.Recommendations {
			rec.UpdateRecommendationForActiveContainer(key, kubeClient)
		}
	}
}

// DeleteRecommendationPodNotExist Delete recommendation whose pods don't exist anymore.
func (cluster *ClusterState) DeleteRecommendationPodNotExist(kubeClient client.Client, scheme *runtime.Scheme) {
	for _, rec := range cluster.Recommendations {
		key := types.NamespacedName{
			Name:      rec.ID.RecommendationName,
			Namespace: rec.ID.Namespace,
		}
		recommendation := &recv1alpha1.Recommendation{}
		if err := kubeClient.Get(context.TODO(), key, recommendation); err != nil {
			klog.Errorf("Fail to get recommendation %s/%s , err: %v", rec.ID.Namespace, rec.ID.RecommendationName, err)
			continue
		}

		if recommendation.Spec.WorkloadRef != nil {
			if err := rec.GetTargetRefObject(kubeClient, scheme); err != nil {
				if _, ok := err.(*common.NotExists); !ok {
					klog.Errorf("Fail to get targetRef object %s/%s , err: %v", rec.ID.Namespace, rec.TargetRef.Name, err)
					continue
				}
			} else {
				continue
			}
		}

		if recommendation.Spec.Selector != nil && rec.PodCount >= 0 {
			continue
		}

		if err := kubeClient.Delete(context.TODO(), recommendation); err != nil {
			klog.Errorf("Fail to delete recommendation %s/%s from ETCD, err: %v", rec.ID.Namespace, rec.ID.RecommendationName, err)
			continue
		}
		if err := cluster.DeleteRecommendation(rec.ID); err != nil {
			klog.Errorf("Fail to delete recommendation %s/%s from cluster, err: %v", rec.ID.Namespace, rec.ID.RecommendationName, err)
		}
	}
}

func (cluster *ClusterState) getActiveAggregateStateKeys() map[AggregateStateKey]bool {
	activeKeys := map[AggregateStateKey]bool{}
	for _, pod := range cluster.Pods {
		// Pods that will not run anymore are considered inactive.
		if pod.Phase == v1.PodSucceeded || pod.Phase == v1.PodFailed {
			continue
		}
		for container := range pod.Containers {
			activeKeys[cluster.MakeAggregateStateKey(pod, container)] = true
		}
	}
	return activeKeys
}

// GetContainer returns the ContainerState object for a given ContainerID or
// null if it's not present in the model.
func (cluster *ClusterState) GetContainer(containerID ContainerID) *ContainerState {
	pod, podExists := cluster.Pods[containerID.PodID]
	if podExists {
		container, containerExists := pod.Containers[containerID.ContainerName]
		if containerExists {
			return container
		}
	}
	return nil
}
