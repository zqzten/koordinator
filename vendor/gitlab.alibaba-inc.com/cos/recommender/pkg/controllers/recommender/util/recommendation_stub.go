package util

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	core "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"gitlab.alibaba-inc.com/cos/recommender/pkg/common"
	recv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	rec_clientset "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned"
	"gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned/typed/autoscaling/v1alpha1"
	rec_lister "gitlab.alibaba-inc.com/cos/unified-resource-api/client/listers/autoscaling/v1alpha1"
)

// RecommendationWithSelector is a pair of VPA and its selector.
type RecommendationWithSelector struct {
	Recommendation *recv1alpha1.Recommendation
	Selector       labels.Selector
}

type patchRecord struct {
	Op    string      `json:"op,inline"`
	Path  string      `json:"path,inline"`
	Value interface{} `json:"value"`
}

func patchRecommendation(recClient v1alpha1.RecommendationInterface, recommendationName string, patches []patchRecord) (result *recv1alpha1.Recommendation, err error) {
	bytes, err := json.Marshal(patches)
	if err != nil {
		klog.Errorf("Cannot marshal Recommendation status patches %+v. Reason: %+v", patches, err)
		return
	}

	return recClient.Patch(context.TODO(), recommendationName, types.JSONPatchType, bytes, meta.PatchOptions{})
}

func UpdateWorkloadStatusIfNeeded(client client.Client, observeRec *recv1alpha1.Recommendation,
	newStatus *common.RecommenderWorkloadStatus) error {
	oldStatus, _ := common.GetRecommenderWorkloadStatus(observeRec.Annotations)
	if apiequality.Semantic.DeepEqual(oldStatus, newStatus) {
		return nil
	}
	if observeRec.Labels == nil {
		observeRec.Labels = map[string]string{}
	}
	observeRec.Labels[common.AnnotationRecommenderWorkloadNamespace] = observeRec.Namespace
	observeRec.Labels[common.LabelKeyRecommenderCPUOperation] = common.GenerateRecommenderOperation(*newStatus.CPUAdjustDegree)
	observeRec.Labels[common.LabelKeyRecommenderMemoryOperation] = common.GenerateRecommenderOperation(*newStatus.MemoryAdjustDegree)
	newStatusStr, err := json.Marshal(newStatus)
	if err != nil {
		return err
	}
	if observeRec.Annotations == nil {
		observeRec.Annotations = map[string]string{}
	}
	observeRec.Annotations[common.AnnotationRecommenderWorkloadStatus] = string(newStatusStr)
	updateErr := client.Update(context.TODO(), observeRec)
	if updateErr == nil {
		klog.Infof("update recommendation %v/%v workload status succeed", observeRec.Namespace, observeRec.Name)
	} else {
		klog.Warningf("update recommendation %v/%v workload status failed, error %v",
			observeRec.Namespace, observeRec.Name, updateErr)
	}
	return updateErr
}

func UpdateStatusRecommendationIfNeeded(client client.Client, observeRec *recv1alpha1.Recommendation,
	newStatus *recv1alpha1.RecommendationStatus) error {
	if !apiequality.Semantic.DeepEqual(observeRec.Status, &newStatus) {
		observeRec.Status = *newStatus
		return client.Status().Update(context.TODO(), observeRec)
	}
	return nil
}

// UpdateRecommendationStatusIfNeeded updates the status field of the Recommendation API object.
func UpdateRecommendationStatusIfNeeded(recClient v1alpha1.RecommendationInterface, recommendationName string, newStatus,
	oldStatus *recv1alpha1.RecommendationStatus) (result *recv1alpha1.Recommendation, err error) {
	patches := []patchRecord{{
		Op:    "replace",
		Path:  "/status",
		Value: *newStatus,
	}}

	if !apiequality.Semantic.DeepEqual(*oldStatus, *newStatus) {
		return patchRecommendation(recClient, recommendationName, patches)
	}
	return nil, nil
}

