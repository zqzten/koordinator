package unified

import (
	"context"
	"strconv"

	kruisepolicyv1alpha1 "github.com/openkruise/kruise-api/policy/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *ControllerFinder) getPubForPod(pod *corev1.Pod) (*kruisepolicyv1alpha1.PodUnavailableBudget, error) {
	if len(pod.Annotations) == 0 || pod.Annotations[PodRelatedPubAnnotation] == "" {
		return nil, nil
	}
	pubName := pod.Annotations[PodRelatedPubAnnotation]
	pub := &kruisepolicyv1alpha1.PodUnavailableBudget{}
	err := c.Get(context.TODO(), client.ObjectKey{Namespace: pod.Namespace, Name: pubName}, pub)
	if err != nil {
		if errors.IsNotFound(err) || meta.IsNoMatchError(err) {
			klog.Warningf("pod(%s/%s) pub(%s) Is NotFound", pod.Namespace, pod.Name, pubName)
			return nil, nil
		}
		return nil, err
	}
	return pub, nil
}

func (c *ControllerFinder) getExpectedCountFromPub(pub *kruisepolicyv1alpha1.PodUnavailableBudget) int32 {
	if pub == nil || pub.Annotations[kruisepolicyv1alpha1.PubProtectTotalReplicas] == "" {
		return 0
	}
	expectedCount, _ := strconv.ParseInt(pub.Annotations[kruisepolicyv1alpha1.PubProtectTotalReplicas], 10, 32)
	return int32(expectedCount)
}
