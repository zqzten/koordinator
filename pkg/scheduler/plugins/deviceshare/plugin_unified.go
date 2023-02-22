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
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	unifiedschedulingv1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	schedulingv1alpha1listers "github.com/koordinator-sh/koordinator/pkg/client/listers/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
)

func (p *Plugin) appendInternalAnnotations(pod *corev1.Pod, allocResult apiext.DeviceAllocations, nodeName string) error {
	ack.AppendAckAnnotations(pod, allocResult)
	if isVirtualGPUCard(allocResult) {
		allocMinor := allocResult[schedulingv1alpha1.GPU][0].Minor
		gpuMemory := p.nodeDeviceCache.getNodeDevice(nodeName).deviceTotal[schedulingv1alpha1.GPU][int(allocMinor)][apiext.ResourceGPUMemory]
		pod.Annotations[ack.AnnotationAliyunEnvMemDev] = fmt.Sprintf("%v", gpuMemory.Value()/1024/1024/1024)
		gpuMemoryPod := allocResult[schedulingv1alpha1.GPU][0].Resources[apiext.ResourceGPUMemory]
		pod.Annotations[ack.AnnotationAliyunEnvMemPod] = fmt.Sprintf("%v", gpuMemoryPod.Value()/1024/1024/1024)
	}
	if err := appendUnifiedDeviceAllocStatus(pod, allocResult); err != nil {
		return err
	}
	if err := appendNetworkingVFMetas(pod, allocResult); err != nil {
		return err
	}
	return appendRundResult(pod, allocResult, p)
}

func isVirtualGPUCard(alloc apiext.DeviceAllocations) bool {
	for deviceType, deviceAllocations := range alloc {
		if deviceType != schedulingv1alpha1.GPU {
			continue
		}
		for _, deviceAlloc := range deviceAllocations {
			if deviceAlloc.Resources.Name(apiext.ResourceGPUCore, apiresource.DecimalSI).Value() < 100 {
				return true
			}
		}
	}
	return false
}

func appendUnifiedDeviceAllocStatus(pod *corev1.Pod, deviceAllocations apiext.DeviceAllocations) error {
	if !enableUnifiedDevice {
		return nil
	}

	allocStatus := &unifiedresourceext.MultiDeviceAllocStatus{}
	allocStatus.AllocStatus = make(map[unifiedschedulingv1beta1.DeviceType][]unifiedresourceext.ContainerDeviceAllocStatus)
	var minors []string
	totalGPUResources := make(corev1.ResourceList)
	for deviceType, allocations := range deviceAllocations {
		if len(allocations) <= 0 {
			continue
		}
		unifiedAllocs := make([]unifiedresourceext.Alloc, 0, len(allocations))
		for _, deviceAllocation := range allocations {
			resources := extunified.ConvertToUnifiedGPUResources(deviceAllocation.Resources)
			resourceList := make(map[string]apiresource.Quantity)
			for name, quantity := range resources {
				resourceList[string(name)] = quantity
			}
			unifiedAlloc := unifiedresourceext.Alloc{
				Minor:     deviceAllocation.Minor,
				Resources: resourceList,
				IsSharing: !isExclusiveGPURes(resourceList),
			}
			unifiedAllocs = append(unifiedAllocs, unifiedAlloc)
			if deviceType == schedulingv1alpha1.GPU {
				minors = append(minors, strconv.Itoa(int(deviceAllocation.Minor)))
				totalGPUResources = quotav1.Add(totalGPUResources, resources)
			}
		}

		containerDeviceAllocStatuses := make([]unifiedresourceext.ContainerDeviceAllocStatus, 1)
		containerDeviceAllocStatuses[0].DeviceAllocStatus.Allocs = unifiedAllocs
		switch deviceType {
		case schedulingv1alpha1.GPU:
			allocStatus.AllocStatus[unifiedschedulingv1beta1.GPU] = containerDeviceAllocStatuses
		case schedulingv1alpha1.RDMA:
			allocStatus.AllocStatus[unifiedschedulingv1beta1.RDMA] = containerDeviceAllocStatuses
		case schedulingv1alpha1.FPGA:
			allocStatus.AllocStatus[unifiedschedulingv1beta1.FPGA] = containerDeviceAllocStatuses
		}
	}
	data, err := json.Marshal(allocStatus)
	if err != nil {
		return err
	}
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[unifiedresourceext.AnnotationMultiDeviceAllocStatus] = string(data)
	visibleDevices := strings.Join(minors, ",")
	if len(minors) > 0 {
		pod.Annotations[unifiedresourceext.AnnotationNVIDIAVisibleDevices] = visibleDevices
	}

	if k8sfeature.DefaultFeatureGate.Enabled(features.UnifiedDeviceScheduling) && len(totalGPUResources) > 0 {
		totalGPUMemory := totalGPUResources[unifiedresourceext.GPUResourceMem]
		totalGPUMemoryRatio := totalGPUResources[unifiedresourceext.GPUResourceMemRatio]
		if totalGPUMemory.IsZero() {
			return fmt.Errorf("unreached error but got, missing GPUResourceMem")
		}
		for i := range pod.Spec.Containers {
			container := &pod.Spec.Containers[i]
			if !hasDeviceResource(container.Resources.Requests, schedulingv1alpha1.GPU) {
				continue
			}
			combination, err := ValidateGPURequest(container.Resources.Requests)
			if err != nil {
				return err
			}
			resources := ConvertGPUResource(container.Resources.Requests, combination)
			gpuMemoryQuantity := resources[apiext.ResourceGPUMemory]
			gpuMemoryRatioQuantity := resources[apiext.ResourceGPUMemoryRatio]
			if gpuMemoryQuantity.IsZero() && gpuMemoryRatioQuantity.IsZero() {
				continue
			}
			needPatch := false
			var memoryRatio int64
			if gpuMemoryQuantity.Value() > 0 {
				needPatch = true
				memoryRatio = gpuMemoryQuantity.Value() * totalGPUMemoryRatio.Value() / totalGPUMemory.Value()
			} else if gpuMemoryRatioQuantity.Value() > 0 {
				needPatch = true
				memoryRatio = gpuMemoryRatioQuantity.Value()
			}
			if needPatch {
				addContainerGPUResourceForPatch(container, unifiedresourceext.GPUResourceMemRatio, memoryRatio)
				setContainerEnv(container, &corev1.EnvVar{Name: "NVIDIA_VISIBLE_DEVICES", Value: visibleDevices})
			}
		}
	}

	return nil
}