// NewRecommendationsLister returns RecommendationLister configured to fetch all Recommendation objects from namespace,
// set namespace to k8sapiv1.NamespaceAll to select all namespaces.
// The method blocks until recommendationLister is initially populated.
func NewRecommendationsLister(recommendationClient *rec_clientset.Clientset, stopChannel <-chan struct{}, namespace string) rec_lister.RecommendationLister {
	vpaListWatch := cache.NewListWatchFromClient(recommendationClient.AutoscalingV1alpha1().RESTClient(), "recommendations", namespace, fields.Everything())
	indexer, controller := cache.NewIndexerInformer(vpaListWatch,
		&recv1alpha1.Recommendation{},
		1*time.Hour,
		&cache.ResourceEventHandlerFuncs{},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	recLister := rec_lister.NewRecommendationLister(indexer)
	go controller.Run(stopChannel)
	if !cache.WaitForCacheSync(make(chan struct{}), controller.HasSynced) {
		klog.Fatalf("Failed to sync Recommendation cache during initialization")
	} else {
		klog.Info("Initial Recommendation synced successfully")
	}
	return recLister
}

// PodMatchesRecommendation returns true iff the vpaWithSelector matches the Pod.
func PodMatchesRecommendation(pod *core.Pod, recommendationWithSelector *RecommendationWithSelector) bool {
	return PodLabelsMatchRecommendation(pod.Namespace, labels.Set(pod.GetLabels()), recommendationWithSelector.Recommendation.Namespace, recommendationWithSelector.Selector)
}

// PodLabelsMatchRecommendation returns true iff the recommenderWithSelector matches the pod labels.
func PodLabelsMatchRecommendation(podNamespace string, labels labels.Set, recNamespace string, recSelector labels.Selector) bool {
	if podNamespace != recNamespace {
		return false
	}
	return recSelector.Matches(labels)
}

// stronger returns true iff a is before b in the order to control a Pod (that matches both Recommendations).
func stronger(a, b *recv1alpha1.Recommendation) bool {
	// Assume a is not nil and each valid object is before nil object.
	if b == nil {
		return true
	}
	// Compare creation timestamps of the Recommendation objects. This is the clue of the stronger logic.
	var aTime, bTime meta.Time
	aTime = a.GetCreationTimestamp()
	bTime = b.GetCreationTimestamp()
	if !aTime.Equal(&bTime) {
		return aTime.Before(&bTime)
	}
	// If the timestamps are the same (unlikely, but possible e.g. in test environments): compare by name to have a complete deterministic order.
	return a.GetName() < b.GetName()
}

// GetControllingRecommendationForPod chooses the earliest created Recommendation from the input list that matches the given Pod.
func GetControllingRecommendationForPod(pod *core.Pod, vpas []*RecommendationWithSelector) *RecommendationWithSelector {
	var controlling *RecommendationWithSelector
	var controllingVpa *recv1alpha1.Recommendation
	// Choose the strongest Recommendation from the ones that match this Pod.
	for _, recWithSelector := range vpas {
		if PodMatchesRecommendation(pod, recWithSelector) && stronger(recWithSelector.Recommendation, controllingVpa) {
			controlling = recWithSelector
			controllingVpa = controlling.Recommendation
		}
	}
	return controlling
}

// CreateOrUpdateRecommendationCheckpoint updates the status field of the Recommendation Checkpoint API object.
// If object doesn't exits it is created.
func CreateOrUpdateRecommendationCheckpoint(recCheckpointClient v1alpha1.RecommendationCheckpointInterface,
	recCheckpoint *recv1alpha1.RecommendationCheckpoint) error {
	patches := make([]patchRecord, 0)
	patches = append(patches, patchRecord{
		Op:    "replace",
		Path:  "/status",
		Value: recCheckpoint.Status,
	})
	bytes, err := json.Marshal(patches)
	if err != nil {
		return fmt.Errorf("can not marshal Recommendation checkpoint status patches %+v. Reason: %+v", patches, err)
	}
	_, err = recCheckpointClient.Patch(context.TODO(), recCheckpoint.ObjectMeta.Name, types.JSONPatchType, bytes, meta.PatchOptions{})
	if err != nil && strings.Contains(err.Error(), fmt.Sprintf("\"%s\" not found", recCheckpoint.ObjectMeta.Name)) {
		_, err = recCheckpointClient.Create(context.TODO(), recCheckpoint, meta.CreateOptions{})
	}
	if err != nil {
		return fmt.Errorf("can not save checkpoint for vpa %v container %v. Reason: %+v", recCheckpoint.ObjectMeta.Name, recCheckpoint.Spec.ContainerName, err)
	}
	return nil
}
