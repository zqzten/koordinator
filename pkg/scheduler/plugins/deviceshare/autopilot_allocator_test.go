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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	koordfake "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned/fake"
	koordinatorinformers "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions"
)

var (
	gpuResourceList = corev1.ResourceList{
		apiext.ResourceGPUCore:        *resource.NewQuantity(100, resource.DecimalSI),
		apiext.ResourceGPUMemoryRatio: *resource.NewQuantity(100, resource.DecimalSI),
		apiext.ResourceGPUMemory:      *resource.NewQuantity(85198045184, resource.BinarySI),
	}

	fakeDeviceCR = &schedulingv1alpha1.Device{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
			Annotations: map[string]string{
				unified.AnnotationDeviceTopology: `{"aswIDs":["ASW-MASTER-G1-P1-S20-1.NA61"],"numaSockets":[{"index":0,"numaNodes":[{"index":0,"pcieSwitches":[{"gpus":[0,1],"index":0,"rdmas":[{"bond":0,"bondSlaves":["eth0","eth1"],"minor":0,"name":"mlx5_bond_0","uVerbs":"/dev/infiniband/uverbs0"},{"bond":1,"bondSlaves":["eth0","eth1"],"minor":1,"name":"mlx5_bond_1","uVerbs":"/dev/infiniband/uverbs0"}]},{"gpus":[2,3],"index":1,"rdmas":[{"bond":2,"bondSlaves":["eth2","eth3"],"minor":2,"name":"mlx5_bond_2","uVerbs":"/dev/infiniband/uverbs1"}]}]}]},{"index":1,"numaNodes":[{"index":1,"pcieSwitches":[{"gpus":[4,5],"index":2,"rdmas":[{"bond":3,"bondSlaves":["eth4","eth5"],"minor":3,"name":"mlx5_bond_3","uVerbs":"/dev/infiniband/uverbs2"}]},{"gpus":[6,7],"index":3,"rdmas":[{"bond":4,"bondSlaves":["eth6","eth7"],"minor":4,"name":"mlx5_bond_4","uVerbs":"/dev/infiniband/uverbs3"}]}]}]}],"pointOfDelivery":"MASTER-G1-P1"}`,
				unified.AnnotationRDMATopology:   `{"vfs":{"1":[{"busID":"0000:1f:00.2","minor":0,"priority":"VFPriorityHigh"},{"busID":"0000:1f:00.3","minor":1,"priority":"VFPriorityHigh"},{"busID":"0000:1f:00.4","minor":2,"priority":"VFPriorityHigh"},{"busID":"0000:1f:00.5","minor":3,"priority":"VFPriorityHigh"},{"busID":"0000:1f:00.6","minor":4,"priority":"VFPriorityHigh"},{"busID":"0000:1f:00.7","minor":5,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.0","minor":6,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.1","minor":7,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.2","minor":8,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.3","minor":9,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.4","minor":10,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.5","minor":11,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.6","minor":12,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.7","minor":13,"priority":"VFPriorityHigh"},{"busID":"0000:1f:02.0","minor":14,"priority":"VFPriorityHigh"},{"busID":"0000:1f:02.1","minor":15,"priority":"VFPriorityLow"},{"busID":"0000:1f:02.2","minor":16,"priority":"VFPriorityLow"},{"busID":"0000:1f:02.3","minor":17,"priority":"VFPriorityLow"},{"busID":"0000:1f:02.4","minor":18,"priority":"VFPriorityLow"},{"busID":"0000:1f:02.5","minor":19,"priority":"VFPriorityLow"},{"busID":"0000:1f:02.6","minor":20,"priority":"VFPriorityLow"},{"busID":"0000:1f:02.7","minor":21,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.0","minor":22,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.1","minor":23,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.2","minor":24,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.3","minor":25,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.4","minor":26,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.5","minor":27,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.6","minor":28,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.7","minor":29,"priority":"VFPriorityLow"}],"2":[{"busID":"0000:90:00.2","minor":0,"priority":"VFPriorityHigh"},{"busID":"0000:90:00.3","minor":1,"priority":"VFPriorityHigh"},{"busID":"0000:90:00.4","minor":2,"priority":"VFPriorityHigh"},{"busID":"0000:90:00.5","minor":3,"priority":"VFPriorityHigh"},{"busID":"0000:90:00.6","minor":4,"priority":"VFPriorityHigh"},{"busID":"0000:90:00.7","minor":5,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.0","minor":6,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.1","minor":7,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.2","minor":8,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.3","minor":9,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.4","minor":10,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.5","minor":11,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.6","minor":12,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.7","minor":13,"priority":"VFPriorityHigh"},{"busID":"0000:90:02.0","minor":14,"priority":"VFPriorityHigh"},{"busID":"0000:90:02.1","minor":15,"priority":"VFPriorityLow"},{"busID":"0000:90:02.2","minor":16,"priority":"VFPriorityLow"},{"busID":"0000:90:02.3","minor":17,"priority":"VFPriorityLow"},{"busID":"0000:90:02.4","minor":18,"priority":"VFPriorityLow"},{"busID":"0000:90:02.5","minor":19,"priority":"VFPriorityLow"},{"busID":"0000:90:02.6","minor":20,"priority":"VFPriorityLow"},{"busID":"0000:90:02.7","minor":21,"priority":"VFPriorityLow"},{"busID":"0000:90:03.0","minor":22,"priority":"VFPriorityLow"},{"busID":"0000:90:03.1","minor":23,"priority":"VFPriorityLow"},{"busID":"0000:90:03.2","minor":24,"priority":"VFPriorityLow"},{"busID":"0000:90:03.3","minor":25,"priority":"VFPriorityLow"},{"busID":"0000:90:03.4","minor":26,"priority":"VFPriorityLow"},{"busID":"0000:90:03.5","minor":27,"priority":"VFPriorityLow"},{"busID":"0000:90:03.6","minor":28,"priority":"VFPriorityLow"},{"busID":"0000:90:03.7","minor":29,"priority":"VFPriorityLow"}],"3":[{"busID":"0000:51:00.2","minor":0,"priority":"VFPriorityHigh"},{"busID":"0000:51:00.3","minor":1,"priority":"VFPriorityHigh"},{"busID":"0000:51:00.4","minor":2,"priority":"VFPriorityHigh"},{"busID":"0000:51:00.5","minor":3,"priority":"VFPriorityHigh"},{"busID":"0000:51:00.6","minor":4,"priority":"VFPriorityHigh"},{"busID":"0000:51:00.7","minor":5,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.0","minor":6,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.1","minor":7,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.2","minor":8,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.3","minor":9,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.4","minor":10,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.5","minor":11,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.6","minor":12,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.7","minor":13,"priority":"VFPriorityHigh"},{"busID":"0000:51:02.0","minor":14,"priority":"VFPriorityHigh"},{"busID":"0000:51:02.1","minor":15,"priority":"VFPriorityLow"},{"busID":"0000:51:02.2","minor":16,"priority":"VFPriorityLow"},{"busID":"0000:51:02.3","minor":17,"priority":"VFPriorityLow"},{"busID":"0000:51:02.4","minor":18,"priority":"VFPriorityLow"},{"busID":"0000:51:02.5","minor":19,"priority":"VFPriorityLow"},{"busID":"0000:51:02.6","minor":20,"priority":"VFPriorityLow"},{"busID":"0000:51:02.7","minor":21,"priority":"VFPriorityLow"},{"busID":"0000:51:03.0","minor":22,"priority":"VFPriorityLow"},{"busID":"0000:51:03.1","minor":23,"priority":"VFPriorityLow"},{"busID":"0000:51:03.2","minor":24,"priority":"VFPriorityLow"},{"busID":"0000:51:03.3","minor":25,"priority":"VFPriorityLow"},{"busID":"0000:51:03.4","minor":26,"priority":"VFPriorityLow"},{"busID":"0000:51:03.5","minor":27,"priority":"VFPriorityLow"},{"busID":"0000:51:03.6","minor":28,"priority":"VFPriorityLow"},{"busID":"0000:51:03.7","minor":29,"priority":"VFPriorityLow"}],"4":[{"busID":"0000:b9:00.2","minor":0,"priority":"VFPriorityHigh"},{"busID":"0000:b9:00.3","minor":1,"priority":"VFPriorityHigh"},{"busID":"0000:b9:00.4","minor":2,"priority":"VFPriorityHigh"},{"busID":"0000:b9:00.5","minor":3,"priority":"VFPriorityHigh"},{"busID":"0000:b9:00.6","minor":4,"priority":"VFPriorityHigh"},{"busID":"0000:b9:00.7","minor":5,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.0","minor":6,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.1","minor":7,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.2","minor":8,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.3","minor":9,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.4","minor":10,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.5","minor":11,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.6","minor":12,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.7","minor":13,"priority":"VFPriorityHigh"},{"busID":"0000:b9:02.0","minor":14,"priority":"VFPriorityHigh"},{"busID":"0000:b9:02.1","minor":15,"priority":"VFPriorityLow"},{"busID":"0000:b9:02.2","minor":16,"priority":"VFPriorityLow"},{"busID":"0000:b9:02.3","minor":17,"priority":"VFPriorityLow"},{"busID":"0000:b9:02.4","minor":18,"priority":"VFPriorityLow"},{"busID":"0000:b9:02.5","minor":19,"priority":"VFPriorityLow"},{"busID":"0000:b9:02.6","minor":20,"priority":"VFPriorityLow"},{"busID":"0000:b9:02.7","minor":21,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.0","minor":22,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.1","minor":23,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.2","minor":24,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.3","minor":25,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.4","minor":26,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.5","minor":27,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.6","minor":28,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.7","minor":29,"priority":"VFPriorityLow"}]}}`,
			},
		},
		Spec: schedulingv1alpha1.DeviceSpec{
			Devices: []schedulingv1alpha1.DeviceInfo{
				{
					Type:   schedulingv1alpha1.RDMA,
					UUID:   "0000:1f:00.0",
					Minor:  pointer.Int32(1),
					Health: true,
					Resources: corev1.ResourceList{
						apiext.ResourceRDMA: *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
				{
					Type:   schedulingv1alpha1.RDMA,
					UUID:   "0000:90:00.0",
					Minor:  pointer.Int32(2),
					Health: true,
					Resources: corev1.ResourceList{
						apiext.ResourceRDMA: *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
				{
					Type:   schedulingv1alpha1.RDMA,
					UUID:   "0000:51:00.0",
					Minor:  pointer.Int32(3),
					Health: true,
					Resources: corev1.ResourceList{
						apiext.ResourceRDMA: *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
				{
					Type:   schedulingv1alpha1.RDMA,
					UUID:   "0000:b9:00.0",
					Minor:  pointer.Int32(4),
					Health: true,
					Resources: corev1.ResourceList{
						apiext.ResourceRDMA: *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
				{
					Type:      schedulingv1alpha1.GPU,
					UUID:      "GPU-8c25ea37-2909-6e62-b7bf-e2fcadebea8d",
					Minor:     pointer.Int32(0),
					Health:    true,
					Resources: gpuResourceList,
				},
				{
					Type:      schedulingv1alpha1.GPU,
					UUID:      "GPU-befd76c3-8a36-7b8a-179c-eae75aa7d9f2",
					Minor:     pointer.Int32(1),
					Health:    true,
					Resources: gpuResourceList,
				},
				{
					Type:      schedulingv1alpha1.GPU,
					UUID:      "GPU-87a9047b-dade-e08c-c067-7fedfd2e2750",
					Minor:     pointer.Int32(2),
					Health:    true,
					Resources: gpuResourceList,
				},
				{
					Type:      schedulingv1alpha1.GPU,
					UUID:      "GPU-44a68f77-c18d-85a6-5425-e314c0e8e182",
					Minor:     pointer.Int32(3),
					Health:    true,
					Resources: gpuResourceList,
				},
				{
					Type:      schedulingv1alpha1.GPU,
					UUID:      "GPU-ac53dc25-2cb7-a11d-417f-ce23331dcea0",
					Minor:     pointer.Int32(4),
					Health:    true,
					Resources: gpuResourceList,
				},
				{
					Type:      schedulingv1alpha1.GPU,
					UUID:      "GPU-3908dbfd-6e0b-013d-549b-fca246a16fa0",
					Minor:     pointer.Int32(5),
					Health:    true,
					Resources: gpuResourceList,
				},
				{
					Type:      schedulingv1alpha1.GPU,
					UUID:      "GPU-7a87e98a-a1a7-28bc-c880-28c870bf0c7d",
					Minor:     pointer.Int32(6),
					Health:    true,
					Resources: gpuResourceList,
				},
				{
					Type:      schedulingv1alpha1.GPU,
					UUID:      "GPU-c3b7de0e-8a41-9bdb-3f71-8175c3438890",
					Minor:     pointer.Int32(7),
					Health:    true,
					Resources: gpuResourceList,
				},
			},
		},
	}
)

func makeTestDeviceTopology(numSocket, numNode, numPCIEPerNode, numGPUPerPCIE, numRDMAPerPCIE int32) *unified.DeviceTopology {
	var deviceTopology unified.DeviceTopology
	var rdmaIndex, gpuIndex, pcieIndex, numaNodeIndex int32
	rdmaIndex = 1
	for i := int32(0); i < numSocket; i++ {
		socket := unified.NUMASocketInfo{
			Index: numaNodeIndex,
		}
		numaNodeIndex++
		for j := int32(0); j < numNode; j++ {
			node := unified.NUMANodeInfo{
				Index: j,
			}
			for k := int32(0); k < numPCIEPerNode; k++ {
				pcie := unified.PCIESwitchInfo{
					Index: pcieIndex,
				}
				pcieIndex++
				for l := int32(0); l < numGPUPerPCIE; l++ {
					pcie.GPUs = append(pcie.GPUs, gpuIndex)
					gpuIndex++
				}
				for m := int32(0); m < numRDMAPerPCIE; m++ {
					pcie.RDMAs = append(pcie.RDMAs, &unified.RDMADeviceInfo{
						Name:  fmt.Sprintf("mlx5_bond_%d", rdmaIndex),
						Bond:  uint32(rdmaIndex),
						Minor: rdmaIndex,
					})
					rdmaIndex++
				}
				node.PCIESwitches = append(node.PCIESwitches, pcie)
			}
			socket.NUMANodes = append(socket.NUMANodes, node)
		}
		deviceTopology.NUMASockets = append(deviceTopology.NUMASockets, socket)
	}
	return &deviceTopology
}

func makeTestRDMATopology(numRDMA, numVFPerRDMA int) *unified.RDMATopology {
	topology := &unified.RDMATopology{
		VFs: map[int32][]unified.VF{},
	}
	vfBaseBDF := 0x1f
	for i := 1; i <= numRDMA; i++ {
		vfBaseBDF++
		var vfs []unified.VF
		for j := 0; j < numVFPerRDMA; j++ {
			busID := fmt.Sprintf("0000:%x:00.%x", vfBaseBDF, j)
			vfs = append(vfs, unified.VF{
				BusID:    busID,
				Minor:    int32(j),
				Priority: unified.VFPriorityHigh,
			})
		}
		topology.VFs[int32(i)] = vfs
	}
	return topology
}

func TestAutopilotAllocator(t *testing.T) {
	tests := []struct {
		name            string
		deviceCR        *schedulingv1alpha1.Device
		deviceTopology  *unified.DeviceTopology
		rdmaTopology    *unified.RDMATopology
		gpuWanted       int
		hostNetwork     bool
		assignedDevices apiext.DeviceAllocations
		want            apiext.DeviceAllocations
		wantErr         bool
	}{
		{
			name:         "request 1 GPU and 1 VF but with invalid RDMA Topology",
			deviceCR:     fakeDeviceCR,
			rdmaTopology: &unified.RDMATopology{},
			gpuWanted:    1,
			want:         nil,
			wantErr:      true,
		},
		{
			name:           "request 1 GPU and 1 VF but invalid Device Topology",
			deviceCR:       fakeDeviceCR,
			deviceTopology: &unified.DeviceTopology{},
			gpuWanted:      1,
			want:           nil,
			wantErr:        true,
		},
		{
			name:         "allocate 0 GPU and 1 VF but invalid RDMA Topology",
			deviceCR:     fakeDeviceCR,
			rdmaTopology: &unified.RDMATopology{},
			gpuWanted:    0,
			want:         nil,
			wantErr:      true,
		},
		{
			name:           "allocate 0 GPU and 1 VF but invalid Device Topology",
			deviceCR:       fakeDeviceCR,
			deviceTopology: &unified.DeviceTopology{},
			gpuWanted:      0,
			want:           nil,
			wantErr:        true,
		},
		{
			name:      "allocate 0 GPU and 1 VF",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 0,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:03.7","minor":29}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 1 GPU and 1 VF",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 1,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 2 GPU and 1 VF",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 2,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 3 GPU and 2 VF",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 3,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:90:00.2","minor":0}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 4 GPU and 2 VF",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 4,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
					{
						Minor:     3,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:90:00.2","minor":0}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 6 GPU and 3 VF",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 6,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
					{
						Minor:     3,
						Resources: gpuResourceList,
					},
					{
						Minor:     4,
						Resources: gpuResourceList,
					},
					{
						Minor:     5,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:90:00.2","minor":0}]}`),
					},
					{
						Minor: 3,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:51:00.2","minor":0}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 8 GPU and 4 VF",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 8,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
					{
						Minor:     3,
						Resources: gpuResourceList,
					},
					{
						Minor:     4,
						Resources: gpuResourceList,
					},
					{
						Minor:     5,
						Resources: gpuResourceList,
					},
					{
						Minor:     6,
						Resources: gpuResourceList,
					},
					{
						Minor:     7,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:90:00.2","minor":0}]}`),
					},
					{
						Minor: 3,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:51:00.2","minor":0}]}`),
					},
					{
						Minor: 4,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:b9:00.2","minor":0}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 2 GPU and 1 VF with assigned devices",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 2,
			assignedDevices: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
				},
			},
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
					{
						Minor:     3,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:90:00.2","minor":0}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 3 GPU and 2 VF with assigned devices",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 3,
			assignedDevices: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
				},
			},
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
					{
						Minor:     3,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.3","minor":1}]}`),
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:90:00.2","minor":0}]}`),
					},
				},
			},
		},
		{
			name:           "Only 1 RDMA and 4 PCIE with 2 GPUs Per PCIE, allocate 4 GPUs",
			deviceCR:       fakeDeviceCR,
			deviceTopology: makeTestDeviceTopology(2, 1, 4, 2, 1),
			rdmaTopology:   makeTestRDMATopology(1, 30),
			gpuWanted:      4,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
					{
						Minor:     3,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:20:00.0","minor":0}]}`),
					},
				},
			},
		},
		{
			name:           "4 RDMA and 4 PCIE with 2 GPUs Per PCIE, allocate 4 GPUs",
			deviceCR:       fakeDeviceCR,
			deviceTopology: makeTestDeviceTopology(2, 1, 4, 2, 1),
			rdmaTopology:   makeTestRDMATopology(2, 30),
			gpuWanted:      4,
			assignedDevices: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
					{
						Minor:     3,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:20:00.0","minor":0}]}`),
					},
				},
			},
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     4,
						Resources: gpuResourceList,
					},
					{
						Minor:     5,
						Resources: gpuResourceList,
					},
					{
						Minor:     6,
						Resources: gpuResourceList,
					},
					{
						Minor:     7,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:20:00.1","minor":1}]}`),
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:21:00.0","minor":0}]}`),
					},
				},
			},
		},
		{
			name:           "1 GPU with hostNetwork",
			deviceCR:       fakeDeviceCR,
			deviceTopology: makeTestDeviceTopology(2, 1, 4, 2, 1),
			rdmaTopology:   &unified.RDMATopology{},
			gpuWanted:      4,
			hostNetwork:    true,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
					{
						Minor:     3,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
					},
					{
						Minor: 3,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
					},
					{
						Minor: 4,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			koordFakeClient := koordfake.NewSimpleClientset()
			deviceCR := tt.deviceCR.DeepCopy()
			if tt.deviceTopology != nil {
				data, err := json.Marshal(tt.deviceTopology)
				assert.NoError(t, err)
				deviceCR.Annotations[unified.AnnotationDeviceTopology] = string(data)
			}
			if tt.rdmaTopology != nil {
				data, err := json.Marshal(tt.rdmaTopology)
				assert.NoError(t, err)
				deviceCR.Annotations[unified.AnnotationRDMATopology] = string(data)
			}
			_, err := koordFakeClient.SchedulingV1alpha1().Devices().Create(context.TODO(), deviceCR, metav1.CreateOptions{})
			assert.NoError(t, err)
			koordShareInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordFakeClient, 0)

			kubeFakeClient := kubefake.NewSimpleClientset(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
			})
			sharedInformerFactory := informers.NewSharedInformerFactory(kubeFakeClient, 0)

			if tt.assignedDevices != nil {
				data, err := json.Marshal(tt.assignedDevices)
				assert.NoError(t, err)
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod",
						UID:       uuid.NewUUID(),
						Annotations: map[string]string{
							apiext.AnnotationDeviceAllocated: string(data),
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "test-node-1",
						Containers: []corev1.Container{
							{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										apiext.ResourceNvidiaGPU: *resource.NewQuantity(1, resource.DecimalSI),
									},
								},
							},
						},
					},
				}
				_, err = kubeFakeClient.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			deviceCache := newNodeDeviceCache()
			registerDeviceEventHandler(deviceCache, koordShareInformerFactory)
			registerPodEventHandler(deviceCache, sharedInformerFactory, koordShareInformerFactory)

			allocator := NewAutopilotAllocator(AllocatorOptions{
				SharedInformerFactory:      sharedInformerFactory,
				KoordSharedInformerFactory: koordShareInformerFactory,
			})

			sharedInformerFactory.Start(nil)
			sharedInformerFactory.WaitForCacheSync(nil)

			nodeDevice := deviceCache.getNodeDevice("test-node-1", false)
			assert.NotNil(t, nodeDevice)

			podRequest := corev1.ResourceList{}
			if tt.gpuWanted > 0 {
				podRequest[apiext.ResourceNvidiaGPU] = *resource.NewQuantity(int64(tt.gpuWanted), resource.DecimalSI)
				combination, err := ValidateDeviceRequest(podRequest)
				assert.NoError(t, err)
				podRequest = ConvertDeviceRequest(podRequest, combination)
			}

			podRequest[apiext.ResourceRDMA] = *resource.NewQuantity(1, resource.DecimalSI)

			nodeDevice.lock.Lock()
			defer nodeDevice.lock.Unlock()

			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					HostNetwork: tt.hostNetwork,
				},
			}

			allocations, err := allocator.Allocate("test-node-1", pod, podRequest, nodeDevice, nil, nil, nil, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Allocate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			sortDeviceAllocations(allocations)
			sortDeviceAllocations(tt.want)
			assert.Equal(t, tt.want, allocations)
		})
	}
}

