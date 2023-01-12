package noderesource

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/config"

	uniext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
)

const (
	name = "updateReclaimedResource"
)

func init() {
	RegisterNodePrepareExtender(name, &ReclaimedResourcePlugin{})
}

type ReclaimedResourcePlugin struct {
}

func (r *ReclaimedResourcePlugin) Execute(strategy *extension.ColocationStrategy, node *corev1.Node) error {
	// clear reclaimed resource if batch resource not exist
	if _, exist := node.Status.Capacity[extension.BatchCPU]; !exist {
		delete(node.Status.Capacity, uniext.AlibabaCloudReclaimedCPU)
	}
	if _, exist := node.Status.Allocatable[extension.BatchCPU]; !exist {
		delete(node.Status.Allocatable, uniext.AlibabaCloudReclaimedCPU)
	}
	if _, exist := node.Status.Capacity[extension.BatchMemory]; !exist {
		delete(node.Status.Capacity, uniext.AlibabaCloudReclaimedMemory)
	}
	if _, exist := node.Status.Allocatable[extension.BatchMemory]; !exist {
		delete(node.Status.Allocatable, uniext.AlibabaCloudReclaimedMemory)
	}

	cfg, err := config.ParseReclaimedResourceConfig(strategy)
	if err != nil {
		return err
	}
	if cfg == nil || cfg.NodeUpdate == nil {
		return nil
	}
	if !*cfg.NodeUpdate {
		delete(node.Status.Capacity, uniext.AlibabaCloudReclaimedCPU)
		delete(node.Status.Allocatable, uniext.AlibabaCloudReclaimedCPU)
		delete(node.Status.Capacity, uniext.AlibabaCloudReclaimedMemory)
		delete(node.Status.Allocatable, uniext.AlibabaCloudReclaimedMemory)
		return nil
	}

	if batchCPU, exist := node.Status.Capacity[extension.BatchCPU]; exist {
		node.Status.Capacity[uniext.AlibabaCloudReclaimedCPU] = batchCPU
	}
	if batchCPU, exist := node.Status.Allocatable[extension.BatchCPU]; exist {
		node.Status.Allocatable[uniext.AlibabaCloudReclaimedCPU] = batchCPU
	}
	if batchMemory, exist := node.Status.Capacity[extension.BatchMemory]; exist {
		node.Status.Capacity[uniext.AlibabaCloudReclaimedMemory] = batchMemory
	}
	if batchMemory, exist := node.Status.Allocatable[extension.BatchMemory]; exist {
		node.Status.Allocatable[uniext.AlibabaCloudReclaimedMemory] = batchMemory
	}
	return nil
}
