package unified

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// GetPodsForRef 内部 VC 场景，WorkLoad 在 Super 侧不存在，需要如下兼容逻辑
func (c *ControllerFinder) GetPodsForRef(ownerReference *metav1.OwnerReference, ns string, labelSelector *metav1.LabelSelector, active bool) ([]*corev1.Pod, int32, error) {
	pods, expectedReplicas, err := c.Interface.GetPodsForRef(ownerReference, ns, labelSelector, active)
	if err != nil {
		return nil, -1, err
	}
	if pods != nil {
		return pods, expectedReplicas, nil
	}
	pods, err = c.Interface.ListPodsByWorkloads([]types.UID{ownerReference.UID}, ns, labelSelector, active)
	if err != nil {
		return nil, -1, err
	}
	if len(pods) != 0 {
		pub, err := c.getPubForPod(pods[0])
		if err != nil {
			return nil, -1, err
		}
		expectedReplicas = c.getExpectedCountFromPub(pub)
	}
	if expectedReplicas == 0 {
		expectedReplicas = int32(len(pods))
	}
	return pods, expectedReplicas, nil
}
