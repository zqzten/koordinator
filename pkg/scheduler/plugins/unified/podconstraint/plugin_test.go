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

package podconstraint

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	unifiedfake "gitlab.alibaba-inc.com/unischeduler/api/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
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
	"k8s.io/utils/pointer"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingconfig "github.com/koordinator-sh/koordinator/apis/scheduling/config"
	schedulingconfigv1beta2 "github.com/koordinator-sh/koordinator/apis/scheduling/config/v1beta2"
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
	*unifiedfake.Clientset
}

func proxyPluginFactory(fakeClientSet *unifiedfake.Clientset, factory runtime.PluginFactory) runtime.PluginFactory {
	return func(configuration apiruntime.Object, f framework.Handle) (framework.Plugin, error) {
		return factory(configuration, &frameworkHandleExtender{
			Handle:    f,
			Clientset: fakeClientSet,
		})
	}
}

type pluginTestSuit struct {
	framework.Handle
	unifiedClientSet *unifiedfake.Clientset
	proxyNew         runtime.PluginFactory
	args             apiruntime.Object
}

func newPluginTestSuit(t *testing.T, nodes []*corev1.Node) *pluginTestSuit {
	var v1beta2args schedulingconfigv1beta2.UnifiedPodConstraintArgs
	schedulingconfigv1beta2.SetDefaults_UnifiedPodConstraintArgs(&v1beta2args)
	var unifiedPodConstraintArgs schedulingconfig.UnifiedPodConstraintArgs
	err := schedulingconfigv1beta2.Convert_v1beta2_UnifiedPodConstraintArgs_To_config_UnifiedPodConstraintArgs(&v1beta2args, &unifiedPodConstraintArgs, nil)
	assert.NoError(t, err)
	unifiedPodConstraintArgs.EnableDefaultPodConstraint = pointer.BoolPtr(true)
	unifiedCPUSetAllocatorPluginConfig := scheduledconfig.PluginConfig{
		Name: Name,
		Args: &unifiedPodConstraintArgs,
	}

	unifiedClientSet := unifiedfake.NewSimpleClientset()

	proxyNew := proxyPluginFactory(unifiedClientSet, New)

	registeredPlugins := []schedulertesting.RegisterPluginFunc{
		func(reg *runtime.Registry, profile *scheduledconfig.KubeSchedulerProfile) {
			profile.PluginConfig = []scheduledconfig.PluginConfig{
				unifiedCPUSetAllocatorPluginConfig,
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
		Handle:           fh,
		unifiedClientSet: unifiedClientSet,
		proxyNew:         proxyNew,
		args:             &unifiedPodConstraintArgs,
	}
}

func (p *pluginTestSuit) start() {
	ctx := context.TODO()
	p.Handle.SharedInformerFactory().Start(ctx.Done())
	p.Handle.SharedInformerFactory().WaitForCacheSync(ctx.Done())
}

func TestPlugin_PreFilter(t *testing.T) {
	type args struct {
		cycleState *framework.CycleState
		pod        *corev1.Pod
	}
	tests := []struct {
		name                 string
		args                 args
		podConstraint        *v1beta1.PodConstraint
		expectPrefilterState *preFilterState
		expectSuccess        bool
		expectReason         []string
	}{
		{
			name: "normal pod without podConstraint",
			args: args{
				cycleState: framework.NewCycleState(),
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-pod",
					},
				},
			},
			podConstraint:        nil,
			expectSuccess:        true,
			expectPrefilterState: &preFilterState{},
		},
		{
			name: "normal pod with non-exist podConstraint",
			args: args{
				cycleState: framework.NewCycleState(),
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-pod",
						Labels: map[string]string{
							extunified.LabelPodConstraint: "test-pod-constraint",
						},
					},
				},
			},
			podConstraint:        nil,
			expectSuccess:        false,
			expectReason:         []string{ErrMissPodConstraint},
			expectPrefilterState: nil,
		},
		{
			name: "normal pod with podConstraint",
			args: args{
				cycleState: framework.NewCycleState(),
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-pod",
						Labels: map[string]string{
							extunified.LabelPodConstraint: "test",
						},
					},
				},
			},
			podConstraint: &v1beta1.PodConstraint{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: v1beta1.PodConstraintSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "node-type",
												Operator: corev1.NodeSelectorOpNotIn,
												Values:   []string{"DDR", "DDR2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectSuccess: true,
			expectReason:  nil,
			expectPrefilterState: &preFilterState{
				items: []*preFilterStateItem{
					{
						TopologySpreadConstraintState: &TopologySpreadConstraintState{
							PodConstraint:        nil,
							TpKeyToTotalMatchNum: map[string]int{},
							TpPairToMatchNum:     map[TopologyPair]int{},
							TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{},
						},
						Weight:       1,
						IgnoredNodes: sets.NewString(),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nil)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			if tt.podConstraint != nil {
				plg.podConstraintCache.SetPodConstraint(tt.podConstraint)
				for _, item := range tt.expectPrefilterState.items {
					item.PodConstraint = tt.podConstraint
				}
			}

			status := plg.PreFilter(context.TODO(), tt.args.cycleState, tt.args.pod)
			assert.Equal(t, tt.expectSuccess, status.IsSuccess())
			if !tt.expectSuccess {
				assert.Equal(t, tt.expectReason, status.Reasons())
			}

			state, status := getPreFilterState(tt.args.cycleState)
			assert.Equal(t, tt.expectPrefilterState, state)
		})
	}
}

