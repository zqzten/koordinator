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

package transformer

import (
	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func init() {
	nodeTransformers = append(nodeTransformers,
		TransformNodeAllocatableWithOverQuota,
		TransformNodeAllocatableWithUnifiedGPUMemoryRatio,
		TransformNodeAllocatableToUnifiedCardRatio,
	)
}

func TransformNodeAllocatableWithOverQuota(node *corev1.Node) {
	if _, ok := node.Annotations[apiext.AnnotationNodeResourceAmplificationRatio]; ok {
		return
	}

	rawAllocatable, err := apiext.GetNodeRawAllocatable(node.Annotations)
	if err != nil {
		klog.ErrorS(err, "Failed to GetNodeRawAllocatable", "node", node.Name)
		return
	}
	if rawAllocatable == nil {
		rawAllocatable = corev1.ResourceList{}
	}

	amplificationRatios := extunified.NewAmplificationRatiosByOverQuota(node.Labels)
	for resourceName, ratio := range amplificationRatios {
		if ratio == 1 {
			delete(amplificationRatios, resourceName)
			continue
		}
		if ratio > 1 {
			quantity := node.Status.Allocatable[resourceName]
			if quantity.IsZero() {
				continue
			}
			if _, ok := rawAllocatable[resourceName]; !ok {
				rawAllocatable[resourceName] = quantity
			}
			apiext.AmplifyResourceList(node.Status.Allocatable, amplificationRatios, resourceName)
		}
	}
	if len(amplificationRatios) > 0 {
		apiext.SetNodeResourceAmplificationRatios(node, amplificationRatios)
	}
	if len(rawAllocatable) > 0 {
		apiext.SetNodeRawAllocatable(node, rawAllocatable)
	}
}

func TransformNodeAllocatableWithUnifiedGPUMemoryRatio(node *corev1.Node) {
	gpuMemoryRatio, ok := node.Status.Allocatable[unifiedresourceext.GPUResourceMemRatio]
	if !ok {
		return
	}
	// The device-plugin has reported unified GPUResourceMemRatio, but it is zero.
	// In this case, the device-plugin itself may be abnormal. Skip this node first.
	if gpuMemoryRatio.Value() == 0 {
		return
	}
	_, ok = node.Status.Allocatable[apiext.ResourceNvidiaGPU]
	if ok {
		return
	}
	if node.Status.Allocatable == nil {
		node.Status.Allocatable = corev1.ResourceList{}
	}
	node.Status.Allocatable[apiext.ResourceNvidiaGPU] = *resource.NewQuantity(gpuMemoryRatio.Value()/100, resource.DecimalSI)
}

func TransformNodeAllocatableToUnifiedCardRatio(node *corev1.Node) {
	if NodeAllocatableContainsGPU(node.Status.Allocatable) {
		node.Status.Allocatable = ParseGPUResourcesByModel(node.Status.Allocatable, node.Labels)
	}
}
