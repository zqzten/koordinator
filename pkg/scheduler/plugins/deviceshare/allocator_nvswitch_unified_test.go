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

func allocateWithNVSwitchHint(allocateStrategy apiext.DeviceAllocateStrategy) func(hints apiext.DeviceAllocateHints) {
	return func(hints apiext.DeviceAllocateHints) {
		nvSwitchHint := hints[unified.NVSwitchDeviceType]
		if nvSwitchHint == nil {
			nvSwitchHint = &apiext.DeviceHint{}
			hints[unified.NVSwitchDeviceType] = nvSwitchHint
		}
		nvSwitchHint.AllocateStrategy = allocateStrategy
	}
}

func TestAutopilotAllocateNVSwitch(t *testing.T) {
	tests := []struct {
		name            string
		deviceCR        *schedulingv1alpha1.Device
		gpuWanted       int
		applyForAll     bool
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
						Extension: &apiext.DeviceAllocationExtension{
							VirtualFunctions: []apiext.VirtualFunction{
								{
									BusID: "0000:1f:00.2",
									Minor: 0,
								},
							},
						},
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
						Extension: &apiext.DeviceAllocationExtension{
							VirtualFunctions: []apiext.VirtualFunction{
								{
									BusID: "0000:1f:00.2",
									Minor: 0,
								},
							},
						},
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
						Extension: &apiext.DeviceAllocationExtension{
							VirtualFunctions: []apiext.VirtualFunction{
								{
									BusID: "0000:1f:00.2",
									Minor: 0,
								},
							},
						},
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: &apiext.DeviceAllocationExtension{
							VirtualFunctions: []apiext.VirtualFunction{
								{
									BusID: "0000:90:00.2",
									Minor: 0,
								},
							},
						},
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
						Extension: &apiext.DeviceAllocationExtension{
							VirtualFunctions: []apiext.VirtualFunction{
								{
									BusID: "0000:1f:00.2",
									Minor: 0,
								},
							},
						},
					},
					{
						Minor: 2,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: &apiext.DeviceAllocationExtension{
							VirtualFunctions: []apiext.VirtualFunction{
								{
									BusID: "0000:90:00.2",
									Minor: 0,
								},
							},
						},
					},
					{
						Minor: 3,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: &apiext.DeviceAllocationExtension{
							VirtualFunctions: []apiext.VirtualFunction{
								{
									BusID: "0000:51:00.2",
									Minor: 0,
								},
							},
						},
					},
					{
						Minor: 4,
						Resources: corev1.ResourceList{
							apiext.ResourceRDMA: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Extension: &apiext.DeviceAllocationExtension{
							VirtualFunctions: []apiext.VirtualFunction{
								{
									BusID: "0000:b9:00.2",
									Minor: 0,
								},
							},
						},
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
						Extension: &apiext.DeviceAllocationExtension{
							VirtualFunctions: []apiext.VirtualFunction{
								{
									BusID: "0000:1f:00.2",
									Minor: 0,
								},
							},
						},
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
						Extension: &apiext.DeviceAllocationExtension{
							VirtualFunctions: []apiext.VirtualFunction{
								{
									BusID: "0000:90:00.2",
									Minor: 0,
								},
							},
						},
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
		{
			name:        "apply for all NVSWitch",
			deviceCR:    fakeDeviceCR,
			gpuWanted:   0,
			applyForAll: true,
			want: apiext.DeviceAllocations{
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

			sharedInformerFactory.Start(nil)
			sharedInformerFactory.WaitForCacheSync(nil)

			nodeDevice := deviceCache.getNodeDevice("test-node-1", false)
			assert.NotNil(t, nodeDevice)

			pod := &corev1.Pod{}

			podRequest := corev1.ResourceList{}
			if tt.gpuWanted > 0 {
				podRequest[apiext.ResourceNvidiaGPU] = *resource.NewQuantity(int64(tt.gpuWanted), resource.DecimalSI)
				combination, err := ValidateDeviceRequest(podRequest)
				assert.NoError(t, err)
				podRequest = ConvertDeviceRequest(podRequest, combination)
				podRequest[apiext.ResourceRDMA] = *resource.NewQuantity(1, resource.DecimalSI)
				setDefaultTestAllocateHints(t, pod, allocateHintWithVFType("fakeG"))
				setDefaultTestDeviceJointAllocate(t, pod)
			} else if tt.applyForAll {
				setDefaultTestAllocateHints(t, pod, allocateWithNVSwitchHint(apiext.ApplyForAllDeviceAllocateStrategy))
			} else {
				setDefaultTestAllocateHints(t, pod, allocateHintWithVFType("fakeC"))
			}

			podRequest[unified.NVSwitchResource] = *resource.NewQuantity(100, resource.DecimalSI)
			pod.Spec.Containers = []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: podRequest,
					},
				},
			}

			state, status := preparePod(pod)
			assert.True(t, status.IsSuccess())

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
			}

			nodeDevice.lock.Lock()
			defer nodeDevice.lock.Unlock()

			allocator := &AutopilotAllocator{
				state:      state,
				nodeDevice: nodeDevice,
				node:       node,
				pod:        pod,
			}

			allocations, status := allocator.Allocate(nil, nil, nil, nil)
			if !status.IsSuccess() != tt.wantErr {
				t.Errorf("Allocate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			sortDeviceAllocations(allocations)
			sortDeviceAllocations(tt.want)
			assert.Equal(t, tt.want, allocations)
		})
	}
}
