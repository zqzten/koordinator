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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	"github.com/koordinator-sh/koordinator/pkg/features"
)

func init() {
	nodeTransformers = append(nodeTransformers,
		TransformNodeAllocatableWithACKShareResources,
	)
}

func TransformNodeAllocatableWithACKShareResources(node *corev1.Node) {
	if !k8sfeature.DefaultFeatureGate.Enabled(features.EnableACKGPUShareScheduling) {
		return
	}

	if transformACKGPUMemory(node) {
		transformACKGPUCorePercentage(node)
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		node.Labels["__internal_gpu-compatible__"] = "koordinator-gpu-as-ack-gpu"
	}
}

func transformACKGPUMemory(node *corev1.Node) bool {
	gpuMemory, ok := node.Status.Allocatable[apiext.ResourceGPUMemory]
	if !ok {
		return false
	}
	if gpuMemory.Value() == 0 {
		return false
	}
	_, ok = node.Status.Allocatable[ack.ResourceAliyunGPUMemory]
	if ok {
		return false
	}
	node.Status.Allocatable[ack.ResourceAliyunGPUMemory] = *resource.NewQuantity(gpuMemory.Value()/1024/1024/1024, resource.DecimalSI)
	return true
}

func transformACKGPUCorePercentage(node *corev1.Node) bool {
	gpuCore, ok := node.Status.Allocatable[apiext.ResourceGPUCore]
	if !ok {
		return false
	}
	if gpuCore.Value() == 0 {
		return false
	}
	_, ok = node.Status.Allocatable[ack.ResourceALiyunGPUCorePercentage]
	if ok {
		return false
	}
	node.Status.Allocatable[ack.ResourceALiyunGPUCorePercentage] = gpuCore.DeepCopy()
	return true
}