func TestAutopilotAllocatorVFByType(t *testing.T) {
	deviceCR := fakeDeviceCR.DeepCopy()
	deviceCR.Annotations[unified.AnnotationDeviceTopology] = `{"numaSockets":[{"index":0,"numaNodes":[{"index":0,"pcieSwitches":[{"gpus":[0,1],"index":0,"rdmas":[{"bond":0,"bondSlaves":["eth0","eth1"],"minor":0,"name":"mlx5_bond_0","uVerbs":"/dev/infiniband/uverbs0","type":"pf_system"},{"bond":1,"bondSlaves":["eth0","eth1"],"minor":1,"name":"mlx5_bond_1","uVerbs":"/dev/infiniband/uverbs0","type":"pf_system"}]},{"gpus":[2,3],"index":1,"rdmas":[{"bond":2,"bondSlaves":["eth2","eth3"],"minor":2,"name":"mlx5_bond_2","uVerbs":"/dev/infiniband/uverbs1","type":"pf_worker"}]}]}]},{"index":1,"numaNodes":[{"index":1,"pcieSwitches":[{"gpus":[4,5],"index":2,"rdmas":[{"bond":3,"bondSlaves":["eth4","eth5"],"minor":3,"name":"mlx5_bond_3","uVerbs":"/dev/infiniband/uverbs2","type":"pf_worker"}]},{"gpus":[6,7],"index":3,"rdmas":[{"bond":4,"bondSlaves":["eth6","eth7"],"minor":4,"name":"mlx5_bond_4","uVerbs":"/dev/infiniband/uverbs3","type":"pf_worker"}]}]}]}]}`
	deviceCR.Annotations[unified.AnnotationRDMATopology] = `{"vfs":{"1":[{"busID":"0000:1f:00.2","minor":0,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:00.3","minor":1,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:00.4","minor":2,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:00.5","minor":3,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:00.6","minor":4,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:00.7","minor":5,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:01.0","minor":6,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:01.1","minor":7,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:01.2","minor":8,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:01.3","minor":9,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:01.4","minor":10,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:01.5","minor":11,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:01.6","minor":12,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:01.7","minor":13,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:02.0","minor":14,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:1f:02.1","minor":15,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:02.2","minor":16,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:02.3","minor":17,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:02.4","minor":18,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:02.5","minor":19,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:02.6","minor":20,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:02.7","minor":21,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:03.0","minor":22,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:03.1","minor":23,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:03.2","minor":24,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:03.3","minor":25,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:03.4","minor":26,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:03.5","minor":27,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:1f:03.6","minor":28,"priority":"VFPriorityLow","type":"vf_storage"},{"busID":"0000:1f:03.7","minor":29,"priority":"VFPriorityLow","type":"vf_storage"}],"2":[{"busID":"0000:90:00.2","minor":0,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:00.3","minor":1,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:00.4","minor":2,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:00.5","minor":3,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:00.6","minor":4,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:00.7","minor":5,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:01.0","minor":6,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:01.1","minor":7,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:01.2","minor":8,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:01.3","minor":9,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:01.4","minor":10,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:01.5","minor":11,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:01.6","minor":12,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:01.7","minor":13,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:02.0","minor":14,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:90:02.1","minor":15,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:02.2","minor":16,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:02.3","minor":17,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:02.4","minor":18,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:02.5","minor":19,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:02.6","minor":20,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:02.7","minor":21,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:03.0","minor":22,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:03.1","minor":23,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:03.2","minor":24,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:03.3","minor":25,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:03.4","minor":26,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:03.5","minor":27,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:90:03.6","minor":28,"priority":"VFPriorityLow","type":"vf_storage"},{"busID":"0000:90:03.7","minor":29,"priority":"VFPriorityLow","type":"vf_storage"}],"3":[{"busID":"0000:51:00.2","minor":0,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:00.3","minor":1,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:00.4","minor":2,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:00.5","minor":3,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:00.6","minor":4,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:00.7","minor":5,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:01.0","minor":6,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:01.1","minor":7,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:01.2","minor":8,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:01.3","minor":9,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:01.4","minor":10,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:01.5","minor":11,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:01.6","minor":12,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:01.7","minor":13,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:02.0","minor":14,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:51:02.1","minor":15,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:02.2","minor":16,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:02.3","minor":17,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:02.4","minor":18,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:02.5","minor":19,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:02.6","minor":20,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:02.7","minor":21,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:03.0","minor":22,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:03.1","minor":23,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:03.2","minor":24,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:03.3","minor":25,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:03.4","minor":26,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:03.5","minor":27,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:51:03.6","minor":28,"priority":"VFPriorityLow","type":"vf_storage"},{"busID":"0000:51:03.7","minor":29,"priority":"VFPriorityLow","type":"vf_storage"}],"4":[{"busID":"0000:b9:00.2","minor":0,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:00.3","minor":1,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:00.4","minor":2,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:00.5","minor":3,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:00.6","minor":4,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:00.7","minor":5,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:01.0","minor":6,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:01.1","minor":7,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:01.2","minor":8,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:01.3","minor":9,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:01.4","minor":10,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:01.5","minor":11,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:01.6","minor":12,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:01.7","minor":13,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:02.0","minor":14,"priority":"VFPriorityHigh","type":"vf_gpu"},{"busID":"0000:b9:02.1","minor":15,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:02.2","minor":16,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:02.3","minor":17,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:02.4","minor":18,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:02.5","minor":19,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:02.6","minor":20,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:02.7","minor":21,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:03.0","minor":22,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:03.1","minor":23,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:03.2","minor":24,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:03.3","minor":25,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:03.4","minor":26,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:03.5","minor":27,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:b9:03.6","minor":28,"priority":"VFPriorityLow","type":"vf_storage"},{"busID":"0000:b9:03.7","minor":29,"priority":"VFPriorityLow","type":"vf_storage"}]}}`
	tests := []struct {
		name            string
		deviceCR        *schedulingv1alpha1.Device
		gpuWanted       int
		hostNetwork     bool
		assignedDevices apiext.DeviceAllocations
		want            apiext.DeviceAllocations
		wantErr         bool
	}{
		{
			name:      "allocate 0 GPU and 1 VF",
			deviceCR:  deviceCR,
			gpuWanted: 0,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:90:03.5","minor":27}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 1 GPU and 1 VF",
			deviceCR:  deviceCR,
			gpuWanted: 1,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:90:00.2","minor":0}]}`),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			koordFakeClient := koordfake.NewSimpleClientset()
			deviceCR := tt.deviceCR.DeepCopy()
			_, err := koordFakeClient.SchedulingV1alpha1().Devices().Create(context.TODO(), deviceCR, metav1.CreateOptions{})
			assert.NoError(t, err)
			koordShareInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordFakeClient, 0)

			kubeFakeClient := kubefake.NewSimpleClientset(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
			})
			sharedInformerFactory := informers.NewSharedInformerFactory(kubeFakeClient, 0)

			if tt.assignedDevices != nil {
				data, err := json.Marshal(tt.assignedDevices)
				assert.NoError(t, err)
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod",
						UID:       uuid.NewUUID(),
						Annotations: map[string]string{
							apiext.AnnotationDeviceAllocated: string(data),
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "test-node-1",
						Containers: []corev1.Container{
							{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										apiext.ResourceNvidiaGPU: *resource.NewQuantity(1, resource.DecimalSI),
									},
								},
							},
						},
					},
				}
				_, err = kubeFakeClient.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			deviceCache := newNodeDeviceCache()
			registerDeviceEventHandler(deviceCache, koordShareInformerFactory)
			registerPodEventHandler(deviceCache, sharedInformerFactory, koordShareInformerFactory)

			allocator := NewAutopilotAllocator(AllocatorOptions{
				SharedInformerFactory:      sharedInformerFactory,
				KoordSharedInformerFactory: koordShareInformerFactory,
			})

			sharedInformerFactory.Start(nil)
			sharedInformerFactory.WaitForCacheSync(nil)

			nodeDevice := deviceCache.getNodeDevice("test-node-1", false)
			assert.NotNil(t, nodeDevice)

			podRequest := corev1.ResourceList{}
			if tt.gpuWanted > 0 {
				podRequest[apiext.ResourceNvidiaGPU] = *resource.NewQuantity(int64(tt.gpuWanted), resource.DecimalSI)
				combination, err := ValidateDeviceRequest(podRequest)
				assert.NoError(t, err)
				podRequest = ConvertDeviceRequest(podRequest, combination)
			}

			podRequest[apiext.ResourceRDMA] = *resource.NewQuantity(1, resource.DecimalSI)

			nodeDevice.lock.Lock()
			defer nodeDevice.lock.Unlock()

			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					HostNetwork: tt.hostNetwork,
				},
			}

			allocations, err := allocator.Allocate("test-node-1", pod, podRequest, nodeDevice, nil, nil, nil, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Allocate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			sortDeviceAllocations(allocations)
			sortDeviceAllocations(tt.want)
			assert.Equal(t, tt.want, allocations)
		})
	}
}

