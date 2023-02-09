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

package unified

import (
	"fmt"
	"sort"

	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	unifiedschedulingv1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/deviceshare"
)

const (
	unifiedGPUExist            = 1 << 50
	unifiedGPUCoreExist        = 1 << 51
	unifiedGPUMemoryExist      = 1 << 52
	unifiedGPUMemoryRatioExist = 1 << 53
	aliyunGPUComputeExist      = 1 << 63
)

var deviceResourceNames = map[schedulingv1alpha1.DeviceType][]corev1.ResourceName{
	schedulingv1alpha1.RDMA: {apiext.KoordRDMA},
	schedulingv1alpha1.FPGA: {apiext.KoordFPGA},
	schedulingv1alpha1.GPU: {
		apiext.NvidiaGPU,
		apiext.KoordGPU,
		apiext.GPUCore,
		apiext.GPUMemory,
		apiext.GPUMemoryRatio,
		unifiedresourceext.GPUResourceAlibaba,
		unifiedresourceext.GPUResourceCore,
		unifiedresourceext.GPUResourceMem,
		unifiedresourceext.GPUResourceMemRatio,
		ack.AliyunGPUCompute,
	},
}

var (
	originGetDeviceAllocations = apiext.GetDeviceAllocations
)

func init() {
	deviceshare.DeviceResourceNames = deviceResourceNames
	deviceshare.ValidateGPURequest = ValidateGPURequest
	deviceshare.ConvertGPUResource = ConvertGPUResource
	apiext.GetDeviceAllocations = GetDeviceAllocations
}

func ValidateGPURequest(podRequest corev1.ResourceList) (uint, error) {
	var gpuCombination uint

	if podRequest == nil || len(podRequest) == 0 {
		return gpuCombination, fmt.Errorf("pod request should not be empty")
	}

	if _, exist := podRequest[apiext.NvidiaGPU]; exist {
		gpuCombination |= deviceshare.NvidiaGPUExist
	}
	if koordGPU, exist := podRequest[apiext.KoordGPU]; exist {
		if koordGPU.Value() > 100 && koordGPU.Value()%100 != 0 {
			return gpuCombination, fmt.Errorf("failed to validate %v: %v", apiext.KoordGPU, koordGPU.Value())
		}
		gpuCombination |= deviceshare.KoordGPUExist
	}
	if gpuCore, exist := podRequest[apiext.GPUCore]; exist {
		// koordinator.sh/gpu-core should be something like: 25, 50, 75, 100, 200, 300
		if gpuCore.Value() > 100 && gpuCore.Value()%100 != 0 {
			return gpuCombination, fmt.Errorf("failed to validate %v: %v", apiext.GPUCore, gpuCore.Value())
		}
		gpuCombination |= deviceshare.GPUCoreExist
	}
	if _, exist := podRequest[apiext.GPUMemory]; exist {
		gpuCombination |= deviceshare.GPUMemoryExist
	}
	if gpuMemRatio, exist := podRequest[apiext.GPUMemoryRatio]; exist {
		if gpuMemRatio.Value() > 100 && gpuMemRatio.Value()%100 != 0 {
			return gpuCombination, fmt.Errorf("failed to validate %v: %v", apiext.GPUMemoryRatio, gpuMemRatio.Value())
		}
		gpuCombination |= deviceshare.GPUMemoryRatioExist
	}
	if gpuCompute, exist := podRequest[ack.AliyunGPUCompute]; exist {
		if gpuCompute.Value() > 100 && gpuCompute.Value()%100 != 0 {
			return gpuCombination, fmt.Errorf("failed to validate %v: %v", ack.AliyunGPUCompute, gpuCompute.Value())
		}
		gpuCombination |= aliyunGPUComputeExist
	}
	if unifiedGPU, exist := podRequest[unifiedresourceext.GPUResourceAlibaba]; exist {
		if unifiedGPU.Value() > 100 && unifiedGPU.Value()%100 != 0 {
			return gpuCombination, fmt.Errorf("failed to validate %v: %v", unifiedresourceext.GPUResourceAlibaba, unifiedGPU.Value())
		}
		gpuCombination |= unifiedGPUExist
	}
	if unifiedGPUCore, exist := podRequest[unifiedresourceext.GPUResourceCore]; exist {
		if unifiedGPUCore.Value() > 100 && unifiedGPUCore.Value()%100 != 0 {
			return gpuCombination, fmt.Errorf("failed to validate %v: %v", unifiedresourceext.GPUResourceCore, unifiedGPUCore.Value())
		}
		gpuCombination |= unifiedGPUCoreExist
	}
	if _, exist := podRequest[unifiedresourceext.GPUResourceMem]; exist {
		gpuCombination |= unifiedGPUMemoryExist
	}
	if gpuMemRatio, exist := podRequest[unifiedresourceext.GPUResourceMemRatio]; exist {
		if gpuMemRatio.Value() > 100 && gpuMemRatio.Value()%100 != 0 {
			return gpuCombination, fmt.Errorf("failed to validate %v: %v", unifiedresourceext.GPUResourceMemRatio, gpuMemRatio.Value())
		}
		gpuCombination |= unifiedGPUMemoryRatioExist
	}

	if gpuCombination == (deviceshare.NvidiaGPUExist) ||
		gpuCombination == (deviceshare.KoordGPUExist) ||
		gpuCombination == (deviceshare.GPUMemoryRatioExist) ||
		gpuCombination == (deviceshare.GPUCoreExist|deviceshare.GPUMemoryExist) ||
		gpuCombination == (deviceshare.GPUCoreExist|deviceshare.GPUMemoryRatioExist) ||
		gpuCombination == (aliyunGPUComputeExist) ||
		gpuCombination == (unifiedGPUExist) ||
		gpuCombination == (unifiedGPUMemoryRatioExist) ||
		gpuCombination == (unifiedGPUCoreExist|unifiedGPUMemoryExist) ||
		gpuCombination == (unifiedGPUCoreExist|unifiedGPUMemoryRatioExist) {
		return gpuCombination, nil
	}

	return gpuCombination, fmt.Errorf("request is not valid, current combination: %b", gpuCombination)
}

