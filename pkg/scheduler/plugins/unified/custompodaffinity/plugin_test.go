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

package custompodaffinity

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	scheduledconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
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
	framework.Handle
	proxyNew runtime.PluginFactory
	args     apiruntime.Object
}

func newPluginTestSuit(t *testing.T, nodes []*corev1.Node) *pluginTestSuit {
	pluginConfig := scheduledconfig.PluginConfig{
		Name: Name,
		Args: nil,
	}

	registeredPlugins := []schedulertesting.RegisterPluginFunc{
		func(reg *runtime.Registry, profile *scheduledconfig.KubeSchedulerProfile) {
			profile.PluginConfig = []scheduledconfig.PluginConfig{
				pluginConfig,
			}
		},
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
		Handle:   fh,
		proxyNew: New,
		args:     nil,
	}
}

func (p *pluginTestSuit) start() {
	ctx := context.TODO()
	p.Handle.SharedInformerFactory().Start(ctx.Done())
	p.Handle.SharedInformerFactory().WaitForCacheSync(ctx.Done())
}

func TestMaxInstancePerHost(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node",
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

	testDeployUnit := "test-du"
	tests := []struct {
		name         string
		allocSpec    *extunified.AllocSpec
		assignedPods []*corev1.Pod
		pod          *corev1.Pod
		expectResult bool
	}{
		{
			name:         "satisfy maxInstancePerHost",
			expectResult: true,
			allocSpec: &extunified.AllocSpec{
				Affinity: &extunified.Affinity{
					PodAntiAffinity: &extunified.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []extunified.PodAffinityTerm{
							{
								PodAffinityTerm: corev1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      extunified.SigmaLabelServiceUnitName,
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{testDeployUnit},
											},
										},
									},
								},
								MaxCount: 2,
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-pod-1",
					Labels: map[string]string{
						extunified.SigmaLabelServiceUnitName: testDeployUnit,
					},
				},
			},
		},
		{
			name:         "failed satisfy maxInstancePerHost",
			expectResult: false,
			allocSpec: &extunified.AllocSpec{
				Affinity: &extunified.Affinity{
					PodAntiAffinity: &extunified.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []extunified.PodAffinityTerm{
							{
								PodAffinityTerm: corev1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      extunified.SigmaLabelServiceUnitName,
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{testDeployUnit},
											},
										},
									},
								},
								MaxCount: 2,
							},
						},
					},
				},
			},
			assignedPods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod-1",
						Labels: map[string]string{
							extunified.SigmaLabelServiceUnitName: testDeployUnit,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod-2",
						Labels: map[string]string{
							extunified.SigmaLabelServiceUnitName: testDeployUnit,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-pod-1",
					Labels: map[string]string{
						extunified.SigmaLabelServiceUnitName: testDeployUnit,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nodes)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get("test-node")
			assert.NoError(t, err)
			assert.NotNil(t, nodeInfo)

			data, err := json.Marshal(tt.allocSpec)
			assert.NoError(t, err)

			for _, v := range tt.assignedPods {
				if v.Annotations == nil {
					v.Annotations = make(map[string]string)
				}
				v.Annotations[extunified.AnnotationPodRequestAllocSpec] = string(data)
				plg.cache.AddPod(v.Spec.NodeName, v)
			}
			if tt.pod.Annotations == nil {
				tt.pod.Annotations = make(map[string]string)
			}
			tt.pod.Annotations[extunified.AnnotationPodRequestAllocSpec] = string(data)
			cycleState := framework.NewCycleState()
			_, status := plg.PreFilter(context.TODO(), cycleState, tt.pod)
			assert.Nil(t, status)
			status = plg.Filter(context.TODO(), cycleState, tt.pod, nodeInfo)
			if status.IsSuccess() != tt.expectResult {
				t.Fatalf("unexpected status result, expect: %v, status: %v, statusMessage: %s", tt.expectResult, status.IsSuccess(), status.Message())
			}
		})
	}
}

