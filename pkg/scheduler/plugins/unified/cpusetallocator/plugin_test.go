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

package cpusetallocator

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	nrtfake "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned/fake"
	nrtinformers "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/informers/externalversions"
	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
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

type frameworkHandleExtender struct {
	framework.Handle
	*nrtfake.Clientset
}

func proxyPluginFactory(fakeClientSet *nrtfake.Clientset, factory runtime.PluginFactory) runtime.PluginFactory {
	return func(configuration apiruntime.Object, f framework.Handle) (framework.Plugin, error) {
		p, err := factory(configuration, &frameworkHandleExtender{
			Handle:    f,
			Clientset: fakeClientSet,
		})
		if err == nil {
			p.(*Plugin).cpuSharePoolUpdater.Start()
		}
		return p, err
	}
}

type pluginTestSuit struct {
	framework.Handle
	nrtClientSet       *nrtfake.Clientset
	nrtInformerFactory nrtinformers.SharedInformerFactory
	proxyNew           runtime.PluginFactory
	args               apiruntime.Object
}

func newPluginTestSuit(t *testing.T, nodes []*corev1.Node) *pluginTestSuit {
	pluginArgs := apiruntime.Unknown{
		ContentType: apiruntime.ContentTypeJSON,
		Raw:         []byte(`{"apiVersion":"kubescheduler.config.k8s.io/v1beta2","defaultCPUBindPolicy":"FullPCPUs","kind":"NodeNUMAResourceArgs","scoringStrategy":{"Resources":[{"Name":"cpu","Weight":1},{"Name":"memory","Weight":1}],"Type":"MostAllocated"}}`),
	}

	nrtClientSet := nrtfake.NewSimpleClientset()
	proxyNew := proxyPluginFactory(nrtClientSet, New)
	nrtInformerFactory := nrtinformers.NewSharedInformerFactoryWithOptions(nrtClientSet, 0)

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
		Handle:             fh,
		nrtClientSet:       nrtClientSet,
		nrtInformerFactory: nrtInformerFactory,
		proxyNew:           proxyNew,
		args:               &pluginArgs,
	}
}

func (p *pluginTestSuit) start() {
	ctx := context.TODO()
	p.Handle.SharedInformerFactory().Start(ctx.Done())
	p.Handle.SharedInformerFactory().WaitForCacheSync(ctx.Done())
	p.nrtInformerFactory.Start(ctx.Done())
	p.nrtInformerFactory.WaitForCacheSync(ctx.Done())
}

func TestNew(t *testing.T) {
	suit := newPluginTestSuit(t, nil)
	p, err := suit.proxyNew(suit.args, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)
	assert.Equal(t, Name, p.Name())
	p, err = suit.proxyNew(nil, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)
	assert.Equal(t, Name, p.Name())
}

func TestPlugin_Filter(t *testing.T) {
	tests := []struct {
		name       string
		nodeLabels map[string]string
		want       *framework.Status
	}{
		{
			name:       "exclude virtual kubelet node(label key=alibabacloud.com/node-type)",
			nodeLabels: map[string]string{uniext.LabelCommonNodeType: uniext.VKType},
			want:       nil,
		},
		{
			name:       "exclude virtual kubelet node(label key=type)",
			nodeLabels: map[string]string{uniext.LabelNodeType: uniext.VKType},
			want:       nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "test-node-1",
						Labels: map[string]string{},
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("96"),
							corev1.ResourceMemory: resource.MustParse("512Gi"),
						},
					},
				},
			}
			for k, v := range tt.nodeLabels {
				nodes[0].Labels[k] = v
			}

			suit := newPluginTestSuit(t, nodes)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get("test-node-1")
			assert.NoError(t, err)
			assert.NotNil(t, nodeInfo)

			if got := plg.Filter(context.TODO(), nil, nil, nodeInfo); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Filter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlugin_Score(t *testing.T) {
	tests := []struct {
		name       string
		nodeLabels map[string]string
		want       *framework.Status
	}{
		{
			name:       "exclude virtual kubelet node(label key=alibabacloud.com/node-type)",
			nodeLabels: map[string]string{uniext.LabelCommonNodeType: uniext.VKType},
			want:       nil,
		},
		{
			name:       "exclude virtual kubelet node(label key=type)",
			nodeLabels: map[string]string{uniext.LabelNodeType: uniext.VKType},
			want:       nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "test-node-1",
						Labels: map[string]string{},
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("96"),
							corev1.ResourceMemory: resource.MustParse("512Gi"),
						},
					},
				},
			}
			for k, v := range tt.nodeLabels {
				nodes[0].Labels[k] = v
			}

			suit := newPluginTestSuit(t, nodes)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get("test-node-1")
			assert.NoError(t, err)
			assert.NotNil(t, nodeInfo)

			gotScore, gotStatus := plg.Score(context.TODO(), nil, nil, "test-node-1")
			if !reflect.DeepEqual(gotStatus, tt.want) {
				t.Errorf("Score() = %v, want %v", gotStatus, tt.want)
			}
			if !tt.want.IsSuccess() {
				return
			}
			assert.Equal(t, int64(0), gotScore)
		})
	}
}

