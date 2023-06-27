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

package overquota

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	scheduledconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/util/transformer"
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
		schedulertesting.RegisterPreFilterPlugin(Name, New),
		schedulertesting.RegisterFilterPlugin(Name, New),
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

func TestPlugin_PreFilter(t *testing.T) {
	tests := []struct {
		name                   string
		cycleState             *framework.CycleState
		pod                    *corev1.Pod
		localInlineVolumeSize  int64
		expectedPrefilterState *preFilterState
	}{
		{
			name:       "normal pod",
			cycleState: framework.NewCycleState(),
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
			expectedPrefilterState: &preFilterState{
				isPodRequireOverQuotaNode: false,
				checkOverQuota:            true,
				podRequestedResource: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
			},
		},
		{
			name:       "empty resources pod",
			cycleState: framework.NewCycleState(),
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: corev1.PodSpec{},
			},
			expectedPrefilterState: &preFilterState{
				isPodRequireOverQuotaNode: false,
				checkOverQuota:            false,
				podRequestedResource:      corev1.ResourceList{},
			},
		},
		{
			name:       "empty resources pod but has local inline volume",
			cycleState: framework.NewCycleState(),
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: corev1.PodSpec{},
			},
			localInlineVolumeSize: 10 * 1024 * 1024 * 1024,
			expectedPrefilterState: &preFilterState{
				isPodRequireOverQuotaNode: false,
				checkOverQuota:            true,
				podRequestedResource:      corev1.ResourceList{},
			},
		},
		{
			name:       "enable over quota",
			cycleState: framework.NewCycleState(),
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
					Annotations: map[string]string{
						extunified.AnnotationDisableOverQuotaFilter: "true",
					},
				},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      extunified.LabelEnableOverQuota,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"true"},
											},
											{
												Key:      "dummy-label",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"test"},
											},
										},
									},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
			expectedPrefilterState: &preFilterState{
				isPodRequireOverQuotaNode: true,
				checkOverQuota:            false,
				podRequestedResource: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
			},
		},
		{
			name:       "set over quota affinity but false",
			cycleState: framework.NewCycleState(),
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
					Annotations: map[string]string{
						extunified.AnnotationDisableOverQuotaFilter: "true",
					},
				},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      extunified.LabelEnableOverQuota,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"false"},
											},
											{
												Key:      "dummy-label",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"test"},
											},
										},
									},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
			expectedPrefilterState: &preFilterState{
				isPodRequireOverQuotaNode: false,
				checkOverQuota:            false,
				podRequestedResource: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
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
			GetLocalInlineVolumeSize = func(volumes []corev1.Volume, storageClassLister storagelisters.StorageClassLister) int64 {
				return tt.localInlineVolumeSize
			}
			_, status := plg.PreFilter(context.TODO(), tt.cycleState, tt.pod)
			assert.True(t, status.IsSuccess())
			state, status := getPreFilterState(tt.cycleState)
			assert.True(t, status.IsSuccess())
			assert.Equal(t, tt.expectedPrefilterState, state)
		})
	}
}

func TestPlugin_Filter(t *testing.T) {
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name       string
		cycleState *framework.CycleState
		pod        *corev1.Pod
		node       *corev1.Node
	}{
		{
			name:       "normal pod",
			cycleState: framework.NewCycleState(),
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: corev1.PodSpec{},
			},
			node: &corev1.Node{},
		},
		{
			name:       "normal pod with normal node",
			cycleState: framework.NewCycleState(),
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("17"),
								},
							},
						},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("16"),
					},
				},
			},
		},
		{
			name:       "enable over quota",
			cycleState: framework.NewCycleState(),
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
					Annotations: map[string]string{
						extunified.AnnotationDisableOverQuotaFilter: "true",
					},
				},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      extunified.LabelEnableOverQuota,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"true"},
											},
											{
												Key:      "dummy-label",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"test"},
											},
										},
									},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("8"),
								},
							},
						},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						extunified.LabelEnableOverQuota: "true",
					},
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("16"),
					},
				},
			},
		},
		{
			name:       "disable over quota check but satisfy over-quota node",
			cycleState: framework.NewCycleState(),
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
					Annotations: map[string]string{
						extunified.AnnotationDisableOverQuotaFilter: "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("8"),
								},
							},
						},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						extunified.LabelEnableOverQuota: "true",
					},
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("16"),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj, err := transformer.TransformNode(tt.node)
			assert.NoError(t, err)
			node := obj.(*corev1.Node)
			suit := newPluginTestSuit(t, []*corev1.Node{node})
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get(node.Name)
			assert.NoError(t, err)
			assert.NotNil(t, nodeInfo)
			assert.NotNil(t, nodeInfo.Node())

			_, status := plg.PreFilter(context.TODO(), tt.cycleState, tt.pod)
			assert.True(t, status.IsSuccess())
			status = plg.Filter(context.TODO(), tt.cycleState, tt.pod, nodeInfo)
			assert.True(t, status.IsSuccess())
		})
	}
}