func TestMaxInstancePerHostUniProtocol(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node",
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

	assignedAppName := "app_1"
	assignedServiceUnitName := "role_1"

	tests := []struct {
		name             string
		assignedPods     []*corev1.Pod
		testSpreadPolicy *uniext.PodSpreadPolicy
		testPod          *corev1.Pod
		expectResult     bool
	}{
		{
			name:         "satisfy serviceUnit maxInstancePerHost",
			expectResult: true,
			assignedPods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod-1",
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			testSpreadPolicy: &uniext.PodSpreadPolicy{
				MaxInstancePerHost: 1,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						extunified.SigmaLabelAppName:         "app_2",
						extunified.SigmaLabelServiceUnitName: assignedServiceUnitName,
					},
				},
			},
			testPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-pod-1",
				},
			},
		},
		{
			name:         "failed serviceUnit maxInstancePerHost",
			expectResult: false,
			assignedPods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod-1",
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			testSpreadPolicy: &uniext.PodSpreadPolicy{
				MaxInstancePerHost: 1,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						extunified.SigmaLabelAppName:         assignedAppName,
						extunified.SigmaLabelServiceUnitName: assignedServiceUnitName,
					},
				},
			},
			testPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-pod-1",
				},
			},
		},
		{
			name:         "satisfy app maxInstancePerHost",
			expectResult: true,
			testSpreadPolicy: &uniext.PodSpreadPolicy{
				MaxInstancePerHost: 1,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						extunified.SigmaLabelAppName: "app_2",
					},
				},
			},
			testPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-pod-1",
				},
			},
		},
		{
			name:         "failed app maxInstancePerHost",
			expectResult: false,
			assignedPods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod-1",
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			testSpreadPolicy: &uniext.PodSpreadPolicy{
				MaxInstancePerHost: 1,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						extunified.SigmaLabelAppName: assignedAppName,
					},
				},
			},
			testPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-pod-1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nodes)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get("test-node")
			assert.NoError(t, err)
			assert.NotNil(t, nodeInfo)

			assignedPodSpreadPolicy := &uniext.PodSpreadPolicy{
				MaxInstancePerHost: 1,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						extunified.SigmaLabelAppName:         assignedAppName,
						extunified.SigmaLabelServiceUnitName: assignedServiceUnitName,
					},
				},
			}
			assignedData, err := json.Marshal(assignedPodSpreadPolicy)
			assert.NoError(t, err)
			for _, v := range tt.assignedPods {
				if v.Annotations == nil {
					v.Annotations = make(map[string]string)
				}
				v.Annotations[uniext.AnnotationPodSpreadPolicy] = string(assignedData)
				plg.cache.AddPod(v.Spec.NodeName, v)
			}

			if tt.testPod.Annotations == nil {
				tt.testPod.Annotations = make(map[string]string)
			}
			data, err := json.Marshal(tt.testSpreadPolicy)
			assert.NoError(t, err)
			tt.testPod.Annotations[uniext.AnnotationPodSpreadPolicy] = string(data)

			cycleState := framework.NewCycleState()
			_, status := plg.PreFilter(context.TODO(), cycleState, tt.testPod)
			assert.Nil(t, status)
			status = plg.Filter(context.TODO(), cycleState, tt.testPod, nodeInfo)
			if status.IsSuccess() != tt.expectResult {
				t.Fatalf("unexpected status result, expect: %v, status: %v, statusMessage: %s", tt.expectResult, status.IsSuccess(), status.Message())
			}
		})
	}
}

