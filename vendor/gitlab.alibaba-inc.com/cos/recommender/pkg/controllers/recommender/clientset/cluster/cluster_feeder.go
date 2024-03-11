/*
Copyright 2018 The Kubernetes Authors.

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

package input

import (
	"context"
	"fmt"
	"time"

	kruise_client "github.com/openkruise/kruise-api/client/clientset/versioned"
	kruise_informer "github.com/openkruise/kruise-api/client/informers/externalversions"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	kube_client "k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	v1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	resourceclient "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"

	prommetrics "gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/metrics"
	controllerfetcher "gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/clientset/controller_fetcher"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/clientset/metrics"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/clientset/oom"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/clientset/spec"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/clientset/target"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/history"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/model"
	recv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	rec_clientset "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned"
	rec_api "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned/typed/autoscaling/v1alpha1"
	rec_lister "gitlab.alibaba-inc.com/cos/unified-resource-api/client/listers/autoscaling/v1alpha1"
)

const (
	evictionWatchRetryWait               = 10 * time.Second
	evictionWatchJitterFactor            = 0.5
	scaleCacheLoopPeriod                 = 7 * time.Second
	scaleCacheEntryLifetime              = time.Hour
	scaleCacheEntryFreshnessTime         = 10 * time.Minute
	scaleCacheEntryJitterFactor  float64 = 1.
	defaultResyncPeriod                  = 10 * time.Minute
)

// ClusterStateFeeder can update state of ClusterState object.
type ClusterStateFeeder interface {
	// InitFromHistoryProvider loads historical pod spec into clusterState.
	InitFromHistoryProvider(historyProvider history.HistoryProvider)

	// InitFromCheckpoints loads historical checkpoints into clusterState.
	InitFromCheckpoints()

	// LoadRecommendations updates clusterState with current state of Recommendations.
	LoadRecommendations()

	// LoadPods updates clusterState with current specification of Pods and their Containers.
	LoadPods()

	// LoadRealTimeMetrics updates clusterState with current usage metrics of containers.
	LoadRealTimeMetrics()

	// GarbageCollectCheckpoints removes historical checkpoints that don't have a matching Recommendation.
	GarbageCollectCheckpoints()
}

// ClusterStateFeederFactory makes instances of ClusterStateFeeder.
type ClusterStateFeederFactory struct {
	ClusterState             *model.ClusterState
	KubeClient               kube_client.Interface
	MetricsClient            metrics.MetricsClient
	RecommendationCheckpoint rec_api.RecommendationCheckpointsGetter

	RecommendationLister rec_lister.RecommendationLister

	PodLister         v1lister.PodLister
	OOMObserver       oom.Observer
	SelectorFetcher   target.RecommendationTargetSelectorFetcher
	MemorySaveMode    bool
	ControllerFetcher controllerfetcher.ControllerFetcher
}

// Make creates new ClusterStateFeeder with internal data providers, based on kube client.
func (m ClusterStateFeederFactory) Make() *clusterStateFeeder {
	return &clusterStateFeeder{
		coreClient:                     m.KubeClient.CoreV1(),
		metricsClient:                  m.MetricsClient,
		oomChan:                        m.OOMObserver.GetObservedOomsChannel(),
		recommendationCheckpointClient: m.RecommendationCheckpoint,

		recommendationLister: m.RecommendationLister,

		clusterState:      m.ClusterState,
		specClient:        spec.NewSpecClient(m.PodLister),
		selectorFetcher:   m.SelectorFetcher,
		memorySaveMode:    m.MemorySaveMode,
		controllerFetcher: m.ControllerFetcher,
	}
}

// NewClusterStateFeeder creates new ClusterStateFeeder with internal data providers, based on kube client config.
// Deprecated; Use ClusterStateFeederFactory instead.
func NewClusterStateFeeder(config *rest.Config, clusterState *model.ClusterState, memorySave bool, namespace string, supportKruise bool) ClusterStateFeeder {
	kubeClient := kube_client.NewForConfigOrDie(config)
	kruiseClient := kruise_client.NewForConfigOrDie(config)
	podLister, oomObserver := NewPodListerAndOOMObserver(kubeClient, namespace)
	factory := informers.NewSharedInformerFactoryWithOptions(kubeClient, defaultResyncPeriod, informers.WithNamespace(namespace))
	kruiseFactory := kruise_informer.NewSharedInformerFactoryWithOptions(kruiseClient, defaultResyncPeriod, kruise_informer.WithNamespace(namespace))
	controllerFetcher := controllerfetcher.NewControllerFetcher(config, kubeClient, factory, kruiseFactory, scaleCacheEntryFreshnessTime,
		scaleCacheEntryLifetime, scaleCacheEntryJitterFactor, supportKruise)
	controllerFetcher.Start(context.TODO(), scaleCacheLoopPeriod)
	return ClusterStateFeederFactory{
		PodLister:                podLister,
		OOMObserver:              oomObserver,
		KubeClient:               kubeClient,
		MetricsClient:            newMetricsClient(config, namespace),
		RecommendationCheckpoint: rec_clientset.NewForConfigOrDie(config).AutoscalingV1alpha1(),
		RecommendationLister:     NewRecommendationLister(rec_clientset.NewForConfigOrDie(config), make(chan struct{}), namespace),
		ClusterState:             clusterState,
		SelectorFetcher:          target.NewRecommendationTargetSelectorFetcher(config, kubeClient, factory, kruiseFactory, supportKruise),
		MemorySaveMode:           memorySave,
		ControllerFetcher:        controllerFetcher,
	}.Make()
}

//	NewRecommendationLister returns RecommendationLister configured to fetch all Recommendations from namespace,
//	set namespace to k8sapiv1.namespaceALL to select all namespaces.
// The method blocks until recommendationLister is initially populated.
func NewRecommendationLister(recommendationClient *rec_clientset.Clientset, stopChannel <-chan struct{}, namespace string) rec_lister.RecommendationLister {

	recommendationListWatch := cache.NewListWatchFromClient(recommendationClient.AutoscalingV1alpha1().RESTClient(), "recommendations", namespace, fields.Everything())
	indexer, controller := cache.NewIndexerInformer(recommendationListWatch,
		&recv1alpha1.Recommendation{},
		1*time.Hour,
		&cache.ResourceEventHandlerFuncs{},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	recommendationLister := rec_lister.NewRecommendationLister(indexer)
	go controller.Run(stopChannel)
	if !cache.WaitForCacheSync(make(chan struct{}), controller.HasSynced) {
		klog.Fatalf("Failed to sync Recommendation cache during initialization")
	} else {
		klog.Infof("Initial Recommendation synced successfully")
	}
	return recommendationLister
}

func newMetricsClient(config *rest.Config, namespace string) metrics.MetricsClient {
	metricsGetter := resourceclient.NewForConfigOrDie(config)
	return metrics.NewMetricsClient(metricsGetter, namespace)
}

// WatchEvictionEventsWithRetries watches new Events with reason=Evicted and passes them to the observer.
func WatchEvictionEventsWithRetries(kubeClient kube_client.Interface, observer oom.Observer, namespace string) {
	go func() {
		options := metav1.ListOptions{
			FieldSelector: "reason=Evicted",
		}

		watchEvictionEventsOnce := func() {
			watchInterface, err := kubeClient.CoreV1().Events(namespace).Watch(context.TODO(), options)
			if err != nil {
				klog.Errorf("Cannot initialize watching events. Reason %v", err)
				return
			}
			watchEvictionEvents(watchInterface.ResultChan(), observer)
		}
		for {
			watchEvictionEventsOnce()
			// Wait between attempts, retrying too often breaks API server.
			waitTime := wait.Jitter(evictionWatchRetryWait, evictionWatchJitterFactor)
			klog.V(1).Infof("An attempt to watch eviction events finished. Waiting %v before the next one.", waitTime)
			time.Sleep(waitTime)
		}
	}()
}

func watchEvictionEvents(evictedEventChan <-chan watch.Event, observer oom.Observer) {
	for {
		evictedEvent, ok := <-evictedEventChan
		if !ok {
			klog.V(3).Infof("Eviction event chan closed")
			return
		}
		if evictedEvent.Type == watch.Added {
			evictedEvent, ok := evictedEvent.Object.(*apiv1.Event)
			if !ok {
				continue
			}
			observer.OnEvent(evictedEvent)
		}
	}
}

// Creates clients watching pods: PodLister (listing only not terminated pods).
func newPodClients(kubeClient kube_client.Interface, resourceEventHandler cache.ResourceEventHandler, namespace string) v1lister.PodLister {
	// We are interested in pods which are Running or Unknown (in case the pod is
	// running but there are some transient errors we don't want to delete it from
	// our model).
	// We don't want to watch Pending pods because they didn't generate any usage
	// yet.
	// Succeeded and Failed failed pods don't generate any usage anymore but we
	// don't necessarily want to immediately delete them.
	selector := fields.ParseSelectorOrDie("status.phase!=" + string(apiv1.PodPending))
	podListWatch := cache.NewListWatchFromClient(kubeClient.CoreV1().RESTClient(), "pods", namespace, selector)
	indexer, controller := cache.NewIndexerInformer(
		podListWatch,
		&apiv1.Pod{},
		time.Hour,
		resourceEventHandler,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	podLister := v1lister.NewPodLister(indexer)
	stopCh := make(chan struct{})
	go controller.Run(stopCh)
	return podLister
}

// NewPodListerAndOOMObserver creates pair of pod lister and OOM observer.
func NewPodListerAndOOMObserver(kubeClient kube_client.Interface, namespace string) (v1lister.PodLister, oom.Observer) {
	oomObserver := oom.NewObserver()
	podLister := newPodClients(kubeClient, oomObserver, namespace)
	WatchEvictionEventsWithRetries(kubeClient, oomObserver, namespace)
	return podLister, oomObserver
}

type clusterStateFeeder struct {
	coreClient                     corev1.CoreV1Interface
	specClient                     spec.SpecClient
	metricsClient                  metrics.MetricsClient
	oomChan                        <-chan oom.OomInfo
	recommendationCheckpointClient rec_api.RecommendationCheckpointsGetter
	recommendationLister           rec_lister.RecommendationLister
	clusterState                   *model.ClusterState
	selectorFetcher                target.RecommendationTargetSelectorFetcher
	memorySaveMode                 bool
	controllerFetcher              controllerfetcher.ControllerFetcher
}

func (feeder *clusterStateFeeder) InitFromHistoryProvider(historyProvider history.HistoryProvider) {
	klog.V(3).Info("Initializing Recommendation from history provider")
	clusterHistory, err := historyProvider.GetClusterHistory()
	if err != nil {
		klog.Errorf("Cannot get cluster history: %v", err)
	}
	for podID, podHistory := range clusterHistory {
		// 【Enhance】从prometheus查询回来的metrics可能包含已经被删除的pod信息，此处可过滤掉
		if len(podHistory.LastLabels) == 0 {
			continue
		}
		klog.V(4).Infof("Adding pod %v with labels %v", podID, podHistory.LastLabels)
		feeder.clusterState.AddOrUpdatePod(podID, podHistory.LastLabels, apiv1.PodUnknown)
		for containerName, sampleList := range podHistory.Samples {
			containerID := model.ContainerID{
				PodID:         podID,
				ContainerName: containerName,
			}
			if err = feeder.clusterState.AddOrUpdateContainer(containerID, nil); err != nil {
				klog.Warningf("Failed to add container %+v, Reason: %+v", containerID, err)
			}
			klog.V(4).Infof("Adding %d samples for container %v", len(sampleList), containerID)
			for _, sample := range sampleList {
				if err := feeder.clusterState.AddSample(
					&model.ContainerUsageSampleWithKey{
						ContainerUsageSample: sample,
						Container:            containerID,
					}); err != nil {
					klog.Warningf("Error adding metric sample for container %v: %v", containerID, err)
				}
			}
		}
	}
}

func (feeder *clusterStateFeeder) setRecommendationCheckpoint(checkpoint *recv1alpha1.RecommendationCheckpoint) error {
	recID := model.RecommendationID{Namespace: checkpoint.Namespace, RecommendationName: checkpoint.Spec.RecommendationObjectName}
	rec, exists := feeder.clusterState.Recommendations[recID]
	if !exists {
		return fmt.Errorf("cannot load checkpoint to missing Recommendation object %+v", recID)
	}

	cs := model.NewAggregateContainerState()
	err := cs.LoadFromCheckpoint(&checkpoint.Status)
	if err != nil {
		return fmt.Errorf("cannot load checkpoint for Recommendation %+v. Reason: %v", rec.ID, err)
	}
	rec.ContainersInitialAggregateState[checkpoint.Spec.ContainerName] = cs
	return nil
}

func (feeder *clusterStateFeeder) InitFromCheckpoints() {
	klog.V(3).Info("Initializing Recommendation from checkpoints")
	feeder.LoadRecommendations()

	namespaces := make(map[string]bool)
	for _, v := range feeder.clusterState.Recommendations {
		namespaces[v.ID.Namespace] = true
	}

	for namespace := range namespaces {
		klog.V(3).Infof("Fetching checkpoints from namespace %s", namespace)
		checkpointList, err := feeder.recommendationCheckpointClient.RecommendationCheckpoints(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			klog.Errorf("Cannot list Recommendation checkpoints from namespace %v. Reason: %+v", namespace, err)
		}
		for _, checkpoint := range checkpointList.Items {

			klog.V(3).Infof("Loading Recommendation %s/%s checkpoint for %s", checkpoint.ObjectMeta.Namespace, checkpoint.Spec.RecommendationObjectName, checkpoint.Spec.ContainerName)
			err = feeder.setRecommendationCheckpoint(&checkpoint)
			if err != nil {
				klog.Errorf("Error while loading checkpoint. Reason: %+v", err)
			}
		}
	}
}

func (feeder *clusterStateFeeder) GarbageCollectCheckpoints() {
	klog.V(3).Info("Starting garbage collection of checkpoints")
	feeder.LoadRecommendations()

	namspaceList, err := feeder.coreClient.Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Cannot list namespaces. Reason: %+v", err)
		return
	}

	for _, namespaceItem := range namspaceList.Items {
		namespace := namespaceItem.Name
		checkpointList, err := feeder.recommendationCheckpointClient.RecommendationCheckpoints(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			klog.Errorf("Cannot list Recommendation checkpoints from namespace %v. Reason: %+v", namespace, err)
		}
		for _, checkpoint := range checkpointList.Items {
			recID := model.RecommendationID{Namespace: checkpoint.Namespace, RecommendationName: checkpoint.Spec.RecommendationObjectName}
			_, exists := feeder.clusterState.Recommendations[recID]
			if !exists {
				err = feeder.recommendationCheckpointClient.RecommendationCheckpoints(namespace).Delete(context.TODO(), checkpoint.Name, metav1.DeleteOptions{})
				if err == nil {
					klog.V(3).Infof("Orphaned Recommendation checkpoint cleanup - deleting %v/%v.", namespace, checkpoint.Name)
				} else {
					klog.Errorf("Cannot delete Recommendation checkpoint %v/%v. Reason: %+v", namespace, checkpoint.Name, err)
				}
			}
		}
	}
}

// LoadRecommendations Fetch Recommendation objects and load them into the clusterstate.
func (feeder *clusterStateFeeder) LoadRecommendations() {
	// List Recommendation API objects.
	recCRDs, err := feeder.recommendationLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("Cannot list Recommendations. Reason: %+v", err)
		return
	}
	klog.V(3).Infof("Fetched %d Recommendations.", len(recCRDs))
	// Add or update existing Recommendations in the model.
	recKeys := make(map[model.RecommendationID]bool)
	for _, recCRD := range recCRDs {
		recID := model.RecommendationID{
			Namespace:          recCRD.Namespace,
			RecommendationName: recCRD.Name,
		}

		selector, conditions := feeder.getSelector(recCRD)
		klog.Infof("Using selector %s for Recommendation %s/%s", selector.String(), recCRD.Namespace, recCRD.Name)

		if feeder.clusterState.AddOrUpdateRecommendation(recCRD, selector) == nil {
			// Successfully added Recommendation to the model.
			recKeys[recID] = true

			for _, condition := range conditions {
				if condition.delete {
					delete(feeder.clusterState.Recommendations[recID].Conditions, condition.conditionType)
				} else {
					feeder.clusterState.Recommendations[recID].Conditions.Set(condition.conditionType, true, "", condition.message)
				}
			}
			if recCRD.Spec.WorkloadRef != nil {
				key := &controllerfetcher.ControllerKeyWithAPIVersion{
					ControllerKey: controllerfetcher.ControllerKey{
						Namespace: recCRD.Namespace,
						Kind:      recCRD.Spec.WorkloadRef.Kind,
						Name:      recCRD.Spec.WorkloadRef.Name,
					},
					ApiVersion: recCRD.Spec.WorkloadRef.APIVersion,
				}
				if workloadReqs, exist := feeder.controllerFetcher.GetWorkloadContainersRequest(key); exist {
					feeder.clusterState.OriginWorkloads[recID] = &model.WorkloadInfo{
						ContainerRequests: workloadReqs,
					}
				}
			}
		}
	}
	// Delete non-existent Recommendations from the model.
	for recID := range feeder.clusterState.Recommendations {
		if _, exists := recKeys[recID]; !exists {
			klog.V(3).Infof("Deleting Recommendation %v", recID)
			feeder.clusterState.DeleteRecommendation(recID)
		}
	}
	feeder.clusterState.ObservedRecommendations = recCRDs
}

// LoadPods Load pod into the cluster state.
func (feeder *clusterStateFeeder) LoadPods() {
	podSpecs, err := feeder.specClient.GetPodSpecs()
	if err != nil {
		klog.Errorf("Cannot get SimplePodSpecs. Reason: %+v", err)
	}
	pods := make(map[model.PodID]*spec.BasicPodSpec)
	for _, spec := range podSpecs {
		pods[spec.ID] = spec
	}
	for key := range feeder.clusterState.Pods {
		if _, exists := pods[key]; !exists {
			klog.V(3).Infof("Deleting Pod %v", key)
			feeder.clusterState.DeletePod(key)
		}
	}
	for _, pod := range pods {
		if feeder.memorySaveMode && !feeder.matchesRecommendation(pod) {
			continue
		}
		feeder.clusterState.AddOrUpdatePod(pod.ID, pod.PodLabels, pod.Phase)
		for _, container := range pod.Containers {
			if err = feeder.clusterState.AddOrUpdateContainer(container.ID, container.Request); err != nil {
				klog.Warningf("Failed to add container %+v. Reason: %+v", container.ID, err)
			}
		}
	}
}

func (feeder *clusterStateFeeder) LoadRealTimeMetrics() {
	containersMetrics, err := feeder.metricsClient.GetContainersMetrics()
	if err != nil {
		klog.Errorf("Cannot get ContainerMetricsSnapshot from MetricsClient. Reason: %+v", err)
	}

	sampleCount := 0
	droppedSampleCount := 0
	for _, containerMetrics := range containersMetrics {
		for _, sample := range newContainerUsageSamplesWithKey(containerMetrics) {
			if err := feeder.clusterState.AddSample(sample); err != nil {
				// Not all pod states are tracked in memory saver mode
				if _, isKeyError := err.(model.KeyError); isKeyError && feeder.memorySaveMode {
					continue
				}
				klog.Warningf("Error adding metric sample for container %v: %v", sample.Container, err)
				droppedSampleCount++
			} else {
				sampleCount++
			}
		}
	}
	klog.V(3).Infof("ClusterSpec fed with #%v ContainerUsageSamples for #%v containers. Dropped #%v samples.", sampleCount, len(containersMetrics), droppedSampleCount)
Loop:
	for {
		select {
		case oomInfo := <-feeder.oomChan:
			klog.V(3).Infof("OOM detected %+v", oomInfo)
			if err = feeder.clusterState.RecordOOM(oomInfo.ContainerID, oomInfo.Timestamp, oomInfo.Memory); err != nil {
				klog.Warningf("Failed to record OOM %+v. Reason: %+v", oomInfo, err)
			}
		default:
			break Loop
		}
	}
	prommetrics.RecordAggregateContainerStatesCount(feeder.clusterState.StateMapSize())
}

func (feeder *clusterStateFeeder) matchesRecommendation(pod *spec.BasicPodSpec) bool {
	for recKey, rec := range feeder.clusterState.Recommendations {
		podLabels := labels.Set(pod.PodLabels)
		if recKey.Namespace == pod.ID.Namespace && rec.PodSelector.Matches(podLabels) {
			return true
		}
	}
	return false
}

func newContainerUsageSamplesWithKey(metrics *metrics.ContainerMetricsSnapshot) []*model.ContainerUsageSampleWithKey {
	var samples []*model.ContainerUsageSampleWithKey

	for metricName, resourceAmount := range metrics.Usage {
		sample := &model.ContainerUsageSampleWithKey{
			Container: metrics.ID,
			ContainerUsageSample: model.ContainerUsageSample{
				MeasureStart: metrics.SnapshotTime,
				Resource:     metricName,
				Usage:        resourceAmount,
			},
		}
		samples = append(samples, sample)
	}
	return samples
}

type condition struct {
	conditionType recv1alpha1.RecommendationConditionType
	delete        bool
	message       string
}

func (feeder *clusterStateFeeder) validateTargetRef(rec *recv1alpha1.Recommendation) (bool, condition) {
	k := controllerfetcher.ControllerKeyWithAPIVersion{
		ControllerKey: controllerfetcher.ControllerKey{
			Namespace: rec.Namespace,
			Kind:      rec.Spec.WorkloadRef.Kind,
			Name:      rec.Spec.WorkloadRef.Name,
		},
		ApiVersion: rec.Spec.WorkloadRef.APIVersion,
	}
	top, err := feeder.controllerFetcher.FindTopMostWellKnownOrScalable(&k)
	if err != nil {
		return false, condition{conditionType: recv1alpha1.ConfigUnsupported, delete: false, message: fmt.Sprintf("Error checking if target is a topmost well-known or scalable controller: %s", err)}
	}
	if top == nil {
		return false, condition{conditionType: recv1alpha1.ConfigUnsupported, delete: false, message: fmt.Sprintf("Unknown error during checking if target is a topmost well-known or scalable controller: %s", err)}
	}
	if *top != k {
		return false, condition{conditionType: recv1alpha1.ConfigUnsupported, delete: false, message: "The targetRef controller has a parent but it should point to a topmost well-known or scalable controller"}
	}
	return true, condition{}
}

func (feeder *clusterStateFeeder) getSelector(rec *recv1alpha1.Recommendation) (labels.Selector, []condition) {
	var selector labels.Selector
	var fetchErr error
	if rec.Spec.Selector != nil {
		selector, fetchErr = metav1.LabelSelectorAsSelector(rec.Spec.Selector)
	} else {
		selector, fetchErr = feeder.selectorFetcher.Fetch(rec)
	}

	if selector != nil {
		if rec.Spec.WorkloadRef != nil {
			validTargetRef, unsupportedCondition := feeder.validateTargetRef(rec)
			if !validTargetRef {
				return labels.Nothing(), []condition{
					unsupportedCondition,
					{conditionType: recv1alpha1.ConfigDeprecated, delete: true},
				}
			}
		}

		return selector, []condition{
			{conditionType: recv1alpha1.ConfigUnsupported, delete: true},
			{conditionType: recv1alpha1.ConfigDeprecated, delete: true},
		}
	}
	msg := "Cannot read targetRef"
	if fetchErr != nil {
		klog.Errorf("Cannot get target selector from Recommendation. Reason: %+v", fetchErr)
		msg = fmt.Sprintf("Cannot read target selector. Reason: %s", fetchErr.Error())
	}
	return labels.Nothing(), []condition{
		{conditionType: recv1alpha1.ConfigUnsupported, delete: false, message: msg},
		{conditionType: recv1alpha1.ConfigDeprecated, delete: true},
	}
}
