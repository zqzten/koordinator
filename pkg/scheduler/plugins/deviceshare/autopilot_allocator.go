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
	"context"
	"encoding/json"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/client-go/tools/cache"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	schedulingv1alpha1listers "github.com/koordinator-sh/koordinator/pkg/client/listers/scheduling/v1alpha1"
	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
)

/*
AutopilotAllocator 核心逻辑是根据 GPU和RDMA所在的 NUMA 拓扑与PCIE拓扑结构分配GPU和RDMA资源。期望分配的GPU和RDMA 获取最佳的性能。
完整的设计文档详见：https://aliyuque.antfin.com/obdvnp/apfnvx/tuv919?singleDoc# 《PAI on Koordinator 混合云 - GPU + RDMA 调度》

内部机器的硬件拓扑结构基本如下：

+--NUMA Socket 0
|	+-- NUMA Node 0
|	|	+-- PCI-E 0
|	|	|	+-- GPU: GPU0, GPU1
|	|	|   +-- NIC: mlx5_bond0(eth0, eth1)
|	|	|		+-- VF in mlx5_bond0(vf_0, vf_1, ...)
|	|	|	+-- NIC: mlx5_bond4 (not enable VF)
|	|	|
|	|	+-- PCI-E 1
|	|	|	+-- GPU: GPU2, GPU3
|	|	|   +-- NIC: mlx5_bond1(eth2, eth3)
|	|	|		+-- VF in mlx5_bond1(vf_0, vf_1, ...)
+--NUMA Socket 1
|	+-- NUMA Node 1
|	|	+-- PCI-E 2
|	|	|	+-- GPU: GPU4, GPU5
|	|	|   +-- NIC: mlx5_bond2(eth4, eth5)
|	|	|		+-- VF in mlx5_bond2(vf_0, vf_1, ...)
|	|	+-- PCI-E 1
|	|	|	+-- GPU: GPU6, GPU7
|	|	|   +-- NIC: mlx5_bond3(eth6, eth7)
|	|	|		+-- VF in mlx5_bond3(vf_0, vf_1, ...)
----+

单独分配 GPU 时，尽量扎堆在 PCI-E/NUMA Node/NUMA Socket，减少资源碎片。

1. 同时申请了GPU和RDMA，优先分配在同一个PCI-e switch下(SamePCIeSwitch) -> 同一个CPU socket(SameCPUSocket) -> 跨CPU socket(CrossCPUSocket)
2. 仅申请了GPU， 优先分配同一个PCIe switch下剩余RDMA少的GPU
3. 仅申请了RDMA网卡, 优先分配同一个PCIe switch下剩余GPU少的RDMA网卡

GPU 和 RDMA 一起分配时，优先选择同一个 PCI-E 上两种资源都存在的情况；再逐步按照 NUMA Node/Socket 拓扑分配资源。
GPU 和 RDMA 一起分配时，如果机器资源是异构的，即 RDMA 和 GPU 一定是跨 PCI-E 的情况，至少分配一个 VF。

为了分配保障分配的多个 GPU 获得最佳性能，还需要考虑 NVIDIA NVLink 机制。该机制在不同的型号上有不同的实现。
一般是大家熟知的总线类的 NVLink，不过目前在 PAI EFLOPS等场景都使用的是 NVIDIA A100 型号 GPU，使用 NVSwitch 实现全链接。
因此目前的实现里暂时只支持了 NVSwitch。
如果要申请多个 GPU 且当前节点存在 NVSwitch 设备，则会按照申请的GPU的数量分配对应的NVSwitch。考虑到 A100 型号的架构，最多有12个NVSwitch，16个GPU，NVSwitch的分配数量按如下方式分配：
- 如果申请的16个GPU，则要求分配12个NVSwitch
- 如果申请个8个GPU，则分配6个NVSwitch
- 剩余的按照申请 GPU 数量减一作为 NVSwitch的分配数量，例如5个GPU，则分配4个NVSwitch

根据以上逻辑，需要在原有的 Device CRD 基础上追加 Device Topology，描述GPU和RDMA所在的PCI-E/NUMA拓扑结构。
还需要追加 RDMA Topology，描述 RDMA 物理网卡和虚拟设备 VF 之间的关系；
Device CRD 中还需要上报 NVSwitch 设备信息，以及要求上报 GPU 和 NVSwitch 的BUS ID，用于支持 RunD 场景直通设备。
*/

func init() {
	allocatorFactories[AutopilotAllocatorName] = NewAutopilotAllocator
}

const (
	AutopilotAllocatorName = "autopilotAllocator"
)

