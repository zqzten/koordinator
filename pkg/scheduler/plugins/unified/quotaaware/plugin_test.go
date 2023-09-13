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

package quotaaware

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	schedclientset "sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned"
	schedfake "sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned/fake"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	koordfake "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned/fake"
	koordinatorinformers "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
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

type fakeFrameworkHandle struct {
	frameworkext.ExtendedHandle
	schedclientset.Interface
}

type pluginTestSuit struct {
	framework.Framework
	schedclient schedclientset.Interface
	proxyNew    runtime.PluginFactory
}

func newPluginTestSuit(t *testing.T, nodes []*corev1.Node) *pluginTestSuit {
	koordClientSet := koordfake.NewSimpleClientset()
	koordSharedInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordClientSet, 0)
	extenderFactory, _ := frameworkext.NewFrameworkExtenderFactory(
		frameworkext.WithKoordinatorClientSet(koordClientSet),
		frameworkext.WithKoordinatorSharedInformerFactory(koordSharedInformerFactory),
	)
	schedclient := schedfake.NewSimpleClientset()

	proxyNew := frameworkext.PluginFactoryProxy(extenderFactory, func(configuration apiruntime.Object, f framework.Handle) (framework.Plugin, error) {
		handle := &fakeFrameworkHandle{
			ExtendedHandle: f.(frameworkext.FrameworkExtender),
			Interface:      schedclient,
		}
		return New(configuration, handle)
	})

	registeredPlugins := []schedulertesting.RegisterPluginFunc{
		schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
		schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
	}

	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newTestSharedLister(nil, nodes)

	fh, err := schedulertesting.NewFramework(
		registeredPlugins,
		"koord-scheduler",
		runtime.WithClientSet(cs),
		runtime.WithInformerFactory(informerFactory),
		runtime.WithSnapshotSharedLister(snapshot),
	)
	assert.Nil(t, err)
	return &pluginTestSuit{
		Framework:   fh,
		schedclient: schedclient,
		proxyNew:    proxyNew,
	}
}

func Test_PreFilterWithMinFirst(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-2",
					corev1.LabelArchStable:   "amd64",
				},
			},
		},
	}
	suit := newPluginTestSuit(t, nodes)

	quotas := []*schedv1alpha1.ElasticQuota{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "quota-a",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
					apiext.LabelQuotaParent:  "second-root-quota-a",
					LabelQuotaID:             "666",
					LabelUserAccountId:       "123",
					LabelInstanceType:        "aaa",
				},
			},
			Spec: schedv1alpha1.ElasticQuotaSpec{
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("40"),
					corev1.ResourceMemory: resource.MustParse("80Gi"),
				},
				Min: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "quota-b",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-2",
					corev1.LabelArchStable:   "amd64",
					apiext.LabelQuotaParent:  "second-root-quota-a",
					LabelQuotaID:             "666",
					LabelUserAccountId:       "123",
					LabelInstanceType:        "aaa",
				},
			},
			Spec: schedv1alpha1.ElasticQuotaSpec{
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("40"),
					corev1.ResourceMemory: resource.MustParse("80Gi"),
				},
				Min: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
	}
	for _, v := range quotas {
		_, err := suit.schedclient.SchedulingV1alpha1().ElasticQuotas("default").Create(context.TODO(), v, metav1.CreateOptions{})
		assert.NoError(t, err)
	}
	p, err := suit.proxyNew(nil, suit.Framework)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	pl := p.(*Plugin)
	cycleState := framework.NewCycleState()
	pod := st.MakePod().Name("test").Label(LabelQuotaID, "666").Label(LabelUserAccountId, "123").Label(LabelInstanceType, "aaa").
		Req(map[corev1.ResourceName]string{corev1.ResourceCPU: "4", corev1.ResourceMemory: "8Gi"}).
		NodeSelector(map[string]string{corev1.LabelArchStable: "amd64", corev1.LabelTopologyZone: "az-1"}).
		Obj()
	pl.podInfoCache.updatePod(nil, pod)
	result, status := pl.PreFilter(context.TODO(), cycleState, pod)
	assert.True(t, status.IsSuccess())
	assert.Equal(t, []string{"node-1", "node-2"}, result.NodeNames.List())
}

