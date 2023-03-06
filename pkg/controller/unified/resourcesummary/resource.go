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

package resourcesummary

import (
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/features"
	utilfeature "github.com/koordinator-sh/koordinator/pkg/util/feature"
)

func GetAllocatableByOverQuota(node *corev1.Node) corev1.ResourceList {
	allocatable := node.Status.Allocatable.DeepCopy()
	cpuOverQuotaRatioSpec, memoryOverQuotaRatioSpec, diskOverQuotaRatioSpec := extunified.GetResourceOverQuotaSpec(node)
	if cpu, found := allocatable[corev1.ResourceCPU]; found {
		allocatable[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpu.MilliValue()*cpuOverQuotaRatioSpec/100, resource.DecimalSI)
	}
	if acu, found := allocatable[uniext.ResourceACU]; found {
		allocatable[uniext.ResourceACU] = *resource.NewMilliQuantity(acu.MilliValue()*cpuOverQuotaRatioSpec/100, resource.DecimalSI)
	}
	if memory, found := allocatable[corev1.ResourceMemory]; found {
		allocatable[corev1.ResourceMemory] = *resource.NewQuantity(memory.Value()*memoryOverQuotaRatioSpec/100, resource.BinarySI)
	}
	if ephemeralStorage, found := allocatable[corev1.ResourceEphemeralStorage]; found {
		allocatable[corev1.ResourceEphemeralStorage] = *resource.NewQuantity(ephemeralStorage.Value()*diskOverQuotaRatioSpec/100, resource.BinarySI)
	}
	return allocatable
}

func GetPodPriorityUsed(pod *corev1.Pod, node *corev1.Node, gpuCapacity corev1.ResourceList, resourceNames ...corev1.ResourceName) v1beta1.PodPriorityUsed {
	requested := corev1.ResourceList{}
	for _, container := range pod.Spec.Containers {
		if extunified.IsContainerIgnoreResource(&container) {
			continue
		}
		requested = quotav1.Add(requested, container.Resources.Requests)
	}
	for _, container := range pod.Spec.InitContainers {
		if extunified.IsContainerIgnoreResource(&container) {
			continue
		}
		requested = quotav1.Max(requested, container.Resources.Requests)
	}
	if pod.Spec.Overhead != nil {
		requested = quotav1.Add(requested, pod.Spec.Overhead)
	}

	unifiedPriority := extunified.GetUnifiedPriorityClass(pod)
	requested = priorityPodRequestedToNormal(requested, unifiedPriority)

	scaleCPUAndACU(pod, node, requested)
	fillGPUResource(requested, gpuCapacity)
	if len(resourceNames) > 0 {
		requested = quotav1.Mask(requested, resourceNames)
	}
	return v1beta1.PodPriorityUsed{
		PriorityClass: unifiedPriority,
		Allocated:     requested,
	}
}

func priorityPodRequestedToNormal(resourceList corev1.ResourceList, unifiedPriority uniext.PriorityClass) corev1.ResourceList {
	result := resourceList.DeepCopy()
	switch unifiedPriority {
	case uniext.PriorityBatch:
		// nolint:staticcheck // SA1019: extension.KoordBatchCPU is deprecated: because of the limitation of extended resource naming
		if quantity, found := result[extension.BatchCPU]; found {
			result[corev1.ResourceCPU] = *resource.NewMilliQuantity(quantity.Value(), resource.DecimalSI)
		} else if quantity, found = result[extension.KoordBatchCPU]; found {
			result[corev1.ResourceCPU] = *resource.NewMilliQuantity(quantity.Value(), resource.DecimalSI)
		}
		// nolint:staticcheck // SA1019: extension.KoordBatchMemory is deprecated: because of the limitation of extended resource naming
		if quantity, found := result[extension.BatchMemory]; found {
			result[corev1.ResourceMemory] = quantity
		} else if quantity, found = result[extension.KoordBatchMemory]; found {
			result[corev1.ResourceMemory] = quantity
		}

		// 删除batch相关资源
		// nolint:staticcheck // SA1019: extension.KoordBatchCPU is deprecated: because of the limitation of extended resource naming
		// nolint:staticcheck // SA1019: extension.KoordBatchMemory is deprecated: because of the limitation of extended resource naming
		for _, resourceName := range []corev1.ResourceName{extension.BatchCPU, extension.KoordBatchCPU, extension.BatchMemory, extension.KoordBatchMemory} {
			delete(result, resourceName)
		}
	}
	return result
}

func scaleCPUAndACU(pod *corev1.Pod, node *corev1.Node, resourceList corev1.ResourceList) {
	podQoS := extension.GetPodQoSClass(pod)
	if isNodeEnabledACU(node) {
		acuRatio := getACURatio(node)
		if podQoS == extension.QoSLS && utilfeature.DefaultFeatureGate.Enabled(features.DefaultEnableACUForLSPod) {
			resourceList[uniext.ResourceACU] = *resourceList.Cpu()
			cpu := int64(float64(resourceList.Cpu().MilliValue()) / acuRatio)
			resourceList[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpu, resource.DecimalSI)
		} else if q := resourceList[uniext.ResourceACU]; q.MilliValue() > 0 && acuRatio >= 1.0 {
			cpu := int64(float64(q.MilliValue()) / acuRatio)
			resourceList[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpu, resource.DecimalSI)
		} else {
			acu := int64(float64(resourceList.Cpu().MilliValue()) * acuRatio)
			resourceList[uniext.ResourceACU] = *resource.NewMilliQuantity(acu, resource.DecimalSI)
		}
	}
}

