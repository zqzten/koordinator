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
	"reflect"
	"testing"

	nrtfake "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	uniapiext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	"gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension/cpuset"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/informers"
	clientsetfake "k8s.io/client-go/kubernetes/fake"

	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
)

type AllocatedStatus struct {
	cpuset.CPUInfo
	// 核上共用的容器数
	Count int
	// 是否为独占核
	Exclusive bool
}

type PodAllocatedStatus map[string]map[int]*AllocatedStatus // containername -> cpuId

func generateConfigmap(nodeName string, topo cpuset.NodeCPUTopology) *v1.ConfigMap {
	cm := &v1.ConfigMap{}
	cm.Name = nodeName + uniapiext.ConfigMapNUMANodeSuffix
	cm.Labels = map[string]string{
		uniapiext.LabelConfigMapCPUInfo: uniapiext.Topology1_20,
	}
	info, _ := json.Marshal(topo)
	cm.BinaryData = map[string][]byte{"info": info}
	return cm
}

type podBuilder struct {
	pod *v1.Pod
}

func createPod() *podBuilder {
	pb := &podBuilder{
		pod: &v1.Pod{},
	}
	pb.pod.Annotations = map[string]string{}
	pb.pod.Labels = map[string]string{}

	return pb
}

func (pb *podBuilder) uid(uid string) *podBuilder {
	pb.pod.UID = types.UID(uid)
	return pb
}

func (pb *podBuilder) ns(ns string) *podBuilder {
	pb.pod.Namespace = ns
	return pb
}

func (pb *podBuilder) name(name string) *podBuilder {
	pb.pod.Name = name
	return pb
}

func (pb *podBuilder) nodeName(name string) *podBuilder {
	pb.pod.Spec.NodeName = name
	return pb
}

func (pb *podBuilder) phase(p v1.PodPhase) *podBuilder {
	pb.pod.Status.Phase = p
	return pb
}

func (pb *podBuilder) cpuset(alloc PodAllocatedStatus) *podBuilder {
	allocateResult := make(cpuset.CPUAllocateResult)
	for containerName, cpusSet := range alloc {
		var numaSet map[int]cpuset.SimpleCPUSet
		var ok bool
		if numaSet, ok = allocateResult[containerName]; !ok {
			numaSet = make(map[int]cpuset.SimpleCPUSet)
			allocateResult[containerName] = numaSet
		}
		for _, c := range cpusSet {
			numa := c.Node
			if _, ok := numaSet[numa]; !ok {
				numaSet[numa] = cpuset.SimpleCPUSet{
					Elems: map[int]struct{}{},
				}
			}
			numaSet[numa].Elems[c.Id] = struct{}{}
		}
	}
	annotations, _ := json.Marshal(allocateResult)
	pb.pod.Annotations[uniapiext.AnnotationPodCPUSet] = string(annotations)
	return pb
}

func (pb *podBuilder) policy(cpuScheduler bool, p string) *podBuilder {
	if cpuScheduler {
		pb.pod.Annotations[uniapiext.AnnotationPodCPUSetScheduler] = string(uniapiext.CPUSetSchedulerTrue)
	}
	pb.pod.Annotations[uniapiext.AnnotationPodCPUPolicy] = p
	return pb
}

func (pb *podBuilder) cpuResource(name string, request, limit string) *podBuilder {
	r := resource.MustParse(request)
	l := resource.MustParse(limit)

	pb.pod.Spec.Containers = append(pb.pod.Spec.Containers, v1.Container{
		Name: name,
		Resources: v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU: l,
			},
			Requests: v1.ResourceList{
				v1.ResourceCPU: r,
			},
		},
	})
	return pb
}

func (pb *podBuilder) annotation(key, value string) *podBuilder {
	pb.pod.Annotations[key] = value
	return pb
}

type testSharedLister struct {
	nodes       []*v1.Node
	nodeInfos   []*framework.NodeInfo
	nodeInfoMap map[string]*framework.NodeInfo
}

