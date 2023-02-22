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
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	nrtinformers "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/informers/externalversions"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	v1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	schedulerconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"
	"sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	pgclientset "sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned"
	pgfake "sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned/fake"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned/fake"
	koordinatorinformers "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config/v1beta2"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota"
)

type ElasticQuotaSetAndHandle struct {
	framework.Handle
	pgclientset.Interface
}

func ElasticQuotaPluginFactoryProxy(clientSet pgclientset.Interface, factoryFn runtime.PluginFactory) runtime.PluginFactory {
	return func(args apiruntime.Object, handle framework.Handle) (framework.Plugin, error) {
		return factoryFn(args, ElasticQuotaSetAndHandle{Handle: handle, Interface: clientSet})
	}
}
func mockPodsList(w http.ResponseWriter, r *http.Request) {
	bear := r.Header.Get("Authorization")
	if bear == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	parts := strings.Split(bear, "Bearer")
	if len(parts) != 2 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	http_token := strings.TrimSpace(parts[1])
	if len(http_token) < 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if http_token != token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	podList := new(corev1.PodList)
	b, err := json.Marshal(podList)
	if err != nil {
		log.Printf("codec error %+v", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}
func parseHostAndPort(rawURL string) (string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "0", err
	}
	return net.SplitHostPort(u.Host)
}

var (
	token string
)

func newPluginTestSuit(t *testing.T, nodes []*corev1.Node) *pluginTestSuit {
	var v1beta2args v1beta2.ElasticQuotaArgs
	v1beta2.SetDefaults_ElasticQuotaArgs(&v1beta2args)
	var elasticQuotaArgs config.ElasticQuotaArgs
	err := v1beta2.Convert_v1beta2_ElasticQuotaArgs_To_config_ElasticQuotaArgs(&v1beta2args, &elasticQuotaArgs, nil)
	assert.NoError(t, err)
	elasticQuotaPluginConfig := schedulerconfig.PluginConfig{
		Name: elasticquota.Name,
		Args: &elasticQuotaArgs,
	}
	koordClientSet := fake.NewSimpleClientset()
	koordSharedInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordClientSet, 0)
	pgClientSet := pgfake.NewSimpleClientset()
	proxyNew := ElasticQuotaPluginFactoryProxy(pgClientSet, elasticquota.New)
	registeredPlugins := []schedulertesting.RegisterPluginFunc{
		func(reg *runtime.Registry, profile *schedulerconfig.KubeSchedulerProfile) {
			profile.PluginConfig = []schedulerconfig.PluginConfig{
				elasticQuotaPluginConfig,
			}
		},
		schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
		schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
		schedulertesting.RegisterPreFilterPlugin(elasticquota.Name, proxyNew),
	}
	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newTestSharedLister(nil, nodes)
	server := httptest.NewTLSServer(http.HandlerFunc(mockPodsList))
	defer server.Close()
	address, portStr, err := parseHostAndPort(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &rest.Config{
		Host:        net.JoinHostPort(address, portStr),
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}
	if token == "" {
		flag.StringVar(&token, "token", "mockTest", "")
		flag.Parse()
	}
	fh, err := schedulertesting.NewFramework(
		registeredPlugins,
		"koord-scheduler",
		runtime.WithClientSet(cs),
		runtime.WithInformerFactory(informerFactory),
		runtime.WithSnapshotSharedLister(snapshot),
		runtime.WithKubeConfig(cfg),
	)
	assert.Nil(t, err)
	return &pluginTestSuit{
		Handle:                           fh,
		koordinatorSharedInformerFactory: koordSharedInformerFactory,
		proxyNew:                         proxyNew,
		elasticQuotaArgs:                 &elasticQuotaArgs,
		client:                           pgClientSet,
	}
}

var _ framework.SharedLister = &testSharedLister{}

type testSharedLister struct {
	nodes       []*corev1.Node
	nodeInfos   []*framework.NodeInfo
	nodeInfoMap map[string]*framework.NodeInfo
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

type pluginTestSuit struct {
	framework.Handle
	framework.Framework
	koordinatorSharedInformerFactory koordinatorinformers.SharedInformerFactory
	nrtSharedInformerFactory         nrtinformers.SharedInformerFactory
	proxyNew                         runtime.PluginFactory
	elasticQuotaArgs                 *config.ElasticQuotaArgs
	client                           *pgfake.Clientset
	plugin                           framework.Plugin
}

func createResourceList(cpu, mem int64) corev1.ResourceList {
	return corev1.ResourceList{
		// use NewMilliQuantity to calculate the runtimeQuota correctly in cpu dimension
		// when the request is smaller than 1 core.
		corev1.ResourceCPU:    *resource.NewMilliQuantity(cpu*1000, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(mem, resource.BinarySI),
	}
}
func CreateQuota2(name string, parentName string, maxCpu, maxMem int64, minCpu, minMem int64,
	scaleCpu, scaleMem int64, isParGroup bool) *v1alpha1.ElasticQuota {
	quota := &v1alpha1.ElasticQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: make(map[string]string),
			Labels:      make(map[string]string),
		},
		Spec: v1alpha1.ElasticQuotaSpec{
			Max: createResourceList(maxCpu, maxMem),
			Min: createResourceList(minCpu, minMem),
		},
	}
	quota.Annotations[extension.AnnotationSharedWeight] = fmt.Sprintf("{\"cpu\":%v, \"memory\":\"%v\"}", scaleCpu, scaleMem)
	quota.Labels[extension.LabelQuotaParent] = parentName
	if isParGroup {
		quota.Labels[extension.LabelQuotaIsParent] = "true"
	} else {
		quota.Labels[extension.LabelQuotaIsParent] = "false"
	}
	return quota
}

func TestPlugin_OnQuotaAdd2(t *testing.T) {
	suit := newPluginTestSuit(t, nil)
	p, _ := suit.proxyNew(suit.elasticQuotaArgs, suit.Handle)
	pl := p.(*elasticquota.Plugin)
	pl.GetGroupQuotaManager().UpdateClusterTotalResource(createResourceList(501952056, 0))
	gqm := pl.GetGroupQuotaManager()
	quota := CreateQuota2("1", "", 0, 0, 0, 0, 0, 0, false)
	quota.Spec.Min = corev1.ResourceList{
		corev1.ResourceCPU:                    *resource.NewMilliQuantity(100*1000, resource.DecimalSI),
		corev1.ResourceMemory:                 *resource.NewQuantity(1000, resource.BinarySI),
		extension.ResourceNvidiaGPU:           *resource.NewQuantity(8, resource.DecimalSI),
		extension.ResourceNvidiaGPU + "-a100": *resource.NewQuantity(8, resource.DecimalSI),
	}
	quota.Spec.Max = corev1.ResourceList{
		corev1.ResourceCPU:                    *resource.NewMilliQuantity(200*1000, resource.DecimalSI),
		corev1.ResourceMemory:                 *resource.NewQuantity(2000, resource.BinarySI),
		extension.ResourceNvidiaGPU:           *resource.NewQuantity(16, resource.DecimalSI),
		extension.ResourceNvidiaGPU + "-a100": *resource.NewQuantity(16, resource.DecimalSI),
	}
	sharedWeight := corev1.ResourceList{
		corev1.ResourceCPU:                    *resource.NewMilliQuantity(300*1000, resource.DecimalSI),
		corev1.ResourceMemory:                 *resource.NewQuantity(3000, resource.BinarySI),
		extension.ResourceNvidiaGPU:           *resource.NewQuantity(32, resource.DecimalSI),
		extension.ResourceNvidiaGPU + "-a100": *resource.NewQuantity(32, resource.DecimalSI),
	}
	sharedWeightStr, _ := json.Marshal(sharedWeight)
	quota.Annotations[extension.AnnotationSharedWeight] = string(sharedWeightStr)
	pl.OnQuotaAdd(quota)

	assert.NotNil(t, gqm.GetQuotaInfoByName("1"))
	quota.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	quota.Name = "2"
	pl.OnQuotaAdd(quota)
	assert.Nil(t, gqm.GetQuotaInfoByName("2"))

	quotaInfo := gqm.GetQuotaInfoByName("1")
	assert.Equal(t, quotaInfo.CalculateInfo.Min, corev1.ResourceList{
		corev1.ResourceCPU:             *resource.NewMilliQuantity(100*1000, resource.DecimalSI),
		corev1.ResourceMemory:          *resource.NewQuantity(1000, resource.BinarySI),
		extension.ResourceNvidiaGPU:    *resource.NewQuantity(8, resource.DecimalSI),
		"nvidia.com/gpu-a100":          *resource.NewQuantity(8, resource.DecimalSI),
		unified.GPUCardRatio:           *resource.NewQuantity(800, resource.DecimalSI),
		unified.GPUCardRatio + "-a100": *resource.NewQuantity(800, resource.DecimalSI),
	})
	assert.Equal(t, quotaInfo.CalculateInfo.Max, corev1.ResourceList{
		corev1.ResourceCPU:             *resource.NewMilliQuantity(200*1000, resource.DecimalSI),
		corev1.ResourceMemory:          *resource.NewQuantity(2000, resource.BinarySI),
		extension.ResourceNvidiaGPU:    *resource.NewQuantity(16, resource.DecimalSI),
		"nvidia.com/gpu-a100":          *resource.NewQuantity(16, resource.DecimalSI),
		unified.GPUCardRatio:           *resource.NewQuantity(1600, resource.DecimalSI),
		unified.GPUCardRatio + "-a100": *resource.NewQuantity(1600, resource.DecimalSI),
	})
	assert.True(t, v1.Equals(quotaInfo.CalculateInfo.SharedWeight, corev1.ResourceList{
		corev1.ResourceCPU:             *resource.NewMilliQuantity(300*1000, resource.DecimalSI),
		corev1.ResourceMemory:          *resource.NewQuantity(3000, resource.BinarySI),
		extension.ResourceNvidiaGPU:    *resource.NewQuantity(32, resource.DecimalSI),
		"nvidia.com/gpu-a100":          *resource.NewQuantity(32, resource.DecimalSI),
		unified.GPUCardRatio:           *resource.NewQuantity(3200, resource.DecimalSI),
		unified.GPUCardRatio + "-a100": *resource.NewQuantity(3200, resource.DecimalSI),
	}))

	//update
	quota = CreateQuota2("1", "", 0, 0, 0, 0, 0, 0, false)
	quota.Spec.Min = corev1.ResourceList{
		corev1.ResourceCPU:                    *resource.NewMilliQuantity(1000*1000, resource.DecimalSI),
		corev1.ResourceMemory:                 *resource.NewQuantity(10000, resource.BinarySI),
		extension.ResourceNvidiaGPU:           *resource.NewQuantity(80, resource.DecimalSI),
		extension.ResourceNvidiaGPU + "-a100": *resource.NewQuantity(80, resource.DecimalSI),
	}
	quota.Spec.Max = corev1.ResourceList{
		corev1.ResourceCPU:                    *resource.NewMilliQuantity(2000*1000, resource.DecimalSI),
		corev1.ResourceMemory:                 *resource.NewQuantity(20000, resource.BinarySI),
		extension.ResourceNvidiaGPU:           *resource.NewQuantity(160, resource.DecimalSI),
		extension.ResourceNvidiaGPU + "-a100": *resource.NewQuantity(160, resource.DecimalSI),
	}
	sharedWeight = corev1.ResourceList{
		corev1.ResourceCPU:                    *resource.NewMilliQuantity(3000*1000, resource.DecimalSI),
		corev1.ResourceMemory:                 *resource.NewQuantity(30000, resource.BinarySI),
		extension.ResourceNvidiaGPU:           *resource.NewQuantity(320, resource.DecimalSI),
		extension.ResourceNvidiaGPU + "-a100": *resource.NewQuantity(320, resource.DecimalSI),
	}
	sharedWeightStr, _ = json.Marshal(sharedWeight)
	quota.Annotations[extension.AnnotationSharedWeight] = string(sharedWeightStr)

	pl.OnQuotaUpdate(nil, quota)
	quotaInfo = gqm.GetQuotaInfoByName("1")
	assert.Equal(t, quotaInfo.CalculateInfo.Min, corev1.ResourceList{
		corev1.ResourceCPU:             *resource.NewMilliQuantity(1000*1000, resource.DecimalSI),
		corev1.ResourceMemory:          *resource.NewQuantity(10000, resource.BinarySI),
		extension.ResourceNvidiaGPU:    *resource.NewQuantity(80, resource.DecimalSI),
		"nvidia.com/gpu-a100":          *resource.NewQuantity(80, resource.DecimalSI),
		unified.GPUCardRatio:           *resource.NewQuantity(8000, resource.DecimalSI),
		unified.GPUCardRatio + "-a100": *resource.NewQuantity(8000, resource.DecimalSI),
	})
	assert.Equal(t, quotaInfo.CalculateInfo.Max, corev1.ResourceList{
		corev1.ResourceCPU:             *resource.NewMilliQuantity(2000*1000, resource.DecimalSI),
		corev1.ResourceMemory:          *resource.NewQuantity(20000, resource.BinarySI),
		extension.ResourceNvidiaGPU:    *resource.NewQuantity(160, resource.DecimalSI),
		"nvidia.com/gpu-a100":          *resource.NewQuantity(160, resource.DecimalSI),
		unified.GPUCardRatio:           *resource.NewQuantity(16000, resource.DecimalSI),
		unified.GPUCardRatio + "-a100": *resource.NewQuantity(16000, resource.DecimalSI),
	})
	assert.True(t, v1.Equals(quotaInfo.CalculateInfo.SharedWeight, corev1.ResourceList{
		corev1.ResourceCPU:             *resource.NewMilliQuantity(3000*1000, resource.DecimalSI),
		corev1.ResourceMemory:          *resource.NewQuantity(30000, resource.BinarySI),
		extension.ResourceNvidiaGPU:    *resource.NewQuantity(320, resource.DecimalSI),
		"nvidia.com/gpu-a100":          *resource.NewQuantity(320, resource.DecimalSI),
		unified.GPUCardRatio:           *resource.NewQuantity(32000, resource.DecimalSI),
		unified.GPUCardRatio + "-a100": *resource.NewQuantity(32000, resource.DecimalSI),
	}))
}

func TestPlugin_OnNodeAdd2(t *testing.T) {
	tests := []struct {
		name     string
		nodes    []*corev1.Node
		totalRes corev1.ResourceList
	}{
		{
			name: "add gpu node",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							extension.LabelGPUModel: "A100",
						},
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:               *resource.NewMilliQuantity(100*1000, resource.DecimalSI),
							corev1.ResourceMemory:            *resource.NewQuantity(1000, resource.BinarySI),
							extension.ResourceGPUMemoryRatio: *resource.NewQuantity(800, resource.DecimalSI),
						},
					},
				},
			},
			totalRes: corev1.ResourceList{
				corev1.ResourceCPU:               *resource.NewMilliQuantity(100*1000, resource.DecimalSI),
				corev1.ResourceMemory:            *resource.NewQuantity(1000, resource.BinarySI),
				extension.ResourceGPUMemoryRatio: *resource.NewQuantity(800, resource.DecimalSI),
				unified.GPUCardRatio:             *resource.NewQuantity(800, resource.DecimalSI),
				unified.GPUCardRatio + "-a100":   *resource.NewQuantity(800, resource.DecimalSI),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nil)
			p, err := suit.proxyNew(suit.elasticQuotaArgs, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)
			eQP := p.(*elasticquota.Plugin)
			for _, node := range tt.nodes {
				eQP.OnNodeAdd(node)
			}
			gqm := eQP.GetGroupQuotaManager()
			assert.NotNil(t, gqm)
			assert.Equal(t, tt.totalRes, gqm.GetClusterTotalResource())
		})
	}
}

