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
	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	cosclientset "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned"
	cosfake "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/utils/pointer"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	koordfeatures "github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
)

type fakeExtendedHandle struct {
	frameworkext.ExtendedHandle
	snapShotSharedLister framework.SharedLister
	cosclientset.Interface
}

func (f *fakeExtendedHandle) SnapshotSharedLister() framework.SharedLister {
	if f.snapShotSharedLister != nil {
		return f.snapShotSharedLister
	}
	return f.ExtendedHandle.SnapshotSharedLister()
}

func Test_appendNetworkingVFMetas(t *testing.T) {
	pod := &corev1.Pod{}

	allocations := apiext.DeviceAllocations{
		schedulingv1alpha1.RDMA: []*apiext.DeviceAllocation{
			{
				Minor: 1,
				Extension: json.RawMessage(`{
					"vfs": [
                    	{
                        	"bondName": "bond1",
                        	"busID": "0000:5f:00.2",
                        	"minor": 0,
                        	"priority": "VFPriorityLow"
                    	}
                	],
					"bondSlaves": ["eth0", "eth1"]
				}`),
			},
			{
				Minor: 2,
				Extension: json.RawMessage(`{
					"vfs": [
                    	{
                        	"bondName": "bond2",
                        	"busID": "0000:6f:00.1",
                        	"minor": 1,
                        	"priority": "VFPriorityLow"
                    	}
                	],
					"bondSlaves": ["eth2", "eth3"]
				}`),
			},
		},
	}

	err := appendNetworkingVFMetas(pod, allocations)
	assert.NoError(t, err)

	var gotMetas []unified.VFMeta
	err = json.Unmarshal([]byte(pod.Annotations[unified.AnnotationNetworkingVFMeta]), &gotMetas)
	assert.NoError(t, err)

	expectedMetas := []unified.VFMeta{
		{
			BondName:   "bond1",
			VFIndex:    0,
			PCIAddress: "0000:5f:00.2",
			BondSlaves: []string{"eth0", "eth1"},
		},
		{
			BondName:   "bond2",
			VFIndex:    1,
			PCIAddress: "0000:6f:00.1",
			BondSlaves: []string{"eth2", "eth3"},
		},
	}
	assert.Equal(t, expectedMetas, gotMetas)
}

func Test_appendRundResult(t *testing.T) {
	tests := []struct {
		name        string
		pod         *corev1.Pod
		allocResult apiext.DeviceAllocations
		want        *corev1.Pod
		wantErr     bool
	}{
		{
			name: "runc pod",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					RuntimeClassName: nil,
					NodeName:         "test-node-1",
				},
			},
			allocResult: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor: 1, Resources: corev1.ResourceList{apiext.ResourceGPU: resource.MustParse("100")},
					},
				},
			},
			want: &corev1.Pod{
				Spec: corev1.PodSpec{
					RuntimeClassName: nil,
					NodeName:         "test-node-1",
				},
			},
		},
		{
			name: "rund pod",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					RuntimeClassName: pointer.String("rund"),
					NodeName:         "test-node-1",
				},
			},
			allocResult: apiext.DeviceAllocations{
				schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
					{
						Minor: 1,
					},
					{
						Minor: 2,
					},
				},
				unified.NVSwitchDeviceType: []*apiext.DeviceAllocation{
					{
						Minor: 3,
					},
					{
						Minor: 4,
					},
				},
			},
			want: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						unified.AnnotationRundPassthoughPCI:       "0000:80:00.1,0000:80:00.2,0000:90:00.3,0000:90:00.4",
						unified.AnnotationRundNVSwitchOrder:       "3,4",
						unified.AnnotationRundNvidiaDriverVersion: "2.2.2",
					},
				},
				Spec: corev1.PodSpec{
					RuntimeClassName: pointer.String("rund"),
					NodeName:         "test-node-1",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nil)

			fakeDevice := fakeDeviceCR.DeepCopy()
			var pciInfos []unified.DevicePCIInfo
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

			for _, v := range fakeDevice.Spec.Devices {
				busIDBase := "0000:80:00"
				if v.Type == unified.NVSwitchDeviceType {
					busIDBase = "0000:90:00"
				}
				pciInfos = append(pciInfos, unified.DevicePCIInfo{
					Type:  v.Type,
					Minor: *v.Minor,
					BusID: fmt.Sprintf("%s.%d", busIDBase, *v.Minor),
				})
			}

			if fakeDevice.Annotations == nil {
				fakeDevice.Annotations = map[string]string{}
			}
			fakeDevice.Annotations[unified.AnnotationNVIDIADriverVersions] = `["2.2.2", "3.3.3"]`
			data, err := json.Marshal(pciInfos)
			assert.NoError(t, err)
			fakeDevice.Annotations[unified.AnnotationDevicePCIInfos] = string(data)

			_, err = suit.ExtenderFactory.KoordinatorClientSet().SchedulingV1alpha1().Devices().Create(context.TODO(), fakeDevice, metav1.CreateOptions{})
			assert.NoError(t, err)

			pl, err := suit.proxyNew(&config.DeviceShareArgs{}, suit.Framework)
			assert.NoError(t, err)

			suit.SharedInformerFactory().Start(nil)
			suit.koordinatorSharedInformerFactory.Start(nil)
			suit.SharedInformerFactory().WaitForCacheSync(nil)
			suit.koordinatorSharedInformerFactory.WaitForCacheSync(nil)

			plugin := pl.(*Plugin)
			err = appendRundResult(tt.pod, tt.allocResult, plugin)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v but got %v", tt.wantErr, err)
			}

			assert.Equal(t, tt.want, tt.pod)
		})
	}
}