func TestPlugin_Reserve(t *testing.T) {
	tests := []struct {
		name       string
		nodeLabels map[string]string
		want       *framework.Status
	}{
		{
			name:       "exclude virtual kubelet node(label key=alibabacloud.com/node-type)",
			nodeLabels: map[string]string{uniext.LabelCommonNodeType: uniext.VKType},
			want:       nil,
		},
		{
			name:       "exclude virtual kubelet node(label key=type)",
			nodeLabels: map[string]string{uniext.LabelNodeType: uniext.VKType},
			want:       nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "test-node-1",
						Labels: map[string]string{},
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("96"),
							corev1.ResourceMemory: resource.MustParse("512Gi"),
						},
					},
				},
			}
			for k, v := range tt.nodeLabels {
				nodes[0].Labels[k] = v
			}

			suit := newPluginTestSuit(t, nodes)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get("test-node-1")
			assert.NoError(t, err)
			assert.NotNil(t, nodeInfo)

			if got := plg.Reserve(context.TODO(), nil, nil, "test-node-1"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Reserve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlugin_CPUSetProtocols(t *testing.T) {
	koordResourceSpec := extension.ResourceSpec{
		PreferredCPUBindPolicy: extension.CPUBindPolicySpreadByPCPUs,
	}
	koordResourceSpecData, err := json.Marshal(koordResourceSpec)
	assert.NoError(t, err)

	unifiedAllocSpec := uniext.ResourceAllocSpec{
		CPU: uniext.CPUBindStrategySpread,
	}
	unifiedResourceSpecWithSpreadData, err := json.Marshal(unifiedAllocSpec)
	assert.NoError(t, err)
	unifiedAllocSpec.CPU = uniext.CPUBindStrategySameCoreFirst
	unifiedResourceSpecWithSameCoreData, err := json.Marshal(unifiedAllocSpec)
	assert.NoError(t, err)

	asiAllocSpec := extunified.AllocSpec{
		Containers: []extunified.Container{
			{
				Name: "container-1",
				Resource: extunified.ResourceRequirements{
					CPU: extunified.CPUSpec{
						CPUSet: &extunified.CPUSetSpec{
							SpreadStrategy: "",
							CPUIDs:         nil,
						},
					},
				},
			},
		},
	}
	asiAllocSpecWthSpreadData, err := json.Marshal(asiAllocSpec)
	assert.NoError(t, err)
	asiAllocSpec.Containers[0].Resource.CPU.CPUSet.SpreadStrategy = extunified.SpreadStrategySameCoreFirst
	asiAllocSpecWthSameCoreData, err := json.Marshal(asiAllocSpec)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		annotations map[string]string
		labels      map[string]string
	}{
		{
			name: "only koord resource spec",
			annotations: map[string]string{
				extension.AnnotationResourceSpec: string(koordResourceSpecData),
			},
			labels: map[string]string{extension.LabelPodQoS: string(extension.QoSLSE)},
		},
		{
			name: "only unified resource spec",
			annotations: map[string]string{
				uniext.AnnotationAllocSpec: string(unifiedResourceSpecWithSpreadData),
			},
			labels: map[string]string{extunified.LabelPodQoSClass: string(extension.QoSLSE)},
		},
		{
			name: "only asi resource spec",
			annotations: map[string]string{
				extunified.AnnotationAllocSpec: string(asiAllocSpecWthSpreadData),
			},
			labels: map[string]string{extunified.LabelPodQoSClass: string(extension.QoSLSE)},
		},
		{
			name: "if both koord and unified exists, use koord",
			annotations: map[string]string{
				extension.AnnotationResourceSpec: string(koordResourceSpecData),
				uniext.AnnotationAllocSpec:       string(unifiedResourceSpecWithSameCoreData),
			},
			labels: map[string]string{extunified.LabelPodQoSClass: string(extension.QoSBE), extension.LabelPodQoS: string(extension.QoSLSE)},
		},
		{
			name: "if both unified and asi exists, use unified",
			annotations: map[string]string{
				uniext.AnnotationAllocSpec:     string(unifiedResourceSpecWithSpreadData),
				extunified.AnnotationAllocSpec: string(asiAllocSpecWthSameCoreData),
			},
			labels: map[string]string{extunified.LabelPodQoSClass: string(extension.QoSLSE)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node-1",
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("8"),
							corev1.ResourceMemory: resource.MustParse("512Gi"),
						},
					},
				},
			}
			suit := newPluginTestSuit(t, nodes)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)
			cpuTopology := extension.CPUTopology{
				Detail: []extension.CPUInfo{
					{ID: 0, Core: 0, Socket: 0, Node: 0},
					{ID: 1, Core: 0, Socket: 0, Node: 0},
					{ID: 2, Core: 1, Socket: 0, Node: 0},
					{ID: 3, Core: 1, Socket: 0, Node: 0},
					{ID: 4, Core: 2, Socket: 0, Node: 0},
					{ID: 5, Core: 2, Socket: 0, Node: 0},
					{ID: 6, Core: 3, Socket: 0, Node: 0},
					{ID: 7, Core: 3, Socket: 0, Node: 0},
				},
			}
			cpuTopologyData, err := json.Marshal(cpuTopology)
			assert.NoError(t, err)
			resourceTopology := &v1alpha1.NodeResourceTopology{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-node-1",
					Annotations: map[string]string{extension.AnnotationNodeCPUTopology: string(cpuTopologyData)},
				},
				TopologyPolicies: nil,
				Zones:            nil,
			}
			_, err = suit.nrtClientSet.TopologyV1alpha1().NodeResourceTopologies().Create(context.TODO(), resourceTopology, metav1.CreateOptions{})
			assert.Nil(t, err)
			plg := p.(*Plugin)
			suit.start()
			time.Sleep(100 * time.Millisecond)
			nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get("test-node-1")
			assert.NoError(t, err)
			assert.NotNil(t, nodeInfo)
			_, err = suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), nodes[0], metav1.CreateOptions{})
			assert.Nil(t, err)

			ctx := context.TODO()
			cycleState := framework.NewCycleState()
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					UID:         uuid.NewUUID(),
					Namespace:   "default",
					Name:        "test-pod-1",
					Labels:      tt.labels,
					Annotations: tt.annotations,
				},
				Spec: corev1.PodSpec{
					Priority: pointer.Int32(extension.PriorityProdValueMax),
					Containers: []corev1.Container{
						{
							Name: "container-1",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("5"),
								},
							},
						},
					},
				},
			}
			_, err = suit.Handle.ClientSet().CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
			assert.Nil(t, err)

			status := plg.PreFilter(ctx, cycleState, pod)
			assert.Nil(t, status)
			status = plg.Filter(ctx, cycleState, pod, nodeInfo)
			assert.Nil(t, status)
			_, status = plg.Score(ctx, cycleState, pod, nodeInfo.Node().Name)
			assert.Nil(t, status)
			status = plg.Reserve(ctx, cycleState, pod, nodeInfo.Node().Name)
			assert.Nil(t, status)
			status = plg.PreBind(ctx, cycleState, pod, nodeInfo.Node().Name)
			assert.Nil(t, status)

			assert.Nil(t, err)
			assert.Equal(t, `{"cpuset":"0-2,4,6"}`, pod.Annotations[extension.AnnotationResourceStatus])
			assert.Equal(t, `{"cpu":[0,1,2,4,6],"gpu":{}}`, pod.Annotations[uniext.AnnotationAllocStatus])
			if tt.annotations[extunified.AnnotationAllocSpec] != "" {
				assert.Equal(t, `{"containers":[{"name":"container-1","resource":{"cpu":{"cpuSet":{"spreadStrategy":"spread","cpuIDs":[0,1,2,4,6]}}}}]}`, pod.Annotations[extunified.AnnotationAllocSpec])
			}
		})
	}
}