func TestAutopilotAllocatorReserveAndUnreserve(t *testing.T) {
	fakeDevice := fakeDeviceCR.DeepCopy()
	for i := 0; i < 6; i++ {
		fakeDevice.Spec.Devices = append(fakeDevice.Spec.Devices, schedulingv1alpha1.DeviceInfo{
			Minor:  pointer.Int32(int32(i)),
			UUID:   fmt.Sprintf("0000:90:00.%d", i),
			Type:   unified.NVSwitchDeviceType,
			Health: true,
			Resources: corev1.ResourceList{
				unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
			},
		})
	}

	koordFakeClient := koordfake.NewSimpleClientset()
	_, err := koordFakeClient.SchedulingV1alpha1().Devices().Create(context.TODO(), fakeDevice, metav1.CreateOptions{})
	assert.NoError(t, err)
	koordShareInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordFakeClient, 0)

	kubeFakeClient := kubefake.NewSimpleClientset()
	sharedInformerFactory := informers.NewSharedInformerFactory(kubeFakeClient, 0)

	deviceCache := newNodeDeviceCache()
	registerDeviceEventHandler(deviceCache, koordShareInformerFactory)
	registerPodEventHandler(deviceCache, sharedInformerFactory, koordShareInformerFactory)

	allocator := NewAutopilotAllocator(AllocatorOptions{
		SharedInformerFactory:      sharedInformerFactory,
		KoordSharedInformerFactory: koordShareInformerFactory,
	})
	nodeDevice := deviceCache.getNodeDevice("test-node-1", false)
	assert.NotNil(t, nodeDevice)

	allocations := apiext.DeviceAllocations{
		schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
			{
				Minor:     0,
				Resources: gpuResourceList,
			},
			{
				Minor:     1,
				Resources: gpuResourceList,
			},
		},
		schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
			{
				Minor: 1,
				Resources: corev1.ResourceList{
					apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
				},
				Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.3","minor":1}]}`),
			},
			{
				Minor: 2,
				Resources: corev1.ResourceList{
					apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
				},
				Extension: json.RawMessage(`{"vfs":[{"busID":"0000:90:00.2","minor":0}]}`),
			},
		},
		unified.NVSwitchDeviceType: []*apiext.DeviceAllocation{
			{
				Minor: 0,
				Resources: corev1.ResourceList{
					unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Namespace: "default",
			Name:      "test-pod",
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node-1",
		},
	}

	nodeDevice.lock.Lock()
	defer nodeDevice.lock.Unlock()

	allocator.Reserve(pod, nodeDevice, allocations)
	expectedUsed := map[schedulingv1alpha1.DeviceType]deviceResources{
		schedulingv1alpha1.GPU: {
			0: gpuResourceList,
			1: gpuResourceList,
		},
		schedulingv1alpha1.RDMA: {
			1: corev1.ResourceList{
				apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
			},
			2: corev1.ResourceList{
				apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
			},
		},
		unified.NVSwitchDeviceType: {
			0: corev1.ResourceList{
				unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
			},
		},
	}
	assert.Equal(t, expectedUsed, nodeDevice.deviceUsed)
	expectedUsedVFs := map[int]sets.String{
		1: sets.NewString("0000:1f:00.3"),
		2: sets.NewString("0000:90:00.2"),
	}
	assert.Equal(t, expectedUsedVFs, allocator.(*AutopilotAllocator).vfCache.getAllocatedVFs("test-node-1"))

	allocator.Unreserve(pod, nodeDevice, allocations)
	assert.Equal(t, map[schedulingv1alpha1.DeviceType]deviceResources{}, nodeDevice.deviceUsed)
	assert.Equal(t, map[int]sets.String(nil), allocator.(*AutopilotAllocator).vfCache.getAllocatedVFs("test-node-1"))
}