func Test_addContainerGPUResourceForPatch(t *testing.T) {
	tests := []struct {
		name        string
		container   *corev1.Container
		memoryRatio int64
		expected    bool
	}{
		{
			name:        "patch memory ratio",
			container:   &corev1.Container{},
			memoryRatio: 100,
			expected:    true,
		},
		{
			name: "patch memory ratio with exist ratio/1",
			container: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
					},
					Requests: corev1.ResourceList{
						unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
					},
				},
			},
			memoryRatio: 100,
			expected:    false,
		},
		{
			name: "patch memory ratio with exist ratio/2",
			container: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						unifiedresourceext.GPUResourceMemRatio: *resource.NewQuantity(100, resource.DecimalSI),
					},
					Requests: corev1.ResourceList{
						unifiedresourceext.GPUResourceMemRatio: *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
			memoryRatio: 100,
			expected:    false,
		},
		{
			name: "patch memory ratio with diff ratio",
			container: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						unifiedresourceext.GPUResourceMemRatio: *resource.NewQuantity(50, resource.DecimalSI),
					},
					Requests: corev1.ResourceList{
						unifiedresourceext.GPUResourceMemRatio: *resource.NewQuantity(50, resource.DecimalSI),
					},
				},
			},
			memoryRatio: 100,
			expected:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patched := addContainerGPUResourceForPatch(tt.container, unifiedresourceext.GPUResourceMemRatio, tt.memoryRatio)
			assert.Equal(t, tt.expected, patched)
		})
	}
}

func TestPreBindUnifiedDevice(t *testing.T) {
	enableUnifiedDevice = true
	defer func() {
		enableUnifiedDevice = false
	}()
	defer featuregatetesting.SetFeatureGateDuringTest(t, k8sfeature.DefaultFeatureGate, koordfeatures.UnifiedDeviceScheduling, true)()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}
	suit := newPluginTestSuit(t, []*corev1.Node{node})
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:       resource.MustParse("4"),
							corev1.ResourceMemory:    resource.MustParse("8Gi"),
							apiext.ResourceNvidiaGPU: resource.MustParse("2"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:       resource.MustParse("4"),
							corev1.ResourceMemory:    resource.MustParse("8Gi"),
							apiext.ResourceNvidiaGPU: resource.MustParse("2"),
						},
					},
				},
			},
		},
	}
	_, err := suit.ClientSet().CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	assert.NoError(t, err)
	extender := suit.ExtenderFactory.NewFrameworkExtender(suit.Framework)
	fakeHandle := &fakeExtendedHandle{
		ExtendedHandle: extender,
		Interface:      cosfake.NewSimpleClientset(),
	}
	p, err := New(&config.DeviceShareArgs{}, fakeHandle)
	assert.NoError(t, err)

	suit.Framework.SharedInformerFactory().Start(nil)
	suit.koordinatorSharedInformerFactory.Start(nil)
	suit.Framework.SharedInformerFactory().WaitForCacheSync(nil)
	suit.koordinatorSharedInformerFactory.WaitForCacheSync(nil)

	cycleState := framework.NewCycleState()
	pl := p.(*Plugin)
	status := pl.PreFilter(context.TODO(), cycleState, pod)
	assert.True(t, status.IsSuccess())
	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	resources := corev1.ResourceList{
		apiext.ResourceGPUCore:        resource.MustParse("100"),
		apiext.ResourceGPUMemoryRatio: resource.MustParse("100"),
		apiext.ResourceGPUMemory:      resource.MustParse("8Gi"),
	}
	state.allocationResult = apiext.DeviceAllocations{
		schedulingv1alpha1.GPU: []*apiext.DeviceAllocation{
			{
				Minor:     0,
				Resources: resources,
			},
			{
				Minor:     1,
				Resources: resources,
			},
		},
	}
	assert.True(t, status.IsSuccess())
	status = pl.PreBind(context.TODO(), cycleState, pod, "test-node")
	assert.True(t, status.IsSuccess())
	patchedPod, err := suit.ClientSet().CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	expectedPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Annotations: map[string]string{
				unifiedresourceext.AnnotationMultiDeviceAllocStatus: `{"allocStatus":{"gpu-device":[{"deviceAllocStatus":{"allocs":[{"minor":0,"resources":{"alibabacloud.com/gpu-core":"100","alibabacloud.com/gpu-mem":"8Gi","alibabacloud.com/gpu-mem-ratio":"100"}},{"minor":1,"resources":{"alibabacloud.com/gpu-core":"100","alibabacloud.com/gpu-mem":"8Gi","alibabacloud.com/gpu-mem-ratio":"100"}}]}}]}}`,
				unifiedresourceext.AnnotationNVIDIAVisibleDevices:   "0,1",
				apiext.AnnotationDeviceAllocated:                    `{"gpu":[{"minor":0,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"8Gi","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"8Gi","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:                     resource.MustParse("4"),
							corev1.ResourceMemory:                  resource.MustParse("8Gi"),
							apiext.ResourceNvidiaGPU:               resource.MustParse("2"),
							unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:                     resource.MustParse("4"),
							corev1.ResourceMemory:                  resource.MustParse("8Gi"),
							apiext.ResourceNvidiaGPU:               resource.MustParse("2"),
							unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  unified.EnvActivelyAddedUnifiedGPUMemoryRatio,
							Value: "true",
						},
						{
							Name:  "NVIDIA_VISIBLE_DEVICES",
							Value: "0,1",
						},
					},
				},
			},
		},
	}
	assert.Equal(t, expectedPod, patchedPod)
}