func newTestSharedLister(pods []*v1.Pod, nodes []*v1.Node) *testSharedLister {
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
		if node == nil {
			continue
		}
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

type pluginTestSuit struct {
	framework.Handle
}

func newPluginTestSuit(t *testing.T, nodes []*v1.Node) *pluginTestSuit {
	registeredPlugins := []schedulertesting.RegisterPluginFunc{
		schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
		schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
	}

	cs := clientsetfake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newTestSharedLister(nil, nodes)
	fh, err := schedulertesting.NewFramework(
		registeredPlugins,
		"koord-scheduler",
		frameworkruntime.WithClientSet(cs),
		frameworkruntime.WithInformerFactory(informerFactory),
		frameworkruntime.WithSnapshotSharedLister(snapshot),
	)
	assert.Nil(t, err)
	return &pluginTestSuit{
		Handle: &frameworkHandleExtender{
			Handle:    fh,
			Clientset: nrtfake.NewSimpleClientset(),
		},
	}
}

func (p *pluginTestSuit) start() {
	ctx := context.TODO()
	p.Handle.SharedInformerFactory().Start(ctx.Done())
	p.Handle.SharedInformerFactory().WaitForCacheSync(ctx.Done())
}

func TestNumaAware_Filter(t *testing.T) {
	type fields struct {
		NodeCPUTopologyMap map[string]*cpuset.NodeCPUTopology
	}
	type args struct {
		p    *v1.Pod
		node *v1.Node
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   framework.Code
	}{
		{
			name:   "normal pod",
			fields: fields{},
			args: args{
				p: createPod().pod,
			},
			want: framework.Success,
		},
		{
			name:   "no node",
			fields: fields{},
			args: args{
				p: createPod().policy(true, string(uniapiext.CPUPolicyCritical)).pod,
			},
			want: framework.UnschedulableAndUnresolvable,
		},
		{
			name:   "no topo",
			fields: fields{},
			args: args{
				p: createPod().
					policy(true, string(uniapiext.CPUPolicyCritical)).
					annotation(uniapiext.AnnotationPodQOSClass, string(uniapiext.QoSLSR)).
					cpuResource("main", "1", "1").
					pod,
				node: &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}, Status: v1.NodeStatus{Allocatable: v1.ResourceList{}}},
			},
			want: framework.UnschedulableAndUnresolvable,
		},
		{
			name: "no topo 2",
			fields: fields{
				NodeCPUTopologyMap: map[string]*cpuset.NodeCPUTopology{"node1": nil},
			},
			args: args{
				p: createPod().
					policy(true, string(uniapiext.CPUPolicyCritical)).
					annotation(uniapiext.AnnotationPodQOSClass, string(uniapiext.QoSLSR)).
					cpuResource("main", "1", "1").
					pod,
				node: &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}, Status: v1.NodeStatus{Allocatable: v1.ResourceList{}}},
			},
			want: framework.UnschedulableAndUnresolvable,
		},
		{
			name: "success",
			fields: fields{
				NodeCPUTopologyMap: map[string]*cpuset.NodeCPUTopology{"node1": {
					0: cpuset.SocketCPUTopology{
						0: cpuset.NUMANodeCPUTopology{
							0: cpuset.L3CPUTopology{
								Topology: cpuset.CPUSet{
									Elems: map[int]cpuset.CPUInfo{
										0: {
											Id:     0,
											Node:   0,
											Socket: 0,
											Core:   0,
										},
										1: {
											Id:     1,
											Node:   0,
											Socket: 0,
											Core:   0,
										},
										2: {
											Id:     2,
											Node:   0,
											Socket: 0,
											Core:   1,
										},
										3: {
											Id:     3,
											Node:   0,
											Socket: 0,
											Core:   1,
										},
									},
								},
							},
						},
					},
					1: cpuset.SocketCPUTopology{
						1: cpuset.NUMANodeCPUTopology{
							1: cpuset.L3CPUTopology{
								Topology: cpuset.CPUSet{
									Elems: map[int]cpuset.CPUInfo{
										4: {
											Id:     4,
											Node:   1,
											Socket: 1,
											Core:   2,
										},
										5: {
											Id:     5,
											Node:   1,
											Socket: 1,
											Core:   2,
										},
										6: {
											Id:     6,
											Node:   1,
											Socket: 1,
											Core:   3,
										},
										7: {
											Id:     7,
											Node:   1,
											Socket: 1,
											Core:   3,
										},
									},
								},
							},
						},
					},
				}},
			},
			args: args{
				p: createPod().
					policy(true, string(uniapiext.CPUPolicyCritical)).
					annotation(uniapiext.AnnotationPodQOSClass, string(uniapiext.QoSLSR)).
					cpuResource("main", "2", "2").
					pod, node: &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}, Status: v1.NodeStatus{Allocatable: v1.ResourceList{}}},
			},
			want: framework.Success,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, []*v1.Node{tt.args.node})

			if tt.args.node != nil {
				var cpuTopology cpuset.NodeCPUTopology
				topology := tt.fields.NodeCPUTopologyMap[tt.args.node.Name]
				if topology != nil {
					cpuTopology = *topology
				}

				configMap := generateConfigmap(tt.args.node.Name, cpuTopology)
				_, err := suit.ClientSet().CoreV1().ConfigMaps("default").Create(context.Background(), configMap, metav1.CreateOptions{})
				assert.Nil(t, err)
			}

			plugin, err := New(nil, suit.Handle)
			assert.NoError(t, err)
			assert.NotNil(t, plugin)

			suit.start()

			n := plugin.(*Plugin)
			nodeInfo := framework.NewNodeInfo()
			if tt.args.node != nil {
				nodeInfo.SetNode(tt.args.node)
			}

			cycleState := framework.NewCycleState()
			status := n.PreFilter(context.Background(), cycleState, tt.args.p)
			assert.True(t, status.IsSuccess())

			if got := n.Filter(context.Background(), cycleState, tt.args.p, nodeInfo); !reflect.DeepEqual(got.Code(), tt.want) {
				t.Errorf("Plugin.Filter() = %v, want %v", got.Code(), tt.want)
			}
		})
	}
}

