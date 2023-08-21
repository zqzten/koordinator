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
	"errors"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	schedulingv1alpha1listers "github.com/koordinator-sh/koordinator/pkg/client/listers/scheduling/v1alpha1"
	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
	reservationutil "github.com/koordinator-sh/koordinator/pkg/util/reservation"
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
	nodeLister   corev1lister.NodeLister
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
	nodeLister := options.SharedInformerFactory.Core().V1().Nodes().Lister()
	return &AutopilotAllocator{
		vfCache:      vfCache,
		nodeLister:   nodeLister,
		deviceLister: deviceLister,
	}
}

func (a *AutopilotAllocator) Name() string {
	return AutopilotAllocatorName
}

func (a *AutopilotAllocator) Allocate(
	nodeName string,
	pod *corev1.Pod,
	podRequest corev1.ResourceList,
	nodeDevice *nodeDevice,
	required, preferred map[schedulingv1alpha1.DeviceType]sets.Int,
	requiredDeviceResources, preemptibleFreeDevices map[schedulingv1alpha1.DeviceType]deviceResources,
	allocationScorer *resourceAllocationScorer,
) (apiext.DeviceAllocations, error) {
	deviceObj, err := a.deviceLister.Get(nodeName)
	if err != nil {
		return nil, err
	}

	rdmaTopology, err := unified.GetRDMATopology(deviceObj.Annotations)
	if err != nil {
		return nil, err
	}
	if len(rdmaTopology.VFs) == 0 {
		rdmaRequest := podRequest[apiext.ResourceRDMA]
		if !rdmaRequest.IsZero() && mustAllocateVF(pod) {
			return nil, fmt.Errorf("invalid RDMA Topology")
		}
	}
	deviceTopology, err := unified.GetDeviceTopology(deviceObj.Annotations)
	if err != nil {
		return nil, err
	}

	podRequest = podRequest.DeepCopy()
	if !HasDeviceResource(podRequest, schedulingv1alpha1.GPU) {
		return a.allocateNonGPUDevices(nodeName, nodeDevice, podRequest, mustAllocateVF(pod), deviceTopology, rdmaTopology, required, preferred, requiredDeviceResources, preemptibleFreeDevices, allocationScorer)
	}

	nodeDeviceTotal := nodeDevice.deviceTotal[schedulingv1alpha1.GPU]
	if len(nodeDeviceTotal) <= 0 {
		return nil, fmt.Errorf("node does not have enough GPU")
	}

	rdmaRequest := podRequest[apiext.ResourceRDMA]
	gpuRequest := quotav1.Mask(podRequest, DeviceResourceNames[schedulingv1alpha1.GPU])
	if err := fillGPUTotalMem(nodeDeviceTotal, gpuRequest); err != nil {
		return nil, fmt.Errorf("node does not have enough GPU")
	}

	gpuWanted := int64(1)
	var gpuRequestPerCard corev1.ResourceList
	if isPodRequestsMultipleDevice(gpuRequest, schedulingv1alpha1.GPU) {
		gpuCore, gpuMem, gpuMemoryRatio := gpuRequest[apiext.ResourceGPUCore], gpuRequest[apiext.ResourceGPUMemory], gpuRequest[apiext.ResourceGPUMemoryRatio]
		gpuWanted = gpuMemoryRatio.Value() / 100
		gpuRequestPerCard = corev1.ResourceList{
			apiext.ResourceGPUCore:        *resource.NewQuantity(gpuCore.Value()/gpuWanted, resource.DecimalSI),
			apiext.ResourceGPUMemory:      *resource.NewQuantity(gpuMem.Value()/gpuWanted, resource.BinarySI),
			apiext.ResourceGPUMemoryRatio: *resource.NewQuantity(gpuMemoryRatio.Value()/gpuWanted, resource.DecimalSI),
		}
	} else {
		gpuRequestPerCard = gpuRequest
	}

	var deviceAllocations apiext.DeviceAllocations
	node, err := a.nodeLister.Get(nodeName)
	if err != nil {
		return nil, fmt.Errorf("missing node")
	}
	if mustAllocateGPUByPartition(node) {
		partitionTable := getGPUPartitionTable(node)
		deviceAllocations, err = a.allocateResourcesByPartition(nodeName, nodeDevice, deviceObj, gpuRequestPerCard, int(gpuWanted), rdmaRequest, mustAllocateVF(pod), unified.VFDeviceTypeGPU, partitionTable, deviceTopology, rdmaTopology, required, preferred, requiredDeviceResources, preemptibleFreeDevices, allocationScorer)
		if err != nil {
			return nil, err
		}
	} else {
		deviceAllocations, _, err = a.allocateResourcesByPCIE(nodeName, nodeDevice, gpuRequestPerCard, int(gpuWanted), rdmaRequest, mustAllocateVF(pod), unified.VFDeviceTypeGPU, deviceTopology, rdmaTopology, required, preferred, requiredDeviceResources, preemptibleFreeDevices, allocationScorer)
		if err != nil {
			var zeroRDMARequest resource.Quantity
			var allocatedPCIEs sets.Int
			deviceAllocations, allocatedPCIEs, err = a.allocateResourcesByPCIE(nodeName, nodeDevice, gpuRequestPerCard, int(gpuWanted), zeroRDMARequest, false, unified.VFDeviceTypeGPU, deviceTopology, rdmaTopology, required, preferred, requiredDeviceResources, preemptibleFreeDevices, allocationScorer)
			if err != nil {
				return nil, err
			}
			if !rdmaRequest.IsZero() && mustAllocateVF(pod) {
				rdmaAllocations, err := a.allocateVFs(nodeName, rdmaRequest, len(allocatedPCIEs), false, allocatedPCIEs, deviceTopology, rdmaTopology, unified.VFDeviceTypeGPU)
				if err != nil {
					return nil, err
				}
				deviceAllocations[schedulingv1alpha1.RDMA] = rdmaAllocations
			}
		}
	}

	if !rdmaRequest.IsZero() && !mustAllocateVF(pod) {
		rdmaAllocations, err := a.allocatePFs(nodeDevice, deviceTopology, rdmaRequest)
		if err != nil {
			return nil, err
		}
		deviceAllocations[schedulingv1alpha1.RDMA] = rdmaAllocations
	}

	// 先暂时不支持为 Reservation 预留 NVSwitch 设备
	if !reservationutil.IsReservePod(pod) && !mustAllocateGPUByPartition(node) {
		nvSwitches, err := a.allocateNVSwitches(len(deviceAllocations[schedulingv1alpha1.GPU]), nodeDevice, required, preferred, requiredDeviceResources, preemptibleFreeDevices, allocationScorer)
		if err != nil {
			return nil, err
		}
		if len(nvSwitches) > 0 {
			deviceAllocations[unified.NVSwitchDeviceType] = nvSwitches
		}
	}

	return deviceAllocations, nil
}

