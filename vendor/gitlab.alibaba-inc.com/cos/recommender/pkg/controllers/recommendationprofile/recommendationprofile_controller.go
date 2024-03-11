package recommendationprofile

import (
	"context"
	"fmt"
	"reflect"
	"time"

	apikruisev1alpha1 "github.com/openkruise/kruise-api/apps/v1alpha1"
	apikruisev1beta1 "github.com/openkruise/kruise-api/apps/v1beta1"
	kruisev1alpha1 "github.com/openkruise/kruise-api/client/listers/apps/v1alpha1"
	kruisev1beta1 "github.com/openkruise/kruise-api/client/listers/apps/v1beta1"
	apiappsv1 "k8s.io/api/apps/v1"
	apibatchv1 "k8s.io/api/batch/v1"
	apibatchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	appsv1 "k8s.io/client-go/listers/apps/v1"
	batchv1 "k8s.io/client-go/listers/batch/v1"
	batchv1beta1 "k8s.io/client-go/listers/batch/v1beta1"
	v1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"gitlab.alibaba-inc.com/cos/recommender/pkg/common"
	autov1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
)

// RedProfileReconciler reconciles a RecommendationProfile object
type RedProfileReconciler struct {
	client.Client
	PodLister                 v1.PodLister
	NamespaceLister           v1.NamespaceLister
	RCLister                  v1.ReplicationControllerLister
	RSLister                  appsv1.ReplicaSetLister
	DeploymentLister          appsv1.DeploymentLister
	StatefulSetLister         appsv1.StatefulSetLister
	DaemonSetLister           appsv1.DaemonSetLister
	JobLister                 batchv1.JobLister
	CronJobLister             batchv1beta1.CronJobLister
	CloneSetList              kruisev1alpha1.CloneSetLister
	AdvancedStatefulSetLister kruisev1beta1.StatefulSetLister
	AdvancedDaemonSetLister   kruisev1alpha1.DaemonSetLister
	AdvancedCronJobLister     kruisev1alpha1.AdvancedCronJobLister

	TimerEvents chan event.GenericEvent
}

var _ reconcile.Reconciler = &RedProfileReconciler{}

// +kubebuilder:rbac:groups=autoscaling.alibabacloud.com,resources=recommendationprofiles;recommendations,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments;replicasets;statefulsets;daemonsets,verbs=get;list
// +kubebuilder:rabc:group="",reources=pods;namespaces;replicationcontrollers,verbs=get;list
// +kubebuilder:rabc:group=batch,resources=jobs;cronjobs,verbs=get;list