func TestPodConstraintNotMatchNodeLabel(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}
	suit := newPluginTestSuit(t, []*corev1.Node{node})
	p, err := suit.proxyNew(suit.args, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)

	plg := p.(*Plugin)
	suit.start()

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: "test-tp-key",
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	cycleState := framework.NewCycleState()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: "test",
			},
		},
	}
	status := plg.PreFilter(context.TODO(), cycleState, pod)
	assert.True(t, status.IsSuccess())
	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	expectPrefilterState := &preFilterState{
		items: []*preFilterStateItem{
			{
				TopologySpreadConstraintState: &TopologySpreadConstraintState{
					PodConstraint: podConstraint,
					RequiredSpreadConstraints: []*TopologySpreadConstraint{
						{
							TopologyKey: "test-tp-key",
							MaxSkew:     1,
						},
					},
					TpKeyToTotalMatchNum: map[string]int{},
					TpPairToMatchNum:     map[TopologyPair]int{},
					TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
						"test-tp-key": NewTopologyCriticalPaths(),
					},
				},
				Weight:       1,
				IgnoredNodes: sets.NewString(),
			},
		},
	}
	assert.Equal(t, expectPrefilterState, state)

	nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(node.Name)
	assert.NoError(t, err)
	status = plg.Filter(context.TODO(), cycleState, pod, nodeInfo)
	assert.False(t, status.IsSuccess())
	expectReason := []string{ErrReasonNodeLabelNotMatch}
	assert.Equal(t, expectReason, status.Reasons())
}

func TestSpreadConstraintWithTopologyZone(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na620",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-4",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
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

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  2    |  2    |  2    |
	//  +-------+-------+-------+
	//
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-3"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-4"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-5"
	plg.podConstraintCache.AddPod(nodes[2], pod)
	pod.Name = "pod-6"
	plg.podConstraintCache.AddPod(nodes[2], pod)

	// normal prefilter
	cycleState := framework.NewCycleState()
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	status := plg.PreFilter(context.TODO(), cycleState, testPod)
	assert.True(t, status.IsSuccess())
	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	expectPrefilterState := &preFilterState{
		items: []*preFilterStateItem{
			{
				TopologySpreadConstraintState: &TopologySpreadConstraintState{
					PodConstraint: podConstraint,
					RequiredSpreadConstraints: []*TopologySpreadConstraint{
						{
							TopologyKey: corev1.LabelTopologyZone,
							MaxSkew:     1,
						},
					},
					TpKeyToTotalMatchNum: map[string]int{
						corev1.LabelTopologyZone: 6,
					},
					TpPairToMatchNum: map[TopologyPair]int{
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: 2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: 2,
					},
					TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
						corev1.LabelTopologyZone: {
							Min: CriticalPath{MatchNum: 2, TopologyValue: "na630"},
							Max: CriticalPath{MatchNum: 2, TopologyValue: "na620"},
						},
					},
				},
				Weight:       1,
				IgnoredNodes: sets.NewString(),
			},
		},
	}
	assert.Equal(t, expectPrefilterState, state)

	// normal filter
	testNode := nodes[3]
	for _, v := range []string{"na610", "na620", "na630"} {
		testNode.Labels[corev1.LabelTopologyZone] = v
		nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
		assert.NoError(t, err)
		status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
		assert.True(t, status.IsSuccess())
	}

	// add new Alloc in na630, skew greater than 1
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  2    |  2    |  3    |
	//  +-------+-------+-------+
	//
	testNode.Labels[corev1.LabelTopologyZone] = "na630"
	pod.Name = "pod-7"
	plg.podConstraintCache.AddPod(testNode, pod)

	// max skew not satisfy in "na630"
	testNode.Labels[corev1.LabelTopologyZone] = "na630"
	nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
	assert.NoError(t, err)
	status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
	assert.False(t, status.IsSuccess())
	assert.Equal(t, []string{ErrReasonConstraintsNotMatch}, status.Reasons())

	// max skew satisfy in "na610", "na620"
	for _, v := range []string{"na610", "na620"} {
		testNode.Labels[corev1.LabelTopologyZone] = v
		nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
		assert.NoError(t, err)
		status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
		assert.True(t, status.IsSuccess())
	}

	// remove new Alloc in na630, skew equal 1
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  2    |  2    |  2    |
	//  +-------+-------+-------+
	testNode.Labels[corev1.LabelTopologyZone] = "na630"
	plg.podConstraintCache.DeletePod(testNode, pod)

	// all satisfy
	for _, v := range []string{"na610", "na620", "na630"} {
		testNode.Labels[corev1.LabelTopologyZone] = v
		nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
		assert.NoError(t, err)
		status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
		assert.True(t, status.IsSuccess())
	}

	// validate MaxCount
	podConstraint.Spec.SpreadRule.Requires[0].MaxCount = pointer.Int32Ptr(2)
	plg.podConstraintCache.SetPodConstraint(podConstraint)
	for _, v := range []string{"na610", "na620", "na630"} {
		testNode.Labels[corev1.LabelTopologyZone] = v
		nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
		assert.NoError(t, err)
		status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
		assert.False(t, status.IsSuccess())
		assert.Equal(t, []string{ErrReasonMaxCountNotMatch}, status.Reasons())
	}

	// validate MinTopologyValue:
	// na630 already exists in the zone topology, so must be failed
	// na640 not exists, so must be success
	podConstraint.Spec.SpreadRule.Requires[0].MaxCount = nil
	podConstraint.Spec.SpreadRule.Requires[0].MinTopologyValue = pointer.Int32Ptr(4)
	plg.podConstraintCache.SetPodConstraint(podConstraint)
	for _, v := range []string{"na610", "na620", "na630"} {
		testNode.Labels[corev1.LabelTopologyZone] = v
		nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
		assert.NoError(t, err)
		status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
		assert.False(t, status.IsSuccess())
		assert.Equal(t, []string{ErrReasonConstraintsNotMatch}, status.Reasons())
	}
	testNode.Labels[corev1.LabelTopologyZone] = "na640"
	nodeInfo, err = suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
	assert.NoError(t, err)
	status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
	assert.True(t, status.IsSuccess())
}

