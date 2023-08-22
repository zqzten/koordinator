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
	"time"

	"github.com/stretchr/testify/assert"
	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	cosclientset "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned"
	cosfake "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	schedconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"
	"k8s.io/utils/pointer"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	koordclientset "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned"
	koordfake "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned/fake"
	koordinatorinformers "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions"
	koordfeatures "github.com/koordinator-sh/koordinator/pkg/features"
	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/defaultprebind"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/deviceshare"
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

var _ framework.SharedLister = &testSharedLister{}

type testSharedLister struct {
	nodes       []*corev1.Node
	nodeInfos   []*framework.NodeInfo
	nodeInfoMap map[string]*framework.NodeInfo
}

func newTestSharedLister(pods []*corev1.Pod, nodes []*corev1.Node) *testSharedLister {
	nodeInfoMap := make(map[string]*framework.NodeInfo)
	nodeInfos := make([]*framework.NodeInfo, 0)
	for _, pod := range pods {
		nodeName := pod.Spec.NodeName
		if _, ok := nodeInfoMap[nodeName]; !ok {
			nodeInfoMap[nodeName] = framework.NewNodeInfo()
		}
		nodeInfoMap[nodeName].AddPod(pod)
	}
	for _, node := range nodes {
		if _, ok := nodeInfoMap[node.Name]; !ok {
			nodeInfoMap[node.Name] = framework.NewNodeInfo()
		}
		nodeInfoMap[node.Name].SetNode(node)
	}

	for _, v := range nodeInfoMap {
		nodeInfos = append(nodeInfos, v)
	}

	return &testSharedLister{
		nodes:       nodes,
		nodeInfos:   nodeInfos,
		nodeInfoMap: nodeInfoMap,
	}
}

func (f *testSharedLister) NodeInfos() framework.NodeInfoLister {
	return f
}

func (f *testSharedLister) List() ([]*framework.NodeInfo, error) {
	return f.nodeInfos, nil
}

func (f *testSharedLister) HavePodsWithAffinityList() ([]*framework.NodeInfo, error) {
	return nil, nil
}

func (f *testSharedLister) HavePodsWithRequiredAntiAffinityList() ([]*framework.NodeInfo, error) {
	return nil, nil
}

func (f *testSharedLister) Get(nodeName string) (*framework.NodeInfo, error) {
	return f.nodeInfoMap[nodeName], nil
}

type pluginTestSuit struct {
	framework.Framework
	koordClientSet                   koordclientset.Interface
	ExtendedHandle                   frameworkext.ExtendedHandle
	ExtenderFactory                  *frameworkext.FrameworkExtenderFactory
	koordinatorSharedInformerFactory koordinatorinformers.SharedInformerFactory
	proxyNew                         frameworkruntime.PluginFactory
}

func newPluginTestSuit(t *testing.T, nodes []*corev1.Node) *pluginTestSuit {
	koordClientSet := koordfake.NewSimpleClientset()
	koordSharedInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordClientSet, 0)
	extenderFactory, _ := frameworkext.NewFrameworkExtenderFactory(
		frameworkext.WithKoordinatorClientSet(koordClientSet),
		frameworkext.WithKoordinatorSharedInformerFactory(koordSharedInformerFactory),
	)
	proxyNew := frameworkext.PluginFactoryProxy(extenderFactory, New)

	registeredPlugins := []schedulertesting.RegisterPluginFunc{
		schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
		schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
	}

	cs := kubefake.NewSimpleClientset()
	for _, v := range nodes {
		_, err := cs.CoreV1().Nodes().Create(context.TODO(), v, metav1.CreateOptions{})
		assert.NoError(t, err)
	}
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newTestSharedLister(nil, nodes)

	fh, err := schedulertesting.NewFramework(
		registeredPlugins,
		"koord-scheduler",
		frameworkruntime.WithClientSet(cs),
		frameworkruntime.WithInformerFactory(informerFactory),
		frameworkruntime.WithSnapshotSharedLister(snapshot),
	)
	assert.Nil(t, err)
	return &pluginTestSuit{
		Framework:                        fh,
		koordClientSet:                   koordClientSet,
		ExtenderFactory:                  extenderFactory,
		koordinatorSharedInformerFactory: koordSharedInformerFactory,
		proxyNew:                         proxyNew,
	}
}

