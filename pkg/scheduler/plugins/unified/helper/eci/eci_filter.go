package eci

import (
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func FilterByECIAffinity(pod *corev1.Pod, node *corev1.Node) bool {
	podECIAffinity := pod.Labels[uniext.LabelECIAffinity]
	switch podECIAffinity {
	case uniext.ECIRequired:
		return extunified.IsVirtualKubeletNode(node)
	case uniext.ECIRequiredNot:
		return !extunified.IsVirtualKubeletNode(node)
	default:
		return true
	}
}