func TestPlugin_CPUSharePool(t *testing.T) {
	nodeName := "test-node-1"
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("512Gi"),
				},
			},
		},
	}
	suit := newPluginTestSuit(t, nodes)

	// register node event handler
	var nodeEventChan = make(chan int)
	suit.Handle.SharedInformerFactory().Core().V1().Nodes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			nodeEventChan <- 1
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			nodeEventChan <- 2
		},
		DeleteFunc: func(obj interface{}) {
			nodeEventChan <- 3
		},
	})

	// create plg, register pod eventHandler and topology eventHandler
	p, err := suit.proxyNew(suit.args, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)

	// register nrt event handler
	var nrtEventChan = make(chan int)
	suit.nrtInformerFactory.Topology().V1alpha1().NodeResourceTopologies().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			nodeResTopology, ok := obj.(*v1alpha1.NodeResourceTopology)
			if !ok {
				return
			}
			reportedCPUTopology, err := extension.GetCPUTopology(nodeResTopology.Annotations)
			assert.NoError(t, err)

			cpuTopology := convertCPUTopology(reportedCPUTopology)
			topologyManager := p.(*Plugin).GetCPUTopologyManager()
			topologyManager.UpdateCPUTopologyOptions(nodeResTopology.Name, func(options *nodenumaresource.CPUTopologyOptions) {
				*options = nodenumaresource.CPUTopologyOptions{
					CPUTopology: cpuTopology,
				}
			})
			nrtEventChan <- 1
		},
	})

	suit.start()

	// create node and nrt
	nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get("test-node-1")
	assert.NoError(t, err)
	assert.NotNil(t, nodeInfo)
	_, err = suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), nodes[0], metav1.CreateOptions{})
	assert.Nil(t, err)
	eventType := <-nodeEventChan
	assert.Equal(t, 1, eventType)
	cpuTopology := extension.CPUTopology{
		Detail: []extension.CPUInfo{
			{ID: 0, Core: 0, Socket: 0, Node: 0},
			{ID: 1, Core: 0, Socket: 0, Node: 0},
			{ID: 2, Core: 1, Socket: 0, Node: 0},
			{ID: 3, Core: 1, Socket: 0, Node: 0},
			{ID: 4, Core: 2, Socket: 0, Node: 0},
			{ID: 5, Core: 2, Socket: 0, Node: 0},
			{ID: 6, Core: 3, Socket: 0, Node: 0},
			{ID: 7, Core: 3, Socket: 0, Node: 0},
		},
	}
	cpuTopologyData, err := json.Marshal(cpuTopology)
	assert.NoError(t, err)
	resourceTopology := &v1alpha1.NodeResourceTopology{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node-1",
			Annotations: map[string]string{extension.AnnotationNodeCPUTopology: string(cpuTopologyData)},
		},
		TopologyPolicies: nil,
		Zones:            nil,
	}
	_, err = suit.nrtClientSet.TopologyV1alpha1().NodeResourceTopologies().Create(context.TODO(), resourceTopology, metav1.CreateOptions{})
	assert.NoError(t, err)
	nrtEvent := <-nrtEventChan
	assert.Equal(t, 1, nrtEvent)

	// create pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Namespace: "default",
			Name:      "test-pod-1",
			Labels:    map[string]string{extension.LabelPodQoS: string(extension.QoSLSE)},
			Annotations: map[string]string{
				extension.AnnotationResourceStatus: `{"cpuset":"0-2,4,6"}`,
			},
		},
		Spec: corev1.PodSpec{
			Priority: pointer.Int32(extension.PriorityProdValueMax),
			Containers: []corev1.Container{
				{
					Name: "container-1",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("5"),
						},
					},
				},
			},
		},
	}
	_, err = suit.Handle.ClientSet().CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
	assert.Nil(t, err)

	assert.Equal(t, "", pod.Spec.NodeName)
	// updateCPUSharePool on assignedPod Add and assignedPod update
	pod.Spec.NodeName = nodeName
	_, err = suit.Handle.ClientSet().CoreV1().Pods("default").Update(context.TODO(), pod, metav1.UpdateOptions{})
	assert.Nil(t, err)
	eventType = <-nodeEventChan
	assert.Equal(t, 2, eventType)
	nodeModified, err := suit.Handle.ClientSet().CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, `{"cpuIDs":[3,5,7]}`, nodeModified.Annotations[extunified.AnnotationNodeCPUSharePool])

	// updateCPUSharePool on assignedPod Delete
	err = suit.Handle.ClientSet().CoreV1().Pods("default").Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
	assert.Nil(t, err)
	eventType = <-nodeEventChan
	assert.Equal(t, 2, eventType)
	nodeModified, err = suit.Handle.ClientSet().CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, `{"cpuIDs":[0,1,2,3,4,5,6,7]}`, nodeModified.Annotations[extunified.AnnotationNodeCPUSharePool])
}

