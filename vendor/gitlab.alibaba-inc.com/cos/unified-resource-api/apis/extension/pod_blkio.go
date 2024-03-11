package extension

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	nodesv1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/nodes/v1beta1"
)

func GetPodBlkioQOSFromAnnotation(pod *corev1.Pod) (*nodesv1beta1.BlkIOQOS, error) {
	if pod == nil || pod.Annotations == nil {
		return nil, fmt.Errorf("pod or pod.Annotations is nil")
	}

	podBlkIOQOS := &nodesv1beta1.BlkIOQOS{}
	if result, exist := pod.Annotations[AnnotationPodBlkioQOS]; exist {
		if err := json.Unmarshal([]byte(result), podBlkIOQOS); err != nil {
			return nil, fmt.Errorf("failed to parse BlkIO qos %s: %s", result, err.Error())
		}
	}

	return podBlkIOQOS, nil
}
