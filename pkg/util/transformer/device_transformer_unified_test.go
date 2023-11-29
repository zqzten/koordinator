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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

var (
	gpuResourceList = corev1.ResourceList{
		apiext.ResourceGPUCore:        *resource.NewQuantity(100, resource.DecimalSI),
		apiext.ResourceGPUMemoryRatio: *resource.NewQuantity(100, resource.DecimalSI),
		apiext.ResourceGPUMemory:      *resource.NewQuantity(85198045184, resource.BinarySI),
	}

	fakeDeviceCR = &schedulingv1alpha1.Device{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node-1",
			Annotations: map[string]string{
				//unified.AnnotationDeviceTopology: `{"aswIDs":["ASW-MASTER-G1-P1-S20-1.NA61"],"numaSockets":[{"index":0,"numaNodes":[{"index":0,"pcieSwitches":[{"gpus":[0,1],"index":0,"rdmas":[{"bond":0,"bondSlaves":["eth0","eth1"],"minor":0,"name":"mlx5_bond_0","uVerbs":"/dev/infiniband/uverbs0"},{"bond":1,"bondSlaves":["eth0","eth1"],"minor":1,"name":"mlx5_bond_1","uVerbs":"/dev/infiniband/uverbs0"}]},{"gpus":[2,3],"index":1,"rdmas":[{"bond":2,"bondSlaves":["eth2","eth3"],"minor":2,"name":"mlx5_bond_2","uVerbs":"/dev/infiniband/uverbs1"}]}]}]},{"index":1,"numaNodes":[{"index":1,"pcieSwitches":[{"gpus":[4,5],"index":2,"rdmas":[{"bond":3,"bondSlaves":["eth4","eth5"],"minor":3,"name":"mlx5_bond_3","uVerbs":"/dev/infiniband/uverbs2"}]},{"gpus":[6,7],"index":3,"rdmas":[{"bond":4,"bondSlaves":["eth6","eth7"],"minor":4,"name":"mlx5_bond_4","uVerbs":"/dev/infiniband/uverbs3"}]}]}]}],"pointOfDelivery":"MASTER-G1-P1"}`,
				//unified.AnnotationRDMATopology:   `{"vfs":{"1":[{"busID":"0000:1f:00.2","minor":0,"priority":"VFPriorityHigh"},{"busID":"0000:1f:00.3","minor":1,"priority":"VFPriorityHigh"},{"busID":"0000:1f:00.4","minor":2,"priority":"VFPriorityHigh"},{"busID":"0000:1f:00.5","minor":3,"priority":"VFPriorityHigh"},{"busID":"0000:1f:00.6","minor":4,"priority":"VFPriorityHigh"},{"busID":"0000:1f:00.7","minor":5,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.0","minor":6,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.1","minor":7,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.2","minor":8,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.3","minor":9,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.4","minor":10,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.5","minor":11,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.6","minor":12,"priority":"VFPriorityHigh"},{"busID":"0000:1f:01.7","minor":13,"priority":"VFPriorityHigh"},{"busID":"0000:1f:02.0","minor":14,"priority":"VFPriorityHigh"},{"busID":"0000:1f:02.1","minor":15,"priority":"VFPriorityLow"},{"busID":"0000:1f:02.2","minor":16,"priority":"VFPriorityLow"},{"busID":"0000:1f:02.3","minor":17,"priority":"VFPriorityLow"},{"busID":"0000:1f:02.4","minor":18,"priority":"VFPriorityLow"},{"busID":"0000:1f:02.5","minor":19,"priority":"VFPriorityLow"},{"busID":"0000:1f:02.6","minor":20,"priority":"VFPriorityLow"},{"busID":"0000:1f:02.7","minor":21,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.0","minor":22,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.1","minor":23,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.2","minor":24,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.3","minor":25,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.4","minor":26,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.5","minor":27,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.6","minor":28,"priority":"VFPriorityLow"},{"busID":"0000:1f:03.7","minor":29,"priority":"VFPriorityLow"}],"2":[{"busID":"0000:90:00.2","minor":0,"priority":"VFPriorityHigh"},{"busID":"0000:90:00.3","minor":1,"priority":"VFPriorityHigh"},{"busID":"0000:90:00.4","minor":2,"priority":"VFPriorityHigh"},{"busID":"0000:90:00.5","minor":3,"priority":"VFPriorityHigh"},{"busID":"0000:90:00.6","minor":4,"priority":"VFPriorityHigh"},{"busID":"0000:90:00.7","minor":5,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.0","minor":6,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.1","minor":7,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.2","minor":8,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.3","minor":9,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.4","minor":10,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.5","minor":11,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.6","minor":12,"priority":"VFPriorityHigh"},{"busID":"0000:90:01.7","minor":13,"priority":"VFPriorityHigh"},{"busID":"0000:90:02.0","minor":14,"priority":"VFPriorityHigh"},{"busID":"0000:90:02.1","minor":15,"priority":"VFPriorityLow"},{"busID":"0000:90:02.2","minor":16,"priority":"VFPriorityLow"},{"busID":"0000:90:02.3","minor":17,"priority":"VFPriorityLow"},{"busID":"0000:90:02.4","minor":18,"priority":"VFPriorityLow"},{"busID":"0000:90:02.5","minor":19,"priority":"VFPriorityLow"},{"busID":"0000:90:02.6","minor":20,"priority":"VFPriorityLow"},{"busID":"0000:90:02.7","minor":21,"priority":"VFPriorityLow"},{"busID":"0000:90:03.0","minor":22,"priority":"VFPriorityLow"},{"busID":"0000:90:03.1","minor":23,"priority":"VFPriorityLow"},{"busID":"0000:90:03.2","minor":24,"priority":"VFPriorityLow"},{"busID":"0000:90:03.3","minor":25,"priority":"VFPriorityLow"},{"busID":"0000:90:03.4","minor":26,"priority":"VFPriorityLow"},{"busID":"0000:90:03.5","minor":27,"priority":"VFPriorityLow"},{"busID":"0000:90:03.6","minor":28,"priority":"VFPriorityLow"},{"busID":"0000:90:03.7","minor":29,"priority":"VFPriorityLow"}],"3":[{"busID":"0000:51:00.2","minor":0,"priority":"VFPriorityHigh"},{"busID":"0000:51:00.3","minor":1,"priority":"VFPriorityHigh"},{"busID":"0000:51:00.4","minor":2,"priority":"VFPriorityHigh"},{"busID":"0000:51:00.5","minor":3,"priority":"VFPriorityHigh"},{"busID":"0000:51:00.6","minor":4,"priority":"VFPriorityHigh"},{"busID":"0000:51:00.7","minor":5,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.0","minor":6,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.1","minor":7,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.2","minor":8,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.3","minor":9,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.4","minor":10,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.5","minor":11,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.6","minor":12,"priority":"VFPriorityHigh"},{"busID":"0000:51:01.7","minor":13,"priority":"VFPriorityHigh"},{"busID":"0000:51:02.0","minor":14,"priority":"VFPriorityHigh"},{"busID":"0000:51:02.1","minor":15,"priority":"VFPriorityLow"},{"busID":"0000:51:02.2","minor":16,"priority":"VFPriorityLow"},{"busID":"0000:51:02.3","minor":17,"priority":"VFPriorityLow"},{"busID":"0000:51:02.4","minor":18,"priority":"VFPriorityLow"},{"busID":"0000:51:02.5","minor":19,"priority":"VFPriorityLow"},{"busID":"0000:51:02.6","minor":20,"priority":"VFPriorityLow"},{"busID":"0000:51:02.7","minor":21,"priority":"VFPriorityLow"},{"busID":"0000:51:03.0","minor":22,"priority":"VFPriorityLow"},{"busID":"0000:51:03.1","minor":23,"priority":"VFPriorityLow"},{"busID":"0000:51:03.2","minor":24,"priority":"VFPriorityLow"},{"busID":"0000:51:03.3","minor":25,"priority":"VFPriorityLow"},{"busID":"0000:51:03.4","minor":26,"priority":"VFPriorityLow"},{"busID":"0000:51:03.5","minor":27,"priority":"VFPriorityLow"},{"busID":"0000:51:03.6","minor":28,"priority":"VFPriorityLow"},{"busID":"0000:51:03.7","minor":29,"priority":"VFPriorityLow"}],"4":[{"busID":"0000:b9:00.2","minor":0,"priority":"VFPriorityHigh"},{"busID":"0000:b9:00.3","minor":1,"priority":"VFPriorityHigh"},{"busID":"0000:b9:00.4","minor":2,"priority":"VFPriorityHigh"},{"busID":"0000:b9:00.5","minor":3,"priority":"VFPriorityHigh"},{"busID":"0000:b9:00.6","minor":4,"priority":"VFPriorityHigh"},{"busID":"0000:b9:00.7","minor":5,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.0","minor":6,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.1","minor":7,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.2","minor":8,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.3","minor":9,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.4","minor":10,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.5","minor":11,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.6","minor":12,"priority":"VFPriorityHigh"},{"busID":"0000:b9:01.7","minor":13,"priority":"VFPriorityHigh"},{"busID":"0000:b9:02.0","minor":14,"priority":"VFPriorityHigh"},{"busID":"0000:b9:02.1","minor":15,"priority":"VFPriorityLow"},{"busID":"0000:b9:02.2","minor":16,"priority":"VFPriorityLow"},{"busID":"0000:b9:02.3","minor":17,"priority":"VFPriorityLow"},{"busID":"0000:b9:02.4","minor":18,"priority":"VFPriorityLow"},{"busID":"0000:b9:02.5","minor":19,"priority":"VFPriorityLow"},{"busID":"0000:b9:02.6","minor":20,"priority":"VFPriorityLow"},{"busID":"0000:b9:02.7","minor":21,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.0","minor":22,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.1","minor":23,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.2","minor":24,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.3","minor":25,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.4","minor":26,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.5","minor":27,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.6","minor":28,"priority":"VFPriorityLow"},{"busID":"0000:b9:03.7","minor":29,"priority":"VFPriorityLow"}]}}`,
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
			Index: i,
		}
		for j := int32(0); j < numNode; j++ {
			node := unified.NUMANodeInfo{
				Index: numaNodeIndex,
			}
			numaNodeIndex++
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

func TestTransformDeviceTopology(t *testing.T) {
	deviceCR := fakeDeviceCR.DeepCopy()
	deviceTopology := makeTestDeviceTopology(2, 1, 4, 2, 1)
	if deviceTopology != nil {
		data, err := json.Marshal(deviceTopology)
		assert.NoError(t, err)
		deviceCR.Annotations[unified.AnnotationDeviceTopology] = string(data)
	}
	TransformDeviceTopology(deviceCR)
	data, _ := json.Marshal(deviceCR)
	t.Logf(string(data))

}
