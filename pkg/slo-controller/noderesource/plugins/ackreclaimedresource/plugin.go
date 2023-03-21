package ackreclaimedresource

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/config"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/noderesource/framework"

	uniext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
)

const Name = "updateReclaimedResource"

var _ framework.NodePreparePlugin = (*Plugin)(nil)

type Plugin struct{}

func (r *Plugin) Name() string {
	return Name
}

func (r *Plugin) Execute(strategy *extension.ColocationStrategy, node *corev1.Node, nr *framework.NodeResource) error {
	// update reclaimed resource according to batch resources
	reclaimResourceMap := map[corev1.ResourceName]corev1.ResourceName{
		extension.BatchCPU:    uniext.AlibabaCloudReclaimedCPU,
		extension.BatchMemory: uniext.AlibabaCloudReclaimedMemory,
	}

	for src, mapped := range reclaimResourceMap {
		if nr.Resets[src] {
			delete(node.Status.Capacity, mapped)
			delete(node.Status.Allocatable, mapped)
		} else if q := nr.Resources[src]; q == nil { // if the specified resource has no quantity
			delete(node.Status.Capacity, mapped)
			delete(node.Status.Allocatable, mapped)
		} else {
			// NOTE: extended resource would be validated as an integer, so it should be checked before the update
			if _, ok := q.AsInt64(); !ok {
				klog.V(2).InfoS("node resource's quantity is not int64 and will be rounded",
					"resource", mapped, "original", *q)
				q.Set(q.Value())
			}
			node.Status.Capacity[mapped] = *q
			node.Status.Allocatable[mapped] = *q
		}
	}

	// clear when reporting disabled
	cfg, err := config.ParseReclaimedResourceConfig(strategy)
	if err != nil {
		return err
	}

	if cfg != nil && cfg.NodeUpdate != nil && !*cfg.NodeUpdate {
		delete(node.Status.Capacity, uniext.AlibabaCloudReclaimedCPU)
		delete(node.Status.Allocatable, uniext.AlibabaCloudReclaimedCPU)
		delete(node.Status.Capacity, uniext.AlibabaCloudReclaimedMemory)
		delete(node.Status.Allocatable, uniext.AlibabaCloudReclaimedMemory)
		return nil
	}

	return nil
}