type AutopilotAllocator struct {
	vfCache      *VFCache
	deviceLister schedulingv1alpha1listers.DeviceLister
}

func NewAutopilotAllocator(
	options AllocatorOptions,
) Allocator {
	vfCache := newVFCache()
	podInformer := options.SharedInformerFactory.Core().V1().Pods().Informer()
	eventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc:    vfCache.onPodAdd,
		UpdateFunc: vfCache.onPodUpdate,
		DeleteFunc: vfCache.onPodDelete,
	}
	// make sure Pods are loaded before scheduler starts working
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), options.SharedInformerFactory, podInformer, eventHandler)
	deviceLister := options.KoordSharedInformerFactory.Scheduling().V1alpha1().Devices().Lister()
	return &AutopilotAllocator{
		vfCache:      vfCache,
		deviceLister: deviceLister,
	}
}

func (a *AutopilotAllocator) Name() string {
	return AutopilotAllocatorName
}

func (a *AutopilotAllocator) Allocate(nodeName string, pod *corev1.Pod, podRequest corev1.ResourceList, nodeDevice *nodeDevice) (apiext.DeviceAllocations, error) {
	device, err := a.deviceLister.Get(nodeName)
	if err != nil {
		return nil, err
	}

	if pod.Spec.RuntimeClassName != nil && *pod.Spec.RuntimeClassName == "rund" && hasDeviceResource(podRequest, schedulingv1alpha1.GPU) {
		matchedVersion, err := matchDriverVersions(pod, device)
		if err != nil {
			return nil, err
		}
		if matchedVersion == "" {
			return nil, fmt.Errorf("unmatched driver versions")
		}
	}

	rdmaTopology, err := unified.GetRDMATopology(device.Annotations)
	if err != nil {
		return nil, err
	}
	if len(rdmaTopology.VFs) == 0 {
		rdmaRequest := podRequest[apiext.ResourceRDMA]
		if !rdmaRequest.IsZero() && mustAllocateVF(pod) {
			return nil, fmt.Errorf("invalid RDMA Topology")
		}
	}
	deviceTopology, err := unified.GetDeviceTopology(device.Annotations)
	if err != nil {
		return nil, err
	}
	removeNonVirtualizedRDMAs(deviceTopology, rdmaTopology)

	podRequest = podRequest.DeepCopy()
	if !hasDeviceResource(podRequest, schedulingv1alpha1.GPU) {
		return a.allocateNonGPUDevices(nodeName, nodeDevice, podRequest, mustAllocateVF(pod), deviceTopology, rdmaTopology)
	}

	nodeDeviceTotal := nodeDevice.deviceTotal[schedulingv1alpha1.GPU]
	if len(nodeDeviceTotal) <= 0 {
		return nil, fmt.Errorf("node does not have enough GPU")
	}

	rdmaRequest := podRequest[apiext.ResourceRDMA]
	gpuRequest := quotav1.Mask(podRequest, DeviceResourceNames[schedulingv1alpha1.GPU])
	fillGPUTotalMem(nodeDeviceTotal, gpuRequest)

	gpuWanted := int64(1)
	var gpuRequestPerCard corev1.ResourceList
	if isMultipleGPUPod(gpuRequest) {
		gpuCore, gpuMem, gpuMemRatio := gpuRequest[apiext.ResourceGPUCore], gpuRequest[apiext.ResourceGPUMemory], gpuRequest[apiext.ResourceGPUMemoryRatio]
		gpuWanted = gpuCore.Value() / 100
		gpuRequestPerCard = corev1.ResourceList{
			apiext.ResourceGPUCore:        *resource.NewQuantity(gpuCore.Value()/gpuWanted, resource.DecimalSI),
			apiext.ResourceGPUMemory:      *resource.NewQuantity(gpuMem.Value()/gpuWanted, resource.BinarySI),
			apiext.ResourceGPUMemoryRatio: *resource.NewQuantity(gpuMemRatio.Value()/gpuWanted, resource.DecimalSI),
		}
	} else {
		gpuRequestPerCard = gpuRequest
	}

	deviceAllocations, _, err := a.allocateResourcesByPCIE(nodeName, nodeDevice, gpuRequestPerCard, rdmaRequest, mustAllocateVF(pod), int(gpuWanted), deviceTopology, rdmaTopology)
	if err != nil {
		var zeroRDMARequest resource.Quantity
		var allocatedPCIEs sets.Int
		deviceAllocations, allocatedPCIEs, err = a.allocateResourcesByPCIE(nodeName, nodeDevice, gpuRequestPerCard, zeroRDMARequest, false, int(gpuWanted), deviceTopology, rdmaTopology)
		if err != nil {
			return nil, err
		}
		if !rdmaRequest.IsZero() && mustAllocateVF(pod) {
			rdmaAllocations, err := a.allocateVFs(nodeName, rdmaRequest, len(allocatedPCIEs), false, allocatedPCIEs, deviceTopology, rdmaTopology)
			if err != nil {
				return nil, err
			}
			deviceAllocations[schedulingv1alpha1.RDMA] = rdmaAllocations
		}
	}
	if !rdmaRequest.IsZero() && !mustAllocateVF(pod) {
		rdmaAllocations, err := a.allocatePFs(nodeDevice, rdmaRequest)
		if err != nil {
			return nil, err
		}
		deviceAllocations[schedulingv1alpha1.RDMA] = rdmaAllocations
	}

	nvSwitches, err := a.allocateNVSwitches(len(deviceAllocations[schedulingv1alpha1.GPU]), nodeDevice)
	if err != nil {
		return nil, err
	}
	if len(nvSwitches) > 0 {
		deviceAllocations[unified.NVSwitchDeviceType] = nvSwitches
	}

	return deviceAllocations, nil
}