func defaultCreateNode(nodeName string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Status: corev1.NodeStatus{
			Allocatable: createResourceList(100, 1000),
		},
	}
}
func defaultCreateNodeWithResourceVersion(nodeName string) *corev1.Node {
	node := defaultCreateNode(nodeName)
	node.ResourceVersion = "3"
	return node
}

func TestPlugin_OnNodeUpdate2(t *testing.T) {
	nodes := []*corev1.Node{defaultCreateNodeWithResourceVersion("1"), defaultCreateNodeWithResourceVersion("2"),
		defaultCreateNodeWithResourceVersion("3")}
	tests := []struct {
		name     string
		nodes    []*corev1.Node
		totalRes corev1.ResourceList
	}{
		{
			name: "add gpu node",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "1",
						Labels: map[string]string{
							extension.LabelGPUModel: "A100",
						},
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:               *resource.NewMilliQuantity(50*1000, resource.DecimalSI),
							corev1.ResourceMemory:            *resource.NewQuantity(500, resource.BinarySI),
							extension.ResourceGPUMemoryRatio: *resource.NewQuantity(800, resource.DecimalSI),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "2",
						Labels: map[string]string{
							extension.LabelGPUModel: "A100",
						},
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:               *resource.NewMilliQuantity(50*1000, resource.DecimalSI),
							corev1.ResourceMemory:            *resource.NewQuantity(500, resource.BinarySI),
							extension.ResourceGPUMemoryRatio: *resource.NewQuantity(800, resource.DecimalSI),
						},
					},
				},
			},
			totalRes: corev1.ResourceList{
				corev1.ResourceCPU:               *resource.NewMilliQuantity(200*1000, resource.DecimalSI),
				corev1.ResourceMemory:            *resource.NewQuantity(2000, resource.BinarySI),
				extension.ResourceGPUMemoryRatio: *resource.NewQuantity(1600, resource.DecimalSI),
				unified.GPUCardRatio:             *resource.NewQuantity(1600, resource.DecimalSI),
				unified.GPUCardRatio + "-a100":   *resource.NewQuantity(1600, resource.DecimalSI),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nil)
			p, _ := suit.proxyNew(suit.elasticQuotaArgs, suit.Handle)
			plugin := p.(*elasticquota.Plugin)
			for _, node := range nodes {
				plugin.OnNodeAdd(node)
			}
			for i, node := range tt.nodes {
				plugin.OnNodeUpdate(nodes[i], node)
			}
			assert.Equal(t, p.(*elasticquota.Plugin).GetGroupQuotaManager().GetClusterTotalResource(), tt.totalRes)
		})
	}
}

