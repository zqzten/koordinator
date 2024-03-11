package common

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	apikruisev1alpha1 "github.com/openkruise/kruise-api/apps/v1alpha1"
	apikruisev1beta1 "github.com/openkruise/kruise-api/apps/v1beta1"
	autov1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	apiappsv1 "k8s.io/api/apps/v1"
	apibatchv1 "k8s.io/api/batch/v1"
	apibatchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	RequeueDuration = 2 * time.Minute
	AllNamespace    = "*"
)

type AlreadyExists struct{}

func (a AlreadyExists) Error() string {
	return "AlreadyExists"
}

type NotExists struct{}

func (n NotExists) Error() string {
	return "NotExists"
}

type InValidWorkload struct{}

func (i InValidWorkload) Error() string {
	return "InValidWorkload"
}

// Int64Ptr returns a int64 pointer for given value
func Int64Ptr(v int64) *int64 {
	return &v
}

// Float64Ptr returns a float64 pointer for given value
func Float64Ptr(v float64) *float64 {
	return &v
}

// BoolPtr returns a boolean pointer for given value
func BoolPtr(v bool) *bool {
	return &v
}

// StringPtr returns a string pointer for given value
func StringPtr(v string) *string {
	return &v
}

func Slice2Strings(arrays []string) sets.String {
	set := sets.NewString()
	for _, array := range arrays {
		set.Insert(array)
	}
	return set
}

func ControllerKind2String(controllerKinds []autov1alpha1.ControllerKind) []string {
	var ret = make([]string, 0)
	for _, controllerKind := range controllerKinds {
		ret = append(ret, string(controllerKind))
	}
	return ret
}

func GetOwnerController(obj *unstructured.Unstructured, own *metav1.OwnerReference) *metav1.OwnerReference {
	metav1.GetControllerOf(obj)
	var owner *metav1.OwnerReference
	switch autov1alpha1.ControllerKind(own.Kind) {
	case autov1alpha1.Deployment, autov1alpha1.DaemonSet, autov1alpha1.StatefulSet, autov1alpha1.ReplicaSet,
		autov1alpha1.ReplicationController, autov1alpha1.Job, autov1alpha1.CronJob, autov1alpha1.CloneSet,
		autov1alpha1.AdvancedDaemonset, autov1alpha1.AdvancedCronJob, autov1alpha1.AdvancedStatefulSet:
		owner = metav1.GetControllerOf(obj)
	default:
		return nil
	}
	return owner
}

func AdaptedKruiseWorkload(owner *metav1.OwnerReference) string {
	switch owner.Kind {
	case "StatefulSet":
		if owner.APIVersion == apikruisev1beta1.GroupVersion.String() {
			return string(autov1alpha1.AdvancedStatefulSet)
		}
	case "DaemonSet":
		if owner.APIVersion == apikruisev1alpha1.GroupVersion.String() {
			return string(autov1alpha1.AdvancedDaemonset)
		}
	}

	return owner.Kind
}

func IsValidAPIVersionAndKind(apiversion, kind string) bool {
	switch autov1alpha1.ControllerKind(kind) {
	case autov1alpha1.Deployment:
		if apiversion != apiappsv1.SchemeGroupVersion.String() {
			return false
		}

	case autov1alpha1.DaemonSet:
		if apiversion != apiappsv1.SchemeGroupVersion.String() {
			return false
		}

	case autov1alpha1.StatefulSet:
		if apiversion != apiappsv1.SchemeGroupVersion.String() {
			return false
		}

	case autov1alpha1.ReplicaSet:
		if apiversion != apiappsv1.SchemeGroupVersion.String() {
			return false
		}

	case autov1alpha1.ReplicationController:
		if apiversion != corev1.SchemeGroupVersion.String() {
			return false
		}

	case autov1alpha1.Job:
		if apiversion != apibatchv1.SchemeGroupVersion.String() {
			return false
		}

	case autov1alpha1.CronJob:
		if apiversion != apibatchv1beta1.SchemeGroupVersion.String() {
			return false
		}

	case autov1alpha1.CloneSet:
		if apiversion != apikruisev1alpha1.SchemeGroupVersion.String() {
			return false
		}

	case autov1alpha1.AdvancedDaemonset:
		if apiversion != apikruisev1alpha1.SchemeGroupVersion.String() {
			return false
		}

	case autov1alpha1.AdvancedCronJob:
		if apiversion != apikruisev1alpha1.SchemeGroupVersion.String() {
			return false
		}

	case autov1alpha1.AdvancedStatefulSet:
		if apiversion != apikruisev1beta1.SchemeGroupVersion.String() {
			return false
		}

	default:
		return false
	}

	return true
}

// return true if workload is k8s deployment, statefulset or daemonset
// for workloads managed by edas, the topmost kind like deployment will include the owner like rollouts/scale
// of edas.aliyun.oam.com, no need to continue search for owner reference for this since it will case more
// queries and authorities
func IsTopmostKubeWorkload(apiVersion, kind string) bool {
	switch autov1alpha1.ControllerKind(kind) {
	case autov1alpha1.Deployment, autov1alpha1.StatefulSet, autov1alpha1.DaemonSet:
		return apiVersion == apiappsv1.SchemeGroupVersion.String()
	default:
		return false
	}
}