func (a *AutopilotAllocator) allocateNonGPUDevices(
	nodeName string,
	nodeDevice *nodeDevice,
	podRequest corev1.ResourceList,
	requireAllocateVF bool,
	deviceTopology *unified.DeviceTopology,
	rdmaTopology *unified.RDMATopology,
) (apiext.DeviceAllocations, error) {
	deviceAllocations := apiext.DeviceAllocations{}
	rdmaRequest := podRequest[apiext.ResourceRDMA]
	if !rdmaRequest.IsZero() {
		var rdmaAllocations []*apiext.DeviceAllocation
		var err error
		if requireAllocateVF {
			rdmaAllocations, err = a.allocateVFs(nodeName, rdmaRequest, 1, false, nil, deviceTopology, rdmaTopology)
		} else {
			rdmaAllocations, err = a.allocatePFs(nodeDevice, rdmaRequest)
		}
		if err != nil {
			return nil, err
		}
		deviceAllocations[schedulingv1alpha1.RDMA] = rdmaAllocations
		delete(podRequest, apiext.ResourceRDMA)
	}
	if len(podRequest) > 0 {
		otherDeviceAllocations, err := nodeDevice.tryAllocateDevice(podRequest)
		if err != nil {
			return nil, err
		}
		for k, v := range otherDeviceAllocations {
			deviceAllocations[k] = v
		}
	}
	return deviceAllocations, nil
}

func (a *AutopilotAllocator) allocateResourcesByPCIE(
	nodeName string,
	nodeDevice *nodeDevice,
	gpuRequestPerCard corev1.ResourceList,
	rdmaRequest resource.Quantity,
	requireAllocateVF bool,
	gpuWanted int,
	deviceTopology *unified.DeviceTopology,
	rdmaTopology *unified.RDMATopology,
) (apiext.DeviceAllocations, sets.Int, error) {
	acc := newDeviceAccumulator(deviceTopology, nodeDevice, gpuWanted)
	pcieSwitchGroups := acc.freePCIESwitchesInNode()
	allocations, allocatedPCIEs, err := a.allocateByPCIEGroup(pcieSwitchGroups, gpuRequestPerCard, rdmaRequest, acc.needCount())
	if err != nil {
		return nil, nil, err
	}
	acc.take(allocations, allocatedPCIEs)

	numaNodeCount := 0
	if len(deviceTopology.NUMASockets) > 0 {
		numaNodeCount = len(deviceTopology.NUMASockets[0].NUMANodes)
	}
	if !acc.isSatisfied() && numaNodeCount > 1 {
		pcieSwitchGroups = acc.freePCIESwitchesInSocket()
		allocations, allocatedPCIEs, err = a.allocateByPCIEGroup(pcieSwitchGroups, gpuRequestPerCard, rdmaRequest, acc.needCount())
		if err != nil {
			return nil, nil, err
		}
		acc.take(allocations, allocatedPCIEs)
	}

	if !acc.isSatisfied() {
		allocations, allocatedPCIEs, err = a.allocateByPCIE(acc.freePCIESwitches(), gpuRequestPerCard, rdmaRequest, acc.needCount())
		if err != nil {
			return nil, nil, err
		}
		acc.take(allocations, allocatedPCIEs)
	}

	if !acc.isSatisfied() {
		return nil, nil, fmt.Errorf("node does not have enough GPU")
	}

	deviceAllocations := acc.result
	if !rdmaRequest.IsZero() && requireAllocateVF {
		rdmaAllocations, err := a.allocateVFs(nodeName, rdmaRequest, acc.allocatedPCIEs.Len(), true, acc.allocatedPCIEs, deviceTopology, rdmaTopology)
		if err != nil {
			return nil, nil, err
		}
		deviceAllocations[schedulingv1alpha1.RDMA] = rdmaAllocations
	}
	return deviceAllocations, acc.allocatedPCIEs, nil
}