func TestPlugin_OnNodeDelete2(t *testing.T) {
	suit := newPluginTestSuit(t, nil)
	p, err := suit.proxyNew(suit.elasticQuotaArgs, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)
	eQP := p.(*elasticquota.Plugin)
	gqp := eQP.GetGroupQuotaManager()
	assert.NotNil(t, gqp)
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "1",
				Labels: map[string]string{
					extension.LabelGPUModel: "A100",
				},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:               *resource.NewMilliQuantity(25*1000, resource.DecimalSI),
					corev1.ResourceMemory:            *resource.NewQuantity(250, resource.BinarySI),
					extension.ResourceGPUMemoryRatio: *resource.NewQuantity(400, resource.DecimalSI),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "2",
				Labels: map[string]string{
					extension.LabelGPUModel: "A100",
				},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:               *resource.NewMilliQuantity(25*1000, resource.DecimalSI),
					corev1.ResourceMemory:            *resource.NewQuantity(250, resource.BinarySI),
					extension.ResourceGPUMemoryRatio: *resource.NewQuantity(400, resource.DecimalSI),
				},
			},
		},
	}
	for _, node := range nodes {
		eQP.OnNodeAdd(node)
	}
	for i, node := range nodes {
		eQP.OnNodeDelete(node)
		assert.Equal(t, gqp.GetClusterTotalResource(), corev1.ResourceList{
			corev1.ResourceCPU:               *resource.NewMilliQuantity(int64(50-25*(i+1))*1000, resource.DecimalSI),
			corev1.ResourceMemory:            *resource.NewQuantity(int64(500-250*(i+1)), resource.BinarySI),
			extension.ResourceGPUMemoryRatio: *resource.NewQuantity(int64(800-400*(i+1)), resource.DecimalSI),
			unified.GPUCardRatio:             *resource.NewQuantity(int64(800-400*(i+1)), resource.DecimalSI),
			unified.GPUCardRatio + "-a100":   *resource.NewQuantity(int64(800-400*(i+1)), resource.DecimalSI),
		})
	}
}