func (r *RedProfileReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// 查询 RecommendationProfile
	redProfile := &autov1alpha1.RecommendationProfile{}
	redProfileKey := request.NamespacedName
	err := r.Client.Get(context.TODO(), redProfileKey, redProfile)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(5).Infof("[RecommendationProfile] recommendationProfile %s doesn't  exist, error: %v", request.Name, err)
			return reconcile.Result{}, nil
		}
		klog.Errorf("[RecommendationProfile]  failed to find recommendationProfile %v, error: %v", request.Name, err)
		return reconcile.Result{Requeue: true}, err
	}
	// 对于没有workload管理的pod，通过redProfile.workloadRefLabelKeys来指定哪些pod label key 与recommendation进行关联
	if redProfile.Spec.Type == autov1alpha1.Pod {
		if err = r.HandleProfileTypeAsPod(redProfile); err != nil {
			return reconcile.Result{Requeue: true}, err
		}
		return reconcile.Result{}, nil
	}

	// 对于有workload管理的pod，通过redProfile.controllerKinds来指定为哪些workload生成recommendation
	if redProfile.Spec.Type == autov1alpha1.Controller || len(redProfile.Spec.Type) == 0 {
		if err = r.HandleProfileTypeAsController(redProfile); err != nil {
			return reconcile.Result{Requeue: true}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *RedProfileReconciler) HandleProfileTypeAsPod(redProfile *autov1alpha1.RecommendationProfile) error {
	// 查询引用该profile的pod
	podList, err := r.QueryPodsUseThisProfile(redProfile.Name, redProfile.Spec.EnabledNamespaces, redProfile.Spec.DisabledNamespaces)
	if err != nil {
		klog.Errorf("[RecommendationProfile], fail to query the pod list that uses this profile %s, err:%v", redProfile.Name, err)
		return err
	}
	// 为pod生成对应的recommendation
	if err = r.GenerateRecommendationForPod(redProfile, podList); err != nil {
		klog.Errorf("[RecommendationProfile], fail to path app metric name to pod, err:%v", err)
		return err
	}

	return nil
}

func (r *RedProfileReconciler) HandleProfileTypeAsController(redProfile *autov1alpha1.RecommendationProfile) error {
	for _, kind := range redProfile.Spec.ControllerKinds {
		objList, err := r.QueryWorkloadRefProfile(kind, redProfile.Spec.EnabledNamespaces, redProfile.Spec.DisabledNamespaces)
		if err != nil {
			klog.Errorf("Fail to fetch workload, err:%v", err)
			return err
		}
		if len(objList) == 0 {
			continue
		}
		if err = r.GenerateRecommendationForWorkload(redProfile, objList); err != nil {
			klog.Errorf("[RecommendationProfile] fali to update app metric name to pod, err:%v", err)
			return err
		}
	}

	return nil
}

func (r *RedProfileReconciler) QueryPodsUseThisProfile(profileName string, enabledNamespaces, disabledNamespaces []string) ([]*corev1.Pod, error) {
	req, err := labels.NewRequirement(autov1alpha1.RecommendationProfileNameKey, selection.Equals, []string{profileName})
	if err != nil {
		klog.Errorf("[RecommendationProfile] fali to new requriement, err:%v", err)
		return nil, err
	}
	selector := labels.NewSelector().Add(*req)

	enabledNamespaceSet := common.Slice2Strings(enabledNamespaces)
	disabledNamespaceSet := common.Slice2Strings(disabledNamespaces)
	if disabledNamespaceSet.Has(common.AllNamespace) {
		return nil, nil
	}

	var ret = make([]*corev1.Pod, 0)
	if enabledNamespaceSet.Has(common.AllNamespace) {
		allNamespaces, err := r.NamespaceLister.List(labels.Everything())
		if err != nil {
			klog.Errorf("[RecommendationProfile] fali to get all namesapces, err:%v", err)
			return nil, err
		}

		for _, namespace := range allNamespaces {
			if namespace.Status.Phase == corev1.NamespaceTerminating {
				continue
			}

			if len(disabledNamespaces) > 0 && disabledNamespaceSet.Has(namespace.Name) {
				continue
			}
			// 查询namespace下引用了该profile的pod
			podList, err := r.PodLister.Pods(namespace.Name).List(selector)
			if err != nil {
				klog.Errorf("[RecommendationProfile] fail to get pods in %s with selector:%v", namespace.Name, selector)
				return nil, err
			}
			ret = append(ret, podList...)
		}
		return ret, nil
	}

	if len(enabledNamespaces) > 0 {
		for _, namespace := range enabledNamespaces {
			podList, err := r.PodLister.Pods(namespace).List(selector)
			if err != nil {
				klog.Errorf("[RecommendationProfile] fail to get pods in %s with selector:%v", namespace, selector)
				return nil, err
			}
			ret = append(ret, podList...)
		}
	}
	return ret, nil
}

func (r *RedProfileReconciler) GenerateRecommendationForPod(redProfile *autov1alpha1.RecommendationProfile, podList []*corev1.Pod) error {
	// 检查pod的annotation字段是否有 "recommender.k8s.io/appMetric-name" key
	var err error
	for _, pod := range podList {
		if err = GenerateRecommendationWithoutWorkloadIfNeed(pod, redProfile, r.Client); err != nil {
			if _, ok := err.(*common.AlreadyExists); !ok {
				return err
			}
		}
	}
	return nil
}

func (r *RedProfileReconciler) QueryWorkloadRefProfile(kind autov1alpha1.ControllerKind, enabledNamespaces, disabledNamespaces []string) ([]runtime.Object, error) {
	enabledNamespaceSet := common.Slice2Strings(enabledNamespaces)
	disabledNamespaceSet := common.Slice2Strings(disabledNamespaces)

	if disabledNamespaceSet.Has(common.AllNamespace) {
		return nil, nil
	}

	var ret = make([]runtime.Object, 0)
	if enabledNamespaceSet.Has(common.AllNamespace) {
		allNamespaces, err := r.NamespaceLister.List(labels.Everything())
		if err != nil {
			return nil, err
		}
		for _, namespace := range allNamespaces {
			if namespace.Status.Phase == corev1.NamespaceTerminating {
				continue
			}
			if len(disabledNamespaces) > 0 && disabledNamespaceSet.Has(namespace.Name) {
				continue
			}
			objs, err := r.FetchWorkload(kind, namespace.Name, labels.Everything())
			if err != nil {
				return nil, err
			}
			ret = append(ret, objs...)
		}
		return ret, nil
	}
	if len(enabledNamespaces) > 0 {
		for _, namespace := range enabledNamespaces {
			objs, err := r.FetchWorkload(kind, namespace, labels.Everything())
			if err != nil {
				return nil, err
			}
			ret = append(ret, objs...)
		}
	}
	return ret, nil
}

func (r *RedProfileReconciler) FetchWorkload(kind autov1alpha1.ControllerKind, namespace string, selector labels.Selector) ([]runtime.Object, error) {
	var objs []runtime.Object
	var err error
	switch kind {
	case autov1alpha1.Deployment:
		var deployments []*apiappsv1.Deployment
		deployments, err = r.DeploymentLister.Deployments(namespace).List(selector)
		if err == nil {
			for _, obj := range deployments {
				obj.Kind = string(autov1alpha1.Deployment)
				obj.APIVersion = apiappsv1.SchemeGroupVersion.String()
				objs = append(objs, obj)
			}
		}

	case autov1alpha1.DaemonSet:
		var daemonsets []*apiappsv1.DaemonSet
		daemonsets, err = r.DaemonSetLister.DaemonSets(namespace).List(selector)
		if err == nil {
			for _, obj := range daemonsets {
				obj.Kind = string(autov1alpha1.DaemonSet)
				obj.APIVersion = apiappsv1.SchemeGroupVersion.String()
				objs = append(objs, obj)
			}
		}

	case autov1alpha1.StatefulSet:
		var statefulsets []*apiappsv1.StatefulSet
		statefulsets, err = r.StatefulSetLister.StatefulSets(namespace).List(selector)
		if err == nil {
			for _, obj := range statefulsets {
				obj.Kind = string(autov1alpha1.StatefulSet)
				obj.APIVersion = apiappsv1.SchemeGroupVersion.String()
				objs = append(objs, obj)
			}
		}

	case autov1alpha1.CronJob:
		var cronjobs []*apibatchv1beta1.CronJob
		cronjobs, err = r.CronJobLister.CronJobs(namespace).List(selector)
		if err == nil {
			for _, obj := range cronjobs {
				obj.Kind = string(autov1alpha1.CronJob)
				obj.APIVersion = apibatchv1beta1.SchemeGroupVersion.String()
				objs = append(objs, obj)
			}
		}

	case autov1alpha1.Job:
		var jobs []*apibatchv1.Job
		jobs, err = r.JobLister.Jobs(namespace).List(selector)
		if err == nil {
			for _, obj := range jobs {
				obj.Kind = string(autov1alpha1.Job)
				obj.APIVersion = apibatchv1.SchemeGroupVersion.String()
				objs = append(objs, obj)
			}
		}

	case autov1alpha1.ReplicaSet:
		var replicasets []*apiappsv1.ReplicaSet
		replicasets, err = r.RSLister.ReplicaSets(namespace).List(selector)
		if err == nil {
			for _, obj := range replicasets {
				obj.Kind = string(autov1alpha1.ReplicaSet)
				obj.APIVersion = apiappsv1.SchemeGroupVersion.String()
				objs = append(objs, obj)
			}
		}

	case autov1alpha1.ReplicationController:
		var rcs []*corev1.ReplicationController
		rcs, err = r.RCLister.ReplicationControllers(namespace).List(selector)
		if err == nil {
			for _, obj := range rcs {
				obj.Kind = string(autov1alpha1.ReplicationController)
				obj.APIVersion = corev1.SchemeGroupVersion.String()
				objs = append(objs, obj)
			}
		}

	case autov1alpha1.CloneSet:
		var rcs []*apikruisev1alpha1.CloneSet
		rcs, err = r.CloneSetList.CloneSets(namespace).List(selector)
		if err == nil {
			for _, obj := range rcs {
				obj.Kind = string(autov1alpha1.CloneSet)
				obj.APIVersion = apikruisev1alpha1.SchemeGroupVersion.String()
				objs = append(objs, obj)
			}
		}

	case autov1alpha1.AdvancedDaemonset:
		var rcs []*apikruisev1alpha1.DaemonSet
		rcs, err = r.AdvancedDaemonSetLister.DaemonSets(namespace).List(selector)
		if err == nil {
			for _, obj := range rcs {
				obj.Kind = string(autov1alpha1.AdvancedDaemonset)
				obj.APIVersion = apikruisev1alpha1.SchemeGroupVersion.String()
				objs = append(objs, obj)
			}
		}

	case autov1alpha1.AdvancedStatefulSet:
		var rcs []*apikruisev1beta1.StatefulSet
		rcs, err = r.AdvancedStatefulSetLister.StatefulSets(namespace).List(selector)
		if err == nil {
			for _, obj := range rcs {
				obj.Kind = string(autov1alpha1.AdvancedStatefulSet)
				obj.APIVersion = apikruisev1beta1.SchemeGroupVersion.String()
				objs = append(objs, obj)
			}
		}

	case autov1alpha1.AdvancedCronJob:
		var rcs []*apikruisev1alpha1.AdvancedCronJob
		rcs, err = r.AdvancedCronJobLister.AdvancedCronJobs(namespace).List(selector)
		if err == nil {
			for _, obj := range rcs {
				obj.Kind = string(autov1alpha1.AdvancedCronJob)
				obj.APIVersion = apikruisev1alpha1.SchemeGroupVersion.String()
				objs = append(objs, obj)
			}
		}

	default:
		objs = nil
		err = fmt.Errorf("unknow gv %v", kind)
	}

	return objs, err
}

func (r *RedProfileReconciler) GenerateRecommendationForWorkload(redProfile *autov1alpha1.RecommendationProfile,
	objLists []runtime.Object) error {
	for _, obj := range objLists {
		workloadName, namespace, err := r.GetInfoFromWorkload(obj)
		if err != nil {
			return err
		}
		owner := &metav1.OwnerReference{
			APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
			Name:       workloadName,
		}

		if err = GenerateRecommendationForWorkloadIfNeed(redProfile, owner, namespace, r.Client); err != nil {
			if _, ok := err.(*common.AlreadyExists); !ok {
				klog.Errorf("Fail to Generate Recommendation, err:%v", err)
				return err
			}
		}
	}
	return nil
}

func (r *RedProfileReconciler) GetInfoFromWorkload(obj runtime.Object) (string, string, error) {
	objVal := reflect.ValueOf(obj).Elem()
	if objVal.IsZero() {
		return "", "", fmt.Errorf("fail to get workload")
	}
	name := objVal.FieldByName("Name").String()
	namespace := objVal.FieldByName("Namespace").String()

	return name, namespace, nil
}

func (r *RedProfileReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&autov1alpha1.RecommendationProfile{}).
		Watches(&source.Channel{Source: r.TimerEvents, DestBufferSize: 1}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}

func (r *RedProfileReconciler) Start(c context.Context) error {
	go func() {
		timer := time.NewTicker(10 * time.Minute)
		for {
			select {
			case <-timer.C:
				klog.V(4).Infof("start recommendation profile timer")
				profileList := &autov1alpha1.RecommendationProfileList{}
				if err := r.Client.List(context.TODO(), profileList); err != nil {
					klog.Warningf("list recommendation profile failed, error %v", err)
					continue
				}
				for _, profile := range profileList.Items {
					evt := event.GenericEvent{
						Object: &profile,
					}
					r.TimerEvents <- evt
				}
			}
		}
	}()
	return nil
}

//func (r *RedProfileReconciler) PatchAppMetricNameToPod(pod *corev1.Pod, appMetricName string) error {
//	if value, ok := pod.Annotations[AnnotationAppMetricNameKey]; !ok || value != appMetricName {
//		if pod.Annotations == nil {
//			pod.Annotations = make(map[string]string)
//		}
//		deepcopy := pod.DeepCopy()
//		pod.Annotations[AnnotationAppMetricNameKey] = appMetricName
//		if err := r.Client.Patch(context.TODO(), pod, client.MergeFrom(deepcopy)); err != nil {
//			return err
//		}
//	}
//	return nil
//}

//func (r RedProfileReconciler) UpdateAppMetricNameToWorkloadAndPod(obj runtime.Object, namespace, appMetricName string) error {
//	kind := autov1alpha1.ControllerKind(obj.GetObjectKind().GroupVersionKind().Kind)
//	switch kind {
//	case autov1alpha1.Deployment:
//		deployment := obj.(*apiappsv1.Deployment)
//		deepcopy := deployment.DeepCopy()
//		if deployment.Spec.Template.Annotations == nil {
//			deployment.Spec.Template.Annotations = make(map[string]string)
//		}
//		deployment.Spec.Template.Annotations[AnnotationAppMetricNameKey] = appMetricName
//		if err := r.AddAppMetricNameToPodAndWorkload(deepcopy, deployment, namespace, appMetricName, deployment.Spec.Selector); err != nil {
//			return err
//		}
//	case autov1alpha1.DaemonSet:
//		daemonset := obj.(*apiappsv1.DaemonSet)
//		deepcopy := daemonset.DeepCopy()
//		if daemonset.Spec.Template.Annotations == nil {
//			daemonset.Spec.Template.Annotations = make(map[string]string)
//		}
//		daemonset.Spec.Template.Annotations[AnnotationAppMetricNameKey] = appMetricName
//		if err := r.AddAppMetricNameToPodAndWorkload(deepcopy, daemonset, namespace, appMetricName, daemonset.Spec.Selector); err != nil {
//			return err
//		}
//	case autov1alpha1.StatefulSet:
//		statfulset := obj.(*apiappsv1.StatefulSet)
//		deepcopy := statfulset.DeepCopy()
//		if statfulset.Spec.Template.Annotations == nil {
//			statfulset.Spec.Template.Annotations = make(map[string]string)
//		}
//		statfulset.Spec.Template.Annotations[AnnotationAppMetricNameKey] = appMetricName
//		if err := r.AddAppMetricNameToPodAndWorkload(deepcopy, statfulset, namespace, appMetricName, statfulset.Spec.Selector); err != nil {
//			return err
//		}
//	case autov1alpha1.ReplicaSet:
//		rs := obj.(*apiappsv1.ReplicaSet)
//		deepcopy := rs.DeepCopy()
//		if rs.Spec.Template.Annotations == nil {
//			rs.Spec.Template.Annotations = make(map[string]string)
//		}
//		rs.Spec.Template.Annotations[AnnotationAppMetricNameKey] = appMetricName
//		if err := r.AddAppMetricNameToPodAndWorkload(deepcopy, rs, namespace, appMetricName, rs.Spec.Selector); err != nil {
//			return err
//		}
//	case autov1alpha1.ReplicationController:
//		rc := obj.(*corev1.ReplicationController)
//		deepcopy := rc.DeepCopy()
//		if rc.Spec.Template.Annotations == nil {
//			rc.Spec.Template.Annotations = make(map[string]string)
//		}
//		rc.Spec.Template.Annotations[AnnotationAppMetricNameKey] = appMetricName
//		if err := r.AddAppMetricNameToPodAndWorkload(deepcopy, rc, namespace, appMetricName, metav1.SetAsLabelSelector(rc.Spec.Selector)); err != nil {
//			return err
//		}
//	case autov1alpha1.Job:
//		job := obj.(*apibatchv1.Job)
//		deepcopy := job.DeepCopy()
//		if job.Spec.Template.Annotations == nil {
//			job.Spec.Template.Annotations = make(map[string]string)
//		}
//		job.Spec.Template.Annotations[AnnotationAppMetricNameKey] = appMetricName
//		if err := r.AddAppMetricNameToPodAndWorkload(deepcopy, job, namespace, appMetricName, job.Spec.Selector); err != nil {
//			return err
//		}
//	case autov1alpha1.CronJob:
//		cronjob := obj.(*apibatchv1beta1.CronJob)
//		deepcopy := cronjob.DeepCopy()
//		if cronjob.Spec.JobTemplate.Spec.Template.Annotations == nil {
//			cronjob.Spec.JobTemplate.Spec.Template.Annotations = make(map[string]string)
//		}
//		cronjob.Spec.JobTemplate.Spec.Template.Annotations[AnnotationAppMetricNameKey] = appMetricName
//		if err := r.AddAppMetricNameToPodAndWorkload(deepcopy, cronjob, namespace, appMetricName, metav1.SetAsLabelSelector(cronjob.Spec.JobTemplate.Spec.Template.Labels)); err != nil {
//			return err
//		}
//	}
//	return nil
//}

//func (r RedProfileReconciler) AddAppMetricNameToPodAndWorkload(deepcopy, obj runtime.Object, namespace, appMetricName string, labelSelector *metav1.LabelSelector) error {
//	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
//	if err != nil {
//		return err
//	}
//	// 查询该workload下管理了哪些Pod,并为pod打上appMetricName annotation
//	podList, err := r.PodLister.Pods(namespace).List(selector)
//	if err != nil {
//		klog.Errorf("fail to get pods in %s with selector:%v", namespace, selector)
//		return err
//	}
//	for _, pod := range podList {
//		if err := r.PatchAppMetricNameToPod(pod, appMetricName); err != nil {
//			return err
//		}
//	}
//
//	if err := r.Patch(context.TODO(), obj, client.MergeFrom(deepcopy)); err != nil {
//		return err
//	}
//
//	return nil
//}