func (a *AutopilotAllocator) allocateByPCIEGroup(
	pcieSwitchGroups []*pcieSwitchGroup,
	gpuRequestPerCard corev1.ResourceList,
	rdmaRequest resource.Quantity,
	gpuWanted int,
) (apiext.DeviceAllocations, sets.Int, error) {
	for _, group := range pcieSwitchGroups {
		count := int(group.free[schedulingv1alpha1.GPU] / 100)
		if count < gpuWanted {
			continue
		}
		allocations, allocatedPCIEs, err := a.allocateByPCIE(group.pcieSwitches, gpuRequestPerCard, rdmaRequest, gpuWanted)
		if err != nil {
			continue
		}
		return allocations, allocatedPCIEs, nil
	}
	return nil, nil, nil
}

func (a *AutopilotAllocator) allocateByPCIE(
	freePCIESwitches []*pcieSwitch,
	gpuRequestPerCard corev1.ResourceList,
	rdmaRequest resource.Quantity,
	gpuWanted int,
) (apiext.DeviceAllocations, sets.Int, error) {
	satisfiedDeviceCount := 0
	totalDeviceAllocations := apiext.DeviceAllocations{}
	allocatedPCIEs := sets.NewInt()
	var skippedPCIEs []*pcieSwitch
allocateLoop:
	for _, alignmentFirst := range []bool{true, false} {
		pcieSwitches := freePCIESwitches
		if !alignmentFirst {
			pcieSwitches = skippedPCIEs
		}
		for _, pcie := range pcieSwitches {
			totalFreeGPUCore := sumDeviceResource(pcie.nodeDevice.deviceFree[schedulingv1alpha1.GPU], apiext.ResourceGPUCore)
			gpuCoreRequest := gpuRequestPerCard[apiext.ResourceGPUCore]
			freeCount := int(totalFreeGPUCore / gpuCoreRequest.Value())
			if freeCount == 0 {
				freeCount = 1
			}
			wanted := gpuWanted - satisfiedDeviceCount
			// FIXME(joseph): 应该要优先考虑全分配场景，例如freeCount对应的分别是2,4,5, wanted=4，那么应该优先选择第二个PCIE
			if alignmentFirst && wanted != freeCount {
				if freeCount == 1 || wanted%freeCount != 0 {
					skippedPCIEs = append(skippedPCIEs, pcie)
					continue
				}
			}
			for i := 0; i < freeCount; i++ {
				resourceRequest := gpuRequestPerCard.DeepCopy()
				if !rdmaRequest.IsZero() {
					resourceRequest[apiext.ResourceRDMA] = rdmaRequest
				}
				deviceAllocations, err := pcie.nodeDevice.tryAllocateDevice(resourceRequest)
				if err != nil {
					break
				}
				for k, v := range deviceAllocations {
					totalDeviceAllocations[k] = append(totalDeviceAllocations[k], v...)
					pcie.nodeDevice.updateDeviceUsed(k, v, true)
				}
				pcie.nodeDevice.resetDeviceFree(schedulingv1alpha1.GPU)
				pcie.nodeDevice.resetDeviceFree(schedulingv1alpha1.RDMA)
				allocatedPCIEs.Insert(pcie.pcieIndex)
				gpuAllocations := deviceAllocations[schedulingv1alpha1.GPU]
				satisfiedDeviceCount += len(gpuAllocations)
				if satisfiedDeviceCount == gpuWanted {
					break allocateLoop
				}
			}
			if satisfiedDeviceCount == gpuWanted {
				break allocateLoop
			}
		}
	}

	if satisfiedDeviceCount != gpuWanted {
		return nil, nil, fmt.Errorf("node does not have enough GPU")
	}

	return totalDeviceAllocations, allocatedPCIEs, nil
}

