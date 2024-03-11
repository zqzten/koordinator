package recommendationprofile

import (
	"context"
	"fmt"
	"strings"

	uuid "github.com/satori/go.uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"gitlab.alibaba-inc.com/cos/recommender/pkg/common"
	autov1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
)

const (
	workloadNameLabelMaxLength = 63
)

func GenerateRecommendationForWorkloadIfNeed(redProfile *autov1alpha1.RecommendationProfile,
	owner *metav1.OwnerReference, namespace string, recClient client.Client) error {
	apiVersion := strings.ReplaceAll(owner.APIVersion, "/", "-")

	shortName := generateShortNameForWorkload(owner.Name)

	recWorkloadLabels := map[string]string{
		autov1alpha1.RecommendationWorkloadApiVersionKey: apiVersion,
		autov1alpha1.RecommendationWorkloadKindKey:       owner.Kind,
		autov1alpha1.RecommendationWorkloadNameKey:       shortName,
	}
	recWorkloadAnnotations := map[string]string{
		autov1alpha1.RecommendationWorkloadNameKey: owner.Name,
	}

	recList := &autov1alpha1.RecommendationList{}
	opt := &client.ListOptions{
		LabelSelector: labels.Set(recWorkloadLabels).AsSelector(),
		Namespace:     namespace,
	}
	err := recClient.List(context.TODO(), recList, opt)
	if err != nil {
		return err
	}

	workloadRecList := getRecommendationByWorkloadName(recList, owner.Name)

	if workloadRecList != nil && len(workloadRecList.Items) > 0 {
		klog.V(5).Infof("recommendation with labels %v has already existed", recWorkloadLabels)
		if updateErr := UpdateRecommendationsAttrsIfNeeded(redProfile, workloadRecList, recClient); updateErr != nil {
			return updateErr
		} else {
			return &common.AlreadyExists{}
		}
	}

	klog.V(5).Infof("recommendation with labels %v doesn't exist so it's supposed to create", recWorkloadLabels)
	// Recommendation 不存在，需要新建
	name := uuid.Must(uuid.NewV4(), nil).String()
	rec := &autov1alpha1.Recommendation{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      recWorkloadLabels,
			Annotations: recWorkloadAnnotations,
		},
	}
	rec.Spec.WorkloadRef = &autov1alpha1.CrossVersionObjectReference{
		Kind:       owner.Kind,
		Name:       owner.Name,
		APIVersion: owner.APIVersion,
	}
	SetProfileAttrToRecommendation(redProfile, rec)

	if err = recClient.Create(context.TODO(), rec); err != nil {
		return err
	}
	return nil
}

func GenerateRecommendationWithoutWorkloadIfNeed(pod *corev1.Pod, redProfile *autov1alpha1.RecommendationProfile, recClient client.Client) error {
	workloadRefLabelKeys := redProfile.Spec.WorkloadRefLabelKeys
	recommendationLabels := map[string]string{}
	for _, workloadRefLabelKey := range workloadRefLabelKeys {
		if value, ok := pod.Labels[workloadRefLabelKey]; ok {
			recommendationLabels[workloadRefLabelKey] = value
		} else {
			return fmt.Errorf("pod %s/%s dosen't have a label with key:%s", pod.Namespace, pod.Name, workloadRefLabelKey)
		}
	}

	recList := &autov1alpha1.RecommendationList{}
	opt := &client.ListOptions{
		LabelSelector: labels.Set(recommendationLabels).AsSelector(),
		Namespace:     pod.Namespace,
	}
	err := recClient.List(context.TODO(), recList, opt)
	if err != nil {
		return err
	}
	if len(recList.Items) > 0 {
		klog.V(5).Infof("recommendation with labels %v has already existed, update attributes", recommendationLabels)
		if updateErr := UpdateRecommendationsAttrsIfNeeded(redProfile, recList, recClient); updateErr != nil {
			return updateErr
		} else {
			return &common.AlreadyExists{}
		}
	}

	klog.V(5).Infof("recommendation with labels %v doesn't exist so it's supposed to create", recommendationLabels)
	// Recommendation 不存在，需要新建
	name := uuid.Must(uuid.NewV4(), nil).String()
	rec := &autov1alpha1.Recommendation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: pod.Namespace,
			Labels:    recommendationLabels,
		},
	}
	// 通过selector模式
	rec.Spec.Selector = metav1.SetAsLabelSelector(recommendationLabels)
	SetProfileAttrToRecommendation(redProfile, rec)

	if err = recClient.Create(context.TODO(), rec); err != nil {
		return err
	}
	return nil
}

func UpdateRecommendationsAttrsIfNeeded(redProfile *autov1alpha1.RecommendationProfile, recList *autov1alpha1.RecommendationList,
	redClient client.Client) error {
	for i := range recList.Items {
		rec := &recList.Items[i]
		SetProfileAttrToRecommendation(redProfile, rec)
		err := redClient.Update(context.TODO(), rec)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func SetProfileAttrToRecommendation(redProfile *autov1alpha1.RecommendationProfile, rec *autov1alpha1.Recommendation) {
	if redProfile == nil || rec == nil {
		return
	}
	if redProfile.Labels != nil {
		if rec.Labels == nil {
			rec.Labels = map[string]string{}
		}
		rec.Labels[autov1alpha1.RecommendationProfileNameKey] = redProfile.Name
		rec.Labels[common.AnnotationRecommenderWorkloadNamespace] = rec.Namespace
		if profileAlias, exist := redProfile.Labels[common.LabelKeyRecommenderProfileAlias]; exist {
			rec.Labels[common.LabelKeyRecommenderProfileAlias] = profileAlias
		}
		if recommendWarmUpHours, exist := redProfile.Annotations[common.AnnotationRecommenderWarmUpHours]; exist {
			rec.Annotations[common.AnnotationRecommenderWarmUpHours] = recommendWarmUpHours
		}
	}
	if redProfile.Annotations != nil {
		if rec.Annotations == nil {
			rec.Annotations = map[string]string{}
		}
		if safetyRatio, exist := redProfile.Annotations[common.AnnotationKeyRecommenderSaftyMargin]; exist {
			rec.Annotations[common.AnnotationKeyRecommenderSaftyMargin] = safetyRatio
		}
	}
}

func getRecommendationByWorkloadName(recList *autov1alpha1.RecommendationList, workloadName string) *autov1alpha1.RecommendationList {
	if recList == nil || len(recList.Items) == 0 {
		return nil
	}

	result := &autov1alpha1.RecommendationList{
		Items: make([]autov1alpha1.Recommendation, 0),
	}
	for _, rec := range recList.Items {
		if value, exist := rec.Annotations[autov1alpha1.RecommendationWorkloadNameKey]; exist && value == workloadName {
			// new generated workload, got full name from annotation
			result.Items = append(result.Items, rec)
		} else if value, exist := rec.Labels[autov1alpha1.RecommendationWorkloadNameKey]; exist && value == workloadName {
			// workload generated before, got full name from label
			result.Items = append(result.Items, rec)
		}
	}
	return result
}

// cut the workload name if it exceeds the limit of label value(63 characters)
func generateShortNameForWorkload(workloadName string) string {
	shortName := workloadName
	if len(workloadName) > workloadNameLabelMaxLength {
		shortName = string([]byte(workloadName)[0:workloadNameLabelMaxLength])
	}
	return shortName
}
