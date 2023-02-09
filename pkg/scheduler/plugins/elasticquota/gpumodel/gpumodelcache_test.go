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

package gpumodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	cosclientset "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned"
	cosfake "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned/fake"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	koordfake "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned/fake"
	koordinatorinformers "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
)

type fakeExtendedHandle struct {
	frameworkext.ExtendedHandle
	cs                    *kubefake.Clientset
	sharedInformerFactory informers.SharedInformerFactory
	cosclientset.Interface
}

func TestGPUModelCache(t *testing.T) {
	koordClientSet := koordfake.NewSimpleClientset()
	koordSharedInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordClientSet, 0)
	extendHandle, _ := frameworkext.NewExtendedHandle(
		frameworkext.WithKoordinatorClientSet(koordClientSet),
		frameworkext.WithKoordinatorSharedInformerFactory(koordSharedInformerFactory),
	)
	fakeHandle := &fakeExtendedHandle{
		ExtendedHandle: extendHandle,
		cs:             kubefake.NewSimpleClientset(),
		Interface:      cosfake.NewSimpleClientset(),
	}
	GlobalGPUModelCache.Init(fakeHandle)
	// node 1: no gpu device
	n1 := newNode("n1", "")
	GlobalGPUModelCache.UpdateByNodeInternal(n1, nil)
	assert.Equal(t, len(GlobalGPUModelCache.ModelToMemCapacity), 0)

	if GlobalGPUModelCache.NodeToGpuModel[n1.Name] != "" {
		t.Error("error")
	}
	if len(GlobalGPUModelCache.NodeToGpuModel) != 0 {
		t.Error("error")
	}

	// node 2: gpu with 2080 with 8Gi
	n2 := newNode("n2", "Tesla-2080Ti")
	d2 := newDevice("n2", "8Gi")
	GlobalGPUModelCache.UpdateByNodeInternal(n2, d2)
	assert.Equal(t, len(GlobalGPUModelCache.ModelToMemCapacity), 1)
	assert.Equal(t, GlobalGPUModelCache.GetModelMemoryCapacity("Tesla-2080Ti"), int64(8*1024*1024*1024))
	assert.Equal(t, GlobalGPUModelCache.GetMaxCapacity(), int64(8*1024*1024*1024))

	if GlobalGPUModelCache.NodeToGpuModel[n2.Name] != "Tesla-2080Ti" {
		t.Error("error")
	}
	if len(GlobalGPUModelCache.NodeToGpuModel) != 1 {
		t.Error("error")
	}

	// node 3: gpu with P100 with 16Gi
	n3 := newNode("n3", "Tesla-P100")
	d3 := newDevice("n3", "16Gi")
	GlobalGPUModelCache.UpdateByNodeInternal(n3, d3)
	assert.Equal(t, len(GlobalGPUModelCache.ModelToMemCapacity), 2)
	assert.Equal(t, GlobalGPUModelCache.GetModelMemoryCapacity("Tesla-P100"), int64(16*1024*1024*1024))
	assert.Equal(t, GlobalGPUModelCache.GetMaxCapacity(), int64(16*1024*1024*1024))

	if GlobalGPUModelCache.NodeToGpuModel[n3.Name] != "Tesla-P100" {
		t.Error("error")
	}
	if len(GlobalGPUModelCache.NodeToGpuModel) != 2 {
		t.Error("error")
	}

	// node 4: gpu with P100 with 16Gi
	n4 := newNode("n4", "Tesla-P100")
	d4 := newDevice("n4", "16Gi")
	GlobalGPUModelCache.UpdateByNodeInternal(n4, d4)
	assert.Equal(t, len(GlobalGPUModelCache.ModelToMemCapacity), 2)
	assert.Equal(t, GlobalGPUModelCache.GetModelMemoryCapacity("Tesla-P100"), int64(16*1024*1024*1024))
	assert.Equal(t, GlobalGPUModelCache.GetMaxCapacity(), int64(16*1024*1024*1024))

	if GlobalGPUModelCache.NodeToGpuModel[n4.Name] != "Tesla-P100" {
		t.Error("error")
	}
	if len(GlobalGPUModelCache.NodeToGpuModel) != 3 {
		t.Error("error")
	}
}

func newNode(name, model string) *v1.Node {
	n := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: v1.NodeStatus{
			Allocatable: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU: resource.MustParse("1"),
			},
		},
	}
	if model != "" {
		n.Labels = map[string]string{
			extension.GPUModel: model,
		}
		n.Status.Allocatable[extension.NvidiaGPU] = resource.MustParse("1")
	}
	return n
}

func newDevice(name, capacity string) *v1alpha1.Device {
	return &v1alpha1.Device{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
		},
		Spec: v1alpha1.DeviceSpec{
			Devices: []v1alpha1.DeviceInfo{
				{
					UUID:   name,
					Minor:  pointer.Int32Ptr(1),
					Health: true,
					Type:   v1alpha1.GPU,
					Resources: v1.ResourceList{
						extension.GPUMemory: resource.MustParse(capacity),
					},
				},
			},
		},
	}
}