func (a *AutopilotAllocator) allocateVFs(
	nodeName string,
	rdmaRequest resource.Quantity,
	rdmaWanted int,
	fullAllocated bool,
	candidatePCIEs sets.Int,
	deviceTopology *unified.DeviceTopology,
	rdmaTopology *unified.RDMATopology,
) ([]*apiext.DeviceAllocation, error) {
	allocatedVFs := a.vfCache.getAllocatedVFs(nodeName)
	alreadyAllocatedPCIEs := sets.NewInt()
	freeVFOnRDMA := map[int32]*unified.VF{}
	var rdmaAllocations []*apiext.DeviceAllocation
allocateVFLoop:
	for _, candidateFirst := range []bool{candidatePCIEs.Len() > 0, false} {
		for _, socket := range deviceTopology.NUMASockets {
			for _, node := range socket.NUMANodes {
				for _, pcie := range node.PCIESwitches {
					if alreadyAllocatedPCIEs.Has(int(pcie.Index)) {
						continue
					}
					if candidateFirst && !candidatePCIEs.Has(int(pcie.Index)) {
						continue
					}
					freeVF, bondSlaves, rdmaMinor := a.allocateVFFromRDMA(&pcie, rdmaTopology, allocatedVFs)
					if freeVF == nil {
						if fullAllocated {
							return nil, fmt.Errorf("node does not have enough RDMA")
						}
						continue
					}

					extension := unified.DeviceAllocationExtension{
						RDMAAllocatedExtension: &unified.RDMAAllocatedExtension{
							VFs:        []*unified.VF{freeVF},
							BondSlaves: bondSlaves,
						},
					}
					data, err := json.Marshal(extension)
					if err != nil {
						return nil, err
					}

					rdmaAllocations = append(rdmaAllocations, &apiext.DeviceAllocation{
						Minor: rdmaMinor,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: rdmaRequest,
						},
						Extension: data,
					})
					if _, ok := freeVFOnRDMA[rdmaMinor]; !ok {
						freeVFOnRDMA[rdmaMinor] = freeVF
						alreadyAllocatedPCIEs.Insert(int(pcie.Index))
					}

					if len(freeVFOnRDMA) == rdmaWanted {
						break allocateVFLoop
					}
				}
			}
		}
		if candidatePCIEs.Len() == 0 {
			break
		}
	}

	if len(freeVFOnRDMA) == 0 || (fullAllocated && len(freeVFOnRDMA) != rdmaWanted) {
		return nil, fmt.Errorf("node does not have enough RDMA")
	}

	return rdmaAllocations, nil
}

func (a *AutopilotAllocator) allocateVFFromRDMA(
	pcie *unified.PCIESwitchInfo,
	rdmaTopology *unified.RDMATopology,
	allocatedVFs map[int]sets.String,
) (*unified.VF, []string, int32) {
	var freeVF *unified.VF
	var bondSlaves []string
	var rdmaMinor int32
	for _, rdma := range pcie.RDMAs {
		allVFS := rdmaTopology.VFs[rdma.Minor]
		allocated := allocatedVFs[int(rdma.Minor)]
		for _, v := range allVFS {
			if allocated.Has(v.BusID) {
				continue
			}
			freeVF = &unified.VF{
				BondName: fmt.Sprintf("bond%d", rdma.Bond),
				BusID:    v.BusID,
				Minor:    v.Minor,
				Priority: v.Priority,
			}
			break
		}
		if freeVF != nil {
			rdmaMinor = rdma.Minor
			bondSlaves = rdma.BondSlaves
			break
		}
	}
	return freeVF, bondSlaves, rdmaMinor
}

func (a *AutopilotAllocator) allocatePFs(nodeDevice *nodeDevice, rdmaRequest resource.Quantity) ([]*apiext.DeviceAllocation, error) {
	totalRDMADevices := nodeDevice.deviceTotal[schedulingv1alpha1.RDMA]
	if len(totalRDMADevices) == 0 {
		return nil, fmt.Errorf("insufficient RDMA devices")
	}
	allocations := make([]*apiext.DeviceAllocation, 0, len(totalRDMADevices))
	for minor, resources := range totalRDMADevices {
		if len(resources) > 0 {
			allocations = append(allocations, &apiext.DeviceAllocation{
				Minor: int32(minor),
				Resources: corev1.ResourceList{
					apiext.ResourceRDMA: rdmaRequest,
				},
			})
		}
	}
	if len(allocations) == 0 {
		return nil, fmt.Errorf("insufficient RDMA devices")
	}
	return allocations, nil
}