func ConvertGPUResource(podRequest corev1.ResourceList, combination uint) corev1.ResourceList {
	if podRequest == nil || len(podRequest) == 0 {
		klog.Warningf("pod request should not be empty")
		return nil
	}
	switch combination {
	case deviceshare.GPUCoreExist | deviceshare.GPUMemoryExist:
		return corev1.ResourceList{
			apiext.GPUCore:   podRequest[apiext.GPUCore],
			apiext.GPUMemory: podRequest[apiext.GPUMemory],
		}
	case deviceshare.GPUCoreExist | deviceshare.GPUMemoryRatioExist:
		return corev1.ResourceList{
			apiext.GPUCore:        podRequest[apiext.GPUCore],
			apiext.GPUMemoryRatio: podRequest[apiext.GPUMemoryRatio],
		}
	case deviceshare.GPUMemoryRatioExist:
		return corev1.ResourceList{
			apiext.GPUCore:        podRequest[apiext.GPUMemoryRatio],
			apiext.GPUMemoryRatio: podRequest[apiext.GPUMemoryRatio],
		}
	case deviceshare.KoordGPUExist:
		return corev1.ResourceList{
			apiext.GPUCore:        podRequest[apiext.KoordGPU],
			apiext.GPUMemoryRatio: podRequest[apiext.KoordGPU],
		}
	case deviceshare.NvidiaGPUExist:
		nvidiaGpu := podRequest[apiext.NvidiaGPU]
		return corev1.ResourceList{
			apiext.GPUCore:        *resource.NewQuantity(nvidiaGpu.Value()*100, resource.DecimalSI),
			apiext.GPUMemoryRatio: *resource.NewQuantity(nvidiaGpu.Value()*100, resource.DecimalSI),
		}
	case aliyunGPUComputeExist:
		return corev1.ResourceList{
			apiext.GPUCore:        podRequest[ack.AliyunGPUCompute],
			apiext.GPUMemoryRatio: podRequest[ack.AliyunGPUCompute],
		}
	case unifiedGPUCoreExist | unifiedGPUMemoryExist:
		return corev1.ResourceList{
			apiext.GPUCore:   podRequest[unifiedresourceext.GPUResourceCore],
			apiext.GPUMemory: podRequest[unifiedresourceext.GPUResourceMem],
		}
	case unifiedGPUCoreExist | unifiedGPUMemoryRatioExist:
		return corev1.ResourceList{
			apiext.GPUCore:        podRequest[unifiedresourceext.GPUResourceCore],
			apiext.GPUMemoryRatio: podRequest[unifiedresourceext.GPUResourceMemRatio],
		}
	case unifiedGPUMemoryRatioExist:
		return corev1.ResourceList{
			apiext.GPUCore:        podRequest[unifiedresourceext.GPUResourceMemRatio],
			apiext.GPUMemoryRatio: podRequest[unifiedresourceext.GPUResourceMemRatio],
		}
	case unifiedGPUExist:
		return corev1.ResourceList{
			apiext.GPUCore:        podRequest[unifiedresourceext.GPUResourceAlibaba],
			apiext.GPUMemoryRatio: podRequest[unifiedresourceext.GPUResourceAlibaba],
		}
	}
	return nil
}