func Test_PreFilterWithMax(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-2",
					corev1.LabelArchStable:   "amd64",
				},
			},
		},
	}
	suit := newPluginTestSuit(t, nodes)

	quotas := []*schedv1alpha1.ElasticQuota{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "quota-a",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
					apiext.LabelQuotaParent:  "second-root-quota-a",
					LabelQuotaID:             "666",
					LabelUserAccountId:       "123",
					LabelInstanceType:        "aaa",
				},
			},
			Spec: schedv1alpha1.ElasticQuotaSpec{
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("40"),
					corev1.ResourceMemory: resource.MustParse("80Gi"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "quota-b",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-2",
					corev1.LabelArchStable:   "amd64",
					apiext.LabelQuotaParent:  "second-root-quota-a",
					LabelQuotaID:             "666",
					LabelUserAccountId:       "123",
					LabelInstanceType:        "aaa",
				},
			},
			Spec: schedv1alpha1.ElasticQuotaSpec{
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("40"),
					corev1.ResourceMemory: resource.MustParse("80Gi"),
				},
			},
		},
	}
	for _, v := range quotas {
		_, err := suit.schedclient.SchedulingV1alpha1().ElasticQuotas("default").Create(context.TODO(), v, metav1.CreateOptions{})
		assert.NoError(t, err)
	}
	p, err := suit.proxyNew(nil, suit.Framework)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	pl := p.(*Plugin)
	cycleState := framework.NewCycleState()
	pod := st.MakePod().Name("test").Label(LabelQuotaID, "666").Label(LabelUserAccountId, "123").Label(LabelInstanceType, "aaa").
		Req(map[corev1.ResourceName]string{corev1.ResourceCPU: "4", corev1.ResourceMemory: "8Gi"}).
		NodeSelector(map[string]string{corev1.LabelArchStable: "amd64", corev1.LabelTopologyZone: "az-1"}).
		Obj()
	pl.podInfoCache.updatePod(nil, pod)
	result, status := pl.PreFilter(context.TODO(), cycleState, pod)
	assert.True(t, status.IsSuccess())
	assert.Equal(t, []string{"node-1", "node-2"}, result.NodeNames.List())
}

func Test_PreFilterWithMaxAndFrozen(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-2",
					corev1.LabelArchStable:   "amd64",
				},
			},
		},
	}
	suit := newPluginTestSuit(t, nodes)

	quotas := []*schedv1alpha1.ElasticQuota{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "quota-a",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
					apiext.LabelQuotaParent:  "second-root-quota-a",
					LabelQuotaID:             "666",
					LabelUserAccountId:       "123",
					LabelInstanceType:        "aaa",
				},
			},
			Spec: schedv1alpha1.ElasticQuotaSpec{
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("40"),
					corev1.ResourceMemory: resource.MustParse("80Gi"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "quota-a-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
					apiext.LabelQuotaParent:  "second-root-quota-a",
					LabelQuotaID:             "666",
					LabelUserAccountId:       "123",
					LabelInstanceType:        "aaa",
				},
			},
			Spec: schedv1alpha1.ElasticQuotaSpec{
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("40"),
					corev1.ResourceMemory: resource.MustParse("80Gi"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "quota-b",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-2",
					corev1.LabelArchStable:   "amd64",
					apiext.LabelQuotaParent:  "second-root-quota-a",
					LabelQuotaID:             "666",
					LabelUserAccountId:       "123",
					LabelInstanceType:        "aaa",
				},
			},
			Spec: schedv1alpha1.ElasticQuotaSpec{
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("40"),
					corev1.ResourceMemory: resource.MustParse("80Gi"),
				},
			},
		},
	}
	for _, v := range quotas {
		_, err := suit.schedclient.SchedulingV1alpha1().ElasticQuotas("default").Create(context.TODO(), v, metav1.CreateOptions{})
		assert.NoError(t, err)
	}
	p, err := suit.proxyNew(nil, suit.Framework)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	pl := p.(*Plugin)
	cycleState := framework.NewCycleState()
	pod := st.MakePod().Name("test").Label(LabelQuotaID, "666").Label(LabelUserAccountId, "123").Label(LabelInstanceType, "aaa").
		Req(map[corev1.ResourceName]string{corev1.ResourceCPU: "4", corev1.ResourceMemory: "8Gi"}).
		NodeSelector(map[string]string{corev1.LabelArchStable: "amd64", corev1.LabelTopologyZone: "az-1"}).
		Obj()
	pl.podInfoCache.updatePod(nil, pod)
	pi := pl.podInfoCache.getPendingPodInfo(pod.UID)
	pi.frozenQuotas.Insert("quota-a-1")
	result, status := pl.PreFilter(context.TODO(), cycleState, pod)
	assert.True(t, status.IsSuccess())
	assert.Equal(t, []string{"node-1", "node-2"}, result.NodeNames.List())
}