func (a *AutopilotAllocator) allocateNVSwitches(numGPU int, nodeDeviceInfo *nodeDevice) ([]*apiext.DeviceAllocation, error) {
	if numGPU == 0 {
		return nil, nil
	}

	wantedNumNVSwitches := 0
	if numGPU == 16 {
		wantedNumNVSwitches = 12
	} else if numGPU == 8 {
		wantedNumNVSwitches = 6
	} else {
		wantedNumNVSwitches = numGPU - 1
	}
	if wantedNumNVSwitches == 0 {
		return nil, nil
	}

	totalNVSwitches := len(nodeDeviceInfo.deviceTotal[unified.NVSwitchDeviceType])
	if totalNVSwitches == 0 {
		return nil, nil
	}
	if wantedNumNVSwitches > 0 && totalNVSwitches == 0 {
		return nil, fmt.Errorf("node does not have enough NVSwitch")
	}

	if wantedNumNVSwitches >= totalNVSwitches {
		wantedNumNVSwitches = totalNVSwitches
	}

	freedNVSwitches := nodeDeviceInfo.deviceFree[unified.NVSwitchDeviceType]
	var nvSwitches []*apiext.DeviceAllocation
	for _, r := range sortDeviceResourcesByMinor(freedNVSwitches) {
		quantity := r.resources[unified.NVSwitchResource]
		if quantity.Value() == 100 {
			nvSwitches = append(nvSwitches, &apiext.DeviceAllocation{
				Minor: int32(r.minor),
				Resources: corev1.ResourceList{
					unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
				},
			})
		}
		if len(nvSwitches) == wantedNumNVSwitches {
			break
		}
	}
	if len(nvSwitches) == wantedNumNVSwitches {
		return nvSwitches, nil
	}

	return nil, fmt.Errorf("node does not have enough NVSwitch")
}

func (a *AutopilotAllocator) Reserve(pod *corev1.Pod, nodeDevice *nodeDevice, allocations apiext.DeviceAllocations) {
	nodeDevice.updateCacheUsed(allocations, pod, true)

	rdmaAllocations := allocations[schedulingv1alpha1.RDMA]
	if len(rdmaAllocations) == 0 {
		return
	}
	vfAllocation := VFAllocation{VFs: map[int]sets.String{}}
	for _, alloc := range rdmaAllocations {
		if len(alloc.Extension) == 0 {
			continue
		}
		var extension unified.DeviceAllocationExtension
		if err := json.Unmarshal(alloc.Extension, &extension); err == nil {
			vfs := vfAllocation.VFs[int(alloc.Minor)]
			if vfs == nil {
				vfs = sets.NewString()
				vfAllocation.VFs[int(alloc.Minor)] = vfs
			}
			for _, vf := range extension.VFs {
				vfs.Insert(vf.BusID)
			}
		}
	}
	if len(vfAllocation.VFs) > 0 {
		a.vfCache.assume(pod, &vfAllocation)
	}
}

func (a *AutopilotAllocator) Unreserve(pod *corev1.Pod, nodeDevice *nodeDevice, allocations apiext.DeviceAllocations) {
	nodeDevice.updateCacheUsed(allocations, pod, false)

	rdmaAllocations := allocations[schedulingv1alpha1.RDMA]
	if len(rdmaAllocations) == 0 {
		return
	}
	vfAllocation := VFAllocation{VFs: map[int]sets.String{}}
	for _, alloc := range rdmaAllocations {
		if len(alloc.Extension) == 0 {
			continue
		}
		var extension unified.DeviceAllocationExtension
		if err := json.Unmarshal(alloc.Extension, &extension); err == nil {
			vfs := vfAllocation.VFs[int(alloc.Minor)]
			if vfs == nil {
				vfs = sets.NewString()
				vfAllocation.VFs[int(alloc.Minor)] = vfs
			}
			for _, vf := range extension.VFs {
				vfs.Insert(vf.BusID)
			}
		}
	}
	if len(vfAllocation.VFs) > 0 {
		a.vfCache.unassume(pod, &vfAllocation)
	}
}

func removeNonVirtualizedRDMAs(topology *unified.DeviceTopology, rdmaTopology *unified.RDMATopology) {
	for _, socket := range topology.NUMASockets {
		for _, node := range socket.NUMANodes {
			for i := range node.PCIESwitches {
				pcie := &node.PCIESwitches[i]
				var rdmas []*unified.RDMADeviceInfo
				for _, rdma := range pcie.RDMAs {
					vfs := rdmaTopology.VFs[rdma.Minor]
					if len(vfs) != 0 {
						rdmas = append(rdmas, rdma)
					}
				}
				pcie.RDMAs = rdmas
			}
		}
	}
}