func TestSpreadConstraintWithOneTopologyZone(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
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

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	//
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+
	//  | na610 |
	//  |-------+
	//  |  2    |
	//  +-------+
	//
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[0], pod)

	// normal prefilter
	cycleState := framework.NewCycleState()
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	status := plg.PreFilter(context.TODO(), cycleState, testPod)
	assert.True(t, status.IsSuccess())
	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	expectPrefilterState := &preFilterState{
		items: []*preFilterStateItem{
			{
				TopologySpreadConstraintState: &TopologySpreadConstraintState{
					PodConstraint: podConstraint,
					RequiredSpreadConstraints: []*TopologySpreadConstraint{
						{
							TopologyKey: corev1.LabelTopologyZone,
							MaxSkew:     1,
						},
					},
					TpKeyToTotalMatchNum: map[string]int{
						corev1.LabelTopologyZone: 2,
					},
					TpPairToMatchNum: map[TopologyPair]int{
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 2,
					},
					TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
						corev1.LabelTopologyZone: {
							Min: CriticalPath{MatchNum: 2, TopologyValue: "na610"},
							Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
						},
					},
				},
				Weight:       1,
				IgnoredNodes: sets.NewString(),
			},
		},
	}
	assert.Equal(t, expectPrefilterState, state)

	// normal filter
	testNode := nodes[1]
	testNode.Labels[corev1.LabelTopologyZone] = "na630"
	nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
	assert.NoError(t, err)
	status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
	assert.True(t, status.IsSuccess())
}

func TestSpreadConstraintWithTwoTopologyZoneButOneInvalid(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na620",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-4",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
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

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	//
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  2    |  2    |  0    |
	//  +-------+-------+-------+
	//
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[1], pod)

	// normal prefilter
	cycleState := framework.NewCycleState()
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	status := plg.PreFilter(context.TODO(), cycleState, testPod)
	assert.True(t, status.IsSuccess())
	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	expectPrefilterState := &preFilterState{
		items: []*preFilterStateItem{
			{
				TopologySpreadConstraintState: &TopologySpreadConstraintState{
					PodConstraint: podConstraint,
					RequiredSpreadConstraints: []*TopologySpreadConstraint{
						{
							TopologyKey: corev1.LabelTopologyZone,
							MaxSkew:     1,
						},
					},
					TpKeyToTotalMatchNum: map[string]int{
						corev1.LabelTopologyZone: 4,
					},
					TpPairToMatchNum: map[TopologyPair]int{
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: 2,
					},
					TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
						corev1.LabelTopologyZone: {
							Min: CriticalPath{MatchNum: 0, TopologyValue: "na630"},
							Max: CriticalPath{MatchNum: 2, TopologyValue: "na620"},
						},
					},
				},
				Weight:       1,
				IgnoredNodes: sets.NewString(),
			},
		},
	}
	assert.Equal(t, expectPrefilterState, state)

	// normal filter
	testNode := nodes[2]
	expectFilterStatuses := map[string]bool{
		"na610": false, // na610不能分配
		"na620": false, // na620不能分配
		"na630": true,
	}
	for zone, expectStatus := range expectFilterStatuses {
		testNode.Labels[corev1.LabelTopologyZone] = zone
		nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
		assert.NoError(t, err)
		status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
		assert.Equal(t, expectStatus, status.IsSuccess())
	}
}