func TestAutopilotAllocateNVSwitch(t *testing.T) {
	tests := []struct {
		name            string
		deviceCR        *schedulingv1alpha1.Device
		gpuWanted       int
		assignedDevices apiext.DeviceAllocations
		want            apiext.DeviceAllocations
		wantErr         bool
	}{
		{
			name:      "allocate 1 GPU and 1 VF and 0 NVSwitch",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 1,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 2 GPU and 1 VF and 1 NVSwitch",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 2,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
				},
				unified.NVSwitchDeviceType: []*apiext.DeviceAllocation{
					{
						Minor: 0,
						Resources: corev1.ResourceList{
							unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
						},
					},
				},
			},
		},
		{
			name:      "allocate 3 GPU and 2 VF and 2 NVSwitch",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 3,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:90:00.2","minor":0}]}`),
					},
				},
				unified.NVSwitchDeviceType: []*apiext.DeviceAllocation{
					{
						Minor: 0,
						Resources: corev1.ResourceList{
							unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
						},
					},
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
						},
					},
				},
			},
		},
		{
			name:      "allocate 8 GPU and 4 VF and 6 VF",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 8,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
					{
						Minor:     3,
						Resources: gpuResourceList,
					},
					{
						Minor:     4,
						Resources: gpuResourceList,
					},
					{
						Minor:     5,
						Resources: gpuResourceList,
					},
					{
						Minor:     6,
						Resources: gpuResourceList,
					},
					{
						Minor:     7,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:90:00.2","minor":0}]}`),
					},
					{
						Minor: 3,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:51:00.2","minor":0}]}`),
					},
					{
						Minor: 4,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:b9:00.2","minor":0}]}`),
					},
				},
				unified.NVSwitchDeviceType: []*apiext.DeviceAllocation{
					{
						Minor: 0,
						Resources: corev1.ResourceList{
							unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
						},
					},
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
						},
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
						},
					},
					{
						Minor: 3,
						Resources: corev1.ResourceList{
							unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
						},
					},
					{
						Minor: 4,
						Resources: corev1.ResourceList{
							unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
						},
					},
					{
						Minor: 5,
						Resources: corev1.ResourceList{
							unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
						},
					},
				},
			},
		},
		{
			name:      "allocate 2 GPU and 1 VF and 1 NVSwitch with assigned devices",
			deviceCR:  fakeDeviceCR,
			gpuWanted: 2,
			assignedDevices: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:1f:00.2","minor":0}]}`),
					},
				},
				unified.NVSwitchDeviceType: []*apiext.DeviceAllocation{
					{
						Minor: 0,
						Resources: corev1.ResourceList{
							unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
						},
					},
				},
			},
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
					{
						Minor:     3,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:90:00.2","minor":0}]}`),
					},
				},
				unified.NVSwitchDeviceType: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeDevice := tt.deviceCR.DeepCopy()
			for i := 0; i < 6; i++ {
				fakeDevice.Spec.Devices = append(fakeDevice.Spec.Devices, schedulingv1alpha1.DeviceInfo{
					Minor:  pointer.Int32(int32(i)),
					UUID:   fmt.Sprintf("0000:90:00.%d", i),
					Type:   unified.NVSwitchDeviceType,
					Health: true,
					Resources: corev1.ResourceList{
						unified.NVSwitchResource: *resource.NewQuantity(100, resource.DecimalSI),
					},
				})
			}

			koordFakeClient := koordfake.NewSimpleClientset()
			_, err := koordFakeClient.SchedulingV1alpha1().Devices().Create(context.TODO(), fakeDevice, metav1.CreateOptions{})
			assert.NoError(t, err)
			koordShareInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordFakeClient, 0)

			kubeFakeClient := kubefake.NewSimpleClientset(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
			})
			sharedInformerFactory := informers.NewSharedInformerFactory(kubeFakeClient, 0)

			if tt.assignedDevices != nil {
				data, err := json.Marshal(tt.assignedDevices)
				assert.NoError(t, err)
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod",
						UID:       uuid.NewUUID(),
						Annotations: map[string]string{
							apiext.AnnotationDeviceAllocated: string(data),
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "test-node-1",
						Containers: []corev1.Container{
							{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										apiext.ResourceNvidiaGPU: *resource.NewQuantity(1, resource.DecimalSI),
									},
								},
							},
						},
					},
				}
				_, err = kubeFakeClient.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			deviceCache := newNodeDeviceCache()
			registerDeviceEventHandler(deviceCache, koordShareInformerFactory)
			registerPodEventHandler(deviceCache, sharedInformerFactory, koordShareInformerFactory)

			allocator := NewAutopilotAllocator(AllocatorOptions{
				SharedInformerFactory:      sharedInformerFactory,
				KoordSharedInformerFactory: koordShareInformerFactory,
			})

			sharedInformerFactory.Start(nil)
			sharedInformerFactory.WaitForCacheSync(nil)

			nodeDevice := deviceCache.getNodeDevice("test-node-1", false)
			assert.NotNil(t, nodeDevice)

			podRequest := corev1.ResourceList{
				apiext.ResourceNvidiaGPU: *resource.NewQuantity(int64(tt.gpuWanted), resource.DecimalSI),
			}
			combination, err := ValidateDeviceRequest(podRequest)
			assert.NoError(t, err)
			podRequest = ConvertDeviceRequest(podRequest, combination)
			podRequest[apiext.ResourceRDMA] = *resource.NewQuantity(1, resource.DecimalSI)

			nodeDevice.lock.Lock()
			defer nodeDevice.lock.Unlock()

			allocations, err := allocator.Allocate("test-node-1", &corev1.Pod{}, podRequest, nodeDevice, nil, nil, nil, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Allocate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			sortDeviceAllocations(allocations)
			sortDeviceAllocations(tt.want)
			assert.Equal(t, tt.want, allocations)
		})
	}
}