func convertCPUTopology(reportedCPUTopology *extension.CPUTopology) *nodenumaresource.CPUTopology {
	builder := nodenumaresource.NewCPUTopologyBuilder()
	for _, info := range reportedCPUTopology.Detail {
		builder.AddCPUInfo(int(info.Socket), int(info.Node), int(info.Core), int(info.ID))
	}
	return builder.Result()
}

func TestPlugin_MaxRefCount(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node-1",
				Labels: map[string]string{extunified.LabelCPUOverQuota: "1.5"},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("512Gi"),
				},
			},
		},
	}

	suit := newPluginTestSuit(t, nodes)

	topologyManager := nodenumaresource.NewCPUTopologyManager()
	assert.NotNil(t, topologyManager)
	registerNodeEventHandler(suit.Handle, topologyManager)
	suit.start()

	nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(nodes[0].Name)
	assert.NoError(t, err)
	assert.NotNil(t, nodeInfo)

	_, err = suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), nodes[0], metav1.CreateOptions{})
	assert.Nil(t, err)
	time.Sleep(100 * time.Millisecond)
	topologyOptions := topologyManager.GetCPUTopologyOptions(nodes[0].Name)
	assert.Equal(t, 2, topologyOptions.MaxRefCount)

	nodes[0].Labels[extunified.LabelCPUOverQuota] = "3.0"
	_, err = suit.Handle.ClientSet().CoreV1().Nodes().Update(context.TODO(), nodes[0], metav1.UpdateOptions{})
	assert.Nil(t, err)
	time.Sleep(100 * time.Millisecond)
	topologyOptions = topologyManager.GetCPUTopologyOptions(nodes[0].Name)
	assert.Equal(t, 3, topologyOptions.MaxRefCount)
}

