package recommendationprofile

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"gitlab.alibaba-inc.com/cos/recommender/pkg/common"
	autov1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	redlister "gitlab.alibaba-inc.com/cos/unified-resource-api/client/listers/autoscaling/v1alpha1"
)

type PodRedProfileReconciler struct {
	client.Client
	RedProfileLister redlister.RecommendationProfileLister
	Scheme           *runtime.Scheme
}

var _ reconcile.Reconciler = &PodRedProfileReconciler{}

func (p *PodRedProfileReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// 查询 pod
	pod := &corev1.Pod{}
	err := p.Client.Get(context.TODO(), request.NamespacedName, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(3).Infof("[PodRedProfileReconciler] the pod %s/%s doesn't exist, error: %v", request.Namespace, request.Name, err)
			return reconcile.Result{}, nil
		}
		klog.Errorf("[PodRedProfileReconciler] failed to find pod %s/%s, error: %v", request.Namespace, request.Name, err)
		return reconcile.Result{Requeue: true}, err
	}
	// Pods that will not run anymore are considered inactive.
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		klog.V(4).Infof("[PodRedProfileReconciler] the phase of pod %s/%s is %v so it will not be reconciled at all",
			pod.Namespace, pod.Name, pod.Status.Phase)
		return reconcile.Result{}, nil
	} else if pod.Status.Phase != corev1.PodRunning {
		klog.V(3).Infof("[PodRedProfileReconciler] the phase of pod %s/%s is %v and it will be reconciled after %v minutes",
			pod.Namespace, pod.Name, pod.Status.Phase, common.RequeueDuration.Minutes())
		return reconcile.Result{RequeueAfter: common.RequeueDuration}, nil
	}

	// 没有workload管理的pod,将selector添加到recommendation
	if value, ok := pod.Labels[autov1alpha1.RecommendationProfileNameKey]; ok {
		if err = p.HandlePodWithoutWorkload(value, pod); err != nil {
			if _, ok := err.(*common.AlreadyExists); ok {
				klog.Infof("[PodRedProfileReconciler] the recommendation attach to pod %s/%s has already existed", pod.Namespace, pod.Name)
				return reconcile.Result{}, nil
			}
			return reconcile.Result{Requeue: true}, err
		}

		return reconcile.Result{}, nil
	}

	// 有workload管理的pod,将workload的name和ns 添加到recommendation
	if err = p.HandlePodWithWorkload(pod); err != nil {
		if _, ok := err.(*common.NotExists); ok {
			klog.Infof("[PodRedProfileReconciler] there isn't recommendation profile matched with the workload of pod %s/%s", pod.Namespace, pod.Name)
			return reconcile.Result{RequeueAfter: common.RequeueDuration}, nil
		}
		if _, ok := err.(*common.AlreadyExists); ok {
			klog.Infof("[PodRedProfileReconciler] the recommendation attach to pod %s/%s has already existed", pod.Namespace, pod.Name)
			return reconcile.Result{}, nil
		}
		if _, ok := err.(*common.InValidWorkload); ok {
			klog.Infof("[PodRedProfileReconciler] the workload of pod %s/%s is invalid", pod.Namespace, pod.Name)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{Requeue: true}, err
	}

	return reconcile.Result{}, nil
}

func (p *PodRedProfileReconciler) HandlePodWithoutWorkload(redProfileName string, pod *corev1.Pod) error {
	// 查询RecommendationProfile
	redProfile, err := p.RedProfileLister.Get(redProfileName)
	if err != nil {
		klog.Errorf("[PodRedProfileReconciler] fail to get recommendationProfile %s, err: %v", redProfileName, err)
		return err
	}
	if err = GenerateRecommendationWithoutWorkloadIfNeed(pod, redProfile, p.Client); err != nil {
		return err
	}
	return nil
}

func (p *PodRedProfileReconciler) HandlePodWithWorkload(pod *corev1.Pod) error {
	owner, err := p.FindTopWorkload(pod)
	if owner == nil || err != nil {
		klog.Infof("[PodRedProfileReconciler] pod %s/%s doesn't have valid owner", pod.Namespace, pod.Name)
		return err
	}
	kind := common.AdaptedKruiseWorkload(owner)
	// 查询是否有匹配的recommendationProfile
	recProfile, profileErr := p.GetMatchedRedProfileForWorkload(pod.Namespace, kind)
	if recProfile == nil || profileErr != nil {
		return err
	}
	if err := GenerateRecommendationForWorkloadIfNeed(recProfile, owner, pod.Namespace, p.Client); err != nil {
		return err
	}

	return nil
}

func (p *PodRedProfileReconciler) GetMatchedRedProfileForWorkload(namespace, kind string) (*autov1alpha1.RecommendationProfile, error) {
	redProfiles, err := p.RedProfileLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, profile := range redProfiles {
		if profile.Spec.Type == autov1alpha1.Pod {
			continue
		}

		enabledNamespaceSet := common.Slice2Strings(profile.Spec.EnabledNamespaces)
		disabledNamespaceSet := common.Slice2Strings(profile.Spec.DisabledNamespaces)
		controllerKindSet := common.Slice2Strings(common.ControllerKind2String(profile.Spec.ControllerKinds))
		if disabledNamespaceSet.Has(common.AllNamespace) {
			return nil, &common.NotExists{}
		}
		if enabledNamespaceSet.Has(common.AllNamespace) && !disabledNamespaceSet.Has(namespace) && controllerKindSet.Has(kind) {
			return profile, nil
		}
		if enabledNamespaceSet.Has(namespace) && controllerKindSet.Has(kind) {
			return profile, nil
		}
	}

	return nil, &common.NotExists{}
}

func (p *PodRedProfileReconciler) FindTopWorkload(pod *corev1.Pod) (*metav1.OwnerReference, error) {
	var newOwner *metav1.OwnerReference
	owner := metav1.GetControllerOf(pod)
	if owner == nil {
		return nil, nil
	}
	for {
		if common.IsTopmostKubeWorkload(owner.APIVersion, owner.Kind) {
			return owner, nil
		}
		if !common.IsValidAPIVersionAndKind(owner.APIVersion, owner.Kind) {
			return nil, &common.InValidWorkload{}
		}
		gvk := schema.FromAPIVersionAndKind(owner.APIVersion, owner.Kind)
		target := &unstructured.Unstructured{}
		target.SetGroupVersionKind(gvk)
		err := p.Get(context.TODO(), client.ObjectKey{Name: owner.Name, Namespace: pod.Namespace}, target)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, &common.InValidWorkload{}
			}
			return nil, err
		}
		newOwner = common.GetOwnerController(target, owner)
		if newOwner == nil {
			break
		}
		owner = newOwner
	}

	return owner, nil
}

func (p *PodRedProfileReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&corev1.Pod{}).
		Named("podRedProfileController").
		WithEventFilter(&podPredicate{}).
		Complete(p)
}