func TestPlugin_PreFilterExtensions(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node",
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

	suit := newPluginTestSuit(t, nodes)
	p, err := suit.proxyNew(suit.args, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)

	plg := p.(*Plugin)
	suit.start()

	nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get("test-node")
	assert.NoError(t, err)
	assert.NotNil(t, nodeInfo)

	assignedPodSpreadPolicy := &uniext.PodSpreadPolicy{
		MaxInstancePerHost: 3,
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				extunified.SigmaLabelAppName:         "app_2",
				extunified.SigmaLabelServiceUnitName: "role_1",
			},
		},
	}
	assignedData, err := json.Marshal(assignedPodSpreadPolicy)
	assert.NoError(t, err)
	assignedPods := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "assigned-pod-1",
			},
			Spec: corev1.PodSpec{
				NodeName: nodes[0].Name,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "assigned-pod-2",
			},
			Spec: corev1.PodSpec{
				NodeName: nodes[0].Name,
			},
		},
	}
	for _, v := range assignedPods {
		if v.Annotations == nil {
			v.Annotations = make(map[string]string)
		}
		v.Annotations[uniext.AnnotationPodSpreadPolicy] = string(assignedData)
		plg.cache.AddPod(v.Spec.NodeName, v)
	}

	testSpreadPolicy := &uniext.PodSpreadPolicy{
		MaxInstancePerHost: 3,
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				extunified.SigmaLabelAppName:         "app_2",
				extunified.SigmaLabelServiceUnitName: "role_1",
			},
		},
	}
	data, err := json.Marshal(testSpreadPolicy)
	assert.NoError(t, err)
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod-1",
		},
	}
	if testPod.Annotations == nil {
		testPod.Annotations = make(map[string]string)
	}
	testPod.Annotations[uniext.AnnotationPodSpreadPolicy] = string(data)

	// check original preFilterState
	cycleState := framework.NewCycleState()
	_, status := plg.PreFilter(context.TODO(), cycleState, testPod)
	assert.Nil(t, status)
	state, status := getPreFilterState(cycleState)
	assert.Nil(t, status)
	expectedState := preFilterState{
		podSpreadInfo: &extunified.PodSpreadInfo{
			AppName:     "app_2",
			ServiceUnit: "role_1",
		},
		maxInstancePerHost: 3,
		preemptivePods:     map[string]sets.String{},
	}
	assert.Equal(t, expectedState.podSpreadInfo, state.podSpreadInfo)
	assert.Equal(t, expectedState.maxInstancePerHost, state.maxInstancePerHost)
	assert.Equal(t, expectedState.preemptivePods, state.preemptivePods)

	// check RemovePod
	status = plg.RemovePod(context.TODO(), cycleState, testPod, framework.NewPodInfo(assignedPods[0]), nodeInfo)
	assert.Nil(t, status)
	state, status = getPreFilterState(cycleState)
	assert.Nil(t, status)
	expectedState = preFilterState{
		podSpreadInfo: &extunified.PodSpreadInfo{
			AppName:     "app_2",
			ServiceUnit: "role_1",
		},
		maxInstancePerHost: 3,
		preemptivePods:     map[string]sets.String{"test-node": sets.NewString("default/assigned-pod-1")},
	}
	assert.Equal(t, expectedState.podSpreadInfo, state.podSpreadInfo)
	assert.Equal(t, expectedState.maxInstancePerHost, state.maxInstancePerHost)
	assert.Equal(t, expectedState.preemptivePods, state.preemptivePods)

	// check AddPod
	status = plg.AddPod(context.TODO(), cycleState, testPod, framework.NewPodInfo(assignedPods[0]), nodeInfo)
	assert.Nil(t, status)
	state, status = getPreFilterState(cycleState)
	assert.Nil(t, status)
	expectedState = preFilterState{
		podSpreadInfo: &extunified.PodSpreadInfo{
			AppName:     "app_2",
			ServiceUnit: "role_1",
		},
		maxInstancePerHost: 3,
		preemptivePods:     map[string]sets.String{},
	}
	assert.Equal(t, expectedState.podSpreadInfo, state.podSpreadInfo)
	assert.Equal(t, expectedState.maxInstancePerHost, state.maxInstancePerHost)
	assert.Equal(t, expectedState.preemptivePods, state.preemptivePods)

	// check AddDuplicatePod
	status = plg.AddPod(context.TODO(), cycleState, testPod, framework.NewPodInfo(assignedPods[0]), nodeInfo)
	assert.Nil(t, status)
	state, status = getPreFilterState(cycleState)
	assert.Nil(t, status)
	expectedState = preFilterState{
		podSpreadInfo: &extunified.PodSpreadInfo{
			AppName:     "app_2",
			ServiceUnit: "role_1",
		},
		maxInstancePerHost: 3,
		preemptivePods:     map[string]sets.String{},
	}
	assert.Equal(t, expectedState.podSpreadInfo, state.podSpreadInfo)
	assert.Equal(t, expectedState.maxInstancePerHost, state.maxInstancePerHost)
	assert.Equal(t, expectedState.preemptivePods, state.preemptivePods)

	// check Delete-NoMatch Pod
	status = plg.RemovePod(context.TODO(), cycleState, testPod, framework.NewPodInfo(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "assigned-pod-3",
		},
			Spec: corev1.PodSpec{
				NodeName: nodes[0].Name,
			},
		}), nodeInfo)
	assert.Nil(t, status)
	state, status = getPreFilterState(cycleState)
	assert.Nil(t, status)
	expectedState = preFilterState{
		podSpreadInfo: &extunified.PodSpreadInfo{
			AppName:     "app_2",
			ServiceUnit: "role_1",
		},
		maxInstancePerHost: 3,
		preemptivePods:     map[string]sets.String{},
	}
	assert.Equal(t, expectedState.podSpreadInfo, state.podSpreadInfo)
	assert.Equal(t, expectedState.maxInstancePerHost, state.maxInstancePerHost)
	assert.Equal(t, expectedState.preemptivePods, state.preemptivePods)
}