func setContainerEnv(container *corev1.Container, envVar *corev1.EnvVar) {
	for i := range container.Env {
		if container.Env[i].Name == envVar.Name {
			container.Env[i] = *envVar
			return
		}
	}

	container.Env = append(container.Env, *envVar)
}

// addContainerResourceForPatch adds container GPU resources to patch bytes to update pod resource specs
func addContainerGPUResourceForPatch(container *corev1.Container, resourceName corev1.ResourceName, resourceQuantity int64) {
	p := apiresource.Quantity{}
	if resourceQuantity <= 0 {
		resourceQuantity = 1
	}
	p.Set(resourceQuantity)
	if container.Resources.Limits == nil {
		container.Resources.Limits = make(corev1.ResourceList)
	}
	if container.Resources.Requests == nil {
		container.Resources.Requests = make(corev1.ResourceList)
	}
	container.Resources.Limits[resourceName] = p
	container.Resources.Requests[resourceName] = p
}

// res contains exclusive GPU if and only if:
// GPU subResources (gpu-mem-ratio, gpu-core) are multiples of 100 and of the same value.
func isExclusiveGPURes(res map[string]apiresource.Quantity) bool {
	var subResVal int64
	for _, resName := range []corev1.ResourceName{unifiedresourceext.GPUResourceMemRatio, unifiedresourceext.GPUResourceCore} {
		if value, ok := res[resName.String()]; ok && (value.Value() > 0) {
			if value.Value()%100 != 0 {
				// sub resource not in full 100s
				return false
			}
			if subResVal == 0 {
				subResVal = value.Value()
			} else {
				if subResVal != value.Value() {
					// sub resources not of the same value
					return false
				}
			}
		} else {
			// missing one of the two sub resources
			return false
		}
	}
	return true
}

func appendNetworkingVFMetas(pod *corev1.Pod, allocResult apiext.DeviceAllocations) error {
	rdmaAllocs, ok := allocResult[schedulingv1alpha1.RDMA]
	if !ok {
		return nil
	}
	var metas []extunified.VFMeta
	for _, v := range rdmaAllocs {
		if len(v.Extension) == 0 {
			continue
		}
		var allocationExt extunified.DeviceAllocationExtension
		if err := json.Unmarshal(v.Extension, &allocationExt); err != nil {
			return err
		}
		if allocationExt.RDMAAllocatedExtension != nil {
			for _, vf := range allocationExt.RDMAAllocatedExtension.VFs {
				metas = append(metas, extunified.VFMeta{
					BondName:   vf.BondName,
					BondSlaves: allocationExt.BondSlaves,
					VFIndex:    int(vf.Minor),
					PCIAddress: vf.BusID,
				})
			}
		}
	}
	if len(metas) == 0 {
		return nil
	}
	return extunified.SetVFMeta(pod, metas)
}