func TestNumaAware_Score(t *testing.T) {
	type fields struct {
		NodeCPUTopologyMap map[string]*cpuset.NodeCPUTopology
	}
	type args struct {
		p        *v1.Pod
		nodeName string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   int64
	}{
		{
			name: "pod not numa scheduler",
			fields: fields{
				NodeCPUTopologyMap: map[string]*cpuset.NodeCPUTopology{"node1": nil},
			},
			args: args{
				p:        createPod().pod,
				nodeName: "node1",
			},
			want: 0,
		},
		{
			name: "no cpu topo",
			fields: fields{
				NodeCPUTopologyMap: map[string]*cpuset.NodeCPUTopology{},
			},
			args: args{
				p: createPod().
					policy(true, string(uniapiext.CPUPolicyCritical)).
					annotation(uniapiext.AnnotationPodQOSClass, string(uniapiext.QoSLSR)).
					cpuResource("main", "2", "2").
					pod,
				nodeName: "node1",
			},
			want: 0,
		},
		{
			name: "no container resource",
			fields: fields{
				NodeCPUTopologyMap: map[string]*cpuset.NodeCPUTopology{"node1": nil},
			},
			args: args{
				p: createPod().
					policy(true, string(uniapiext.CPUPolicyCritical)).
					annotation(uniapiext.AnnotationPodQOSClass, string(uniapiext.QoSLSR)).
					cpuResource("main", "0", "0").
					pod,
				nodeName: "node1",
			},
			want: 0,
		},
		// {
		// 	name: "static-burst score",
		// 	fields: fields{
		// 		NodeCPUTopologyMap:          map[string]*cpuset.NodeCPUTopology{"node1": nil},
		// 		AllocatedNodeCPUTopologyMap: map[string]*AllocatedCPUTopology{"node1": nil},
		// 		AllocatorBuilder: &mockBuilder{
		// 			cpus:  []int{},
		// 			score: 95,
		// 		},
		// 	},
		// 	args: args{
		// 		p:        createPod().policy(true, string(uniapiext.CPUPolicyStaticBurst)).cpuResource("c1", "4", "4").pod,
		// 		nodeName: "node1",
		// 	},
		// 	want: 95,
		// },
		{
			name: "critical score",
			fields: fields{
				NodeCPUTopologyMap: map[string]*cpuset.NodeCPUTopology{"node1": {
					0: cpuset.SocketCPUTopology{
						0: cpuset.NUMANodeCPUTopology{
							0: cpuset.L3CPUTopology{
								Topology: cpuset.CPUSet{
									Elems: map[int]cpuset.CPUInfo{
										0: {
											Id:     0,
											Node:   0,
											Socket: 0,
											Core:   0,
										},
										1: {
											Id:     1,
											Node:   0,
											Socket: 0,
											Core:   0,
										},
										2: {
											Id:     2,
											Node:   0,
											Socket: 0,
											Core:   1,
										},
										3: {
											Id:     3,
											Node:   0,
											Socket: 0,
											Core:   1,
										},
									},
								},
							},
						},
					},
					1: cpuset.SocketCPUTopology{
						1: cpuset.NUMANodeCPUTopology{
							1: cpuset.L3CPUTopology{
								Topology: cpuset.CPUSet{
									Elems: map[int]cpuset.CPUInfo{
										4: {
											Id:     4,
											Node:   1,
											Socket: 1,
											Core:   2,
										},
										5: {
											Id:     5,
											Node:   1,
											Socket: 1,
											Core:   2,
										},
										6: {
											Id:     6,
											Node:   1,
											Socket: 1,
											Core:   3,
										},
										7: {
											Id:     7,
											Node:   1,
											Socket: 1,
											Core:   3,
										},
									},
								},
							},
						},
					},
				}},
			},
			args: args{
				p: createPod().
					policy(true, string(uniapiext.CPUPolicyCritical)).
					annotation(uniapiext.AnnotationPodQOSClass, string(uniapiext.QoSLSR)).
					cpuResource("main", "4", "4").
					pod,
				nodeName: "node1",
			},
			want: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nodes []*v1.Node
			if tt.args.nodeName != "" {
				node := &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: tt.args.nodeName,
					},
				}
				nodes = append(nodes, node)
			}

			suit := newPluginTestSuit(t, nodes)

			if tt.args.nodeName != "" {
				var cpuTopology cpuset.NodeCPUTopology
				topology := tt.fields.NodeCPUTopologyMap[tt.args.nodeName]
				if topology != nil {
					cpuTopology = *topology
				}

				configMap := generateConfigmap(tt.args.nodeName, cpuTopology)
				_, err := suit.ClientSet().CoreV1().ConfigMaps("default").Create(context.Background(), configMap, metav1.CreateOptions{})
				assert.Nil(t, err)
			}

			plugin, err := New(nil, suit.Handle)
			assert.NoError(t, err)
			assert.NotNil(t, plugin)

			suit.start()

			n := plugin.(*Plugin)

			cycleState := framework.NewCycleState()
			status := n.PreFilter(context.Background(), cycleState, tt.args.p)
			assert.True(t, status.IsSuccess())

			got, _ := n.Score(context.Background(), cycleState, tt.args.p, tt.args.nodeName)
			if got != tt.want {
				t.Errorf("Plugin.Score() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNumaAware_PreBind(t *testing.T) {
	type args struct {
		p        *v1.Pod
		nodeName string
	}
	tests := []struct {
		name string
		args args
		want *v1.Pod
	}{
		{
			name: "no numa scheduler",
			args: args{
				p:        createPod().pod,
				nodeName: "node1",
			},
			want: nil,
		},
		{
			name: "no allocated cpu topo",
			args: args{
				p: createPod().
					uid("11").
					policy(true, string(uniapiext.CPUPolicyCritical)).
					annotation(uniapiext.AnnotationPodQOSClass, string(uniapiext.QoSLSR)).
					cpuResource("main", "0", "0").
					pod,
				nodeName: "node1",
			},
			want: nil,
		},
		{
			name: "common prebind",
			args: args{
				p: createPod().
					uid("11").
					policy(true, string(uniapiext.CPUPolicyCritical)).
					annotation(uniapiext.AnnotationPodQOSClass, string(uniapiext.QoSLSR)).
					cpuResource("c1", "2", "2").
					pod,
				nodeName: "node1",
			},
			want: createPod().uid("11").policy(true, "").cpuset(PodAllocatedStatus{
				"c1": map[int]*AllocatedStatus{
					0: {CPUInfo: cpuset.CPUInfo{Id: 0, Node: 0}, Count: 1},
					1: {CPUInfo: cpuset.CPUInfo{Id: 1, Node: 0}, Count: 1},
				},
			}).pod,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nodes []*v1.Node
			if tt.args.nodeName != "" {
				node := &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: tt.args.nodeName,
					},
				}
				nodes = append(nodes, node)
			}

			suit := newPluginTestSuit(t, nodes)

			if tt.args.nodeName != "" {
				cpuTopology := cpuset.NodeCPUTopology{
					0: cpuset.SocketCPUTopology{
						0: cpuset.NUMANodeCPUTopology{
							0: cpuset.L3CPUTopology{
								Topology: cpuset.CPUSet{
									Elems: map[int]cpuset.CPUInfo{
										0: {
											Id:     0,
											Node:   0,
											Socket: 0,
											Core:   0,
										},
										1: {
											Id:     1,
											Node:   0,
											Socket: 0,
											Core:   0,
										},
										2: {
											Id:     2,
											Node:   0,
											Socket: 0,
											Core:   1,
										},
										3: {
											Id:     3,
											Node:   0,
											Socket: 0,
											Core:   1,
										},
									},
								},
							},
						},
					},
					1: cpuset.SocketCPUTopology{
						1: cpuset.NUMANodeCPUTopology{
							1: cpuset.L3CPUTopology{
								Topology: cpuset.CPUSet{
									Elems: map[int]cpuset.CPUInfo{
										4: {
											Id:     4,
											Node:   1,
											Socket: 1,
											Core:   2,
										},
										5: {
											Id:     5,
											Node:   1,
											Socket: 1,
											Core:   2,
										},
										6: {
											Id:     6,
											Node:   1,
											Socket: 1,
											Core:   3,
										},
										7: {
											Id:     7,
											Node:   1,
											Socket: 1,
											Core:   3,
										},
									},
								},
							},
						},
					},
				}
				configMap := generateConfigmap(tt.args.nodeName, cpuTopology)
				_, err := suit.ClientSet().CoreV1().ConfigMaps("default").Create(context.Background(), configMap, metav1.CreateOptions{})
				assert.Nil(t, err)
			}

			if tt.args.p != nil {
				_, err := suit.ClientSet().CoreV1().Pods(tt.args.p.Namespace).Create(context.Background(), tt.args.p, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			plugin, err := New(nil, suit.Handle)
			assert.NoError(t, err)
			assert.NotNil(t, plugin)

			suit.start()

			n := plugin.(*Plugin)

			cycleState := framework.NewCycleState()
			status := n.PreFilter(context.Background(), cycleState, tt.args.p)
			assert.True(t, status.IsSuccess())

			status = n.Reserve(context.Background(), cycleState, tt.args.p, tt.args.nodeName)
			assert.True(t, status.IsSuccess())

			tt.args.p.Spec.NodeName = tt.args.nodeName
			n.PreBind(context.Background(), cycleState, tt.args.p, tt.args.nodeName)
			got, _ := suit.ClientSet().CoreV1().Pods(tt.args.p.Namespace).Get(context.Background(), tt.args.p.Name, metav1.GetOptions{})
			if tt.want == nil {
				if got != nil && got.Annotations[uniapiext.AnnotationPodCPUSet] != "" {
					t.Errorf("Plugin.PreBind() pod cpuset = %v, want %v", got.Annotations[uniapiext.AnnotationPodCPUSet], "")
				}
			} else {
				if !reflect.DeepEqual(got.Annotations[uniapiext.AnnotationPodCPUSet], tt.want.Annotations[uniapiext.AnnotationPodCPUSet]) {
					t.Errorf("Plugin.PreBind() pod = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestNumaAware_addConfigMap(t *testing.T) {
	type fields struct {
		Pods []runtime.Object
	}
	type args struct {
		obj interface{}
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   map[string]*cpuset.NodeCPUTopology
		want1  string
	}{
		{
			name:   "no running pod",
			fields: fields{},
			args: args{
				obj: generateConfigmap("node1", cpuset.NodeCPUTopology{
					0: {0: {0: {Topology: cpuset.CPUSet{Elems: map[int]cpuset.CPUInfo{0: {Id: 0, Socket: 0, Node: 0, L3: 0, Core: 0}, 1: {Id: 1, Socket: 0, Node: 0, L3: 0, Core: 0}}}}}},
					1: {1: {1: {Topology: cpuset.CPUSet{Elems: map[int]cpuset.CPUInfo{2: {Id: 2, Socket: 1, Node: 1, L3: 1, Core: 1}, 3: {Id: 3, Socket: 1, Node: 1, L3: 1, Core: 1}}}}}},
				}),
			},
			want: map[string]*cpuset.NodeCPUTopology{"node1": {
				0: {0: {0: {Topology: cpuset.CPUSet{Elems: map[int]cpuset.CPUInfo{0: {Id: 0, Socket: 0, Node: 0, L3: 0, Core: 0}, 1: {Id: 1, Socket: 0, Node: 0, L3: 0, Core: 0}}}}}},
				1: {1: {1: {Topology: cpuset.CPUSet{Elems: map[int]cpuset.CPUInfo{2: {Id: 2, Socket: 1, Node: 1, L3: 1, Core: 1}, 3: {Id: 3, Socket: 1, Node: 1, L3: 1, Core: 1}}}}}},
			}},
			want1: "0-3",
		},
		{
			name: "running pod",
			fields: fields{
				Pods: []runtime.Object{
					createPod().nodeName("node1").name("p2").uid("p2").cpuResource("c1", "2", "2").phase(v1.PodRunning).
						cpuset(PodAllocatedStatus{"c1": map[int]*AllocatedStatus{
							2: {CPUInfo: cpuset.CPUInfo{Id: 2, Socket: 1, Node: 1, L3: 1, Core: 1}, Count: 1, Exclusive: true},
							3: {CPUInfo: cpuset.CPUInfo{Id: 3, Socket: 1, Node: 1, L3: 1, Core: 1}, Count: 1, Exclusive: true}}}).
						policy(true, string(uniapiext.CPUPolicyCritical)).pod,
				},
			},
			args: args{
				obj: generateConfigmap("node1", cpuset.NodeCPUTopology{
					0: {0: {0: {Topology: cpuset.CPUSet{Elems: map[int]cpuset.CPUInfo{0: {Id: 0, Socket: 0, Node: 0, L3: 0, Core: 0}, 1: {Id: 1, Socket: 0, Node: 0, L3: 0, Core: 0}}}}}},
					1: {1: {1: {Topology: cpuset.CPUSet{Elems: map[int]cpuset.CPUInfo{2: {Id: 2, Socket: 1, Node: 1, L3: 1, Core: 1}, 3: {Id: 3, Socket: 1, Node: 1, L3: 1, Core: 1}}}}}},
				}),
			},
			want: map[string]*cpuset.NodeCPUTopology{"node1": {
				0: {0: {0: {Topology: cpuset.CPUSet{Elems: map[int]cpuset.CPUInfo{0: {Id: 0, Socket: 0, Node: 0, L3: 0, Core: 0}, 1: {Id: 1, Socket: 0, Node: 0, L3: 0, Core: 0}}}}}},
				1: {1: {1: {Topology: cpuset.CPUSet{Elems: map[int]cpuset.CPUInfo{2: {Id: 2, Socket: 1, Node: 1, L3: 1, Core: 1}, 3: {Id: 3, Socket: 1, Node: 1, L3: 1, Core: 1}}}}}},
			}},
			want1: "0-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nil)

			if len(tt.fields.Pods) > 0 {
				for _, v := range tt.fields.Pods {
					_, err := suit.ClientSet().CoreV1().Pods(v.(*v1.Pod).Namespace).Create(context.TODO(), v.(*v1.Pod), metav1.CreateOptions{})
					assert.NoError(t, err)
				}
			}

			if tt.args.obj != nil {
				_, err := suit.ClientSet().CoreV1().ConfigMaps(tt.args.obj.(*v1.ConfigMap).Namespace).Create(context.TODO(), tt.args.obj.(*v1.ConfigMap), metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			plugin, err := New(nil, suit.Handle)
			assert.NoError(t, err)
			assert.NotNil(t, plugin)
			n := plugin.(*Plugin)

			suit.start()

			cpuTopology := n.Plugin.GetCPUTopologyManager().GetCPUTopologyOptions("node1").CPUTopology
			assert.NotNil(t, cpuTopology)
			assert.True(t, cpuTopology.IsValid())
			cpuTopologies := map[string]*nodenumaresource.CPUTopology{
				"node1": cpuTopology,
			}

			expectedCPUTopologies := make(map[string]*nodenumaresource.CPUTopology)
			for k, v := range tt.want {
				expectedCPUTopologies[k] = convertToNodeNUMAResourceCPUTopology(*v)
			}

			assert.Equal(t, expectedCPUTopologies, cpuTopologies)

			remainedCPUs, _, err := n.Plugin.GetCPUManager().GetAvailableCPUs("node1")
			assert.NoError(t, err)
			assert.Equal(t, tt.want1, remainedCPUs.String())
		})
	}
}

func convertToNodeNUMAResourceCPUTopology(topology cpuset.NodeCPUTopology) *nodenumaresource.CPUTopology {
	var cpuTopology nodenumaresource.CPUTopology
	details := nodenumaresource.NewCPUDetails()
	cpuTopoInfo := make(map[int] /*socket*/ map[int] /*node*/ map[int] /*core*/ struct{})
	for _, socket := range topology {
		for _, numa := range socket {
			for _, l3 := range numa {
				for cpuIndex := range l3.Topology.Elems {
					info := l3.Topology.Elems[cpuIndex]

					cpuID := info.Id
					socketID := info.Socket
					coreID := info.Socket<<16 | info.Core
					nodeID := info.Socket<<16 | info.Node
					cpuInfo := &nodenumaresource.CPUInfo{
						CPUID:    cpuID,
						CoreID:   coreID,
						NodeID:   nodeID,
						SocketID: socketID,
					}
					details[cpuInfo.CPUID] = *cpuInfo
					if cpuTopoInfo[cpuInfo.SocketID] == nil {
						cpuTopology.NumSockets++
						cpuTopoInfo[cpuInfo.SocketID] = make(map[int]map[int]struct{})
					}
					if cpuTopoInfo[cpuInfo.SocketID][nodeID] == nil {
						cpuTopology.NumNodes++
						cpuTopoInfo[cpuInfo.SocketID][nodeID] = make(map[int]struct{})
					}
					if _, ok := cpuTopoInfo[cpuInfo.SocketID][nodeID][coreID]; !ok {
						cpuTopology.NumCores++
						cpuTopoInfo[cpuInfo.SocketID][nodeID][coreID] = struct{}{}
					}
				}
			}
		}
	}
	cpuTopology.CPUDetails = details
	cpuTopology.NumCPUs = len(details)
	return &cpuTopology
}
