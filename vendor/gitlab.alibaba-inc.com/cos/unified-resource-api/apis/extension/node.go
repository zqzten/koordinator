package extension

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	"gitlab.alibaba-inc.com/cos/unified-resource-api/apis/scheduling/v1beta1"
)

const (
	AnnotationResourceReclaimEnabled = "alibabacloud.com/resourceReclaimEnabled"
)

func IsGPUReclaimEnabled(node *corev1.Node) bool {
	if node == nil || node.Annotations == nil {
		return false
	}
	value, exist := node.Annotations[AnnotationResourceReclaimEnabled]
	if !exist {
		return false
	}
	for _, resource := range strings.Split(value, ",") {
		if resource == string(v1beta1.GPU) {
			return true
		}
	}
	return false
}
