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

package resourcepolicy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/apis/core"
	scheduledconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	koordfake "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned/fake"
	koordinatorinformers "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions"
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
	koordClient          *koordfake.Clientset
	koordInformerFactory koordinatorinformers.SharedInformerFactory
	proxyNew             runtime.PluginFactory
	args                 apiruntime.Object
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

	koordcs := koordfake.NewSimpleClientset()
	koordInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordcs, 0)
	return &pluginTestSuit{
		Handle:               fh,
		koordClient:          koordcs,
		koordInformerFactory: koordInformerFactory,
		proxyNew: func(configuration apiruntime.Object, f framework.Handle) (framework.Plugin, error) {
			return &Plugin{
				handle:               fh,
				resourcePolicyLister: koordInformerFactory.Scheduling().V1alpha1().ResourcePolicies().Lister(),
			}, nil
		},
	}
}

func (p *pluginTestSuit) start() {
	ctx := context.TODO()
	p.Handle.SharedInformerFactory().Start(ctx.Done())
	p.Handle.SharedInformerFactory().WaitForCacheSync(ctx.Done())
	p.koordInformerFactory.Start(ctx.Done())
	p.koordInformerFactory.WaitForCacheSync(ctx.Done())
}

func TestPlugin_PreBind(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "normal-node",
				Labels: map[string]string{"elastic-pool": "normal", "az": "hz"},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("512Gi"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "hot-node",
				Labels: map[string]string{"elastic-pool": "hot", "az": "hz"},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("512Gi"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "cold-node",
				Labels: map[string]string{"elastic-pool": "cold", "az": "sz"},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("512Gi"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "uncharted-node",
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("512Gi"),
				},
			},
		},
	}

	tests := []struct {
		name             string
		pod              *corev1.Pod
		resourcePolicies []*schedulingv1alpha1.ResourcePolicy
		nodeName         string
		want             string
	}{
		{
			name: "normal",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: corev1.NamespaceDefault,
					Name:      "test-pod-1",
				},
			},
			resourcePolicies: []*schedulingv1alpha1.ResourcePolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "default-resource-policy"},
				Spec: schedulingv1alpha1.ResourcePolicySpec{
					Units: []schedulingv1alpha1.ResourcePolicyUnit{
						{
							Name:         "normal",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "normal"}},
						},
						{
							Name:         "hot",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "hot"}},
						},
						{
							Name:         "cold",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "cold"}},
						},
					},
				},
			}},
			nodeName: "normal-node",
			want:     "100",
		},
		{
			name: "hot",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: corev1.NamespaceDefault,
					Name:      "test-pod-1",
				},
			},
			resourcePolicies: []*schedulingv1alpha1.ResourcePolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "default-resource-policy"},
				Spec: schedulingv1alpha1.ResourcePolicySpec{
					Units: []schedulingv1alpha1.ResourcePolicyUnit{
						{
							Name:         "normal",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "normal"}},
						},
						{
							Name:         "hot",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "hot"}},
						},
						{
							Name:         "cold",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "cold"}},
						},
					},
				},
			}},
			nodeName: "hot-node",
			want:     "67",
		},
		{
			name: "hot;multi-resource-policy",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: corev1.NamespaceDefault,
					Name:      "test-pod-1",
				},
			},
			resourcePolicies: []*schedulingv1alpha1.ResourcePolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "first-resource-policy",
						CreationTimestamp: metav1.Time{Time: metav1.Now().Add(time.Minute * 120)},
					},
					Spec: schedulingv1alpha1.ResourcePolicySpec{
						Units: []schedulingv1alpha1.ResourcePolicyUnit{
							{
								Name:         "hot",
								NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "hot"}},
							},
							{
								Name:         "normal",
								NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "normal"}},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "second-resource-policy",
						CreationTimestamp: metav1.Now(),
					},
					Spec: schedulingv1alpha1.ResourcePolicySpec{
						Units: []schedulingv1alpha1.ResourcePolicyUnit{
							{
								Name:         "normal",
								NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "normal"}},
							},
							{
								Name:         "hot",
								NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "hot"}},
							},
							{
								Name:         "cold",
								NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "cold"}},
							},
						},
					},
				},
			},
			nodeName: "hot-node",
			want:     "100",
		},
		{
			name: "cold",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: corev1.NamespaceDefault,
					Name:      "test-pod-1",
				},
			},
			resourcePolicies: []*schedulingv1alpha1.ResourcePolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "default-resource-policy"},
				Spec: schedulingv1alpha1.ResourcePolicySpec{
					Units: []schedulingv1alpha1.ResourcePolicyUnit{
						{
							Name:         "normal",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "normal"}},
						},
						{
							Name:         "hot",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "hot"}},
						},
						{
							Name:         "cold",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "cold"}},
						},
					},
				},
			}},
			nodeName: "cold-node",
			want:     "34",
		},
		{
			name: "cold;pod-deletion-cost exists;",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   corev1.NamespaceDefault,
					Name:        "test-pod-1",
					Annotations: map[string]string{core.PodDeletionCost: "1"},
				},
			},
			resourcePolicies: []*schedulingv1alpha1.ResourcePolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "default-resource-policy"},
				Spec: schedulingv1alpha1.ResourcePolicySpec{
					Units: []schedulingv1alpha1.ResourcePolicyUnit{
						{
							Name:         "normal",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "normal"}},
						},
						{
							Name:         "hot",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "hot"}},
						},
						{
							Name:         "cold",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "cold"}},
						},
					},
				},
			}},
			nodeName: "cold-node",
			want:     "1",
		},
		{
			name: "uncharted",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: corev1.NamespaceDefault,
					Name:      "test-pod-1",
				},
			},
			resourcePolicies: []*schedulingv1alpha1.ResourcePolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "default-resource-policy"},
				Spec: schedulingv1alpha1.ResourcePolicySpec{
					Units: []schedulingv1alpha1.ResourcePolicyUnit{
						{
							Name:         "normal",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "normal"}},
						},
						{
							Name:         "hot",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "hot"}},
						},
						{
							Name:         "cold",
							NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"elastic-pool": "cold"}},
						},
					},
				},
			}},
			nodeName: "uncharted-node",
			want:     "0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nodes)

			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			_, err = suit.Handle.ClientSet().CoreV1().Pods(tt.pod.Namespace).Create(context.TODO(), tt.pod, metav1.CreateOptions{})
			assert.NoError(t, err)

			for _, resourcePolicy := range tt.resourcePolicies {
				_, err = suit.koordClient.SchedulingV1alpha1().ResourcePolicies().Create(context.TODO(), resourcePolicy, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			plg := p.(*Plugin)
			suit.start()

			status := plg.PreBind(context.TODO(), framework.NewCycleState(), tt.pod, tt.nodeName)
			assert.Nil(t, status)
			assert.Equal(t, tt.want, tt.pod.Annotations[core.PodDeletionCost])
		})
	}
}
