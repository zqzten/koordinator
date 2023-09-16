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
	"sort"
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
	nodeaffinityhelper "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/nodeaffinity"
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

func Test_PreFilter(t *testing.T) {
	suit := newPluginTestSuit(t, nil)

	quotas := []*schedv1alpha1.ElasticQuota{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "quota-a",
				Namespace: "default",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
					corev1.LabelArchStable:   "amd64",
					apiext.LabelQuotaParent:  "",
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
				Name:      "quota-b",
				Namespace: "default",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-2",
					corev1.LabelArchStable:   "amd64",
					apiext.LabelQuotaParent:  "",
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
		NodeSelector(map[string]string{corev1.LabelArchStable: "amd64"}).
		Obj()
	result, status := pl.PreFilter(context.TODO(), cycleState, pod)
	assert.True(t, status.IsSuccess())
	assert.Nil(t, result)
	sd := getStateData(cycleState)
	sort.Slice(sd.availableQuotas, func(i, j int) bool {
		return sd.availableQuotas[i].Name < sd.availableQuotas[j].Name
	})
	expectedState := &stateData{
		skip: false,
		podRequests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
		},
		availableQuotas: []*QuotaWrapper{
			{
				Name: "quota-a",
				Used: corev1.ResourceList{},
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("40"),
					corev1.ResourceMemory: resource.MustParse("80Gi"),
				},
				Min: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Obj: quotas[0],
			},
			{
				Name: "quota-b",
				Used: corev1.ResourceList{},
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("40"),
					corev1.ResourceMemory: resource.MustParse("80Gi"),
				},
				Min: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Obj: quotas[1],
			},
		},
		replicasWithMin:   nil,
		replicasWithMax:   nil,
		selectedQuotaName: "",
	}
	assert.Equal(t, expectedState, sd)
	affinity := nodeaffinityhelper.GetTemporaryNodeAffinity(cycleState)
	assert.True(t, affinity.Match(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				corev1.LabelTopologyZone: "az-1",
			},
		},
	}))
	assert.True(t, affinity.Match(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				corev1.LabelTopologyZone: "az-2",
			},
		},
	}))
	assert.False(t, affinity.Match(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				corev1.LabelTopologyZone: "az-3",
			},
		},
	}))
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
	result, status := pl.PreFilter(context.TODO(), cycleState, pod)
	assert.Equal(t, "No available Quotas", status.Message())
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
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				corev1.LabelTopologyZone: "az-1",
				corev1.LabelArchStable:   "amd64",
			},
		},
	}

	tests := []struct {
		name          string
		state         *stateData
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
				availableQuotas: []*QuotaWrapper{
					{
						Name: elasticQuota.Name,
						Obj:  elasticQuota,
					},
				},
				selectedQuotaName: elasticQuota.Name,
			},
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
			suit := newPluginTestSuit(t, []*corev1.Node{node})
			_, err := suit.schedclient.SchedulingV1alpha1().ElasticQuotas("default").Create(context.TODO(), elasticQuota, metav1.CreateOptions{})
			assert.NoError(t, err)
			p, err := suit.proxyNew(nil, suit.Framework)
			assert.NoError(t, err)
			assert.NotNil(t, p)
			pl := p.(*Plugin)
			pod := st.MakePod().UID("123456").Node("test-node").Obj()
			pod.Spec.Containers = []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: tt.state.podRequests,
					},
				},
			}

			cycleState := framework.NewCycleState()
			cycleState.Write(Name, tt.state)
			status := pl.Reserve(context.TODO(), cycleState, pod, "test-node")
			assert.True(t, status.IsSuccess())

			quotaObj := pl.Plugin.GetGroupQuotaManagerForQuota(tt.state.selectedQuotaName).GetQuotaInfoByName(tt.state.selectedQuotaName)
			if tt.state.selectedQuotaName == "" {
				assert.Nil(t, quotaObj)
				return
			}
			assert.True(t, equality.Semantic.DeepEqual(tt.wantReserve, quotaObj.GetUsed()))

			pl.Unreserve(context.TODO(), cycleState, pod, "test-node")
			quotaObj = pl.Plugin.GetGroupQuotaManagerForQuota(tt.state.selectedQuotaName).GetQuotaInfoByName(tt.state.selectedQuotaName)
			assert.True(t, equality.Semantic.DeepEqual(tt.wantUnreserve, quotaObj.GetUsed()))
		})
	}
}