func TestSpreadConstraintWithTopologyZoneAndRatio(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na620",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-4",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
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

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	// constructs PodConstraint with TopologyRatio
	// topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  20%  |  20%  |  60%  |
	//  +-------+-------+-------+
	//
	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey:   corev1.LabelTopologyZone,
						MaxSkew:       1,
						PodSpreadType: v1beta1.PodSpreadTypeRatio,
						TopologyRatios: []v1beta1.TopologyRatio{
							{
								TopologyValue: "na610",
								Ratio:         pointer.Int32Ptr(2),
							},
							{
								TopologyValue: "na620",
								Ratio:         pointer.Int32Ptr(2),
							},
							{
								TopologyValue: "na630",
								Ratio:         pointer.Int32Ptr(6),
							},
						},
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  2    |  2    |  2    |
	//  +-------+-------+-------+
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-3"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-4"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-5"
	plg.podConstraintCache.AddPod(nodes[2], pod)
	pod.Name = "pod-6"
	plg.podConstraintCache.AddPod(nodes[2], pod)

	// normal prefilter
	cycleState := framework.NewCycleState()
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	status := plg.PreFilter(context.TODO(), cycleState, testPod)
	assert.True(t, status.IsSuccess())
	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	expectPrefilterState := &preFilterState{
		items: []*preFilterStateItem{
			{
				TopologySpreadConstraintState: &TopologySpreadConstraintState{
					PodConstraint: podConstraint,
					RequiredSpreadConstraints: []*TopologySpreadConstraint{
						{
							TopologyKey: corev1.LabelTopologyZone,
							MaxSkew:     1,
							TopologyRatios: map[string]int{
								"na610": 2,
								"na620": 2,
								"na630": 6,
							},
							TopologySumRatio: 10,
						},
					},
					TpKeyToTotalMatchNum: map[string]int{
						corev1.LabelTopologyZone: 6,
					},
					TpPairToMatchNum: map[TopologyPair]int{
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: 2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: 2,
					},
					TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
						corev1.LabelTopologyZone: {
							Min: CriticalPath{MatchNum: 2, TopologyValue: "na630"},
							Max: CriticalPath{MatchNum: 2, TopologyValue: "na620"},
						},
					},
				},
				Weight:       1,
				IgnoredNodes: sets.NewString(),
			},
		},
	}
	assert.Equal(t, expectPrefilterState, state)

	// normal filter
	testNode := nodes[3]
	testNode.Labels[corev1.LabelTopologyZone] = "na630"
	nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
	assert.NoError(t, err)
	status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
	assert.True(t, status.IsSuccess())

	testNode.Labels[corev1.LabelTopologyZone] = "na610"
	nodeInfo, err = suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
	assert.NoError(t, err)
	status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
	assert.False(t, status.IsSuccess())
	assert.Equal(t, []string{ErrReasonConstraintsNotMatch}, status.Reasons())

	testNode.Labels[corev1.LabelTopologyZone] = "na620"
	nodeInfo, err = suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
	assert.NoError(t, err)
	status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
	assert.False(t, status.IsSuccess())
	assert.Equal(t, []string{ErrReasonConstraintsNotMatch}, status.Reasons())

	// add new Alloc in na630 align to 60%, all zone can be satisfied
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  2    |  2    |  6    |
	//  +-------+-------+-------+
	testNode.Labels[corev1.LabelTopologyZone] = "na630"
	for i := 0; i < 4; i++ {
		pod.Name = fmt.Sprintf("pod-extra-%d", i)
		plg.podConstraintCache.AddPod(testNode, pod)
	}
	for _, v := range []string{"na610", "na620", "na630"} {
		testNode.Labels[corev1.LabelTopologyZone] = v
		nodeInfo, err = suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
		assert.NoError(t, err)
		status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
		assert.True(t, status.IsSuccess())
	}
}

func TestSpreadConstraintWithTwoConstraints(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
					corev1.LabelHostname:     "test-node-1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na620",
					corev1.LabelHostname:     "test-node-2",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-3",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-4",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na640",
					corev1.LabelHostname:     "test-node-4",
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

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     10,
					},
					{
						TopologyKey: corev1.LabelHostname,
						MaxSkew:     1,
						MaxCount:    pointer.Int32Ptr(2),
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+-------+
	//  | na610 | na620 | na630 | na640 |
	//  |-------+-------+-------|-------|
	//  |  2    |  2    |  2    |  0    |
	//  +-------+-------+-------+-------+
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-3"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-4"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-5"
	plg.podConstraintCache.AddPod(nodes[2], pod)
	pod.Name = "pod-6"
	plg.podConstraintCache.AddPod(nodes[2], pod)

	// normal prefilter
	cycleState := framework.NewCycleState()
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	status := plg.PreFilter(context.TODO(), cycleState, testPod)
	assert.True(t, status.IsSuccess())
	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	expectPrefilterState := &preFilterState{
		items: []*preFilterStateItem{
			{
				TopologySpreadConstraintState: &TopologySpreadConstraintState{
					PodConstraint: podConstraint,
					RequiredSpreadConstraints: []*TopologySpreadConstraint{
						{
							TopologyKey: corev1.LabelTopologyZone,
							MaxSkew:     10,
						},
						{
							TopologyKey: corev1.LabelHostname,
							MaxSkew:     1,
							MaxCount:    2,
						},
					},
					TpKeyToTotalMatchNum: map[string]int{
						corev1.LabelTopologyZone: 6,
						corev1.LabelHostname:     6,
					},
					TpPairToMatchNum: map[TopologyPair]int{
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}:   2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}:   2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}:   2,
						{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-1"}: 2,
						{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-2"}: 2,
						{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-3"}: 2,
					},
					TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
						corev1.LabelTopologyZone: {
							Min: CriticalPath{MatchNum: 0, TopologyValue: "na640"},
							Max: CriticalPath{MatchNum: 2, TopologyValue: "na630"},
						},
						corev1.LabelHostname: {
							Min: CriticalPath{MatchNum: 0, TopologyValue: "test-node-4"},
							Max: CriticalPath{MatchNum: 2, TopologyValue: "test-node-3"},
						},
					},
				},
				Weight:       1,
				IgnoredNodes: sets.NewString(),
			},
		},
	}
	assert.Equal(t, expectPrefilterState, state)

	// normal filter
	testNode := nodes[3]
	hostnames := []string{"test-node-1", "test-node-2", "test-node-3"}
	for i, v := range []string{"na610", "na620", "na630"} {
		testNode.Labels[corev1.LabelTopologyZone] = v
		testNode.Labels[corev1.LabelHostname] = hostnames[i]
		nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
		assert.NoError(t, err)
		status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
		assert.False(t, status.IsSuccess())
		assert.Equal(t, []string{ErrReasonMaxCountNotMatch}, status.Reasons())
	}

	testNode.Labels[corev1.LabelTopologyZone] = "na640"
	testNode.Labels[corev1.LabelHostname] = "test-node-4"
	pod.Name = "pod-7"
	plg.podConstraintCache.AddPod(testNode, pod)

	nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(testNode.Name)
	assert.NoError(t, err)
	status = plg.Filter(context.TODO(), cycleState, testPod, nodeInfo)
	assert.True(t, status.IsSuccess())
}

