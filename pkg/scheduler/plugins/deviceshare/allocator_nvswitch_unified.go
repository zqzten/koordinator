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

package deviceshare

import (
	"strings"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	reservationutil "github.com/koordinator-sh/koordinator/pkg/util/reservation"
)

var (
	gpuModelsSupportNVSwitch = []string{"a100", "a800", "h100", "h800"}
)

const (
	NVSwitch = 1 << 40
)

func init() {
	pflag.StringSliceVar(&gpuModelsSupportNVSwitch, "gpu-model-support-nvswitch", gpuModelsSupportNVSwitch, "The GPU model list that support NVSwitch")

	deviceAllocators[unified.NVSwitchDeviceType] = &NVSwitchHandler{}
	deviceHandlers[unified.NVSwitchDeviceType] = &NVSwitchHandler{}

	DeviceResourceNames[unified.NVSwitchDeviceType] = []corev1.ResourceName{
		unified.NVSwitchResource,
	}
	DeviceResourceFlags[unified.NVSwitchResource] = NVSwitch
	ValidDeviceResourceCombinations[NVSwitch] = true
	ResourceCombinationsMapper[NVSwitch] = func(podRequest corev1.ResourceList) corev1.ResourceList {
		return corev1.ResourceList{
			unified.NVSwitchResource: podRequest[unified.NVSwitchResource],
		}
	}
}

type NVSwitchHandler struct{}

func (a *NVSwitchHandler) CalcDesiredRequestsAndCount(node *corev1.Node, pod *corev1.Pod, podRequests corev1.ResourceList, nodeDevice *nodeDevice, hint *apiext.DeviceHint) (corev1.ResourceList, int, *framework.Status) {
	if reservationutil.IsReservePod(pod) {
		return nil, 0, framework.NewStatus(framework.Skip)
	}

	applyForAll := hint != nil && hint.AllocateStrategy == apiext.ApplyForAllDeviceAllocateStrategy
	if !applyForAll && unified.MustAllocateGPUByPartition(node) {
		return nil, 0, framework.NewStatus(framework.Skip)
	}

	totalDevices := nodeDevice.deviceTotal[unified.NVSwitchDeviceType]
	totalNVSwitches := len(totalDevices)
	if totalNVSwitches == 0 {
		if applyForAll {
			return nil, 0, framework.NewStatus(framework.Unschedulable, "Insufficient NVSwitch devices")
		}
		return nil, 0, framework.NewStatus(framework.Skip)
	}

	gpuModels := strings.ToLower(node.Labels[unified.LabelGPUModelSeries])
	found := false
	for _, v := range gpuModelsSupportNVSwitch {
		if v == gpuModels {
			found = true
			break
		}
	}
	if !found {
		return nil, 0, framework.NewStatus(framework.Skip)
	}
	// Allocate at least 1 NVSwitch instance. The final number of allocations is calculated in Allocator.
	return podRequests.DeepCopy(), 1, nil
}

func (a *NVSwitchHandler) Allocate(requestCtx *requestContext, nodeDevice *nodeDevice, desiredCount int, maxDesiredCount int, preferredPCIEs sets.String) ([]*apiext.DeviceAllocation, *framework.Status) {
	numGPU := requestCtx.desiredCountPerDeviceType[schedulingv1alpha1.GPU]
	nvSwitchHint := requestCtx.hints[unified.NVSwitchDeviceType]
	applyForAll := nvSwitchHint != nil && nvSwitchHint.AllocateStrategy == apiext.ApplyForAllDeviceAllocateStrategy
	if numGPU == 0 && !applyForAll {
		return nil, nil
	}

	if !applyForAll {
		// NVIDIA DGX A100 和 HGX A100 直通 GPU 到 VM 时，还需要根据 GPU 的数量直通一定量的 NVSwitch 实现 GPU 间的联通
		// 参考 NVIDIA fabric-manager-user-guide.pdf Chapter6 Full Passthrough Virtualization Model 给出了如下的 GPU&NVSwitch 的分配关系
		/*
			| Number of GPUs per VM | Number of NVSwitches per VM | Enabled NVLink Interconnects Per GPU | Enabled NVLink Interconnects Per NVSwitch | Constraints                                |
			|-----------------------|-----------------------------|--------------------------------------|-------------------------------------------|--------------------------------------------|
			| 16                    | 12                          | 12 out of 12                         | 8                                        | One set of eight GPUs from each GPU Baseboard. |
			| 8                     | 6                           | 12 out of 12                         | 6                                        | Two sets of four GPUs from each GPU Baseboard. |
			| 4                     | 3                           | 6 out of 12                          | 4                                        | Four sets of two GPUs from each GPU Baseboard. |
			| 2                     | 1                           | 2 out of 12                          | 2                                        | None                                       |
			| 1                     | 0                           | 0 out of 12                          | 0                                        | None                                       |
		*/
		if numGPU == 16 {
			desiredCount = 12
		} else if numGPU == 7 {
			// 基于 sGPU 支持 GPU Share 时，需要在 Host 保留一个 NVSwitch
			desiredCount = 5
		} else if numGPU == 8 {
			desiredCount = 6
		} else {
			// 这里是一种内部特殊用法，如果分配的 GPU 不是按上述结构划分的，也需要分配 NVSwitch
			desiredCount = numGPU - 1
		}
	}
	if desiredCount == 0 && !applyForAll {
		return nil, nil
	}
	totalNVSwitches := len(nodeDevice.deviceTotal[unified.NVSwitchDeviceType])
	if desiredCount > totalNVSwitches || applyForAll {
		desiredCount = totalNVSwitches
	}

	return defaultAllocateDevices(
		nodeDevice,
		requestCtx,
		requestCtx.requestsPerInstance[unified.NVSwitchDeviceType],
		desiredCount,
		desiredCount,
		unified.NVSwitchDeviceType,
		nil)
}