func TestPluginPreScore(t *testing.T) {
	tests := []struct {
		name                string
		quotas              []*schedv1alpha1.ElasticQuota
		wantReplicasWithMin map[string]int
		wantReplicasWithMax map[string]int
	}{
		{
			name: "the most allocated min, the high score",
			quotas: []*schedv1alpha1.ElasticQuota{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "quota-a",
						Namespace: "default",
						Labels: map[string]string{
							corev1.LabelTopologyZone: "az-1",
							corev1.LabelArchStable:   "amd64",
							apiext.LabelQuotaParent:  "",
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
						Name:      "quota-b",
						Namespace: "default",
						Labels: map[string]string{
							corev1.LabelTopologyZone: "az-2",
							corev1.LabelArchStable:   "amd64",
							apiext.LabelQuotaParent:  "",
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
							corev1.ResourceCPU:    resource.MustParse("8"),
							corev1.ResourceMemory: resource.MustParse("16Gi"),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "quota-c",
						Namespace: "default",
						Labels: map[string]string{
							corev1.LabelTopologyZone: "az-3",
							corev1.LabelArchStable:   "amd64",
							apiext.LabelQuotaParent:  "",
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
			},
			wantReplicasWithMin: map[string]int{
				"quota-a": 1,
				"quota-b": 2,
			},
			wantReplicasWithMax: map[string]int{
				"quota-c": 10,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
					},
				},
			}
			suit := newPluginTestSuit(t, nodes)
			for _, v := range tt.quotas {
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
				NodeSelector(map[string]string{corev1.LabelArchStable: "amd64"}).
				Obj()
			result, status := pl.PreFilter(context.TODO(), cycleState, pod)
			assert.True(t, status.IsSuccess())
			assert.Nil(t, result)
			status = pl.PreScore(context.TODO(), cycleState, pod, nodes)
			assert.True(t, status.IsSuccess())
			sd := getStateData(cycleState)
			assert.Equal(t, tt.wantReplicasWithMin, sd.replicasWithMin)
			assert.Equal(t, tt.wantReplicasWithMax, sd.replicasWithMax)
		})
	}
}

func TestPluginScore(t *testing.T) {
	tests := []struct {
		name                       string
		quotas                     []*schedv1alpha1.ElasticQuota
		nodes                      []*corev1.Node
		wantNodeScoreList          framework.NodeScoreList
		wantNormalizeNodeScoreList framework.NodeScoreList
	}{
		{
			name: "the most allocated min, the high score",
			quotas: []*schedv1alpha1.ElasticQuota{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "quota-a",
						Namespace: "default",
						Labels: map[string]string{
							corev1.LabelTopologyZone: "az-1",
							corev1.LabelArchStable:   "amd64",
							apiext.LabelQuotaParent:  "",
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
						Name:      "quota-b",
						Namespace: "default",
						Labels: map[string]string{
							corev1.LabelTopologyZone: "az-2",
							corev1.LabelArchStable:   "amd64",
							apiext.LabelQuotaParent:  "",
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
							corev1.ResourceCPU:    resource.MustParse("8"),
							corev1.ResourceMemory: resource.MustParse("16Gi"),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "quota-c",
						Namespace: "default",
						Labels: map[string]string{
							corev1.LabelTopologyZone: "az-3",
							corev1.LabelArchStable:   "amd64",
							apiext.LabelQuotaParent:  "",
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
			},
			nodes: []*corev1.Node{
				st.MakeNode().Name("node-1").Label(corev1.LabelTopologyZone, "az-1").Label(corev1.LabelArchStable, "amd64").Obj(),
				st.MakeNode().Name("node-2").Label(corev1.LabelTopologyZone, "az-2").Label(corev1.LabelArchStable, "amd64").Obj(),
				st.MakeNode().Name("node-3").Label(corev1.LabelTopologyZone, "az-3").Label(corev1.LabelArchStable, "amd64").Obj(),
			},
			wantNodeScoreList: framework.NodeScoreList{
				{
					Name:  "node-1",
					Score: 9000,
				},
				{
					Name:  "node-2",
					Score: 8000,
				},
				{
					Name:  "node-3",
					Score: 500,
				},
			},
			wantNormalizeNodeScoreList: framework.NodeScoreList{
				{
					Name:  "node-1",
					Score: 100,
				},
				{
					Name:  "node-2",
					Score: 88,
				},
				{
					Name:  "node-3",
					Score: 5,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, tt.nodes)
			for _, v := range tt.quotas {
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
				NodeSelector(map[string]string{corev1.LabelArchStable: "amd64"}).
				Obj()
			result, status := pl.PreFilter(context.TODO(), cycleState, pod)
			assert.True(t, status.IsSuccess())
			assert.Nil(t, result)
			status = pl.PreScore(context.TODO(), cycleState, pod, tt.nodes)
			assert.True(t, status.IsSuccess())
			var nodeScores framework.NodeScoreList
			for _, v := range tt.nodes {
				score, status := pl.Score(context.TODO(), cycleState, pod, v.Name)
				assert.True(t, status.IsSuccess())
				nodeScores = append(nodeScores, framework.NodeScore{
					Name:  v.Name,
					Score: score,
				})
			}
			assert.Equal(t, tt.wantNodeScoreList, nodeScores)
			status = pl.ScoreExtensions().NormalizeScore(context.TODO(), cycleState, pod, nodeScores)
			assert.True(t, status.IsSuccess())
			assert.Equal(t, tt.wantNormalizeNodeScoreList, nodeScores)
		})
	}
}
