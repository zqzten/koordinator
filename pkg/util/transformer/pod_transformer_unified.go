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
	"encoding/json"
	"sort"

	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	unifiedschedulingv1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/scheduling/v1beta1"
	asiquotav1 "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"
	syncerconsts "sigs.k8s.io/cluster-api-provider-nested/virtualcluster/pkg/syncer/constants"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/deviceshare"
)

func init() {
	podTransformers = append(podTransformers,
		TransformSigmaIgnoreResourceContainers,
		TransformTenantPod,
		TransformNonProdPodResourceSpec,
		TransformUnifiedGPUMemoryRatio,
		TransformENIResource,
		TransformPodQoSClass,
		TransformUnifiedDeviceAllocation,
		TransformUnifiedQuotaName,
		TransformResourceSpec,
		TransformResourceStatus,
		TransformPodGPUResourcesToCardRatio,
	)
}

func TransformSigmaIgnoreResourceContainers(pod *corev1.Pod) {
	transformSigmaIgnoreResourceContainers(&pod.Spec)
}

func transformSigmaIgnoreResourceContainers(podSpec *corev1.PodSpec) {
	for _, containers := range [][]corev1.Container{podSpec.InitContainers, podSpec.Containers} {
		for i := range containers {
			container := &containers[i]
			if unified.IsContainerIgnoreResource(container) {
				for k, v := range container.Resources.Requests {
					container.Resources.Requests[k] = *resource.NewQuantity(0, v.Format)
				}
			}
		}
	}
}

func TransformTenantPod(pod *corev1.Pod) {
	if len(pod.OwnerReferences) != 0 {
		return
	}
	if tenantOwnerReferences, ok := pod.Annotations[syncerconsts.LabelOwnerReferences]; ok {
		_ = json.Unmarshal([]byte(tenantOwnerReferences), &pod.OwnerReferences)
	}
}

func TransformNonProdPodResourceSpec(pod *corev1.Pod) {
	transformNonProdPodResourceSpec(&pod.Spec)
}

func transformNonProdPodResourceSpec(podSpec *corev1.PodSpec) {
	priorityClass := unified.GetPriorityClass(&corev1.Pod{Spec: *podSpec})
	if priorityClass == extension.PriorityNone || priorityClass == extension.PriorityProd {
		return
	}

	for _, containers := range [][]corev1.Container{podSpec.InitContainers, podSpec.Containers} {
		for i := range containers {
			container := &containers[i]
			replaceAndEraseResource(container.Resources.Requests, corev1.ResourceCPU, extension.ResourceNameMap[priorityClass][corev1.ResourceCPU])
			replaceAndEraseResource(container.Resources.Requests, corev1.ResourceMemory, extension.ResourceNameMap[priorityClass][corev1.ResourceMemory])

			replaceAndEraseResource(container.Resources.Limits, corev1.ResourceCPU, extension.ResourceNameMap[priorityClass][corev1.ResourceCPU])
			replaceAndEraseResource(container.Resources.Limits, corev1.ResourceMemory, extension.ResourceNameMap[priorityClass][corev1.ResourceMemory])
		}
	}

	if podSpec.Overhead != nil {
		replaceAndEraseResource(podSpec.Overhead, corev1.ResourceCPU, extension.ResourceNameMap[priorityClass][corev1.ResourceCPU])
		replaceAndEraseResource(podSpec.Overhead, corev1.ResourceMemory, extension.ResourceNameMap[priorityClass][corev1.ResourceMemory])
	}
}

func TransformUnifiedGPUMemoryRatio(pod *corev1.Pod) {
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if unified.IsContainerActivelyAddedGPUMemRatio(container) {
			delete(container.Resources.Requests, unifiedresourceext.GPUResourceMemRatio)
			delete(container.Resources.Limits, unifiedresourceext.GPUResourceMemRatio)
		}
	}
	for i := range pod.Spec.InitContainers {
		container := &pod.Spec.InitContainers[i]
		if unified.IsContainerActivelyAddedGPUMemRatio(container) {
			delete(container.Resources.Requests, unifiedresourceext.GPUResourceMemRatio)
			delete(container.Resources.Limits, unifiedresourceext.GPUResourceMemRatio)
		}
	}
}