func TestScore(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
					corev1.LabelHostname:     "test-node-1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na620",
					corev1.LabelHostname:     "test-node-2",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-3",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-4",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-4",
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

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	//
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+-------+
	//  | na610 | na620 | na630 | na640 |
	//  |-------+-------+-------|-------|
	//  |  1    |  2    |  2    |  0    |
	//  +-------+-------+-------+-------+
	//
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-3"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-4"
	plg.podConstraintCache.AddPod(nodes[2], pod)
	pod.Name = "pod-5"
	plg.podConstraintCache.AddPod(nodes[2], pod)

	// normal prefilter
	cycleState := framework.NewCycleState()
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	status := plg.PreFilter(context.TODO(), cycleState, testPod)
	assert.True(t, status.IsSuccess())
	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	expectPrefilterState := &preFilterState{
		items: []*preFilterStateItem{
			{
				TopologySpreadConstraintState: &TopologySpreadConstraintState{
					PodConstraint: podConstraint,
					RequiredSpreadConstraints: []*TopologySpreadConstraint{
						{
							TopologyKey: corev1.LabelTopologyZone,
							MaxSkew:     1,
						},
					},
					TpKeyToTotalMatchNum: map[string]int{
						corev1.LabelTopologyZone: 5,
					},
					TpPairToMatchNum: map[TopologyPair]int{
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 1,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: 2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: 2,
					},
					TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
						corev1.LabelTopologyZone: {
							Min: CriticalPath{MatchNum: 1, TopologyValue: "na610"},
							Max: CriticalPath{MatchNum: 2, TopologyValue: "na630"},
						},
					},
				},
				Weight:       1,
				IgnoredNodes: sets.NewString(),
			},
		},
	}
	assert.Equal(t, expectPrefilterState, state)

	status = plg.PreScore(context.TODO(), cycleState, testPod, nodes)
	assert.True(t, status.IsSuccess())

	testNode := nodes[3]
	var gotNodeScores framework.NodeScoreList
	for i, v := range []string{"na610", "na620", "na630", "na640"} {
		testNode.Labels[corev1.LabelTopologyZone] = v
		score, status := plg.Score(context.TODO(), cycleState, testPod, testNode.Name)
		assert.Nil(t, status)
		gotNodeScores = append(gotNodeScores, framework.NodeScore{
			Name:  fmt.Sprintf("%v-%d", testNode.Name, i),
			Score: score,
		})
	}
	expectNodeScores := framework.NodeScoreList{
		{
			Name:  "test-node-4-0",
			Score: 804,
		},
		{
			Name:  "test-node-4-1",
			Score: 1609,
		},
		{
			Name:  "test-node-4-2",
			Score: 1609,
		},
		{
			Name:  "test-node-4-3",
			Score: 0,
		},
	}
	assert.Equal(t, expectNodeScores, gotNodeScores)

	status = plg.NormalizeScore(context.TODO(), cycleState, pod, gotNodeScores)
	assert.True(t, status.IsSuccess())
	expectNodeScores = framework.NodeScoreList{
		{
			Name:  "test-node-4-0",
			Score: 50,
		},
		{
			Name:  "test-node-4-1",
			Score: 0,
		},
		{
			Name:  "test-node-4-2",
			Score: 0,
		},
		{
			Name:  "test-node-4-3",
			Score: 100,
		},
	}
	assert.Equal(t, expectNodeScores, gotNodeScores)
}