func Test_New(t *testing.T) {
	koordClientSet := koordfake.NewSimpleClientset()
	koordSharedInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordClientSet, 0)
	extenderFactory, _ := frameworkext.NewFrameworkExtenderFactory(
		frameworkext.WithKoordinatorClientSet(koordClientSet),
		frameworkext.WithKoordinatorSharedInformerFactory(koordSharedInformerFactory),
	)
	proxyNew := frameworkext.PluginFactoryProxy(extenderFactory, New)

	registeredPlugins := []schedulertesting.RegisterPluginFunc{
		schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
		schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
	}

	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newTestSharedLister(nil, nil)
	fh, err := schedulertesting.NewFramework(
		registeredPlugins,
		"koord-scheduler",
		frameworkruntime.WithClientSet(cs),
		frameworkruntime.WithInformerFactory(informerFactory),
		frameworkruntime.WithSnapshotSharedLister(snapshot),
	)
	assert.Nil(t, err)
	args := &apiruntime.Unknown{
		ContentType: apiruntime.ContentTypeJSON,
		Raw:         []byte(`{"apiVersion":"kubescheduler.config.k8s.io/v1beta2", "allocator": "default","scoringStrategy":{"type":"LeastAllocated","resources":[{"Name":"koordinator.sh/gpu-memory-ratio", "Weight":1}]}}`),
	}

	p, err := proxyNew(args, fh)
	assert.NotNil(t, p)
	assert.Nil(t, err)
	assert.Equal(t, Name, p.Name())
}

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
			suit := newPluginTestSuit(t, []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node-1",
					},
				},
			})
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

			pl, err := suit.proxyNew(nil, suit.Framework)
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
	deviceshare.EnableUnifiedDevice = true
	defer func() {
		deviceshare.EnableUnifiedDevice = false
	}()
	defer featuregatetesting.SetFeatureGateDuringTest(t, k8sfeature.DefaultFeatureGate, koordfeatures.UnifiedDeviceScheduling, true)()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}
	suit := newPluginTestSuit(t, []*corev1.Node{node})

	fakeDevice := fakeDeviceCR.DeepCopy()
	fakeDevice.Name = node.Name
	_, err := suit.ExtenderFactory.KoordinatorClientSet().SchedulingV1alpha1().Devices().Create(context.TODO(), fakeDevice, metav1.CreateOptions{})
	assert.NoError(t, err)

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
	_, err = suit.ClientSet().CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	assert.NoError(t, err)
	extender := suit.ExtenderFactory.NewFrameworkExtender(suit.Framework)
	fakeHandle := &fakeExtendedHandle{
		ExtendedHandle: extender,
		Interface:      cosfake.NewSimpleClientset(),
	}
	p, err := New(nil, fakeHandle)
	assert.NoError(t, err)

	suit.Framework.SharedInformerFactory().Start(nil)
	suit.koordinatorSharedInformerFactory.Start(nil)
	suit.Framework.SharedInformerFactory().WaitForCacheSync(nil)
	suit.koordinatorSharedInformerFactory.WaitForCacheSync(nil)

	cycleState := framework.NewCycleState()
	pl := p.(*Plugin)
	_, status := pl.PreFilter(context.TODO(), cycleState, pod)
	assert.True(t, status.IsSuccess())

	pod.Spec.NodeName = "test-node"
	status = pl.Reserve(context.TODO(), cycleState, pod, "test-node")
	assert.True(t, status.IsSuccess())

	originalPod := pod.DeepCopy()
	status = pl.PreBind(context.TODO(), cycleState, pod, "test-node")
	assert.True(t, status.IsSuccess())

	prebindPlugin, err := defaultprebind.New(nil, fakeHandle)
	assert.NoError(t, err)
	status = prebindPlugin.(frameworkext.PreBindExtensions).ApplyPatch(context.TODO(), cycleState, originalPod, pod)
	assert.True(t, status.IsSuccess())

	patchedPod, err := suit.ClientSet().CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	expectedPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Annotations: map[string]string{
				unifiedresourceext.AnnotationMultiDeviceAllocStatus: `{"allocStatus":{"gpu-device":[{"deviceAllocStatus":{"allocs":[{"minor":0,"resources":{"alibabacloud.com/gpu-core":"100","alibabacloud.com/gpu-mem":"83201216Ki","alibabacloud.com/gpu-mem-ratio":"100"}},{"minor":1,"resources":{"alibabacloud.com/gpu-core":"100","alibabacloud.com/gpu-mem":"83201216Ki","alibabacloud.com/gpu-mem-ratio":"100"}}]}}]}}`,
				unifiedresourceext.AnnotationNVIDIAVisibleDevices:   "0,1",
				apiext.AnnotationDeviceAllocated:                    `{"gpu":[{"minor":0,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"83201216Ki","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"83201216Ki","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
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

func Test_getDeviceShareArgs(t *testing.T) {
	defaultArgs, err := getDefaultDeviceShareArgs()
	assert.NoError(t, err)
	tests := []struct {
		name    string
		obj     runtime.Object
		want    *schedulingconfig.DeviceShareArgs
		wantErr bool
	}{
		{
			name:    "no args",
			obj:     nil,
			want:    defaultArgs,
			wantErr: false,
		},
		{
			name: "configured args",
			obj: &runtime.Unknown{
				Raw:         []byte(`{"allocator": "autopilotAllocator", "apiVersion": "kubescheduler.config.k8s.io/v1beta2"}`),
				ContentType: runtime.ContentTypeJSON,
			},
			want: &schedulingconfig.DeviceShareArgs{
				Allocator: "autopilotAllocator",
				ScoringStrategy: &schedulingconfig.ScoringStrategy{
					Type: schedulingconfig.LeastAllocated,
					Resources: []schedconfig.ResourceSpec{
						{
							Name:   string(apiext.ResourceGPUMemoryRatio),
							Weight: 1,
						},
						{
							Name:   string(apiext.ResourceRDMA),
							Weight: 1,
						},
						{
							Name:   string(apiext.ResourceFPGA),
							Weight: 1,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid configured args",
			obj: &runtime.Unknown{
				Raw:         []byte(`{"allocator": `),
				ContentType: runtime.ContentTypeJSON,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid configured args type",
			obj:     defaultArgs,
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := getDeviceShareArgs(tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("getDeviceShareArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, args)
		})
	}
}

func TestMatchDriverVersions(t *testing.T) {
	tests := []struct {
		name           string
		runc           bool
		selector       *unified.GPUSelector
		driverVersions unified.NVIDIADriverVersions
		wantErr        bool
	}{
		{
			name:           "no selector and have driver version",
			driverVersions: unified.NVIDIADriverVersions{"2.2.2", "3.3.3"},
			wantErr:        false,
		},
		{
			name:    "no selector, runc and nodes no driver versions",
			runc:    true,
			wantErr: false,
		},
		{
			name:    "no selector, rund and nodes no driver versions",
			wantErr: true,
		},
		{
			name: "selector and matched",
			selector: &unified.GPUSelector{
				DriverVersions: []string{"1.1.1", "2.2.2"},
			},
			driverVersions: unified.NVIDIADriverVersions{"2.2.2", "3.3.3"},
		},
		{
			name: "selector and unmatched",
			selector: &unified.GPUSelector{
				DriverVersions: []string{"1.1.1", "2.2.2"},
			},
			driverVersions: unified.NVIDIADriverVersions{"3.3.3"},
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeDevice := fakeDeviceCR.DeepCopy()
			if tt.driverVersions != nil {
				data, err := json.Marshal(tt.driverVersions)
				assert.NoError(t, err)
				if fakeDevice.Annotations == nil {
					fakeDevice.Annotations = map[string]string{}
				}
				fakeDevice.Annotations[unified.AnnotationNVIDIADriverVersions] = string(data)
			}

			suit := newPluginTestSuit(t, []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node-1",
					},
				},
			})

			_, err := suit.koordClientSet.SchedulingV1alpha1().Devices().Create(context.TODO(), fakeDevice, metav1.CreateOptions{})
			assert.NoError(t, err)

			extender := suit.ExtenderFactory.NewFrameworkExtender(suit.Framework)
			fakeHandle := &fakeExtendedHandle{
				ExtendedHandle: extender,
				Interface:      cosfake.NewSimpleClientset(),
			}
			p, err := New(nil, fakeHandle)
			assert.NoError(t, err)

			suit.Framework.SharedInformerFactory().Start(nil)
			suit.koordinatorSharedInformerFactory.Start(nil)
			suit.Framework.SharedInformerFactory().WaitForCacheSync(nil)
			suit.koordinatorSharedInformerFactory.WaitForCacheSync(nil)

			podRequest := corev1.ResourceList{
				apiext.ResourceNvidiaGPU: *resource.NewQuantity(int64(1), resource.DecimalSI),
			}

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
								Requests: podRequest,
							},
						},
					},
				},
			}
			if tt.selector != nil {
				data, err := json.Marshal(tt.selector)
				assert.NoError(t, err)
				pod.Annotations = map[string]string{
					unified.AnnotationNVIDIAGPUSelector: string(data),
				}
			}
			if !tt.runc {
				pod.Spec.RuntimeClassName = pointer.String("rund")
			}

			cycleState := framework.NewCycleState()
			pl := p.(*Plugin)
			_, status := pl.PreFilter(context.TODO(), cycleState, pod)
			assert.True(t, status.IsSuccess())

			nodeInfo, err := suit.Framework.SnapshotSharedLister().NodeInfos().Get("test-node-1")
			assert.NoError(t, err)
			assert.NotNil(t, nodeInfo)

			status = pl.Filter(context.TODO(), cycleState, pod, nodeInfo)
			if !status.IsSuccess() != tt.wantErr {
				t.Errorf("Filter error = %v, wantErr %v", status, tt.wantErr)
				return
			}
		})
	}
}

func TestPreBindWithACKGPUMemory(t *testing.T) {
	now := time.Now()
	originalNowFn := ack.NowFn
	ack.NowFn = func() time.Time {
		return now
	}
	defer func() {
		ack.NowFn = originalNowFn
	}()

	defer featuregatetesting.SetFeatureGateDuringTest(t, k8sfeature.DefaultFeatureGate, koordfeatures.EnableACKGPUMemoryScheduling, true)()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}
	suit := newPluginTestSuit(t, []*corev1.Node{node})

	fakeDevice := fakeDeviceCR.DeepCopy()
	fakeDevice.Name = node.Name
	_, err := suit.ExtenderFactory.KoordinatorClientSet().SchedulingV1alpha1().Devices().Create(context.TODO(), fakeDevice, metav1.CreateOptions{})
	assert.NoError(t, err)

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
							ack.ResourceAliyunGPUMemory: resource.MustParse("8"),
						},
						Requests: corev1.ResourceList{
							ack.ResourceAliyunGPUMemory: resource.MustParse("8"),
						},
					},
				},
			},
		},
	}
	_, err = suit.ClientSet().CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	assert.NoError(t, err)
	extender := suit.ExtenderFactory.NewFrameworkExtender(suit.Framework)
	fakeHandle := &fakeExtendedHandle{
		ExtendedHandle: extender,
		Interface:      cosfake.NewSimpleClientset(),
	}
	p, err := New(nil, fakeHandle)
	assert.NoError(t, err)

	suit.Framework.SharedInformerFactory().Start(nil)
	suit.koordinatorSharedInformerFactory.Start(nil)
	suit.Framework.SharedInformerFactory().WaitForCacheSync(nil)
	suit.koordinatorSharedInformerFactory.WaitForCacheSync(nil)

	cycleState := framework.NewCycleState()
	pl := p.(*Plugin)
	_, status := pl.PreFilter(context.TODO(), cycleState, pod)
	assert.True(t, status.IsSuccess())

	pod.Spec.NodeName = "test-node"
	status = pl.Reserve(context.TODO(), cycleState, pod, "test-node")
	assert.True(t, status.IsSuccess())

	originalPod := pod.DeepCopy()
	status = pl.PreBind(context.TODO(), cycleState, pod, "test-node")
	assert.True(t, status.IsSuccess())

	prebindPlugin, err := defaultprebind.New(nil, fakeHandle)
	assert.NoError(t, err)
	status = prebindPlugin.(frameworkext.PreBindExtensions).ApplyPatch(context.TODO(), cycleState, originalPod, pod)
	assert.True(t, status.IsSuccess())

	patchedPod, err := suit.ClientSet().CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	expectedPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Annotations: map[string]string{
				ack.AnnotationACKGPUShareAllocation: `{"0":{"0":8}}`,
				ack.AnnotationACKGPUShareAssigned:   "true",
				ack.AnnotationACKGPUShareAssumeTime: fmt.Sprintf("%d", now.UnixNano()),
				apiext.AnnotationDeviceAllocated:    `{"gpu":[{"minor":0,"resources":{"koordinator.sh/gpu-memory":"8Gi","koordinator.sh/gpu-memory-ratio":"10"}}]}`,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							ack.ResourceAliyunGPUMemory: resource.MustParse("8"),
						},
						Requests: corev1.ResourceList{
							ack.ResourceAliyunGPUMemory: resource.MustParse("8"),
						},
					},
				},
			},
		},
	}
	assert.Equal(t, expectedPod, patchedPod)
}
