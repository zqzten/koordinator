package unified

import (
	corev1 "k8s.io/api/core/v1"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
)

const (
	ACSType = "virtual-cluster-node"

	LabelAlibabaCPUOverQuota    = "alibabacloud.com/cpu-over-quota"
	LabelAlibabaMemoryOverQuota = "alibabacloud.com/memory-over-quota"
	LabelAlibabaDiskOverQuota   = "alibabacloud.com/disk-over-quota"
)

func IsACSVirtualNode(labels map[string]string) bool {
	return labels[uniext.LabelNodeType] == ACSType
}

func GetAlibabaResourceOverQuotaSpec(node *corev1.Node) (cpuOverQuotaRatio, memoryOverQuotaRatio, diskOverQuotaRatio int64) {
	cpuOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelAlibabaCPUOverQuota])
	memoryOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelAlibabaMemoryOverQuota])
	diskOverQuotaRatio = parseOverQuotaRatio(node.Labels[LabelAlibabaDiskOverQuota])
	return
}