func TestScoreWithWeightedSpreadUnits(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
					corev1.LabelHostname:     "test-node-1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na620",
					corev1.LabelHostname:     "test-node-2",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-3",
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

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	//
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  1    |  2    |  2    |
	//  +-------+-------+-------+
	// constructs topology: kubernetes.io/hostname for appA-hostA
	//  +-------------+-------------+-------------+
	//  | test-node-1 | test-node-2 | test-node-3 |
	//  |-------------+-------------+-------------|
	//  |  1          |        2    |        2    |
	//  +-------------+-------------+-------------+
	//
	// constructs topology: kubernetes.io/hostname for appA-hostB
	//  +-------------+-------------+-------------+
	//  | test-node-1 | test-node-2 | test-node-3 |
	//  |-------------+-------------+-------------|
	//  |  0          |        0    |        0    |
	//  +-------------+-------------+-------------+
	//
	weightedSpreadUnits := []extunified.WeightedSpreadUnit{
		{
			Name:   "appA",
			Weight: 100,
		},
		{
			Name:   "appA-hostA",
			Weight: 10,
		},
	}
	weightedSpreadUnitsData, err := json.Marshal(weightedSpreadUnits)
	assert.NoError(t, err)
	assert.NotEmpty(t, weightedSpreadUnitsData)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Annotations: map[string]string{
				extunified.AnnotationWeightedSpreadUnits: string(weightedSpreadUnitsData),
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-3"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-4"
	plg.podConstraintCache.AddPod(nodes[2], pod)
	pod.Name = "pod-5"
	plg.podConstraintCache.AddPod(nodes[2], pod)

	// normal prefilter
	weightedSpreadUnits = []extunified.WeightedSpreadUnit{
		{
			Name:   "appA",
			Weight: 100,
		},
		{
			Name:   "appA-hostB",
			Weight: 10,
		},
	}
	weightedSpreadUnitsData, err = json.Marshal(weightedSpreadUnits)
	assert.NoError(t, err)
	assert.NotEmpty(t, weightedSpreadUnitsData)

	cycleState := framework.NewCycleState()
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Annotations: map[string]string{
				extunified.AnnotationWeightedSpreadUnits: string(weightedSpreadUnitsData),
			},
		},
	}
	status := plg.PreFilter(context.TODO(), cycleState, testPod)
	assert.True(t, status.IsSuccess())
	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	expectPrefilterState := &preFilterState{
		items: []*preFilterStateItem{
			{
				TopologySpreadConstraintState: &TopologySpreadConstraintState{
					SpreadTypeRequired:   false,
					DefaultPodConstraint: true,
					PodConstraint: plg.podConstraintCache.GetState(
						getNamespacedName("default", GetDefaultPodConstraintName("appA"))).PodConstraint,
					PreferredSpreadConstraints: []*TopologySpreadConstraint{
						{
							TopologyKey: corev1.LabelHostname,
							MaxSkew:     1,
						},
						{
							TopologyKey: corev1.LabelTopologyZone,
							MaxSkew:     1,
						},
					},
					TpKeyToTotalMatchNum: map[string]int{
						corev1.LabelHostname:     5,
						corev1.LabelTopologyZone: 5,
					},
					TpPairToMatchNum: map[TopologyPair]int{
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}:   1,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}:   2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}:   2,
						{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-1"}: 1,
						{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-2"}: 2,
						{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-3"}: 2,
					},
					TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
						corev1.LabelHostname: {
							Min: CriticalPath{MatchNum: 1, TopologyValue: "test-node-1"},
							Max: CriticalPath{MatchNum: 2, TopologyValue: "test-node-3"},
						},
						corev1.LabelTopologyZone: {
							Min: CriticalPath{MatchNum: 1, TopologyValue: "na610"},
							Max: CriticalPath{MatchNum: 2, TopologyValue: "na630"},
						},
					},
				},
				Weight:       100,
				IgnoredNodes: sets.NewString(),
			},
			{
				TopologySpreadConstraintState: &TopologySpreadConstraintState{
					SpreadTypeRequired:   false,
					DefaultPodConstraint: true,
					PodConstraint:        BuildDefaultPodConstraint(pod.Namespace, GetDefaultPodConstraintName("appA-hostB"), false),
					PreferredSpreadConstraints: []*TopologySpreadConstraint{
						{
							TopologyKey: corev1.LabelHostname,
							MaxSkew:     1,
						},
						{
							TopologyKey: corev1.LabelTopologyZone,
							MaxSkew:     1,
						},
					},
					TpKeyToTotalMatchNum: map[string]int{},
					TpPairToMatchNum:     map[TopologyPair]int{},
					TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
						corev1.LabelHostname:     NewTopologyCriticalPaths(),
						corev1.LabelTopologyZone: NewTopologyCriticalPaths(),
					},
				},
				Weight:       10,
				IgnoredNodes: sets.NewString(),
			},
		},
	}
	assert.Equal(t, expectPrefilterState, state)

	status = plg.PreScore(context.TODO(), cycleState, pod, nodes)
	assert.True(t, status.IsSuccess())

	var gotNodeScores framework.NodeScoreList
	for _, node := range nodes {
		score, status := plg.Score(context.TODO(), cycleState, testPod, node.Name)
		assert.Nil(t, status)
		gotNodeScores = append(gotNodeScores, framework.NodeScore{
			Name:  node.Name,
			Score: score,
		})
	}
	expectNodeScores := framework.NodeScoreList{
		{
			Name:  "test-node-1",
			Score: 80600,
		},
		{
			Name:  "test-node-2",
			Score: 161200,
		},
		{
			Name:  "test-node-3",
			Score: 161200,
		},
	}
	assert.Equal(t, expectNodeScores, gotNodeScores)

	status = plg.NormalizeScore(context.TODO(), cycleState, pod, gotNodeScores)
	assert.True(t, status.IsSuccess())
	expectNodeScores = framework.NodeScoreList{
		{
			Name:  "test-node-1",
			Score: 100,
		},
		{
			Name:  "test-node-2",
			Score: 50,
		},
		{
			Name:  "test-node-3",
			Score: 50,
		},
	}
	assert.Equal(t, expectNodeScores, gotNodeScores)
}

