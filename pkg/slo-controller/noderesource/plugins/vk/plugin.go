/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vk

import (
	"fmt"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/configuration"
	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/noderesource/framework"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

const PluginName = "VKResource"

type Plugin struct{}

var (
	_ framework.ResourceCalculatePlugin = (*Plugin)(nil)
)

func (p *Plugin) Name() string {
	return PluginName
}

func (p *Plugin) Reset(node *corev1.Node, message string) []framework.ResourceItem {
	return nil
}

func (p *Plugin) Calculate(strategy *configuration.ColocationStrategy, node *corev1.Node, podList *corev1.PodList,
	metrics *framework.ResourceMetrics) ([]framework.ResourceItem, error) {
	if !uniext.IsVirtualKubeletNode(node) {
		return nil, nil
	}

	var resourceItems []framework.ResourceItem

	// 1. VK Batch resources (batch.allocatable := node.total - HP.request - node.reserved)
	batchResources, err := p.calculateForBatchResource(strategy, node, podList)
	if err != nil {
		klog.V(4).InfoS("failed to calculate Batch resources for VK node", "node", node.Name,
			"err", err)
	} else {
		resourceItems = append(resourceItems, batchResources...)
	}

	// 2. VK Mid resources (mid.allocatable = zero)
	midResources, err := p.calculateForMidResources(strategy, node, podList)
	if err != nil {
		klog.V(4).InfoS("failed to calculate Mid resources for VK node", "node", node.Name,
			"err", err)
	} else {
		resourceItems = append(resourceItems, midResources...)
	}

	return resourceItems, nil
}

func (p *Plugin) calculateForBatchResource(strategy *configuration.ColocationStrategy, node *corev1.Node,
	podList *corev1.PodList) ([]framework.ResourceItem, error) {
	// HP means High-Priority (i.e. not Batch or Free) pods
	podHPRequest := util.NewZeroResourceList()

	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}

		priorityClass := extension.GetPodPriorityClassWithDefault(pod)
		podRequest := util.GetPodRequest(pod, corev1.ResourceCPU, corev1.ResourceMemory)
		isPodHighPriority := priorityClass != extension.PriorityBatch && priorityClass != extension.PriorityFree
		if isPodHighPriority {
			podHPRequest = quotav1.Add(podHPRequest, podRequest)
		}
	}

	nodeAllocatable := getNodeAllocatable(node)
	nodeReservation := getNodeReservation(strategy, node)

	batchAllocatable := quotav1.Max(quotav1.Subtract(quotav1.Subtract(nodeAllocatable, nodeReservation),
		podHPRequest), util.NewZeroResourceList())
	cpuMsg := fmt.Sprintf("batchAllocatable[CPU(Milli-Core)]:%v = nodeAllocatable:%v - nodeReservation:%v - podHPRequest:%v",
		batchAllocatable.Cpu().MilliValue(), nodeAllocatable.Cpu().MilliValue(), nodeReservation.Cpu().MilliValue(),
		podHPRequest.Cpu().MilliValue())
	memMsg := fmt.Sprintf("batchAllocatable[Mem(GB)]:%v = nodeAllocatable:%v - nodeReservation:%v - podHPRequest:%v",
		batchAllocatable.Memory().ScaledValue(resource.Giga), nodeAllocatable.Memory().ScaledValue(resource.Giga),
		nodeReservation.Memory().ScaledValue(resource.Giga), podHPRequest.Memory().ScaledValue(resource.Giga))

	klog.V(6).InfoS("calculate batch resource for virtual-kubelet node", "node", node.Name,
		"batch resource", batchAllocatable, "cpu", cpuMsg, "memory", memMsg)

	return []framework.ResourceItem{
		{
			Name:     extension.BatchCPU,
			Quantity: resource.NewQuantity(batchAllocatable.Cpu().MilliValue(), resource.DecimalSI),
			Reset:    false,
			Message:  cpuMsg,
		},
		{
			Name:     extension.BatchMemory,
			Quantity: batchAllocatable.Memory(),
			Reset:    false,
			Message:  memMsg,
		},
	}, nil
}

func (p *Plugin) calculateForMidResources(strategy *configuration.ColocationStrategy, node *corev1.Node,
	podList *corev1.PodList) ([]framework.ResourceItem, error) {
	// currently, vk node does not support Mid-tier overcommitment
	return nil, nil
}

// getNodeAllocatable gets node allocatable and filters out non-CPU and non-Mem resources
func getNodeAllocatable(node *corev1.Node) corev1.ResourceList {
	result := node.Status.Allocatable.DeepCopy()
	result = quotav1.Mask(result, []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory})
	return result
}

// getNodeReservation gets node-level safe-guarding reservation with the node's allocatable
func getNodeReservation(strategy *configuration.ColocationStrategy, node *corev1.Node) corev1.ResourceList {
	nodeAllocatable := getNodeAllocatable(node)
	cpuReserveQuant := util.MultiplyMilliQuant(nodeAllocatable[corev1.ResourceCPU], getReserveRatio(*strategy.CPUReclaimThresholdPercent))
	memReserveQuant := util.MultiplyQuant(nodeAllocatable[corev1.ResourceMemory], getReserveRatio(*strategy.MemoryReclaimThresholdPercent))

	return corev1.ResourceList{
		corev1.ResourceCPU:    cpuReserveQuant,
		corev1.ResourceMemory: memReserveQuant,
	}
}

// getReserveRatio returns resource reserved ratio
func getReserveRatio(reclaimThreshold int64) float64 {
	return float64(100-reclaimThreshold) / 100.0
}
