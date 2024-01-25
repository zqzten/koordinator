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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	koordfeatures "github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/deviceshare"
	utilfeature "github.com/koordinator-sh/koordinator/pkg/util/feature"
)

func TestValidateDeviceRequest(t *testing.T) {
	tests := []struct {
		name       string
		podRequest corev1.ResourceList
		want       uint
		wantErr    bool
	}{
		{
			name: "valid PPU request 2",
			podRequest: corev1.ResourceList{
				unified.ResourcePPU: resource.MustParse("2"),
			},
			want:    aliyunPPU,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := deviceshare.ValidateDeviceRequest(tt.podRequest)
			assert.Equal(t, tt.wantErr, err != nil)
			if !tt.wantErr {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestConvertDeviceRequest(t *testing.T) {
	type args struct {
		podRequest  corev1.ResourceList
		combination uint
	}
	tests := []struct {
		name string
		args args
		want corev1.ResourceList
	}{
		{
			name: "aliyunPPU",
			args: args{
				podRequest: corev1.ResourceList{
					unified.ResourcePPU: resource.MustParse("2"),
				},
				combination: aliyunPPU,
			},
			want: corev1.ResourceList{
				apiext.ResourceGPUCore:        *resource.NewQuantity(200, resource.DecimalSI),
				apiext.ResourceGPUMemoryRatio: *resource.NewQuantity(200, resource.DecimalSI),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deviceshare.ConvertDeviceRequest(tt.args.podRequest, tt.args.combination)
			assert.Equal(t, tt.want, got)
		})
	}
}

var damoPodData = `{
    "apiVersion": "v1",
    "kind": "Pod",
    "metadata": {
        "name": "dlc1et7xfpj4pxnl-master-0",
        "namespace": "quota15aqio4hzh2"
    },
    "spec": {
        "containers": [
            {
                "name": "pytorch",
                "resources": {
                    "limits": {
                        "cpu": "32",
                        "koordinator.sh/rdma": "1",
                        "memory": "400Gi",
                        "nvidia.com/gpu": "8"
                    },
                    "requests": {
                        "cpu": "32",
                        "koordinator.sh/rdma": "1",
                        "memory": "400Gi",
                        "nvidia.com/gpu": "8"
                    }
                }
            }
        ],
        "hostNetwork": true,
        "priority": 4,
        "priorityClassName": "pai-priority-4",
        "runtimeClassName": "nvidia",
        "schedulerName": "default-scheduler"
    }
}`

var damoDeviceData = []byte(`{
    "apiVersion": "scheduling.koordinator.sh/v1alpha1",
    "kind": "Device",
    "metadata": {
        "annotations": {
            "internal.scheduling.koordinator.sh/device-pci-infos": "[{\"type\":\"gpu\",\"busID\":\"0000:13:00.0\"},{\"type\":\"gpu\",\"minor\":1,\"busID\":\"0000:19:00.0\"},{\"type\":\"gpu\",\"minor\":2,\"busID\":\"0000:48:00.0\"},{\"type\":\"gpu\",\"minor\":3,\"busID\":\"0000:4d:00.0\"},{\"type\":\"gpu\",\"minor\":4,\"busID\":\"0000:89:00.0\"},{\"type\":\"gpu\",\"minor\":5,\"busID\":\"0000:8e:00.0\"},{\"type\":\"gpu\",\"minor\":6,\"busID\":\"0000:ad:00.0\"},{\"type\":\"gpu\",\"minor\":7,\"busID\":\"0000:b3:00.0\"},{\"type\":\"nvswitch\",\"busID\":\"0000:c0:00.0\"},{\"type\":\"nvswitch\",\"minor\":1,\"busID\":\"0000:c1:00.0\"},{\"type\":\"nvswitch\",\"minor\":2,\"busID\":\"0000:c2:00.0\"},{\"type\":\"nvswitch\",\"minor\":3,\"busID\":\"0000:c3:00.0\"},{\"type\":\"nvswitch\",\"minor\":4,\"busID\":\"0000:c4:00.0\"},{\"type\":\"nvswitch\",\"minor\":5,\"busID\":\"0000:c5:00.0\"}]",
            "internal.scheduling.koordinator.sh/device-topology": "{\"numaSockets\":[{\"index\":0,\"numaNodes\":[{\"index\":0,\"pcieSwitches\":[{\"index\":0,\"rdmas\":[{\"type\":\"pf_system\",\"minor\":0,\"uVerbs\":\"/dev/infiniband/uverbs0\",\"bondSlaves\":[\"eth0\",\"eth1\"],\"bw\":200}]},{\"index\":1,\"gpus\":[0,1],\"rdmas\":[{\"type\":\"pf_worker\",\"minor\":1,\"uVerbs\":\"/dev/infiniband/uverbs1\",\"bond\":1,\"bondSlaves\":[\"eth2\",\"eth3\"],\"bw\":200}]},{\"index\":2,\"gpus\":[2,3],\"rdmas\":[{\"type\":\"pf_worker\",\"minor\":2,\"uVerbs\":\"/dev/infiniband/uverbs2\",\"bond\":2,\"bondSlaves\":[\"eth4\",\"eth5\"],\"bw\":200}]}]}]},{\"index\":1,\"numaNodes\":[{\"index\":1,\"pcieSwitches\":[{\"index\":3,\"gpus\":[4,5],\"rdmas\":[{\"type\":\"pf_worker\",\"minor\":3,\"uVerbs\":\"/dev/infiniband/uverbs3\",\"bond\":3,\"bondSlaves\":[\"eth6\",\"eth7\"],\"bw\":200}]},{\"index\":4,\"gpus\":[6,7],\"rdmas\":[{\"type\":\"pf_worker\",\"minor\":4,\"uVerbs\":\"/dev/infiniband/uverbs4\",\"bond\":4,\"bondSlaves\":[\"eth8\",\"eth9\"],\"bw\":200}]}]}]}]}",
            "internal.scheduling.koordinator.sh/gpu-topology": "{\"links\":[[{},{\"gpuLinkType\":\"MultipleSwitch\",\"bandwidth\":12},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5}],[{\"gpuLinkType\":\"MultipleSwitch\",\"bandwidth\":12},{},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5}],[{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{},{\"gpuLinkType\":\"MultipleSwitch\",\"bandwidth\":12},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5}],[{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{\"gpuLinkType\":\"MultipleSwitch\",\"bandwidth\":12},{},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5}],[{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{},{\"gpuLinkType\":\"MultipleSwitch\",\"bandwidth\":12},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9}],[{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"MultipleSwitch\",\"bandwidth\":12},{},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9}],[{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{},{\"gpuLinkType\":\"MultipleSwitch\",\"bandwidth\":12}],[{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"CrossCPUSocket\",\"bandwidth\":5},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{\"gpuLinkType\":\"SameCPUSocket\",\"bandwidth\":9},{\"gpuLinkType\":\"MultipleSwitch\",\"bandwidth\":12},{}]]}",
            "internal.scheduling.koordinator.sh/rdma-topology": "{}"
        },
        "name": "lj-m21740n1500e"
    },
    "spec": {
        "devices": [
            {
                "health": true,
                "id": "0000:06:00.0",
                "labels": {
                    "type": "pf_system"
                },
                "minor": 0,
                "resources": {
                    "koordinator.sh/rdma": "100"
                },
                "type": "rdma"
            },
            {
                "health": true,
                "id": "0000:1a:00.0",
                "labels": {
                    "type": "pf_worker"
                },
                "minor": 1,
                "resources": {
                    "koordinator.sh/rdma": "100"
                },
                "type": "rdma"
            },
            {
                "health": true,
                "id": "0000:4f:00.0",
                "labels": {
                    "type": "pf_worker"
                },
                "minor": 2,
                "resources": {
                    "koordinator.sh/rdma": "100"
                },
                "type": "rdma"
            },
            {
                "health": true,
                "id": "0000:90:00.0",
                "labels": {
                    "type": "pf_worker"
                },
                "minor": 3,
                "resources": {
                    "koordinator.sh/rdma": "100"
                },
                "type": "rdma"
            },
            {
                "health": true,
                "id": "0000:b4:00.0",
                "labels": {
                    "type": "pf_worker"
                },
                "minor": 4,
                "resources": {
                    "koordinator.sh/rdma": "100"
                },
                "type": "rdma"
            },
            {
                "health": true,
                "id": "GPU-931323b8-c566-3541-774a-a66bc7c4b5c2",
                "minor": 0,
                "moduleID": 6,
                "resources": {
                    "koordinator.sh/gpu-core": "100",
                    "koordinator.sh/gpu-memory": "85197848576",
                    "koordinator.sh/gpu-memory-ratio": "100"
                },
                "type": "gpu"
            },
            {
                "health": true,
                "id": "GPU-03565ec3-39e2-de12-b936-dd7bfe3977bc",
                "minor": 1,
                "moduleID": 4,
                "resources": {
                    "koordinator.sh/gpu-core": "100",
                    "koordinator.sh/gpu-memory": "85197848576",
                    "koordinator.sh/gpu-memory-ratio": "100"
                },
                "type": "gpu"
            },
            {
                "health": true,
                "id": "GPU-3889acf3-5ad6-db44-0bba-8e1306fd8727",
                "minor": 2,
                "moduleID": 7,
                "resources": {
                    "koordinator.sh/gpu-core": "100",
                    "koordinator.sh/gpu-memory": "85197848576",
                    "koordinator.sh/gpu-memory-ratio": "100"
                },
                "type": "gpu"
            },
            {
                "health": true,
                "id": "GPU-0d271976-4767-7fc5-8b49-05baa8133e76",
                "minor": 3,
                "moduleID": 5,
                "resources": {
                    "koordinator.sh/gpu-core": "100",
                    "koordinator.sh/gpu-memory": "85197848576",
                    "koordinator.sh/gpu-memory-ratio": "100"
                },
                "type": "gpu"
            },
            {
                "health": true,
                "id": "GPU-112d5576-34e6-3f5b-e847-45aa94f60e58",
                "minor": 4,
                "moduleID": 2,
                "resources": {
                    "koordinator.sh/gpu-core": "100",
                    "koordinator.sh/gpu-memory": "85197848576",
                    "koordinator.sh/gpu-memory-ratio": "100"
                },
                "type": "gpu"
            },
            {
                "health": true,
                "id": "GPU-b07261d5-b3f0-7094-14b0-663dc7f8a91d",
                "minor": 5,
                "moduleID": 0,
                "resources": {
                    "koordinator.sh/gpu-core": "100",
                    "koordinator.sh/gpu-memory": "85197848576",
                    "koordinator.sh/gpu-memory-ratio": "100"
                },
                "type": "gpu"
            },
            {
                "health": true,
                "id": "GPU-8bb3d81f-bd2c-5777-c1e6-0b5c156eb795",
                "minor": 6,
                "moduleID": 3,
                "resources": {
                    "koordinator.sh/gpu-core": "100",
                    "koordinator.sh/gpu-memory": "85197848576",
                    "koordinator.sh/gpu-memory-ratio": "100"
                },
                "type": "gpu"
            },
            {
                "health": true,
                "id": "GPU-a28ca268-fa9c-f607-12e7-686625d8ba32",
                "minor": 7,
                "moduleID": 1,
                "resources": {
                    "koordinator.sh/gpu-core": "100",
                    "koordinator.sh/gpu-memory": "85197848576",
                    "koordinator.sh/gpu-memory-ratio": "100"
                },
                "type": "gpu"
            },
            {
                "health": true,
                "id": "0000:c0:00.0",
                "minor": 0,
                "resources": {
                    "koordinator.sh/nvswitch": "100"
                },
                "type": "nvswitch"
            },
            {
                "health": true,
                "id": "0000:c1:00.0",
                "minor": 1,
                "resources": {
                    "koordinator.sh/nvswitch": "100"
                },
                "type": "nvswitch"
            },
            {
                "health": true,
                "id": "0000:c2:00.0",
                "minor": 2,
                "resources": {
                    "koordinator.sh/nvswitch": "100"
                },
                "type": "nvswitch"
            },
            {
                "health": true,
                "id": "0000:c3:00.0",
                "minor": 3,
                "resources": {
                    "koordinator.sh/nvswitch": "100"
                },
                "type": "nvswitch"
            },
            {
                "health": true,
                "id": "0000:c4:00.0",
                "minor": 4,
                "resources": {
                    "koordinator.sh/nvswitch": "100"
                },
                "type": "nvswitch"
            },
            {
                "health": true,
                "id": "0000:c5:00.0",
                "minor": 5,
                "resources": {
                    "koordinator.sh/nvswitch": "100"
                },
                "type": "nvswitch"
            }
        ]
    },
    "status": {}
}`)

func TestDamoDeviceScheduling(t *testing.T) {
	var pod corev1.Pod
	err := json.Unmarshal([]byte(damoPodData), &pod)
	assert.NoError(t, err)
	defer utilfeature.SetFeatureGateDuringTest(t, k8sfeature.DefaultMutableFeatureGate, koordfeatures.EnableDefaultDeviceAllocateHint, true)()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lj-m21740n1500e",
		},
	}
	suit := newPluginTestSuit(t, []*corev1.Node{node})

	var device schedulingv1alpha1.Device
	assert.NoError(t, json.Unmarshal(damoDeviceData, &device))
	_, err = suit.koordClientSet.SchedulingV1alpha1().Devices().Create(context.TODO(), &device, metav1.CreateOptions{})
	assert.NoError(t, err)

	p, err := suit.proxyNew(nil, suit.Framework)
	assert.NoError(t, err)
	pl := p.(*Plugin)
	cycleState := framework.NewCycleState()
	_, status := pl.PreFilter(context.TODO(), cycleState, &pod)
	assert.True(t, status.IsSuccess())

	nodeInfo, err := suit.SnapshotSharedLister().NodeInfos().Get(node.Name)
	assert.NoError(t, err)
	status = pl.Filter(context.TODO(), cycleState, &pod, nodeInfo)
	assert.True(t, status.IsSuccess())
}
