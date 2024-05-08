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

package eci

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	scheduledconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"
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

func (f *testSharedLister) StorageInfos() framework.StorageInfoLister {
	return f
}

func (f *testSharedLister) IsPVCUsedByPods(key string) bool {
	return false
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
		schedulertesting.RegisterFilterPlugin(Name, New),
		schedulertesting.RegisterScorePlugin(Name, New, 1),
		schedulertesting.RegisterPreBindPlugin(Name, New),
	}

	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newTestSharedLister(nil, nodes)
	fh, err := schedulertesting.NewFramework(
		context.TODO(),
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

func TestPlugin_Filter(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "vk-node",
				Labels: map[string]string{uniext.LabelCommonNodeType: uniext.VKType},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("96"),
					corev1.ResourceMemory: resource.MustParse("512Gi"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "normal-node",
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
	type args struct {
		pod      *corev1.Pod
		nodeName string
	}
	tests := []struct {
		name string
		args args
		want *framework.Status
	}{
		{
			name: "node is VKNode and pod ECIRequired",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-pod-1",
						Labels:    map[string]string{uniext.LabelECIAffinity: uniext.ECIRequired},
					},
				},
				nodeName: "vk-node",
			},
			want: nil,
		},
		{
			name: "node is VKNode and pod ECIRequiredNot",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-pod-1",
						Labels:    map[string]string{uniext.LabelECIAffinity: uniext.ECIRequiredNot},
					},
				},
				nodeName: "vk-node",
			},
			want: framework.NewStatus(framework.Unschedulable, ErrReasonCannotRunOnECI),
		},
		{
			name: "node isn't VKNode and pod ECIRequired",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-pod-1",
						Labels:    map[string]string{uniext.LabelECIAffinity: uniext.ECIRequired},
					},
				},
				nodeName: "normal-node",
			},
			want: framework.NewStatus(framework.Unschedulable, ErrReasonMustRunOnECI),
		},
		{
			name: "node isn't VKNode and pod ECIRequiredNot",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-pod-1",
						Labels:    map[string]string{uniext.LabelECIAffinity: uniext.ECIRequiredNot},
					},
				},
				nodeName: "normal-node",
			},
			want: nil,
		},
		{
			name: "node isn't VKNode and pod doesn't have eci label",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-pod-1",
					},
				},
				nodeName: "normal-node",
			},
			want: nil,
		},
		{
			name: "node isn't VKNode and pod doesn't have eci label",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-pod-1",
					},
				},
				nodeName: "normal-node",
			},
			want: nil,
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

			nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(tt.args.nodeName)
			assert.NoError(t, err)
			assert.NotNil(t, nodeInfo)

			if got := plg.Filter(context.TODO(), nil, tt.args.pod, nodeInfo); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Filter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlugin_Score(t *testing.T) {
	preferNotUseECILabels := map[string]string{
		uniext.LabelECIAffinity: uniext.ECIPreferredNot,
	}
	preferUseECILabels := map[string]string{
		uniext.LabelECIAffinity: uniext.ECIPreferred,
	}
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "vk-node",
				Labels: map[string]string{uniext.LabelCommonNodeType: uniext.VKType},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("96"),
					corev1.ResourceMemory: resource.MustParse("512Gi"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "normal-node",
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
	tests := []struct {
		pod          *corev1.Pod
		wantErr      error
		expectedList framework.NodeScoreList
		name         string
	}{
		{
			// pod without eci label
			// Node1 is vk node,Score =  MinNodeScore
			// Node2 is normal node, Score: (MaxNodeScore + MinNodeScore) / 2
			name: "default pod",
			pod:  &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-pod-0"}},
			expectedList: []framework.NodeScore{
				{Score: framework.MinNodeScore},
				{Score: (framework.MaxNodeScore + framework.MinNodeScore) / 2},
			},
		},
		{
			// pod with eci label = ECIPreferredNot
			// Node1 is vk node, for pod with eci label = false, score = skeleton.MaxNodeScore / 4
			// Node2 is normal node, Score: (MaxNodeScore + MinNodeScore) / 2
			name: "pod with eci label = ECIPreferredNot",
			pod:  &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-pod-0", Labels: preferNotUseECILabels}},
			expectedList: []framework.NodeScore{
				{Score: framework.MaxNodeScore / 4},
				{Score: (framework.MaxNodeScore + framework.MinNodeScore) / 2},
			},
		},
		{
			// pod with eci label = ECIPreferred
			// Node1 is vk node, for pod with eci label = false, score = MaxNodeScore/4*3
			// Node2 is normal node, Score: (MaxNodeScore + MinNodeScore) / 2
			name: "pod with eci label = ECIPreferred",
			pod:  &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-pod-0", Labels: preferUseECILabels}},
			expectedList: []framework.NodeScore{
				{Score: framework.MaxNodeScore / 4 * 3},
				{Score: (framework.MaxNodeScore + framework.MinNodeScore) / 2},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nodes)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			for i := range nodes {
				score, status := plg.Score(context.TODO(), nil, test.pod, nodes[i].Name)
				if status != nil {
					t.Errorf("unexpected error: %v", status)
				}
				if !reflect.DeepEqual(test.expectedList[i].Score, score) {
					t.Errorf("expected %#v, got %#v", test.expectedList[i].Score, score)
				}
			}
		})
	}
}

func TestPlugin_PreBind(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "vk-node",
				Labels: map[string]string{uniext.LabelNodeType: uniext.VKType},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("96"),
					corev1.ResourceMemory: resource.MustParse("512Gi"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "normal-node",
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
	type args struct {
		*corev1.Node
		*corev1.Pod
	}
	tests := []struct {
		name        string
		args        args
		want        *framework.Status
		wantVKLabel string
	}{
		{
			name: "vk node without label",
			args: args{
				Node: nodes[0],
				Pod:  &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-pod-0"}},
			},
			want:        nil,
			wantVKLabel: uniext.VKType,
		},
		{
			name: "normal node",
			args: args{
				Node: nodes[1],
				Pod:  &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-pod-0"}},
			},
			want:        nil,
			wantVKLabel: "",
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
			_, err = suit.Handle.ClientSet().CoreV1().Pods("default").Create(context.TODO(), tt.args.Pod, metav1.CreateOptions{})
			assert.Nil(t, err)

			if got := plg.PreBind(context.TODO(), nil, tt.args.Pod, tt.args.Node.Name); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PreBind() = %v, want %v", got, tt.want)
			}
			assert.Equal(t, tt.wantVKLabel, tt.args.Pod.Labels[uniext.LabelCommonNodeType])
		})
	}
}
