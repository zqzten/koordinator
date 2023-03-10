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

package cache

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	unifiedfake "gitlab.alibaba-inc.com/unischeduler/api/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"
	"k8s.io/utils/pointer"

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
		nodeInfos = append(nodeInfos, nodeInfoMap[node.Name])
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

func TestPodConstraintCache_AddConstraint(t *testing.T) {
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

	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newTestSharedLister(nil, nodes)
	fh, err := schedulertesting.NewFramework(
		[]schedulertesting.RegisterPluginFunc{
			schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
			schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
		},
		"koord-scheduler",
		runtime.WithClientSet(cs),
		runtime.WithInformerFactory(informerFactory),
		runtime.WithSnapshotSharedLister(snapshot),
	)
	assert.Nil(t, err)

	for _, node := range nodes {
		_, err := fh.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}
	unifiedClientSet := unifiedfake.NewSimpleClientset()
	podConstraintCache, err := NewPodConstraintCache(frameworkHandleExtender{
		Handle:    fh,
		Clientset: unifiedClientSet,
	}, true)
	assert.NoError(t, err)

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
	podConstraintCache.SetPodConstraint(podConstraint)

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
	podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-3"
	podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-4"
	podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-5"
	podConstraintCache.AddPod(nodes[2], pod)
	pod.Name = "pod-6"
	podConstraintCache.AddPod(nodes[2], pod)

	gotState := podConstraintCache.GetState(GetNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState := &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey:        corev1.LabelTopologyZone,
				MaxSkew:            1,
				NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
				NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
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
	}
	assert.Equal(t, expectState, gotState)
}

func TestPodConstraintCache_AddConstraintWithNewConstraint(t *testing.T) {
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
	}

	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newTestSharedLister(nil, nodes)
	fh, err := schedulertesting.NewFramework(
		[]schedulertesting.RegisterPluginFunc{
			schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
			schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
		},
		"koord-scheduler",
		runtime.WithClientSet(cs),
		runtime.WithInformerFactory(informerFactory),
		runtime.WithSnapshotSharedLister(snapshot),
	)
	assert.Nil(t, err)

	for _, node := range nodes {
		_, err := fh.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}
	unifiedClientSet := unifiedfake.NewSimpleClientset()
	podConstraintCache, err := NewPodConstraintCache(frameworkHandleExtender{
		Handle:    fh,
		Clientset: unifiedClientSet,
	}, true)
	assert.NoError(t, err)

	var ch = make(chan int)
	fh.SharedInformerFactory().Core().V1().Nodes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ch <- 1
		},
	})
	fh.SharedInformerFactory().Start(context.TODO().Done())
	fh.SharedInformerFactory().WaitForCacheSync(context.TODO().Done())
	<-ch

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
	podConstraintCache.SetPodConstraint(podConstraint)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodes[0].Name,
		},
	}
	podConstraintCache.AddPod(nodes[0], pod)

	gotState := podConstraintCache.GetState(GetNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState := &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey:        corev1.LabelTopologyZone,
				MaxSkew:            1,
				NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
				NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{
			corev1.LabelTopologyZone: 1,
		},
		TpPairToMatchNum: map[TopologyPair]int{
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 1,
		},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: 1, TopologyValue: "na610"},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""}},
		},
	}
	assert.Equal(t, expectState, gotState)

	podConstraint.Spec.SpreadRule.Requires = append(podConstraint.Spec.SpreadRule.Requires, v1beta1.SpreadRuleItem{
		TopologyKey: corev1.LabelHostname,
		MaxSkew:     1,
	})
	podConstraintCache.SetPodConstraint(podConstraint)

	gotState = podConstraintCache.GetState(GetNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState = &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey:        corev1.LabelTopologyZone,
				MaxSkew:            1,
				NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
				NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
			},
			{
				TopologyKey:        corev1.LabelHostname,
				MaxSkew:            1,
				NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
				NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{
			corev1.LabelTopologyZone: 1,
			corev1.LabelHostname:     1,
		},
		TpPairToMatchNum: map[TopologyPair]int{
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}:   1,
			{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-1"}: 1,
		},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: 1, TopologyValue: "na610"},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
			},
			corev1.LabelHostname: {
				Min: CriticalPath{MatchNum: 1, TopologyValue: "test-node-1"},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
			},
		},
	}
	assert.Equal(t, expectState, gotState)
}