func (a *AutopilotAllocator) Score(
	nodeName string,
	pod *corev1.Pod,
	podRequest corev1.ResourceList,
	nodeDevice *nodeDevice,
	requiredDeviceResources, preemptibleDeviceResources map[schedulingv1alpha1.DeviceType]deviceResources,
	allocationScorer *resourceAllocationScorer,
) (int64, error) {
	return nodeDevice.score(podRequest, requiredDeviceResources, preemptibleDeviceResources, allocationScorer)
}

func (a *AutopilotAllocator) allocateNonGPUDevices(
	nodeName string,
	nodeDevice *nodeDevice,
	podRequest corev1.ResourceList,
	requireAllocateVF bool,
	deviceTopology *unified.DeviceTopology,
	rdmaTopology *unified.RDMATopology,
	required, preferred map[schedulingv1alpha1.DeviceType]sets.Int,
	requiredDeviceResources, preemptibleFreeDevices map[schedulingv1alpha1.DeviceType]deviceResources,
	allocationScorer *resourceAllocationScorer,
) (apiext.DeviceAllocations, error) {
	deviceAllocations := apiext.DeviceAllocations{}
	rdmaRequest := podRequest[apiext.ResourceRDMA]
	if !rdmaRequest.IsZero() {
		var rdmaAllocations []*apiext.DeviceAllocation
		var err error
		if requireAllocateVF {
			rdmaAllocations, err = a.allocateVFs(nodeName, rdmaRequest, 1, false, nil, deviceTopology, rdmaTopology, unified.VFDeviceTypeCPU)
		} else {
			rdmaAllocations, err = a.allocatePFs(nodeDevice, deviceTopology, rdmaRequest)
		}
		if err != nil {
			return nil, err
		}
		deviceAllocations[schedulingv1alpha1.RDMA] = rdmaAllocations
		delete(podRequest, apiext.ResourceRDMA)
	}
	if len(podRequest) > 0 {
		otherDeviceAllocations, err := nodeDevice.tryAllocateDevice(podRequest, required, preferred, requiredDeviceResources, preemptibleFreeDevices, allocationScorer)
		if err != nil {
			return nil, err
		}
		for k, v := range otherDeviceAllocations {
			deviceAllocations[k] = v
		}
	}
	return deviceAllocations, nil
}