func isNodeEnabledACU(node *corev1.Node) bool {
	q := node.Status.Allocatable[uniext.ResourceACU]
	return !q.IsZero()
}

func getACURatio(node *corev1.Node) float64 {
	acuRatio := 1.0
	cpu := node.Status.Allocatable.Cpu()
	acu := node.Status.Allocatable[uniext.ResourceACU]
	if cpu.MilliValue() == 0 || acu.MilliValue() == 0 {
		return acuRatio
	}
	acuRatio = float64(acu.MilliValue()) / float64(cpu.MilliValue())
	return acuRatio
}

func GetNodePriorityResource(resourceList corev1.ResourceList, priorityClassType uniext.PriorityClass, node *corev1.Node) corev1.ResourceList {
	result := resourceList.DeepCopy()
	if priorityClassType != uniext.PriorityProd {
		delete(result, corev1.ResourceCPU)
		delete(result, uniext.ResourceACU)
		delete(result, corev1.ResourceMemory)
	}
	switch priorityClassType {
	case uniext.PriorityBatch:
		// nolint:staticcheck // SA1019: extension.KoordBatchCPU is deprecated: because of the limitation of extended resource naming
		if quantity, found := result[extension.BatchCPU]; found {
			result[corev1.ResourceCPU] = quantity
		} else if quantity, found = result[extension.KoordBatchCPU]; found {
			result[corev1.ResourceCPU] = quantity
		}
		// nolint:staticcheck // SA1019: extension.KoordBatchMemory is deprecated: because of the limitation of extended resource naming
		if quantity, found := result[extension.BatchMemory]; found {
			result[corev1.ResourceMemory] = quantity
		} else if quantity, found = result[extension.KoordBatchMemory]; found {
			result[corev1.ResourceMemory] = quantity
		}
	}
	if priorityClassType != uniext.PriorityProd {
		if quantity, found := result[corev1.ResourceCPU]; found {
			result[corev1.ResourceCPU] = *resource.NewMilliQuantity(quantity.Value(), resource.DecimalSI)
			if isNodeEnabledACU(node) {
				acuRatio := getACURatio(node)
				acu := int64(float64(result.Cpu().MilliValue()) * acuRatio)
				result[uniext.ResourceACU] = *resource.NewMilliQuantity(acu, resource.DecimalSI)
			}
		}
	}
	// 删除非标资源
	// nolint:staticcheck // SA1019: extension.KoordBatchCPU is deprecated: because of the limitation of extended resource naming
	// nolint:staticcheck // SA1019: extension.KoordBatchMemory is deprecated: because of the limitation of extended resource naming
	for _, resourceName := range []corev1.ResourceName{extension.BatchCPU, extension.KoordBatchCPU, extension.BatchMemory, extension.KoordBatchMemory} {
		delete(result, resourceName)
	}
	return result
}

func CalculateFree(capacityByPriorityClass, requested map[uniext.PriorityClass]corev1.ResourceList, allRequested corev1.ResourceList,
) map[uniext.PriorityClass]corev1.ResourceList {
	var overSellable []corev1.ResourceName
	for resourceName := range overSellPercents.percents {
		overSellable = append(overSellable, resourceName)
	}
	noOverSellable := quotav1.Difference(quotav1.ResourceNames(capacityByPriorityClass[uniext.PriorityProd]), overSellable)
	guaranteedCapacity := scaleResourceByOverSell(capacityByPriorityClass[uniext.PriorityProd], overSellPercents.percents)
	guaranteedFree := quotav1.Mask(quotav1.Subtract(guaranteedCapacity, allRequested), overSellable)

	free := map[uniext.PriorityClass]corev1.ResourceList{}
	for priorityClassType, capacity := range capacityByPriorityClass {
		free[priorityClassType] = quotav1.Subtract(quotav1.Mask(capacity, noOverSellable), quotav1.Mask(allRequested, noOverSellable))
		overSellableFree := quotav1.Subtract(quotav1.Mask(capacity, overSellable), quotav1.Mask(requested[priorityClassType], overSellable))
		for resourceName, quantity := range guaranteedFree {
			if quantity.Cmp(overSellableFree[resourceName]) < 0 {
				overSellableFree[resourceName] = quantity
			}
		}
		free[priorityClassType] = quotav1.Add(free[priorityClassType], overSellableFree)
	}
	return free
}

func scaleResourceByOverSell(resourceList corev1.ResourceList, oversellPercents map[corev1.ResourceName]int64) corev1.ResourceList {
	result := corev1.ResourceList{}
	for resourceName, percent := range oversellPercents {
		if quantity, found := resourceList[resourceName]; found {
			result[resourceName] = *resource.NewMilliQuantity(quantity.MilliValue()*percent/100, quantity.Format)
		}
	}
	return result
}
