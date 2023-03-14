package unified

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

// 内部 VC 场景，WorkLoad 在 Super 侧不存在，需要如下兼容逻辑
// 1. get expected scale info directly from workload
// 2. get expected scale info from pub annotations, Kruise sync workload replicas into annotations, vc-syncer sync pub from tenant to super
// 3. get expected scale as len(pod with same ownerReference) in case pod doesn't have a pub and can't find workload

func (c *ControllerFinder) GetExpectedScaleForPod(pod *corev1.Pod) (int32, error) {
	if pod == nil {
		return 0, nil
	}
	expectedReplicas, err := c.Interface.GetExpectedScaleForPod(pod)
	if err != nil {
		return -1, err
	}
	if expectedReplicas > 0 {
		return expectedReplicas, nil
	}
	pub, err := c.getPubForPod(pod)
	if err != nil {
		return -1, err
	}
	expectedReplicas = c.getExpectedCountFromPub(pub)
	if expectedReplicas > 0 {
		return expectedReplicas, nil
	}
	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return 0, nil
	}
	pods, err := c.Interface.ListPodsByWorkloads([]types.UID{ref.UID}, pod.Namespace, nil, false)
	if err != nil {
		return -1, err
	}
	return int32(len(pods)), nil
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