var fakeH800DeviceCR = &schedulingv1alpha1.Device{
	ObjectMeta: metav1.ObjectMeta{
		Name: "test-node-1",
		Annotations: map[string]string{
			unified.AnnotationDevicePCIInfos:       `[{"type":"gpu","busID":"0000:08:00.0"},{"type":"gpu","minor":1,"busID":"0000:7e:00.0"},{"type":"gpu","minor":2,"busID":"0000:a2:00.0"},{"type":"gpu","minor":3,"busID":"0000:c6:00.0"},{"type":"gpu","minor":4,"busID":"0001:09:00.0"},{"type":"gpu","minor":5,"busID":"0001:7f:00.0"},{"type":"gpu","minor":6,"busID":"0001:a3:00.0"},{"type":"gpu","minor":7,"busID":"0001:c7:00.0"}]`,
			unified.AnnotationDeviceTopology:       `{"numaSockets":[{"index":0,"numaNodes":[{"index":0,"pcieSwitches":[{"index":0,"gpus":[0],"rdmas":[{"type":"pf_worker","minor":1,"uVerbs":"/dev/infiniband/uverbs1","bond":1,"bondSlaves":["eth0","eth1"],"bw":400}]},{"index":1,"rdmas":[{"type":"pf_system","minor":0,"uVerbs":"/dev/infiniband/uverbs2","bondSlaves":["eth2","eth3"],"bw":400}]},{"index":2,"gpus":[1],"rdmas":[{"type":"pf_worker","minor":2,"uVerbs":"/dev/infiniband/uverbs0","bond":2,"bondSlaves":["eth4","eth5"],"bw":400}]},{"index":3,"gpus":[2],"rdmas":[{"type":"pf_worker","minor":3,"uVerbs":"/dev/infiniband/uverbs4","bond":3,"bondSlaves":["eth6","eth7"],"bw":400}]},{"index":4,"gpus":[3],"rdmas":[{"type":"pf_worker","minor":4,"uVerbs":"/dev/infiniband/uverbs5","bond":4,"bondSlaves":["eth8","eth9"],"bw":400}]}]}]},{"index":1,"numaNodes":[{"index":1,"pcieSwitches":[{"index":5,"gpus":[4],"rdmas":[{"type":"pf_worker","minor":5,"uVerbs":"/dev/infiniband/uverbs6","bond":5,"bondSlaves":["eth10","eth11"],"bw":400}]},{"index":6,"gpus":[5],"rdmas":[{"type":"pf_worker","minor":6,"uVerbs":"/dev/infiniband/uverbs3","bond":6,"bondSlaves":["eth12","eth13"],"bw":400}]},{"index":7,"gpus":[6],"rdmas":[{"type":"pf_worker","minor":7,"uVerbs":"/dev/infiniband/uverbs7","bond":7,"bondSlaves":["eth14","eth15"],"bw":400}]},{"index":8,"gpus":[7],"rdmas":[{"type":"pf_worker","minor":8,"uVerbs":"/dev/infiniband/uverbs8","bond":8,"bondSlaves":["eth16","eth17"],"bw":400}]}]}]}]}`,
			unified.AnnotationRDMATopology:         `{"vfs":{"1":[{"name":"vf_eth0_0","busID":"0000:09:00.2","minor":0,"priority":"VFPriorityLow","type":"vf_storage"},{"busID":"0000:09:00.3","minor":1,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:09:00.4","minor":2,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:09:00.5","minor":3,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:09:00.6","minor":4,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:09:02.2","minor":16,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:09:02.3","minor":17,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:09:02.4","minor":18,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:09:02.5","minor":19,"priority":"VFPriorityLow","type":"vf_cpu"}],"2":[{"name":"vf_eth4_0","busID":"0000:7f:00.2","minor":0,"priority":"VFPriorityLow","type":"vf_storage"},{"busID":"0000:7f:00.3","minor":1,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:7f:00.4","minor":2,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:7f:00.5","minor":3,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:7f:00.6","minor":4,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:7f:02.2","minor":16,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:7f:02.3","minor":17,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:7f:02.4","minor":18,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:7f:02.5","minor":19,"priority":"VFPriorityLow","type":"vf_cpu"}],"3":[{"name":"vf_eth6_0","busID":"0000:a3:00.2","minor":0,"priority":"VFPriorityLow","type":"vf_storage"},{"busID":"0000:a3:00.3","minor":1,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:a3:00.4","minor":2,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:a3:00.5","minor":3,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:a3:00.6","minor":4,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:a3:02.2","minor":16,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:a3:02.3","minor":17,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:a3:02.4","minor":18,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:a3:02.5","minor":19,"priority":"VFPriorityLow","type":"vf_cpu"}],"4":[{"name":"vf_eth8_0","busID":"0000:c7:00.2","minor":0,"priority":"VFPriorityLow","type":"vf_storage"},{"busID":"0000:c7:00.3","minor":1,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:c7:00.4","minor":2,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:c7:00.5","minor":3,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:c7:00.6","minor":4,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0000:c7:02.2","minor":16,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:c7:02.3","minor":17,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:c7:02.4","minor":18,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0000:c7:02.5","minor":19,"priority":"VFPriorityLow","type":"vf_cpu"}],"5":[{"name":"vf_eth10_0","busID":"0001:08:00.2","minor":0,"priority":"VFPriorityLow","type":"vf_storage"},{"busID":"0001:08:00.3","minor":1,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:08:00.4","minor":2,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:08:00.5","minor":3,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:08:00.6","minor":4,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:08:02.2","minor":16,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0001:08:02.3","minor":17,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0001:08:02.4","minor":18,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0001:08:02.5","minor":19,"priority":"VFPriorityLow","type":"vf_cpu"}],"6":[{"name":"vf_eth12_0","busID":"0001:7e:00.2","minor":0,"priority":"VFPriorityLow","type":"vf_storage"},{"busID":"0001:7e:00.3","minor":1,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:7e:00.4","minor":2,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:7e:00.5","minor":3,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:7e:00.6","minor":4,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:7e:02.2","minor":16,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0001:7e:02.3","minor":17,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0001:7e:02.4","minor":18,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0001:7e:02.5","minor":19,"priority":"VFPriorityLow","type":"vf_cpu"}],"7":[{"name":"vf_eth14_0","busID":"0001:a2:00.2","minor":0,"priority":"VFPriorityLow","type":"vf_storage"},{"busID":"0001:a2:00.3","minor":1,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:a2:00.4","minor":2,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:a2:00.5","minor":3,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:a2:00.6","minor":4,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:a2:02.2","minor":16,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0001:a2:02.3","minor":17,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0001:a2:02.4","minor":18,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0001:a2:02.5","minor":19,"priority":"VFPriorityLow","type":"vf_cpu"}],"8":[{"name":"vf_eth16_0","busID":"0001:c6:00.2","minor":0,"priority":"VFPriorityLow","type":"vf_storage"},{"busID":"0001:c6:00.3","minor":1,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:c6:00.4","minor":2,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:c6:00.5","minor":3,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:c6:00.6","minor":4,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:c6:02.1","minor":15,"priority":"VFPriorityLow","type":"vf_gpu"},{"busID":"0001:c6:02.2","minor":16,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0001:c6:02.3","minor":17,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0001:c6:02.4","minor":18,"priority":"VFPriorityLow","type":"vf_cpu"},{"busID":"0001:c6:02.5","minor":19,"priority":"VFPriorityLow","type":"vf_cpu"}]}}`,
			unified.AnnotationNVIDIADriverVersions: `["470.141.10"]`,
		},
	},
	Spec: schedulingv1alpha1.DeviceSpec{
		Devices: []schedulingv1alpha1.DeviceInfo{
			{
				Type:   schedulingv1alpha1.RDMA,
				UUID:   "0000:09:00.0",
				Minor:  pointer.Int32(1),
				Health: true,
				Resources: corev1.ResourceList{
					apiext.ResourceRDMA: *resource.NewQuantity(100, resource.DecimalSI),
				},
			},
			{
				Type:   schedulingv1alpha1.RDMA,
				UUID:   "0000:7f:00.0",
				Minor:  pointer.Int32(2),
				Health: true,
				Resources: corev1.ResourceList{
					apiext.ResourceRDMA: *resource.NewQuantity(100, resource.DecimalSI),
				},
			},
			{
				Type:   schedulingv1alpha1.RDMA,
				UUID:   "0000:a3:00.0",
				Minor:  pointer.Int32(3),
				Health: true,
				Resources: corev1.ResourceList{
					apiext.ResourceRDMA: *resource.NewQuantity(100, resource.DecimalSI),
				},
			},
			{
				Type:   schedulingv1alpha1.RDMA,
				UUID:   "0000:c7:00.0",
				Minor:  pointer.Int32(4),
				Health: true,
				Resources: corev1.ResourceList{
					apiext.ResourceRDMA: *resource.NewQuantity(100, resource.DecimalSI),
				},
			},
			{
				Type:   schedulingv1alpha1.RDMA,
				UUID:   "0001:08:00.0",
				Minor:  pointer.Int32(1),
				Health: true,
				Resources: corev1.ResourceList{
					apiext.ResourceRDMA: *resource.NewQuantity(100, resource.DecimalSI),
				},
			},
			{
				Type:   schedulingv1alpha1.RDMA,
				UUID:   "0001:7e:00.0",
				Minor:  pointer.Int32(2),
				Health: true,
				Resources: corev1.ResourceList{
					apiext.ResourceRDMA: *resource.NewQuantity(100, resource.DecimalSI),
				},
			},
			{
				Type:   schedulingv1alpha1.RDMA,
				UUID:   "0001:a2:00.0",
				Minor:  pointer.Int32(3),
				Health: true,
				Resources: corev1.ResourceList{
					apiext.ResourceRDMA: *resource.NewQuantity(100, resource.DecimalSI),
				},
			},
			{
				Type:   schedulingv1alpha1.RDMA,
				UUID:   "0001:c6:00.0",
				Minor:  pointer.Int32(4),
				Health: true,
				Resources: corev1.ResourceList{
					apiext.ResourceRDMA: *resource.NewQuantity(100, resource.DecimalSI),
				},
			},

			{
				Type:      schedulingv1alpha1.GPU,
				UUID:      "GPU-6b1ff724-4fe2-17b8-adfe-1f4c8a4148d1",
				Minor:     pointer.Int32(0),
				ModuleID:  pointer.Int32(6),
				Health:    true,
				Resources: gpuResourceList,
			},
			{
				Type:      schedulingv1alpha1.GPU,
				UUID:      "GPU-702ee422-96de-2dde-438f-b6eda3ef7efc",
				Minor:     pointer.Int32(1),
				ModuleID:  pointer.Int32(8),
				Health:    true,
				Resources: gpuResourceList,
			},
			{
				Type:      schedulingv1alpha1.GPU,
				UUID:      "GPU-e5a60856-a994-ec0b-1b04-eaa18205b00b",
				Minor:     pointer.Int32(2),
				ModuleID:  pointer.Int32(7),
				Health:    true,
				Resources: gpuResourceList,
			},
			{
				Type:      schedulingv1alpha1.GPU,
				UUID:      "GPU-e1f2597e-0996-be81-19c4-8dd19bca3761",
				Minor:     pointer.Int32(3),
				ModuleID:  pointer.Int32(5),
				Health:    true,
				Resources: gpuResourceList,
			},
			{
				Type:      schedulingv1alpha1.GPU,
				UUID:      "GPU-02d911ef-e297-3063-1734-b8d3c3b3f5fc",
				Minor:     pointer.Int32(4),
				ModuleID:  pointer.Int32(1),
				Health:    true,
				Resources: gpuResourceList,
			},
			{
				Type:      schedulingv1alpha1.GPU,
				UUID:      "GPU-8b1e54b7-84a6-960f-2723-ecee87de9d46",
				Minor:     pointer.Int32(5),
				ModuleID:  pointer.Int32(3),
				Health:    true,
				Resources: gpuResourceList,
			},
			{
				Type:      schedulingv1alpha1.GPU,
				UUID:      "GPU-5a1654e4-2a92-19a4-e97e-6b4ca19509cd",
				Minor:     pointer.Int32(6),
				ModuleID:  pointer.Int32(4),
				Health:    true,
				Resources: gpuResourceList,
			},
			{
				Type:      schedulingv1alpha1.GPU,
				UUID:      "GPU-f1646769-f47c-47b5-fe73-f7635d845bdf",
				Minor:     pointer.Int32(7),
				ModuleID:  pointer.Int32(2),
				Health:    true,
				Resources: gpuResourceList,
			},
		},
	},
}