func GetDeviceAllocations(podAnnotations map[string]string) (apiext.DeviceAllocations, error) {
	if _, ok := podAnnotations[apiext.AnnotationDeviceAllocated]; ok {
		return originGetDeviceAllocations(podAnnotations)
	}
	if _, ok := podAnnotations[unifiedresourceext.AnnotationMultiDeviceAllocStatus]; ok {
		deviceAllocStatus, err := unifiedresourceext.GetMultiDeviceAllocStatus(podAnnotations)
		if err != nil {
			return nil, err
		}
		deviceAllocations := make(apiext.DeviceAllocations)
		for deviceType, allocs := range deviceAllocStatus.AllocStatus {
			switch deviceType {
			case unifiedschedulingv1beta1.GPU:
				koordGPUAllocations, err := convertUnifiedGPUAllocs(allocs)
				if err != nil {
					return nil, err
				}
				if len(koordGPUAllocations) > 0 {
					deviceAllocations[schedulingv1alpha1.GPU] = koordGPUAllocations
				}
			}
		}
		return deviceAllocations, nil
	}
	return nil, nil
}

func convertUnifiedGPUAllocs(allocs []unifiedresourceext.ContainerDeviceAllocStatus) ([]*apiext.DeviceAllocation, error) {
	koordGPUAllocs := make(map[int32]*apiext.DeviceAllocation)
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
			combination, err := ValidateGPURequest(resourceList)
			if err != nil {
				return nil, err
			}
			resourceList = ConvertGPUResource(resourceList, combination)
			koordAllocation := koordGPUAllocs[alloc.Minor]
			if koordAllocation == nil {
				koordAllocation = &apiext.DeviceAllocation{
					Minor: alloc.Minor,
				}
				koordGPUAllocs[alloc.Minor] = koordAllocation
			}
			koordAllocation.Resources = quotav1.Add(koordAllocation.Resources, resourceList)
		}
	}
	if len(koordGPUAllocs) == 0 {
		return nil, nil
	}
	koordDeviceAllocation := make([]*apiext.DeviceAllocation, 0, len(koordGPUAllocs))
	for _, allocation := range koordGPUAllocs {
		koordDeviceAllocation = append(koordDeviceAllocation, allocation)
	}
	if len(koordDeviceAllocation) > 1 {
		sort.Slice(koordDeviceAllocation, func(i, j int) bool {
			return koordDeviceAllocation[i].Minor < koordDeviceAllocation[j].Minor
		})
	}
	return koordDeviceAllocation, nil
}