func Test_PreFilterWithNoQuotas(t *testing.T) {
	suit := newPluginTestSuit(t, nil)

	p, err := suit.proxyNew(nil, suit.Framework)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	pl := p.(*Plugin)
	cycleState := framework.NewCycleState()
	pod := st.MakePod().Name("test").Label(LabelQuotaID, "666").Label(LabelUserAccountId, "123").Label(LabelInstanceType, "aaa").
		Req(map[corev1.ResourceName]string{corev1.ResourceCPU: "4", corev1.ResourceMemory: "8Gi"}).
		NodeSelector(map[string]string{corev1.LabelArchStable: "amd64", corev1.LabelTopologyZone: "az-1"}).
		Obj()
	pl.podInfoCache.updatePod(nil, pod)
	result, status := pl.PreFilter(context.TODO(), cycleState, pod)
	assert.Equal(t, "No matching Quota objects", status.Message())
	assert.Nil(t, result)
}

func Test_PreFilterWithNoAvailableQuota(t *testing.T) {
	suit := newPluginTestSuit(t, nil)

	quotas := []*schedv1alpha1.ElasticQuota{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "quota-a",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
					apiext.LabelQuotaParent:  "second-root-quota-a",
					LabelQuotaID:             "666",
					LabelUserAccountId:       "123",
					LabelInstanceType:        "aaa",
				},
			},
			Spec: schedv1alpha1.ElasticQuotaSpec{
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		},
	}
	for _, v := range quotas {
		_, err := suit.schedclient.SchedulingV1alpha1().ElasticQuotas("default").Create(context.TODO(), v, metav1.CreateOptions{})
		assert.NoError(t, err)
	}
	p, err := suit.proxyNew(nil, suit.Framework)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	pl := p.(*Plugin)
	cycleState := framework.NewCycleState()
	pod := st.MakePod().Name("test").Label(LabelQuotaID, "666").Label(LabelUserAccountId, "123").Label(LabelInstanceType, "aaa").
		Req(map[corev1.ResourceName]string{corev1.ResourceCPU: "4", corev1.ResourceMemory: "8Gi"}).
		NodeSelector(map[string]string{corev1.LabelArchStable: "amd64", corev1.LabelTopologyZone: "az-1"}).
		Obj()
	pl.podInfoCache.updatePod(nil, pod)
	result, status := pl.PreFilter(context.TODO(), cycleState, pod)
	assert.Equal(t, "No available Quotas", status.Message())
	assert.Nil(t, result)
}