func Test_allowUseCPUSet(t *testing.T) {
	unifiedCPUSet := uniext.ResourceAllocSpec{
		CPU: uniext.CPUBindStrategySameCoreFirst,
	}
	unifiedCPUSetData, err := json.Marshal(unifiedCPUSet)
	assert.NoError(t, err)
	unifiedCPUShare := uniext.ResourceAllocSpec{}
	unifiedCPUShareData, err := json.Marshal(unifiedCPUShare)
	assert.NoError(t, err)
	tests := []struct {
		name          string
		qosClass      extension.QoSClass
		priorityClass extension.PriorityClass
		annotations   map[string]string
		want          bool
	}{
		{
			name:          "unified cpuset",
			qosClass:      extension.QoSNone,
			priorityClass: extension.PriorityProd,
			annotations:   map[string]string{uniext.AnnotationAllocSpec: string(unifiedCPUSetData)},
			want:          true,
		},
		{
			name:          "unified cpushare",
			qosClass:      extension.QoSNone,
			priorityClass: extension.PriorityProd,
			annotations:   map[string]string{uniext.AnnotationAllocSpec: string(unifiedCPUShareData)},
			want:          false,
		},
		{
			name:          "koord",
			qosClass:      extension.QoSLSE,
			priorityClass: extension.PriorityProd,
			annotations:   nil,
			want:          true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					UID:         uuid.NewUUID(),
					Namespace:   "default",
					Name:        "test-pod-1",
					Labels:      map[string]string{extension.LabelPodQoS: string(tt.qosClass)},
					Annotations: tt.annotations,
				},
				Spec: corev1.PodSpec{Priority: &extension.PriorityProdValueMin},
			}
			assert.Equal(t, tt.want, allowUseCPUSet(pod))
		})
	}
}