func TestScoreWithPreferredConstraints(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
					corev1.LabelHostname:     "test-node-1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na620",
					corev1.LabelHostname:     "test-node-2",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-3",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-4",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-4",
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

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Affinities: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	//
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+-------+
	//  | na610 | na620 | na630 | na640 |
	//  |-------+-------+-------|-------|
	//  |  1    |  2    |  2    |  0    |
	//  +-------+-------+-------+-------+
	//
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-3"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-4"
	plg.podConstraintCache.AddPod(nodes[2], pod)
	pod.Name = "pod-5"
	plg.podConstraintCache.AddPod(nodes[2], pod)

	// normal prefilter
	cycleState := framework.NewCycleState()
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	status := plg.PreFilter(context.TODO(), cycleState, testPod)
	assert.True(t, status.IsSuccess())
	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	expectPrefilterState := &preFilterState{
		items: []*preFilterStateItem{
			{
				TopologySpreadConstraintState: &TopologySpreadConstraintState{
					PodConstraint: podConstraint,
					PreferredSpreadConstraints: []*TopologySpreadConstraint{
						{
							TopologyKey: corev1.LabelTopologyZone,
							MaxSkew:     1,
						},
					},
					TpKeyToTotalMatchNum: map[string]int{
						corev1.LabelTopologyZone: 5,
					},
					TpPairToMatchNum: map[TopologyPair]int{
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 1,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: 2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: 2,
					},
					TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
						corev1.LabelTopologyZone: {
							Min: CriticalPath{MatchNum: 1, TopologyValue: "na610"},
							Max: CriticalPath{MatchNum: 2, TopologyValue: "na630"},
						},
					},
				},
				Weight:       1,
				IgnoredNodes: sets.NewString(),
			},
		},
	}
	assert.Equal(t, expectPrefilterState, state)

	status = plg.PreScore(context.TODO(), cycleState, testPod, nodes)
	assert.True(t, status.IsSuccess())

	testNode := nodes[3]
	var gotNodeScores framework.NodeScoreList
	for i, v := range []string{"na610", "na620", "na630", "na640"} {
		testNode.Labels[corev1.LabelTopologyZone] = v
		score, status := plg.Score(context.TODO(), cycleState, testPod, testNode.Name)
		assert.Nil(t, status)
		gotNodeScores = append(gotNodeScores, framework.NodeScore{
			Name:  fmt.Sprintf("%v-%d", testNode.Name, i),
			Score: score,
		})
	}
	expectNodeScores := framework.NodeScoreList{
		{
			Name:  "test-node-4-0",
			Score: 804,
		},
		{
			Name:  "test-node-4-1",
			Score: 1609,
		},
		{
			Name:  "test-node-4-2",
			Score: 1609,
		},
		{
			Name:  "test-node-4-3",
			Score: 0,
		},
	}
	assert.Equal(t, expectNodeScores, gotNodeScores)

	status = plg.NormalizeScore(context.TODO(), cycleState, pod, gotNodeScores)
	assert.True(t, status.IsSuccess())
	expectNodeScores = framework.NodeScoreList{
		{
			Name:  "test-node-4-0",
			Score: 50,
		},
		{
			Name:  "test-node-4-1",
			Score: 0,
		},
		{
			Name:  "test-node-4-2",
			Score: 0,
		},
		{
			Name:  "test-node-4-3",
			Score: 100,
		},
	}
	assert.Equal(t, expectNodeScores, gotNodeScores)
}