func TransformENIResource(pod *corev1.Pod) {
	transformENIResource(&pod.Spec)
}

func transformENIResource(podSpec *corev1.PodSpec) {
	if !enableENIResourceTransform {
		return
	}
	for _, containers := range [][]corev1.Container{podSpec.InitContainers, podSpec.Containers} {
		for i := range containers {
			container := &containers[i]
			replaceAndEraseResource(container.Resources.Requests, unified.ResourceSigmaENI, unified.ResourceAliyunMemberENI)
			replaceAndEraseResource(container.Resources.Limits, unified.ResourceSigmaENI, unified.ResourceAliyunMemberENI)
		}
	}
}

func TransformPodQoSClass(pod *corev1.Pod) {
	if val := pod.Labels[extension.LabelPodQoS]; val != "" {
		return
	}
	if val := pod.Labels[unified.LabelPodQoSClass]; val != "" {
		pod.Labels[extension.LabelPodQoS] = val
	}
}

func TransformUnifiedDeviceAllocation(pod *corev1.Pod) {
	if val := pod.Annotations[extension.AnnotationDeviceAllocated]; val != "" {
		return
	}

	val := pod.Annotations[unifiedresourceext.AnnotationMultiDeviceAllocStatus]
	if val == "" {
		return
	}

	deviceAllocStatus, err := unifiedresourceext.GetMultiDeviceAllocStatus(pod.Annotations)
	if err != nil {
		klog.ErrorS(err, "Failed to unifiedresourceext.GetMultiDeviceAllocStatus", "pod", klog.KObj(pod))
		return
	}

	deviceAllocations := make(extension.DeviceAllocations)
	for deviceType, allocs := range deviceAllocStatus.AllocStatus {
		switch deviceType {
		case unifiedschedulingv1beta1.GPU:
			resourceGPUAllocations, err := convertUnifiedGPUAllocs(allocs)
			if err != nil {
				klog.ErrorS(err, "Failed to convertUnifiedGPUAllocs", "pod", klog.KObj(pod))
				return
			}
			if len(resourceGPUAllocations) > 0 {
				deviceAllocations[schedulingv1alpha1.GPU] = resourceGPUAllocations
			}
		}
	}

	if len(deviceAllocations) > 0 {
		if err := extension.SetDeviceAllocations(pod, deviceAllocations); err != nil {
			klog.ErrorS(err, "Failed to extension.SetDeviceAllocations", "pod", klog.KObj(pod))
		}
	}
}

func convertUnifiedGPUAllocs(allocs []unifiedresourceext.ContainerDeviceAllocStatus) ([]*extension.DeviceAllocation, error) {
	ResourceGPUAllocs := make(map[int32]*extension.DeviceAllocation)
	for _, v := range allocs {
		for _, alloc := range v.DeviceAllocStatus.Allocs {
			if len(alloc.Resources) == 0 {
				continue
			}
			resourceList := make(corev1.ResourceList)
			for name, quantity := range alloc.Resources {
				if name == unifiedresourceext.GPUResourceMemRatio {
					continue
				}
				resourceList[corev1.ResourceName(name)] = quantity
			}
			combination, err := deviceshare.ValidateDeviceRequest(resourceList)
			if err != nil {
				return nil, err
			}
			resourceList = deviceshare.ConvertDeviceRequest(resourceList, combination)
			koordAllocation := ResourceGPUAllocs[alloc.Minor]
			if koordAllocation == nil {
				koordAllocation = &extension.DeviceAllocation{
					Minor: alloc.Minor,
				}
				ResourceGPUAllocs[alloc.Minor] = koordAllocation
			}
			koordAllocation.Resources = quotav1.Add(koordAllocation.Resources, resourceList)
		}
	}
	if len(ResourceGPUAllocs) == 0 {
		return nil, nil
	}
	koordDeviceAllocation := make([]*extension.DeviceAllocation, 0, len(ResourceGPUAllocs))
	for _, allocation := range ResourceGPUAllocs {
		koordDeviceAllocation = append(koordDeviceAllocation, allocation)
	}
	if len(koordDeviceAllocation) > 1 {
		sort.Slice(koordDeviceAllocation, func(i, j int) bool {
			return koordDeviceAllocation[i].Minor < koordDeviceAllocation[j].Minor
		})
	}
	return koordDeviceAllocation, nil
}