func TestAllocateByPartition(t *testing.T) {
	tests := []struct {
		name            string
		deviceCR        *schedulingv1alpha1.Device
		deviceTopology  *unified.DeviceTopology
		rdmaTopology    *unified.RDMATopology
		gpuWanted       int
		hostNetwork     bool
		assignedDevices apiext.DeviceAllocations
		want            apiext.DeviceAllocations
		wantErr         bool
	}{
		{
			name:      "allocate 0 GPU and 1 VF",
			deviceCR:  fakeH800DeviceCR,
			gpuWanted: 0,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:09:02.5","minor":19}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 1 GPU and 1 VF",
			deviceCR:  fakeH800DeviceCR,
			gpuWanted: 1,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     4,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 5,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:08:00.3","minor":1}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 2 GPU and 2 VF",
			deviceCR:  fakeH800DeviceCR,
			gpuWanted: 2,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     4,
						Resources: gpuResourceList,
					},
					{
						Minor:     5,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 5,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:08:00.3","minor":1}]}`),
					},
					{
						Minor: 6,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:7e:00.3","minor":1}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 3 GPU",
			deviceCR:  fakeH800DeviceCR,
			gpuWanted: 3,
			wantErr:   true,
		},
		{
			name:      "allocate 4 GPU and 4 VF",
			deviceCR:  fakeH800DeviceCR,
			gpuWanted: 4,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     4,
						Resources: gpuResourceList,
					},
					{
						Minor:     5,
						Resources: gpuResourceList,
					},
					{
						Minor:     6,
						Resources: gpuResourceList,
					},
					{
						Minor:     7,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 5,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:08:00.3","minor":1}]}`),
					},
					{
						Minor: 6,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:7e:00.3","minor":1}]}`),
					},
					{
						Minor: 7,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:a2:00.3","minor":1}]}`),
					},
					{
						Minor: 8,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:c6:00.3","minor":1}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 6 GPU and 3 VF",
			deviceCR:  fakeH800DeviceCR,
			gpuWanted: 6,
			wantErr:   true,
		},
		{
			name:      "allocate 8 GPU and 8 VF",
			deviceCR:  fakeH800DeviceCR,
			gpuWanted: 8,
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     0,
						Resources: gpuResourceList,
					},
					{
						Minor:     1,
						Resources: gpuResourceList,
					},
					{
						Minor:     2,
						Resources: gpuResourceList,
					},
					{
						Minor:     3,
						Resources: gpuResourceList,
					},
					{
						Minor:     4,
						Resources: gpuResourceList,
					},
					{
						Minor:     5,
						Resources: gpuResourceList,
					},
					{
						Minor:     6,
						Resources: gpuResourceList,
					},
					{
						Minor:     7,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 1,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:09:00.3","minor":1}]}`),
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:7f:00.3","minor":1}]}`),
					},
					{
						Minor: 3,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:a3:00.3","minor":1}]}`),
					},
					{
						Minor: 4,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0000:c7:00.3","minor":1}]}`),
					},
					{
						Minor: 5,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:08:00.3","minor":1}]}`),
					},
					{
						Minor: 6,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:7e:00.3","minor":1}]}`),
					},
					{
						Minor: 7,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:a2:00.3","minor":1}]}`),
					},
					{
						Minor: 8,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:c6:00.3","minor":1}]}`),
					},
				},
			},
		},
		{
			name:      "allocate 2 GPU and 2 VF with assigned devices",
			deviceCR:  fakeH800DeviceCR,
			gpuWanted: 2,
			assignedDevices: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     4,
						Resources: gpuResourceList,
					},
					{
						Minor:     5,
						Resources: gpuResourceList,
					},
				},

				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 5,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:08:00.3","minor":1}]}`),
					},
					{
						Minor: 6,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:7e:00.3","minor":1}]}`),
					},
				},
			},
			want: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor:     6,
						Resources: gpuResourceList,
					},
					{
						Minor:     7,
						Resources: gpuResourceList,
					},
				},
				schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
					{
						Minor: 7,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:a2:00.3","minor":1}]}`),
					},
					{
						Minor: 8,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: json.RawMessage(`{"vfs":[{"busID":"0001:c6:00.3","minor":1}]}`),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			koordFakeClient := koordfake.NewSimpleClientset()
			deviceCR := tt.deviceCR.DeepCopy()
			if tt.deviceTopology != nil {
				data, err := json.Marshal(tt.deviceTopology)
				assert.NoError(t, err)
				deviceCR.Annotations[unified.AnnotationDeviceTopology] = string(data)
			}
			if tt.rdmaTopology != nil {
				data, err := json.Marshal(tt.rdmaTopology)
				assert.NoError(t, err)
				deviceCR.Annotations[unified.AnnotationRDMATopology] = string(data)
			}
			_, err := koordFakeClient.SchedulingV1alpha1().Devices().Create(context.TODO(), deviceCR, metav1.CreateOptions{})
			assert.NoError(t, err)
			koordShareInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordFakeClient, 0)

			kubeFakeClient := kubefake.NewSimpleClientset(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					Labels: map[string]string{
						apiext.LabelGPUModel: "H800",
					},
				},
			})
			sharedInformerFactory := informers.NewSharedInformerFactory(kubeFakeClient, 0)

			if tt.assignedDevices != nil {
				data, err := json.Marshal(tt.assignedDevices)
				assert.NoError(t, err)
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod",
						UID:       uuid.NewUUID(),
						Annotations: map[string]string{
							apiext.AnnotationDeviceAllocated: string(data),
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "test-node-1",
						Containers: []corev1.Container{
							{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										apiext.ResourceNvidiaGPU: *resource.NewQuantity(1, resource.DecimalSI),
									},
								},
							},
						},
					},
				}
				_, err = kubeFakeClient.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			deviceCache := newNodeDeviceCache()
			registerDeviceEventHandler(deviceCache, koordShareInformerFactory)
			registerPodEventHandler(deviceCache, sharedInformerFactory, koordShareInformerFactory)

			allocator := NewAutopilotAllocator(AllocatorOptions{
				SharedInformerFactory:      sharedInformerFactory,
				KoordSharedInformerFactory: koordShareInformerFactory,
			})

			sharedInformerFactory.Start(nil)
			sharedInformerFactory.WaitForCacheSync(nil)

			nodeDevice := deviceCache.getNodeDevice("test-node-1", false)
			assert.NotNil(t, nodeDevice)

			podRequest := corev1.ResourceList{}
			if tt.gpuWanted > 0 {
				podRequest[apiext.ResourceNvidiaGPU] = *resource.NewQuantity(int64(tt.gpuWanted), resource.DecimalSI)
				combination, err := ValidateDeviceRequest(podRequest)
				assert.NoError(t, err)
				podRequest = ConvertDeviceRequest(podRequest, combination)
			}

			podRequest[apiext.ResourceRDMA] = *resource.NewQuantity(1, resource.DecimalSI)

			nodeDevice.lock.Lock()
			defer nodeDevice.lock.Unlock()

			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					HostNetwork: tt.hostNetwork,
				},
			}

			allocations, err := allocator.Allocate("test-node-1", pod, podRequest, nodeDevice, nil, nil, nil, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Allocate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			sortDeviceAllocations(allocations)
			sortDeviceAllocations(tt.want)
			assert.Equal(t, tt.want, allocations)
		})
	}
}