func (a *AutopilotAllocator) allocateResourcesByPartition(
	nodeName string,
	nodeDevice *nodeDevice,
	deviceObj *schedulingv1alpha1.Device,
	gpuRequestPerCard corev1.ResourceList,
	gpuWanted int,
	rdmaRequest resource.Quantity,
	requireAllocateVF bool,
	vfDeviceType unified.VFDeviceType,
	partitionTable map[int][]unified.GPUPartition,
	deviceTopology *unified.DeviceTopology,
	rdmaTopology *unified.RDMATopology,
	required, preferred map[schedulingv1alpha1.DeviceType]sets.Int,
	requiredDeviceResources, preemptibleFreeDevices map[schedulingv1alpha1.DeviceType]deviceResources,
	allocationScorer *resourceAllocationScorer,
) (apiext.DeviceAllocations, error) {
	partitions, ok := partitionTable[gpuWanted]
	if !ok {
		return nil, errors.New("node(s) Unsupported number of GPU requests")
	}

	deviceAllocations := apiext.DeviceAllocations{}
	satisfiedDeviceCount := 0
	var allocatedPCIEs sets.Int
	for _, partition := range partitions {
		minors := sets.NewInt()
		for _, moduleID := range partition.ModuleIDs {
			for _, v := range deviceObj.Spec.Devices {
				if v.ModuleID != nil && int(*v.ModuleID) == moduleID && v.Minor != nil {
					minors.Insert(int(*v.Minor))
				}
			}
		}
		devices := map[schedulingv1alpha1.DeviceType][]int{
			schedulingv1alpha1.GPU: minors.UnsortedList(),
		}
		partitionDevice := nodeDevice.filter(devices, requiredDeviceResources, preemptibleFreeDevices)

		totalFreeGPUMemoryRatio := sumDeviceResource(partitionDevice.deviceFree[schedulingv1alpha1.GPU], apiext.ResourceGPUMemoryRatio)
		gpuMemoryRatioRequest := gpuRequestPerCard[apiext.ResourceGPUMemoryRatio]
		freeCount := int(totalFreeGPUMemoryRatio / gpuMemoryRatioRequest.Value())
		if freeCount < gpuWanted {
			continue
		}

		var gpuAllocations []*apiext.DeviceAllocation
		for i := 0; i < gpuWanted; i++ {
			resourceRequest := gpuRequestPerCard.DeepCopy()
			allocations, err := partitionDevice.tryAllocateDevice(resourceRequest, required, preferred, nil, nil, allocationScorer)
			if err != nil {
				break
			}
			for k, v := range allocations {
				gpuAllocations = append(gpuAllocations, v...)
				partitionDevice.updateDeviceUsed(k, v, true)
			}
			partitionDevice.resetDeviceFree(schedulingv1alpha1.GPU)
		}

		satisfiedDeviceCount = len(gpuAllocations)
		if satisfiedDeviceCount == gpuWanted {
			deviceAllocations[schedulingv1alpha1.GPU] = gpuAllocations
			allocatedPCIEs = sets.NewInt()
			for _, numaSocket := range deviceTopology.NUMASockets {
				for _, numaNode := range numaSocket.NUMANodes {
					for _, pcie := range numaNode.PCIESwitches {
						for _, minor := range pcie.GPUs {
							if minors.Has(int(minor)) {
								allocatedPCIEs.Insert(int(pcie.Index))
							}
						}
					}
				}
			}
			break
		}
	}

	if satisfiedDeviceCount != gpuWanted {
		return nil, fmt.Errorf("node does not have enough GPU")
	}

	if !rdmaRequest.IsZero() && requireAllocateVF {
		rdmaAllocations, err := a.allocateVFs(nodeName, rdmaRequest, len(allocatedPCIEs), false, allocatedPCIEs, deviceTopology, rdmaTopology, vfDeviceType)
		if err != nil {
			return nil, err
		}
		deviceAllocations[schedulingv1alpha1.RDMA] = rdmaAllocations
	}
	return deviceAllocations, nil
}

