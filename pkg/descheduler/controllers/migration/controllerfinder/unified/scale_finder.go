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
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/controllerfinder"
)

const (
	PodRelatedPubAnnotation = "kruise.io/related-pub"
)

var koordNew func(manager manager.Manager) (controllerfinder.Interface, error)

func init() {
	koordNew = controllerfinder.New
	controllerfinder.New = New
}

type ControllerFinder struct {
	client.Client
	controllerfinder.Interface
}

func (c *ControllerFinder) GetExpectedScaleForPod(pod *corev1.Pod) (int32, error) {
	if pod == nil {
		return 0, nil
	}
	expectedReplicas, err := c.Interface.GetExpectedScaleForPod(pod)
	if err != nil {
		return 0, err
	}
	if expectedReplicas == 0 {
		pub, err := c.getPubForPod(pod)
		if err != nil {
			return -1, err
		}
		if pub == nil || pub.Annotations[kruisepolicyv1alpha1.PubProtectTotalReplicas] == "" {
			return 0, nil
		}
		expectedCount, _ := strconv.ParseInt(pub.Annotations[kruisepolicyv1alpha1.PubProtectTotalReplicas], 10, 32)
		return int32(expectedCount), nil
	}
	return expectedReplicas, nil
}

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

func New(manager manager.Manager) (controllerfinder.Interface, error) {
	finder, err := koordNew(manager)
	if err != nil {
		return nil, err
	}
	return &ControllerFinder{
		Client:    manager.GetClient(),
		Interface: finder,
	}, nil
}