func TestPodConstraintCache_AddConstraintWithDeleteOneConstraint(t *testing.T) {
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
	}
	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newTestSharedLister(nil, nodes)
	fh, err := schedulertesting.NewFramework(
		[]schedulertesting.RegisterPluginFunc{
			schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
			schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
		},
		"koord-scheduler",
		runtime.WithClientSet(cs),
		runtime.WithInformerFactory(informerFactory),
		runtime.WithSnapshotSharedLister(snapshot),
	)
	assert.Nil(t, err)

	for _, node := range nodes {
		_, err := fh.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}
	unifiedClientSet := unifiedfake.NewSimpleClientset()
	podConstraintCache, err := NewPodConstraintCache(frameworkHandleExtender{
		Handle:    fh,
		Clientset: unifiedClientSet,
	}, true)
	assert.NoError(t, err)

	var ch = make(chan int)
	fh.SharedInformerFactory().Core().V1().Nodes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ch <- 1
		},
	})
	fh.SharedInformerFactory().Start(context.TODO().Done())
	fh.SharedInformerFactory().WaitForCacheSync(context.TODO().Done())
	<-ch

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
					{
						TopologyKey: corev1.LabelHostname,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	podConstraintCache.SetPodConstraint(podConstraint)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodes[0].Name,
		},
	}
	podConstraintCache.AddPod(nodes[0], pod)

	gotState := podConstraintCache.GetState(GetNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState := &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey:        corev1.LabelTopologyZone,
				MaxSkew:            1,
				NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
				NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
			},
			{
				TopologyKey:        corev1.LabelHostname,
				MaxSkew:            1,
				NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
				NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{
			corev1.LabelTopologyZone: 1,
			corev1.LabelHostname:     1,
		},
		TpPairToMatchNum: map[TopologyPair]int{
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}:   1,
			{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-1"}: 1,
		},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: 1, TopologyValue: "na610"},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
			},
			corev1.LabelHostname: {
				Min: CriticalPath{MatchNum: 1, TopologyValue: "test-node-1"},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
			},
		},
	}
	assert.Equal(t, expectState, gotState)

	podConstraint = &v1beta1.PodConstraint{
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
	podConstraintCache.SetPodConstraint(podConstraint)

	gotState = podConstraintCache.GetState(GetNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState = &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey:        corev1.LabelTopologyZone,
				MaxSkew:            1,
				NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
				NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{
			corev1.LabelTopologyZone: 1,
		},
		TpPairToMatchNum: map[TopologyPair]int{
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 1,
		},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: 1, TopologyValue: "na610"},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""}},
		},
	}
	assert.Equal(t, expectState, gotState)
}

func TestPodConstraintCache_DelPodConstraint(t *testing.T) {
	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newTestSharedLister(nil, nil)
	fh, err := schedulertesting.NewFramework(
		[]schedulertesting.RegisterPluginFunc{
			schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
			schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
		},
		"koord-scheduler",
		runtime.WithClientSet(cs),
		runtime.WithInformerFactory(informerFactory),
		runtime.WithSnapshotSharedLister(snapshot),
	)
	assert.Nil(t, err)

	unifiedClientSet := unifiedfake.NewSimpleClientset()
	podConstraintCache, err := NewPodConstraintCache(frameworkHandleExtender{
		Handle:    fh,
		Clientset: unifiedClientSet,
	}, true)
	assert.NoError(t, err)

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
	podConstraintCache.SetPodConstraint(podConstraint)

	gotState := podConstraintCache.GetState(GetNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState := &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey:        corev1.LabelTopologyZone,
				MaxSkew:            1,
				NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
				NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{},
		TpPairToMatchNum:     map[TopologyPair]int{},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""}},
		},
	}
	assert.Equal(t, expectState, gotState)

	podConstraintCache.DelPodConstraint(podConstraint)

	gotState = podConstraintCache.GetState(GetNamespacedName(podConstraint.Namespace, podConstraint.Name))
	assert.Nil(t, gotState)
}

func TestConstraintTopologyValue(t *testing.T) {
	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newTestSharedLister(nil, nil)
	fh, err := schedulertesting.NewFramework(
		[]schedulertesting.RegisterPluginFunc{
			schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
			schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
		},
		"koord-scheduler",
		runtime.WithClientSet(cs),
		runtime.WithInformerFactory(informerFactory),
		runtime.WithSnapshotSharedLister(snapshot),
	)
	assert.Nil(t, err)
	unifiedClientSet := unifiedfake.NewSimpleClientset()
	podConstraintCache, err := NewPodConstraintCache(frameworkHandleExtender{
		Handle:    fh,
		Clientset: unifiedClientSet,
	}, true)
	assert.NoError(t, err)

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
	podConstraintCache.SetPodConstraint(podConstraint)

	gotState := podConstraintCache.GetState(GetNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState := &TopologySpreadConstraintState{
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
				TopologySumRatio:   10,
				NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
				NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{},
		TpPairToMatchNum:     map[TopologyPair]int{},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
			},
		},
	}
	assert.Equal(t, expectState, gotState)
}