func (a *AutopilotAllocator) allocateResourcesByPCIE(
	nodeName string,
	nodeDevice *nodeDevice,
	gpuRequestPerCard corev1.ResourceList,
	gpuWanted int,
	rdmaRequest resource.Quantity,
	requireAllocateVF bool,
	vfDeviceType unified.VFDeviceType,
	deviceTopology *unified.DeviceTopology,
	rdmaTopology *unified.RDMATopology,
	required, preferred map[schedulingv1alpha1.DeviceType]sets.Int,
	requiredDeviceResources, preemptibleFreeDevices map[schedulingv1alpha1.DeviceType]deviceResources,
	allocationScorer *resourceAllocationScorer,
) (apiext.DeviceAllocations, sets.Int, error) {
	acc := newDeviceAccumulator(deviceTopology, nodeDevice, gpuWanted, requiredDeviceResources, preemptibleFreeDevices)
	pcieSwitchGroups := acc.freePCIESwitchesInNode()
	allocations, allocatedPCIEs, err := a.allocateByPCIEGroup(pcieSwitchGroups, gpuRequestPerCard, rdmaRequest, acc.needCount(), required, preferred, allocationScorer)
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
		allocations, allocatedPCIEs, err = a.allocateByPCIEGroup(pcieSwitchGroups, gpuRequestPerCard, rdmaRequest, acc.needCount(), required, preferred, allocationScorer)
		if err != nil {
			return nil, nil, err
		}
		acc.take(allocations, allocatedPCIEs)
	}

	if !acc.isSatisfied() {
		allocations, allocatedPCIEs, err = a.allocateByPCIE(acc.freePCIESwitches(), gpuRequestPerCard, rdmaRequest, acc.needCount(), required, preferred, allocationScorer)
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
		// FIXME: VF should also support preempting
		rdmaAllocations, err := a.allocateVFs(nodeName, rdmaRequest, acc.allocatedPCIEs.Len(), true, acc.allocatedPCIEs, deviceTopology, rdmaTopology, vfDeviceType)
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
	required, preferred map[schedulingv1alpha1.DeviceType]sets.Int,
	allocationScorer *resourceAllocationScorer,
) (apiext.DeviceAllocations, sets.Int, error) {
	for _, group := range pcieSwitchGroups {
		count := int(group.free[schedulingv1alpha1.GPU] / 100)
		if count < gpuWanted {
			continue
		}
		allocations, allocatedPCIEs, err := a.allocateByPCIE(group.pcieSwitches, gpuRequestPerCard, rdmaRequest, gpuWanted, required, preferred, allocationScorer)
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
	required, preferred map[schedulingv1alpha1.DeviceType]sets.Int,
	allocationScorer *resourceAllocationScorer,
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
			totalFreeGPUMemoryRatio := sumDeviceResource(pcie.nodeDevice.deviceFree[schedulingv1alpha1.GPU], apiext.ResourceGPUMemoryRatio)
			gpuCoreRequest := gpuRequestPerCard[apiext.ResourceGPUMemoryRatio]
			freeCount := int(totalFreeGPUMemoryRatio / gpuCoreRequest.Value())
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
				deviceAllocations, err := pcie.nodeDevice.tryAllocateDevice(resourceRequest, required, preferred, nil, nil, allocationScorer)
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
	vfDeviceType unified.VFDeviceType,
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
					freeVF, _, rdmaMinor := a.allocateVFFromRDMA(&pcie, rdmaTopology, allocatedVFs, vfDeviceType)
					if freeVF == nil {
						if fullAllocated {
							return nil, fmt.Errorf("node does not have enough RDMA")
						}
						continue
					}

					extension := unified.DeviceAllocationExtension{
						RDMAAllocatedExtension: &unified.RDMAAllocatedExtension{
							VFs: []*unified.VF{freeVF},
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
	vfDeviceType unified.VFDeviceType,
) (*unified.VF, []string, int32) {
	var freeVF *unified.VF
	var bondSlaves []string
	var rdmaMinor int32
	for _, rdma := range pcie.RDMAs {
		if rdma.Type == unified.RDMADeviceTypeSystem {
			continue
		}

		allVFS := rdmaTopology.VFs[rdma.Minor]
		allocated := allocatedVFs[int(rdma.Minor)]
		remainingVFs := make([]unified.VF, 0, len(allVFS))
		for _, v := range allVFS {
			if allocated.Has(v.BusID) || (v.Type != "" && v.Type != vfDeviceType) {
				continue
			}
			remainingVFs = append(remainingVFs, v)
		}
		if len(remainingVFs) > 0 {
			sort.Slice(remainingVFs, func(i, j int) bool {
				return remainingVFs[i].BusID < remainingVFs[j].BusID
			})
			var vf *unified.VF
			if vfDeviceType == unified.VFDeviceTypeGPU {
				vf = &remainingVFs[0]
			} else {
				vf = &remainingVFs[len(remainingVFs)-1]
			}
			freeVF = &unified.VF{
				BusID: vf.BusID,
				Minor: vf.Minor,
			}
		}
		if freeVF != nil {
			rdmaMinor = rdma.Minor
			bondSlaves = rdma.BondSlaves
			break
		}
	}
	return freeVF, bondSlaves, rdmaMinor
}

func (a *AutopilotAllocator) allocatePFs(nodeDevice *nodeDevice, deviceTopology *unified.DeviceTopology, rdmaRequest resource.Quantity) ([]*apiext.DeviceAllocation, error) {
	totalRDMADevices := nodeDevice.deviceFree[schedulingv1alpha1.RDMA]
	if len(totalRDMADevices) == 0 {
		return nil, fmt.Errorf("insufficient RDMA devices")
	}
	var allocations []*apiext.DeviceAllocation
	request := corev1.ResourceList{
		apiext.ResourceRDMA: rdmaRequest,
	}
	freeRDMADevices := nodeDevice.deviceFree[schedulingv1alpha1.RDMA]

	availableRDMAs := sets.NewInt()
	for _, socket := range deviceTopology.NUMASockets {
		for _, node := range socket.NUMANodes {
			for _, pcie := range node.PCIESwitches {
				for _, rdma := range pcie.RDMAs {
					if rdma.Type != unified.RDMADeviceTypeSystem {
						availableRDMAs.Insert(int(rdma.Minor))
					}
				}
			}
		}
	}

	for minor, resources := range freeRDMADevices {
		if !availableRDMAs.Has(minor) {
			continue
		}

		if satisfied, _ := quotav1.LessThanOrEqual(request, resources); !satisfied {
			continue
		}

		allocations = append(allocations, &apiext.DeviceAllocation{
			Minor: int32(minor),
			Resources: corev1.ResourceList{
				apiext.ResourceRDMA: rdmaRequest,
			},
		})
	}
	if len(allocations) == 0 {
		return nil, fmt.Errorf("insufficient RDMA devices")
	}
	return allocations, nil
}

func (a *AutopilotAllocator) allocateNVSwitches(
	numGPU int,
	nodeDeviceInfo *nodeDevice,
	required, preferred map[schedulingv1alpha1.DeviceType]sets.Int,
	requiredDeviceResources, preemptibleFreeDevices map[schedulingv1alpha1.DeviceType]deviceResources,
	allocationScorer *resourceAllocationScorer,
) ([]*apiext.DeviceAllocation, error) {
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

	var freedNVSwitches deviceResources
	if len(requiredDeviceResources[unified.NVSwitchDeviceType]) > 0 {
		freedNVSwitches = requiredDeviceResources[unified.NVSwitchDeviceType]
	} else {
		freedNVSwitches = nodeDeviceInfo.calcFreeWithPreemptible(unified.NVSwitchDeviceType, preemptibleFreeDevices[unified.NVSwitchDeviceType])
	}

	var nvSwitches []*apiext.DeviceAllocation
	podRequestPerCard := corev1.ResourceList{
		unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
	}
	orderedDeviceResources := scoreDevices(podRequestPerCard, nodeDeviceInfo.deviceTotal[unified.NVSwitchDeviceType], freedNVSwitches, allocationScorer)
	orderedDeviceResources = sortDeviceResourcesByMinor(orderedDeviceResources, preferred[unified.NVSwitchDeviceType])
	for _, r := range orderedDeviceResources {
		if required[unified.NVSwitchDeviceType].Len() > 0 && !required[unified.NVSwitchDeviceType].Has(r.minor) {
			continue
		}
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

func (n *nodeDevice) filter(devices map[schedulingv1alpha1.DeviceType][]int, requiredDeviceResources, preemptibleDeviceResources map[schedulingv1alpha1.DeviceType]deviceResources) *nodeDevice {
	totalDeviceResources := map[schedulingv1alpha1.DeviceType]deviceResources{}
	usedDeviceResources := map[schedulingv1alpha1.DeviceType]deviceResources{}
	for deviceType, deviceMinors := range devices {
		if totalDeviceResources[deviceType] == nil {
			totalDeviceResources[deviceType] = make(deviceResources)
		}
		if usedDeviceResources[deviceType] == nil {
			usedDeviceResources[deviceType] = make(deviceResources)
		}

		var freeDevices deviceResources
		requiredResources := requiredDeviceResources[deviceType]
		if len(requiredResources) > 0 {
			freeDevices = requiredResources
		} else {
			freeDevices = n.calcFreeWithPreemptible(deviceType, preemptibleDeviceResources[deviceType])
		}

		minors := sets.NewInt(deviceMinors...)

		for minor, free := range freeDevices {
			if !minors.Has(minor) {
				continue
			}
			totalDeviceResources[deviceType][minor] = n.deviceTotal[deviceType][minor].DeepCopy()
			used := quotav1.SubtractWithNonNegativeResult(n.deviceTotal[deviceType][minor], free)
			usedDeviceResources[deviceType][minor] = used
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

func newDeviceAccumulator(topology *unified.DeviceTopology, device *nodeDevice, gpuWanted int, requiredDeviceResources, preemptibleDeviceResources map[schedulingv1alpha1.DeviceType]deviceResources) *deviceAccumulator {
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
				nodeDevices := device.filter(devices, requiredDeviceResources, preemptibleDeviceResources)
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
		return sumDeviceResource(iDeviceResources, apiext.ResourceGPUMemoryRatio) < sumDeviceResource(jDeviceResources, apiext.ResourceGPUMemoryRatio)
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
			group.free[schedulingv1alpha1.GPU] += sumDeviceResource(resources, apiext.ResourceGPUMemoryRatio)
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
			group.free[schedulingv1alpha1.GPU] += sumDeviceResource(resources, apiext.ResourceGPUMemoryRatio)
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

func mustAllocateVF(pod *corev1.Pod) bool {
	return !pod.Spec.HostNetwork || (pod.Spec.RuntimeClassName != nil && *pod.Spec.RuntimeClassName == "rund")
}

func HasDeviceResource(podRequest corev1.ResourceList, deviceType schedulingv1alpha1.DeviceType) bool {
	if podRequest == nil || len(podRequest) == 0 {
		klog.V(5).Infof("skip checking HasDeviceResource, because pod request is empty")
		return false
	}
	for _, resourceName := range DeviceResourceNames[deviceType] {
		if _, ok := podRequest[resourceName]; ok {
			return true
		}
	}
	klog.V(5).Infof("pod does not request %v resource", deviceType)
	return false
}