func getDevicesBusID(pod *corev1.Pod, allocResult apiext.DeviceAllocations, deviceLister schedulingv1alpha1listers.DeviceLister) (map[schedulingv1alpha1.DeviceType]map[int]string, error) {
	device, err := deviceLister.Get(pod.Spec.NodeName)
	if err != nil {
		return nil, fmt.Errorf("not found nodeDevice for node %v", pod.Spec.NodeName)
	}

	pciInfos, err := extunified.GetDevicePCIInfos(device.Annotations)
	if err != nil {
		return nil, err
	}
	pciInfoMap := map[schedulingv1alpha1.DeviceType]map[int32]extunified.DevicePCIInfo{}
	for _, v := range pciInfos {
		m := pciInfoMap[v.Type]
		if m == nil {
			m = map[int32]extunified.DevicePCIInfo{}
			pciInfoMap[v.Type] = m
		}
		m[v.Minor] = v
	}

	devicesBusID := map[schedulingv1alpha1.DeviceType]map[int]string{}

	for deviceType, allocations := range allocResult {
		if deviceType != schedulingv1alpha1.GPU && deviceType != extunified.NVSwitchDeviceType {
			continue
		}
		devices := pciInfoMap[deviceType]
		if len(devices) == 0 {
			return nil, fmt.Errorf("not found %v devices on node %v", deviceType, pod.Spec.NodeName)
		}
		m := devicesBusID[deviceType]
		if m == nil {
			m = map[int]string{}
			devicesBusID[deviceType] = m
		}
		for _, v := range allocations {
			d, ok := devices[v.Minor]
			if !ok {
				return nil, fmt.Errorf("not found %v device by minor %d on node %v", deviceType, v.Minor, pod.Spec.NodeName)
			}
			m[int(v.Minor)] = d.BusID
		}
	}

	return devicesBusID, nil
}

type deviceMinorBusIDPair struct {
	minor int
	busID string
}

func toDeviceMinorBusIDPairs(busIDs map[int]string) []deviceMinorBusIDPair {
	if len(busIDs) == 0 {
		return nil
	}
	var pairs []deviceMinorBusIDPair
	for k, v := range busIDs {
		pairs = append(pairs, deviceMinorBusIDPair{
			minor: k,
			busID: v,
		})
	}
	return pairs
}

func appendRundResult(pod *corev1.Pod, allocResult apiext.DeviceAllocations, pl *Plugin) error {
	if pod.Spec.RuntimeClassName == nil || *pod.Spec.RuntimeClassName != "rund" {
		return nil
	}
	extendedHandle, ok := pl.handle.(frameworkext.ExtendedHandle)
	if !ok {
		return fmt.Errorf("expect handle to be type frameworkext.ExtendedHandle, got %T", pl.handle)
	}
	deviceLister := extendedHandle.KoordinatorSharedInformerFactory().Scheduling().V1alpha1().Devices().Lister()
	devicesBusID, err := getDevicesBusID(pod, allocResult, deviceLister)
	if err != nil {
		return err
	}

	var passthroughDevices []string
	var nvSwitches []string
	for _, deviceType := range []schedulingv1alpha1.DeviceType{schedulingv1alpha1.GPU, extunified.NVSwitchDeviceType} {
		busIDs := devicesBusID[deviceType]
		if len(busIDs) == 0 {
			continue
		}
		pairs := toDeviceMinorBusIDPairs(busIDs)
		sort.Slice(pairs, func(i, j int) bool {
			return pairs[i].minor < pairs[j].minor
		})
		for _, v := range pairs {
			passthroughDevices = append(passthroughDevices, v.busID)
			if deviceType == extunified.NVSwitchDeviceType {
				nvSwitches = append(nvSwitches, strconv.Itoa(v.minor))
			}
		}
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	if len(passthroughDevices) > 0 {
		pod.Annotations[extunified.AnnotationRundPassthoughPCI] = strings.Join(passthroughDevices, ",")
	}
	if len(nvSwitches) > 0 {
		pod.Annotations[extunified.AnnotationRundNVSwitchOrder] = strings.Join(nvSwitches, ",")
	}

	device, err := deviceLister.Get(pod.Spec.NodeName)
	if err != nil {
		return err
	}
	matchedVersion, err := matchDriverVersions(pod, device)
	if err != nil {
		return nil
	}
	if matchedVersion == "" {
		return fmt.Errorf("unmatched driver versions")
	}

	pod.Annotations[extunified.AnnotationRundNvidiaDriverVersion] = matchedVersion
	return nil
}
