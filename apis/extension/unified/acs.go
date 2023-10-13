package unified

import (
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
)

const (
	ACSType = "virtual-cluster-node"
)

func IsACSVirtualNode(labels map[string]string) bool {
	return labels[uniext.LabelNodeType] == ACSType
}