func Test_PreFilterWithNoAvailableNodes(t *testing.T) {
	suit := newPluginTestSuit(t, nil)

	quotas := []*schedv1alpha1.ElasticQuota{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "quota-a",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
					apiext.LabelQuotaParent:  "second-root-quota-a",
					LabelQuotaID:             "666",
					LabelUserAccountId:       "123",
					LabelInstanceType:        "aaa",
				},
			},
			Spec: schedv1alpha1.ElasticQuotaSpec{
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("40"),
					corev1.ResourceMemory: resource.MustParse("80Gi"),
				},
			},
		},
	}
	for _, v := range quotas {
		_, err := suit.schedclient.SchedulingV1alpha1().ElasticQuotas("default").Create(context.TODO(), v, metav1.CreateOptions{})
		assert.NoError(t, err)
	}
	p, err := suit.proxyNew(nil, suit.Framework)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	pl := p.(*Plugin)
	cycleState := framework.NewCycleState()
	pod := st.MakePod().Name("test").Label(LabelQuotaID, "666").Label(LabelUserAccountId, "123").Label(LabelInstanceType, "aaa").
		Req(map[corev1.ResourceName]string{corev1.ResourceCPU: "4", corev1.ResourceMemory: "8Gi"}).
		NodeSelector(map[string]string{corev1.LabelArchStable: "amd64", corev1.LabelTopologyZone: "az-1"}).
		Obj()
	pl.podInfoCache.updatePod(nil, pod)
	result, status := pl.PreFilter(context.TODO(), cycleState, pod)
	assert.Equal(t, "No available nodes", status.Message())
	assert.Nil(t, result)
}

func TestReserveAndUnreserve(t *testing.T) {
	elasticQuota := &schedv1alpha1.ElasticQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name: "quota-a",
			Labels: map[string]string{
				corev1.LabelTopologyZone: "az-1",
				corev1.LabelArchStable:   "amd64",
				apiext.LabelQuotaParent:  "second-root-quota-a",
				LabelQuotaID:             "666",
				LabelUserAccountId:       "123",
				LabelInstanceType:        "aaa",
			},
		},
		Spec: schedv1alpha1.ElasticQuotaSpec{
			Max: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("40"),
				corev1.ResourceMemory: resource.MustParse("80Gi"),
			},
		},
	}
	tests := []struct {
		name          string
		state         *stateData
		quotaName     string
		wantReserve   corev1.ResourceList
		wantUnreserve corev1.ResourceList
	}{
		{
			name: "no quota",
			state: &stateData{
				skip: true,
			},
			wantReserve: nil,
		},
		{
			name: "reserve and unreserve 4C8Gi",
			state: &stateData{
				podRequests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			quotaName: elasticQuota.Name,
			wantReserve: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
			wantUnreserve: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("0"),
				corev1.ResourceMemory: resource.MustParse("0"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nil)
			_, err := suit.schedclient.SchedulingV1alpha1().ElasticQuotas("default").Create(context.TODO(), elasticQuota, metav1.CreateOptions{})
			assert.NoError(t, err)
			p, err := suit.proxyNew(nil, suit.Framework)
			assert.NoError(t, err)
			assert.NotNil(t, p)
			pl := p.(*Plugin)
			pod := st.MakePod().UID("123456").Obj()
			pl.podInfoCache.updatePod(nil, pod)
			pi := pl.podInfoCache.getPendingPodInfo(pod.UID)
			pi.selectedQuotaName = tt.quotaName

			cycleState := framework.NewCycleState()
			cycleState.Write(Name, tt.state)
			status := pl.Reserve(context.TODO(), cycleState, pod, "test-node")
			assert.True(t, status.IsSuccess())

			quotaObj := pl.quotaCache.getQuota(tt.quotaName)
			if tt.quotaName == "" {
				assert.Nil(t, quotaObj)
				return
			}
			assert.True(t, equality.Semantic.DeepEqual(tt.wantReserve, quotaObj.used))

			pl.Unreserve(context.TODO(), cycleState, pod, "test-node")
			quotaObj = pl.quotaCache.getQuota(tt.quotaName)
			assert.True(t, equality.Semantic.DeepEqual(tt.wantUnreserve, quotaObj.used))
		})
	}
}