func TestScoreWithPreferredConstraintsIgnoreNodes(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
					corev1.LabelHostname:     "test-node-1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na620",
					corev1.LabelHostname:     "test-node-2",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-3",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-4",
				Labels: map[string]string{
					corev1.LabelHostname: "test-node-4",
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

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Affinities: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	//
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+-------+
	//  | na610 | na620 | na630 | na640 |
	//  |-------+-------+-------|-------|
	//  |  1    |  2    |  2    |  0    |
	//  +-------+-------+-------+-------+
	//
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-3"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-4"
	plg.podConstraintCache.AddPod(nodes[2], pod)
	pod.Name = "pod-5"
	plg.podConstraintCache.AddPod(nodes[2], pod)

	cycleState := framework.NewCycleState()
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	status := plg.PreFilter(context.TODO(), cycleState, testPod)
	assert.True(t, status.IsSuccess())

	status = plg.PreScore(context.TODO(), cycleState, pod, nodes)
	assert.True(t, status.IsSuccess())

	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	expectPrefilterState := &preFilterState{
		items: []*preFilterStateItem{
			{
				TopologySpreadConstraintState: &TopologySpreadConstraintState{
					PodConstraint: podConstraint,
					PreferredSpreadConstraints: []*TopologySpreadConstraint{
						{
							TopologyKey: corev1.LabelTopologyZone,
							MaxSkew:     1,
						},
					},
					TpKeyToTotalMatchNum: map[string]int{
						corev1.LabelTopologyZone: 5,
					},
					TpPairToMatchNum: map[TopologyPair]int{
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 1,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: 2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: 2,
					},
					TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
						corev1.LabelTopologyZone: {
							Min: CriticalPath{MatchNum: 1, TopologyValue: "na610"},
							Max: CriticalPath{MatchNum: 2, TopologyValue: "na630"},
						},
					},
				},
				Weight:                    1,
				TopologyNormalizingWeight: []float64{804.7189562170502},
				SpreadConstraintsInScore: []*TopologySpreadConstraint{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
				TpPairToMatchNumInScore: map[TopologyPair]int{
					{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 1,
					{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: 2,
					{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: 2,
				},
				IgnoredNodes: sets.NewString(nodes[3].Name),
			},
		},
	}
	assert.Equal(t, expectPrefilterState, state)

	var gotNodeScores framework.NodeScoreList
	for _, node := range nodes {
		score, status := plg.Score(context.TODO(), cycleState, testPod, node.Name)
		assert.Nil(t, status)
		gotNodeScores = append(gotNodeScores, framework.NodeScore{
			Name:  node.Name,
			Score: score,
		})
	}
	expectNodeScores := framework.NodeScoreList{
		{
			Name:  "test-node-1",
			Score: 804,
		},
		{
			Name:  "test-node-2",
			Score: 1609,
		},
		{
			Name:  "test-node-3",
			Score: 1609,
		},
		{
			Name:  "test-node-4",
			Score: 0,
		},
	}
	assert.Equal(t, expectNodeScores, gotNodeScores)

	status = plg.NormalizeScore(context.TODO(), cycleState, pod, gotNodeScores)
	assert.True(t, status.IsSuccess())
	expectNodeScores = framework.NodeScoreList{
		{
			Name:  "test-node-1",
			Score: 100,
		},
		{
			Name:  "test-node-2",
			Score: 49,
		},
		{
			Name:  "test-node-3",
			Score: 49,
		},
		{
			Name:  "test-node-4",
			Score: 0,
		},
	}
	assert.Equal(t, expectNodeScores, gotNodeScores)
}

func TestScoreWithTopologyRatio(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
					corev1.LabelHostname:     "test-node-1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na620",
					corev1.LabelHostname:     "test-node-2",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-3",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-4",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-4",
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

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	//
	// constructs PodConstraint with TopologyRatio
	// topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  20%  |  20%  |  60%  |
	//  +-------+-------+-------+
	//
	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey:   corev1.LabelTopologyZone,
						MaxSkew:       1,
						PodSpreadType: v1beta1.PodSpreadTypeRatio,
						TopologyRatios: []v1beta1.TopologyRatio{
							{
								TopologyValue: "na610",
								Ratio:         pointer.Int32Ptr(2),
							},
							{
								TopologyValue: "na620",
								Ratio:         pointer.Int32Ptr(2),
							},
							{
								TopologyValue: "na630",
								Ratio:         pointer.Int32Ptr(6),
							},
						},
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	//
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  2    |  2    |  2    |
	//  +-------+-------+-------+
	//
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-3"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-4"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-5"
	plg.podConstraintCache.AddPod(nodes[2], pod)
	pod.Name = "pod-6"
	plg.podConstraintCache.AddPod(nodes[2], pod)

	cycleState := framework.NewCycleState()
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	status := plg.PreFilter(context.TODO(), cycleState, testPod)
	assert.True(t, status.IsSuccess())

	state, status := getPreFilterState(cycleState)
	assert.True(t, status.IsSuccess())
	expectPrefilterState := &preFilterState{
		items: []*preFilterStateItem{
			{
				TopologySpreadConstraintState: &TopologySpreadConstraintState{
					PodConstraint: podConstraint,
					RequiredSpreadConstraints: []*TopologySpreadConstraint{
						{
							TopologyKey: corev1.LabelTopologyZone,
							MaxSkew:     1,
							TopologyRatios: map[string]int{
								"na610": 2,
								"na620": 2,
								"na630": 6,
							},
							TopologySumRatio: 10,
						},
					},
					TpKeyToTotalMatchNum: map[string]int{
						corev1.LabelTopologyZone: 6,
					},
					TpPairToMatchNum: map[TopologyPair]int{
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: 2,
						{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: 2,
					},
					TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
						corev1.LabelTopologyZone: {
							Min: CriticalPath{MatchNum: 2, TopologyValue: "na630"},
							Max: CriticalPath{MatchNum: 2, TopologyValue: "na620"},
						},
					},
				},
				Weight:       1,
				IgnoredNodes: sets.NewString(),
			},
		},
	}
	assert.Equal(t, expectPrefilterState, state)

	status = plg.PreScore(context.TODO(), cycleState, pod, nodes)
	assert.True(t, status.IsSuccess())

	testNode := nodes[3]
	var gotNodeScores framework.NodeScoreList
	for i, v := range []string{"na610", "na620", "na630", "na640"} {
		testNode.Labels[corev1.LabelTopologyZone] = v
		score, status := plg.Score(context.TODO(), cycleState, testPod, testNode.Name)
		assert.Nil(t, status)
		gotNodeScores = append(gotNodeScores, framework.NodeScore{
			Name:  fmt.Sprintf("%v-%d", testNode.Name, i),
			Score: score,
		})
	}
	expectNodeScores := framework.NodeScoreList{
		{
			Name:  "test-node-4-0",
			Score: 1609,
		},
		{
			Name:  "test-node-4-1",
			Score: 1609,
		},
		{
			Name:  "test-node-4-2",
			Score: 1609,
		},
		{
			Name:  "test-node-4-3",
			Score: 0,
		},
	}
	assert.Equal(t, expectNodeScores, gotNodeScores)

	status = plg.NormalizeScore(context.TODO(), cycleState, pod, gotNodeScores)
	assert.True(t, status.IsSuccess())
	expectNodeScores = framework.NodeScoreList{
		{
			Name:  "test-node-4-0",
			Score: 0,
		},
		{
			Name:  "test-node-4-1",
			Score: 0,
		},
		{
			Name:  "test-node-4-2",
			Score: 0,
		},
		{
			Name:  "test-node-4-3",
			Score: 100,
		},
	}
	assert.Equal(t, expectNodeScores, gotNodeScores)
}