func TransformUnifiedQuotaName(pod *corev1.Pod) {
	if extension.GetQuotaName(pod) != "" {
		return
	}
	if quotaName := pod.Labels[asiquotav1.LabelQuotaName]; quotaName != "" {
		pod.Labels[extension.LabelQuotaName] = quotaName
	}
}

func TransformResourceSpec(pod *corev1.Pod) {
	if val := pod.Annotations[extension.AnnotationResourceSpec]; val != "" {
		return
	}
	resourceSpec, from, err := unified.GetResourceSpec(pod.Annotations)
	if err != nil {
		klog.ErrorS(err, "Failed to unified.GetResourceSpec", "pod", klog.KObj(pod))
		return
	}
	if from == "" {
		return
	}
	if resourceSpec.PreferredCPUBindPolicy == extension.CPUBindPolicyFullPCPUs ||
		resourceSpec.PreferredCPUBindPolicy == extension.CPUBindPolicySpreadByPCPUs {
		err = extension.SetResourceSpec(pod, resourceSpec)
		if err != nil {
			klog.ErrorS(err, "Failed to extension.SetResourceSpec", "pod", klog.KObj(pod))
			return
		}

		if extension.GetPodQoSClass(pod) == extension.QoSNone {
			if pod.Labels == nil {
				pod.Labels = map[string]string{}
			}
			pod.Labels[extension.LabelPodQoS] = string(extension.QoSLSR)
		}
	}
}

func TransformResourceStatus(pod *corev1.Pod) {
	if val := pod.Annotations[extension.AnnotationResourceStatus]; val != "" {
		return
	}
	status, err := unified.GetResourceStatus(pod.Annotations)
	if err != nil {
		klog.ErrorS(err, "Failed to unified.GetResourceStatus", "pod", klog.KObj(pod))
		return
	}
	if status != nil && status.CPUSet != "" {
		err = extension.SetResourceStatus(pod, status)
		if err != nil {
			klog.ErrorS(err, "Failed to extension.SetResourceStatus", "pod", klog.KObj(pod))
			return
		}
	}
}

func TransformPodGPUResourcesToCardRatio(pod *corev1.Pod) {
	if !enableTransformGPUCardRatio {
		return
	}
	transformPodGPUResourcesToCardRatio(pod)
}

func transformPodGPUResourcesToCardRatio(pod *corev1.Pod) {
	gpuModel := GetPodGPUModel(pod)
	for _, container := range pod.Spec.Containers {
		replaceUnifiedGPUResource(container.Resources.Requests)
		replaceUnifiedGPUResource(container.Resources.Limits)
		NormalizeGPUResourcesToCardRatioForPod(container.Resources.Requests, gpuModel)
		NormalizeGPUResourcesToCardRatioForPod(container.Resources.Limits, gpuModel)
	}
	for _, container := range pod.Spec.InitContainers {
		replaceUnifiedGPUResource(container.Resources.Requests)
		replaceUnifiedGPUResource(container.Resources.Limits)
		NormalizeGPUResourcesToCardRatioForPod(container.Resources.Requests, gpuModel)
		NormalizeGPUResourcesToCardRatioForPod(container.Resources.Limits, gpuModel)
	}
	if pod.Spec.Overhead != nil {
		replaceUnifiedGPUResource(pod.Spec.Overhead)
		NormalizeGPUResourcesToCardRatioForPod(pod.Spec.Overhead, gpuModel)
	}
}

func replaceUnifiedGPUResource(list corev1.ResourceList) {
	replaceAndEraseResource(list, unifiedresourceext.GPUResourceMemRatio, extension.ResourceGPUMemoryRatio)
	replaceAndEraseResource(list, unifiedresourceext.GPUResourceAlibaba, extension.ResourceNvidiaGPU)
}