func (n *nodeDevice) filter(devices map[schedulingv1alpha1.DeviceType][]int) *nodeDevice {
	totalDeviceResources := map[schedulingv1alpha1.DeviceType]deviceResources{}
	usedDeviceResources := map[schedulingv1alpha1.DeviceType]deviceResources{}
	for deviceType, deviceMinors := range devices {
		if totalDeviceResources[deviceType] == nil {
			totalDeviceResources[deviceType] = make(deviceResources)
		}
		if usedDeviceResources[deviceType] == nil {
			usedDeviceResources[deviceType] = make(deviceResources)
		}
		for _, minor := range deviceMinors {
			totalDeviceResources[deviceType][minor] = n.deviceTotal[deviceType][minor].DeepCopy()
			usedDeviceResources[deviceType][minor] = n.deviceUsed[deviceType][minor].DeepCopy()
		}
	}
	r := newNodeDevice()
	r.deviceUsed = usedDeviceResources
	r.resetDeviceTotal(totalDeviceResources)
	return r
}

type pcieSwitch struct {
	socket     int
	node       int
	pcieIndex  int
	nodeDevice *nodeDevice
}

type deviceAccumulator struct {
	pcieSwitches   []*pcieSwitch
	gpuWanted      int
	allocatedGPUs  int
	allocatedPCIEs sets.Int
	result         apiext.DeviceAllocations
}

func newDeviceAccumulator(topology *unified.DeviceTopology, device *nodeDevice, gpuWanted int) *deviceAccumulator {
	var pcieSwitches []*pcieSwitch
	for _, socket := range topology.NUMASockets {
		for _, node := range socket.NUMANodes {
			for _, pcie := range node.PCIESwitches {
				devices := make(map[schedulingv1alpha1.DeviceType][]int)
				for _, minor := range pcie.GPUs {
					devices[schedulingv1alpha1.GPU] = append(devices[schedulingv1alpha1.GPU], int(minor))
				}
				for _, rdma := range pcie.RDMAs {
					devices[schedulingv1alpha1.RDMA] = append(devices[schedulingv1alpha1.RDMA], int(rdma.Minor))
				}
				nodeDevices := device.filter(devices)
				pcieSwitches = append(pcieSwitches, &pcieSwitch{
					socket:     int(socket.Index),
					node:       int(socket.Index)<<32 | int(node.Index),
					pcieIndex:  int(pcie.Index),
					nodeDevice: nodeDevices,
				})
			}
		}
	}

	return &deviceAccumulator{
		pcieSwitches:   pcieSwitches,
		gpuWanted:      gpuWanted,
		allocatedPCIEs: sets.NewInt(),
	}
}

func (a *deviceAccumulator) isSatisfied() bool {
	return a.allocatedGPUs == a.gpuWanted
}

func (a *deviceAccumulator) take(allocations apiext.DeviceAllocations, allocatedPCIEs sets.Int) {
	if a.result == nil {
		a.result = apiext.DeviceAllocations{}
	}
	for k, v := range allocations {
		a.result[k] = append(a.result[k], v...)
	}
	a.allocatedPCIEs.Insert(allocatedPCIEs.UnsortedList()...)
	a.allocatedGPUs = len(a.result[schedulingv1alpha1.GPU])
}

func (a *deviceAccumulator) needCount() int {
	return a.gpuWanted - a.allocatedGPUs
}

func (a *deviceAccumulator) freePCIESwitches() []*pcieSwitch {
	sort.Slice(a.pcieSwitches, func(i, j int) bool {
		iPCIE := a.pcieSwitches[i]
		jPCIE := a.pcieSwitches[j]
		if iPCIE.socket != jPCIE.socket {
			return iPCIE.socket < jPCIE.socket
		}
		if iPCIE.node != jPCIE.node {
			return iPCIE.node < jPCIE.node
		}
		iDeviceResources := iPCIE.nodeDevice.deviceFree[schedulingv1alpha1.GPU]
		jDeviceResources := jPCIE.nodeDevice.deviceFree[schedulingv1alpha1.GPU]
		return sumDeviceResource(iDeviceResources, apiext.ResourceGPUCore) < sumDeviceResource(jDeviceResources, apiext.ResourceGPUCore)
	})
	return a.pcieSwitches
}

type pcieSwitchGroup struct {
	groupID      int
	total        map[schedulingv1alpha1.DeviceType]int
	free         map[schedulingv1alpha1.DeviceType]int64
	pcieSwitches []*pcieSwitch
}

