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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	reservationutil "github.com/koordinator-sh/koordinator/pkg/util/reservation"
)

const (
	NVSwitch = 1 << 40
)

func init() {
	deviceAllocators[unified.NVSwitchDeviceType] = &NVSwitchHandler{}

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

	deviceHandlers[unified.NVSwitchDeviceType] = &DefaultDeviceHandler{resourceName: unified.NVSwitchResource}
}

type NVSwitchHandler struct{}

func (a *NVSwitchHandler) Allocate(requestCtx *requestContext, nodeDevice *nodeDevice, desiredCount int, maxDesiredCount int, preferredPCIEs sets.String) ([]*apiext.DeviceAllocation, *framework.Status) {
	if reservationutil.IsReservePod(requestCtx.pod) {
		return nil, nil
	}
	if requestCtx.node.Labels[unified.LabelGPUModelSeries] == "PPU" {
		// Aliyun PPU does not support NVSwitch
		return nil, nil
	}
	numGPU := requestCtx.desiredCountPerDeviceType[schedulingv1alpha1.GPU]
	nvSwitchHint := requestCtx.hints[unified.NVSwitchDeviceType]
	applyForAll := nvSwitchHint != nil && nvSwitchHint.AllocateStrategy == apiext.ApplyForAllDeviceAllocateStrategy
	if numGPU == 0 && !applyForAll {
		return nil, nil
	}
	if !applyForAll && unified.MustAllocateGPUByPartition(requestCtx.node) {
		return nil, nil
	}

	totalNVSwitches := len(nodeDevice.deviceTotal[unified.NVSwitchDeviceType])
	if totalNVSwitches == 0 {
		if applyForAll {
			return nil, framework.NewStatus(framework.Unschedulable, "Insufficient NVSwitch devices")
		}
		return nil, nil
	}

	if !applyForAll {
		if numGPU == 16 {
			desiredCount = 12
		} else if numGPU == 7 {
			desiredCount = 5
		} else if numGPU == 8 {
			desiredCount = 6
		} else {
			desiredCount = numGPU - 1
		}
	}

	if desiredCount == 0 && !applyForAll {
		return nil, nil
	}
	if desiredCount > totalNVSwitches || applyForAll {
		desiredCount = totalNVSwitches
	}

	requestPerInstance := corev1.ResourceList{
		unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
	}
	return defaultAllocateDevices(
		nodeDevice,
		requestCtx,
		requestPerInstance,
		desiredCount,
		desiredCount,
		unified.NVSwitchDeviceType,
		nil)
}
