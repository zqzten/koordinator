package extension

import (
	"strings"

	v1 "k8s.io/api/core/v1"
)

const (
	ConfigMapNUMANodeSuffix = "-numa-info"
	LabelConfigMapCPUInfo   = "ack.node.cpu.schedule"
	DataConfigMapCPUInfo    = "info"
	Topology1_18            = "topology"
	Topology1_20            = "topology.20"
)

func IsCPUTopology1_20(cm *v1.ConfigMap) bool {
	if cm == nil {
		return false
	}
	v, ok := cm.Labels[LabelConfigMapCPUInfo]
	if strings.HasSuffix(cm.Name, ConfigMapNUMANodeSuffix) && ok && v == Topology1_20 {
		return true
	}
	return false
}

func IsCPUTopology1_18(cm *v1.ConfigMap) bool {
	if cm == nil {
		return false
	}
	v, ok := cm.Labels[LabelConfigMapCPUInfo]
	if strings.HasSuffix(cm.Name, ConfigMapNUMANodeSuffix) && ok && v == Topology1_18 {
		return true
	}
	return false
}