func (a *deviceAccumulator) freePCIESwitchesInNode() []*pcieSwitchGroup {
	pcieSwitchesByNode := map[int][]*pcieSwitch{}
	for _, v := range a.pcieSwitches {
		pcieSwitchesByNode[v.node] = append(pcieSwitchesByNode[v.node], v)
	}
	var groups []*pcieSwitchGroup
	for node, pcieSwitches := range pcieSwitchesByNode {
		group := &pcieSwitchGroup{
			groupID:      node,
			pcieSwitches: pcieSwitches,
			total:        map[schedulingv1alpha1.DeviceType]int{},
			free:         map[schedulingv1alpha1.DeviceType]int64{},
		}
		for _, pcie := range pcieSwitches {
			for k, v := range pcie.nodeDevice.deviceTotal {
				group.total[k] += len(v)
			}
			resources := pcie.nodeDevice.deviceFree[schedulingv1alpha1.GPU]
			group.free[schedulingv1alpha1.GPU] += sumDeviceResource(resources, apiext.ResourceGPUCore)
			resources = pcie.nodeDevice.deviceFree[schedulingv1alpha1.RDMA]
			group.free[schedulingv1alpha1.RDMA] += sumDeviceResource(resources, apiext.ResourceRDMA)
		}
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		iGroup := groups[i]
		jGroup := groups[j]
		if iGroup.groupID != jGroup.groupID {
			return iGroup.groupID < jGroup.groupID
		}
		iFreeGPUs := iGroup.free[schedulingv1alpha1.GPU]
		jFreeGPUs := jGroup.free[schedulingv1alpha1.GPU]
		if iFreeGPUs != jFreeGPUs {
			return iFreeGPUs < jFreeGPUs
		}
		return true
	})
	return groups
}

func (a *deviceAccumulator) freePCIESwitchesInSocket() []*pcieSwitchGroup {
	pcieSwitchesBySocket := map[int][]*pcieSwitch{}
	for _, v := range a.pcieSwitches {
		pcieSwitchesBySocket[v.socket] = append(pcieSwitchesBySocket[v.socket], v)
	}
	var groups []*pcieSwitchGroup
	for socket, pcieSwitches := range pcieSwitchesBySocket {
		group := &pcieSwitchGroup{
			groupID:      socket,
			pcieSwitches: pcieSwitches,
			total:        map[schedulingv1alpha1.DeviceType]int{},
			free:         map[schedulingv1alpha1.DeviceType]int64{},
		}
		for _, pcie := range pcieSwitches {
			for k, v := range pcie.nodeDevice.deviceTotal {
				group.total[k] += len(v)
			}
			resources := pcie.nodeDevice.deviceFree[schedulingv1alpha1.GPU]
			group.free[schedulingv1alpha1.GPU] += sumDeviceResource(resources, apiext.ResourceGPUCore)
			resources = pcie.nodeDevice.deviceFree[schedulingv1alpha1.RDMA]
			group.free[schedulingv1alpha1.RDMA] += sumDeviceResource(resources, apiext.ResourceRDMA)
		}
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		iGroup := groups[i]
		jGroup := groups[j]
		if iGroup.groupID != jGroup.groupID {
			return iGroup.groupID < jGroup.groupID
		}
		iFreeGPUs := iGroup.free[schedulingv1alpha1.GPU]
		jFreeGPUs := jGroup.free[schedulingv1alpha1.GPU]
		if iFreeGPUs != jFreeGPUs {
			return iFreeGPUs < jFreeGPUs
		}
		return true
	})
	return groups
}

func sumDeviceResource(resources deviceResources, resName corev1.ResourceName) int64 {
	var total int64
	for _, resList := range resources {
		q := resList[resName]
		total += q.Value()
	}
	return total
}

func matchDriverVersions(pod *corev1.Pod, device *schedulingv1alpha1.Device) (string, error) {
	driverVersions, err := unified.GetDriverVersions(device.Annotations)
	if err != nil {
		return "", err
	}
	if len(driverVersions) == 0 {
		return "", err
	}
	sort.Strings(driverVersions)
	selector, err := unified.GetGPUSelector(pod.Annotations)
	if err != nil {
		return "", err
	}
	if len(selector.DriverVersions) == 0 {
		return driverVersions[0], nil
	}

	versions := sets.NewString(driverVersions...)
	var matchedVersion string
	for _, v := range selector.DriverVersions {
		if versions.Has(v) {
			matchedVersion = v
			break
		}
	}
	return matchedVersion, nil
}

func mustAllocateVF(pod *corev1.Pod) bool {
	return !pod.Spec.HostNetwork || (pod.Spec.RuntimeClassName != nil && *pod.Spec.RuntimeClassName == "rund")
}
