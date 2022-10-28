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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/deviceshare"
)

const (
	GPUComputeExist = 1 << 63
)

var deviceResourceNames = map[schedulingv1alpha1.DeviceType][]corev1.ResourceName{
	schedulingv1alpha1.GPU:  {apiext.NvidiaGPU, apiext.KoordGPU, apiext.GPUCore, apiext.GPUMemory, apiext.GPUMemoryRatio, ack.AliyunGPUCompute},
	schedulingv1alpha1.RDMA: {apiext.KoordRDMA},
	schedulingv1alpha1.FPGA: {apiext.KoordFPGA},
}

func init() {
	deviceshare.DeviceResourceNames = deviceResourceNames
	deviceshare.ValidateGPURequest = ValidateGPURequest
	deviceshare.ConvertGPUResource = ConvertGPUResource
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
		gpuCombination |= GPUComputeExist
	}

	if gpuCombination == (deviceshare.NvidiaGPUExist) ||
		gpuCombination == (deviceshare.KoordGPUExist) ||
		gpuCombination == (deviceshare.GPUCoreExist|deviceshare.GPUMemoryExist) ||
		gpuCombination == (deviceshare.GPUCoreExist|deviceshare.GPUMemoryRatioExist) ||
		gpuCombination == (GPUComputeExist) {
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
	case GPUComputeExist:
		return corev1.ResourceList{
			apiext.GPUCore:        podRequest[ack.AliyunGPUCompute],
			apiext.GPUMemoryRatio: podRequest[ack.AliyunGPUCompute],
		}
	}
	return nil
}
