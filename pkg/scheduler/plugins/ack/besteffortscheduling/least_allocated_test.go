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

package besteffortscheduling

import (
	"context"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"

	uniapiext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
)

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

func TestNodeBEResourceLeastAllocated(t *testing.T) {
	labels1 := map[string]string{
		"foo": "bar",
		"baz": "blah",
	}
	labels2 := map[string]string{
		"bar": "foo",
		"baz": "blah",
	}
	machine1Spec := v1.PodSpec{
		NodeName: "machine1",
	}
	machine2Spec := v1.PodSpec{
		NodeName: "machine2",
	}
	cpuOnly := v1.PodSpec{
		NodeName: "machine1",
		Containers: []v1.Container{
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						BatchCPU:    resource.MustParse("1000"),
						BatchMemory: resource.MustParse("0"),
					},
				},
			},
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						BatchCPU:    resource.MustParse("2000"),
						BatchMemory: resource.MustParse("0"),
					},
				},
			},
		},
	}
	oldCPUOnly := v1.PodSpec{
		NodeName: "machine1",
		Containers: []v1.Container{
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						uniapiext.AlibabaCloudReclaimedCPU:    resource.MustParse("1000"),
						uniapiext.AlibabaCloudReclaimedMemory: resource.MustParse("0"),
					},
				},
			},
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						uniapiext.AlibabaCloudReclaimedCPU:    resource.MustParse("2000"),
						uniapiext.AlibabaCloudReclaimedMemory: resource.MustParse("0"),
					},
				},
			},
		},
	}
	normalCPUOnly := v1.PodSpec{
		NodeName: "machine1",
		Containers: []v1.Container{
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("1000"),
						v1.ResourceMemory: resource.MustParse("0"),
					},
				},
			},
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("2000"),
						v1.ResourceMemory: resource.MustParse("0"),
					},
				},
			},
		},
	}
	cpuOnly2 := cpuOnly
	cpuOnly2.NodeName = "machine2"
	noResources := v1.PodSpec{
		Containers: []v1.Container{},
	}
	cpuAndMemory := v1.PodSpec{
		NodeName: "machine2",
		Containers: []v1.Container{
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						BatchCPU:    resource.MustParse("1000"),
						BatchMemory: resource.MustParse("2000"),
					},
				},
			},
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						BatchCPU:    resource.MustParse("2000"),
						BatchMemory: resource.MustParse("3000"),
					},
				},
			},
		},
	}
	oldCPUAndMemory := v1.PodSpec{
		NodeName: "machine2",
		Containers: []v1.Container{
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						uniapiext.AlibabaCloudReclaimedCPU:    resource.MustParse("1000"),
						uniapiext.AlibabaCloudReclaimedMemory: resource.MustParse("2000"),
					},
				},
			},
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						uniapiext.AlibabaCloudReclaimedCPU:    resource.MustParse("2000"),
						uniapiext.AlibabaCloudReclaimedMemory: resource.MustParse("3000"),
					},
				},
			},
		},
	}
	normalCPUAndMemory := v1.PodSpec{
		NodeName: "machine2",
		Containers: []v1.Container{
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("1000"),
						v1.ResourceMemory: resource.MustParse("2000"),
					},
				},
			},
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("2000"),
						v1.ResourceMemory: resource.MustParse("3000"),
					},
				},
			},
		},
	}
	tests := []struct {
		pod          *v1.Pod
		pods         []*v1.Pod
		nodes        []*v1.Node
		wantErr      string
		expectedList framework.NodeScoreList
		name         string
	}{
		// Node1 scores (remaining resources) on 0-MaxNodeScore scale
		// CPU Score: ((4000 - 0) * MaxNodeScore) / 4000 = MaxNodeScore
		// Memory Score: ((10000 - 0) * MaxNodeScore) / 10000 = MaxNodeScore
		// Node1 Score: (100 + 100) / 2 = 100
		// Node2 scores (remaining resources) on 0-MaxNodeScore scale
		// CPU Score: ((4000 - 0) * MaxNodeScore) / 4000 = MaxNodeScore
		// Memory Score: ((10000 - 0) * MaxNodeScore) / 10000 = MaxNodeScore
		// Node2 Score: (MaxNodeScore + MaxNodeScore) / 2 = MaxNodeScore
		{
			pod:          &v1.Pod{Spec: noResources},
			nodes:        []*v1.Node{makeBENode("machine1", 4000, 10000), makeBENode("machine2", 4000, 10000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "nothing scheduled, nothing requested",
		},
		{
			// Node1 scores on 0-MaxNodeScore scale
			// CPU Score: ((4000 - 3000) * MaxNodeScore) / 4000 = 25
			// Memory Score: ((10000 - 5000) * MaxNodeScore) / 10000 = 50
			// Node1 Score: (25 + 50) / 2 = 37
			// Node2 scores on 0-MaxNodeScore scale
			// CPU Score: ((6000 - 3000) * MaxNodeScore) / 6000 = 50
			// Memory Score: ((10000 - 5000) * MaxNodeScore) / 10000 = 50
			// Node2 Score: (50 + 50) / 2 = 50
			pod:          &v1.Pod{Spec: cpuAndMemory},
			nodes:        []*v1.Node{makeBENode("machine1", 4000, 10000), makeBENode("machine2", 6000, 10000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 37}, {Name: "machine2", Score: 50}},
			name:         "nothing scheduled, resources requested, differently sized machines",
		},
		{
			// Node1 scores on 0-MaxNodeScore scale
			// CPU Score: ((4000 - 3000) * MaxNodeScore) / 4000 = 25
			// Memory Score: ((10000 - 5000) * MaxNodeScore) / 10000 = 50
			// Node1 Score: (25 + 50) / 2 = 37
			// Node2 scores on 0-MaxNodeScore scale
			// CPU Score: ((6000 - 3000) * MaxNodeScore) / 6000 = 50
			// Memory Score: ((10000 - 5000) * MaxNodeScore) / 10000 = 50
			// Node2 Score: (50 + 50) / 2 = 50
			pod:          &v1.Pod{Spec: cpuAndMemory},
			nodes:        []*v1.Node{makeOldBENode("machine1", 4000, 10000), makeOldBENode("machine2", 6000, 10000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 37}, {Name: "machine2", Score: 50}},
			name:         "nothing scheduled, resources requested, differently sized machines with old batch resource",
		},
		{
			// Node1 scores on 0-MaxNodeScore scale
			// CPU Score: ((4000 - 3000) * MaxNodeScore) / 4000 = 25
			// Memory Score: ((10000 - 5000) * MaxNodeScore) / 10000 = 50
			// Node1 Score: (25 + 50) / 2 = 37
			// Node2 scores on 0-MaxNodeScore scale
			// CPU Score: ((6000 - 3000) * MaxNodeScore) / 6000 = 50
			// Memory Score: ((10000 - 5000) * MaxNodeScore) / 10000 = 50
			// Node2 Score: (50 + 50) / 2 = 50
			pod:          &v1.Pod{Spec: oldCPUAndMemory},
			nodes:        []*v1.Node{makeOldBENode("machine1", 4000, 10000), makeOldBENode("machine2", 6000, 10000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 37}, {Name: "machine2", Score: 50}},
			name:         "nothing scheduled, resources requested, differently sized machines with old format pod",
		},
		{
			// Node1 scores on 0-MaxNodeScore scale
			// CPU Score: ((4000 - 0) * MaxNodeScore) / 4000 = MaxNodeScore
			// Memory Score: ((10000 - 0) * MaxNodeScore) / 10000 = MaxNodeScore
			// Node1 Score: (MaxNodeScore + MaxNodeScore) / 2 = MaxNodeScore
			// Node2 scores on 0-MaxNodeScore scale
			// CPU Score: ((4000 - 0) * MaxNodeScore) / 4000 = MaxNodeScore
			// Memory Score: ((10000 - 0) * MaxNodeScore) / 10000 = MaxNodeScore
			// Node2 Score: (MaxNodeScore + MaxNodeScore) / 2 = MaxNodeScore
			pod:          &v1.Pod{Spec: noResources},
			nodes:        []*v1.Node{makeBENode("machine1", 4000, 10000), makeBENode("machine2", 4000, 10000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "no resources requested, pods scheduled",
			pods: []*v1.Pod{
				{Spec: machine1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: machine1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: machine2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: machine2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			},
		},
		{
			// Node1 scores on 0-MaxNodeScore scale
			// CPU Score: ((10000 - 6000) * MaxNodeScore) / 10000 = 40
			// Memory Score: ((20000 - 0) * MaxNodeScore) / 20000 = MaxNodeScore
			// Node1 Score: (40 + 100) / 2 = 70
			// Node2 scores on 0-MaxNodeScore scale
			// CPU Score: ((10000 - 6000) * MaxNodeScore) / 10000 = 40
			// Memory Score: ((20000 - 5000) * MaxNodeScore) / 20000 = 75
			// Node2 Score: (40 + 75) / 2 = 57
			pod:          &v1.Pod{Spec: noResources},
			nodes:        []*v1.Node{makeBENode("machine1", 10000, 20000), makeBENode("machine2", 10000, 20000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 70}, {Name: "machine2", Score: 57}},
			name:         "no resources requested, pods scheduled with resources",
			pods: []*v1.Pod{
				{Spec: cpuOnly, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: cpuOnly, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: cpuOnly2, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: cpuAndMemory, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			},
		},
		{
			// Node1 scores on 0-MaxNodeScore scale
			// CPU Score: ((10000 - 6000) * MaxNodeScore) / 10000 = 40
			// Memory Score: ((20000 - 0) * MaxNodeScore) / 20000 = MaxNodeScore
			// Node1 Score: (40 + 100) / 2 = 70
			// Node2 scores on 0-MaxNodeScore scale
			// CPU Score: ((10000 - 6000) * MaxNodeScore) / 10000 = 40
			// Memory Score: ((20000 - 5000) * MaxNodeScore) / 20000 = 75
			// Node2 Score: (40 + 75) / 2 = 57
			pod:          &v1.Pod{Spec: noResources},
			nodes:        []*v1.Node{makeBENode("machine1", 10000, 20000), makeBENode("machine2", 10000, 20000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 70}, {Name: "machine2", Score: 57}},
			name:         "no resources requested, old and format pods scheduled with resources",
			pods: []*v1.Pod{
				{Spec: cpuOnly, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: oldCPUOnly, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: cpuOnly2, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: oldCPUAndMemory, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			},
		},
		{
			// Node1 scores on 0-MaxNodeScore scale
			// CPU Score: ((10000 - 6000) * MaxNodeScore) / 10000 = 40
			// Memory Score: ((20000 - 5000) * MaxNodeScore) / 20000 = 75
			// Node1 Score: (40 + 75) / 2 = 57
			// Node2 scores on 0-MaxNodeScore scale
			// CPU Score: ((10000 - 6000) * MaxNodeScore) / 10000 = 40
			// Memory Score: ((20000 - 10000) * MaxNodeScore) / 20000 = 50
			// Node2 Score: (40 + 50) / 2 = 45
			pod:          &v1.Pod{Spec: cpuAndMemory},
			nodes:        []*v1.Node{makeBENode("machine1", 10000, 20000), makeBENode("machine2", 10000, 20000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 57}, {Name: "machine2", Score: 45}},
			name:         "resources requested, pods scheduled with resources",
			pods: []*v1.Pod{
				{Spec: cpuOnly},
				{Spec: cpuAndMemory},
			},
		},
		{
			// Node1 scores on 0-MaxNodeScore scale
			// CPU Score: ((10000 - 6000) * MaxNodeScore) / 10000 = 40
			// Memory Score: ((20000 - 5000) * MaxNodeScore) / 20000 = 75
			// Node1 Score: (40 + 75) / 2 = 57
			// Node2 scores on 0-MaxNodeScore scale
			// CPU Score: ((10000 - 6000) * MaxNodeScore) / 10000 = 40
			// Memory Score: ((20000 - 10000) * MaxNodeScore) / 20000 = 50
			// Node2 Score: (40 + 50) / 2 = 45
			pod:          &v1.Pod{Spec: oldCPUAndMemory},
			nodes:        []*v1.Node{makeBENode("machine1", 10000, 20000), makeBENode("machine2", 10000, 20000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 57}, {Name: "machine2", Score: 45}},
			name:         "old and new resources requested, old pods scheduled with resources",
			pods: []*v1.Pod{
				{Spec: oldCPUOnly},
				{Spec: cpuAndMemory},
			},
		},
		{
			// Node1 scores on 0-MaxNodeScore scale
			// CPU Score: ((10000 - 6000) * MaxNodeScore) / 10000 = 40
			// Memory Score: ((20000 - 5000) * MaxNodeScore) / 20000 = 75
			// Node1 Score: (40 + 75) / 2 = 57
			// Node2 scores on 0-MaxNodeScore scale
			// CPU Score: ((10000 - 6000) * MaxNodeScore) / 10000 = 40
			// Memory Score: ((50000 - 10000) * MaxNodeScore) / 50000 = 80
			// Node2 Score: (40 + 80) / 2 = 60
			pod:          &v1.Pod{Spec: cpuAndMemory},
			nodes:        []*v1.Node{makeBENode("machine1", 10000, 20000), makeBENode("machine2", 10000, 50000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 57}, {Name: "machine2", Score: 60}},
			name:         "resources requested, pods scheduled with resources, differently sized machines",
			pods: []*v1.Pod{
				{Spec: cpuOnly},
				{Spec: cpuAndMemory},
			},
		},
		{
			// Node1 scores on 0-MaxNodeScore scale
			// CPU Score: ((10000 - 6000) * MaxNodeScore) / 10000 = 40
			// Memory Score: ((20000 - 5000) * MaxNodeScore) / 20000 = 75
			// Node1 Score: (40 + 75) / 2 = 57
			// Node2 scores on 0-MaxNodeScore scale
			// CPU Score: ((10000 - 6000) * MaxNodeScore) / 10000 = 40
			// Memory Score: ((50000 - 10000) * MaxNodeScore) / 50000 = 80
			// Node2 Score: (40 + 80) / 2 = 60
			pod:          &v1.Pod{Spec: oldCPUAndMemory},
			nodes:        []*v1.Node{makeBENode("machine1", 10000, 20000), makeBENode("machine2", 10000, 50000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 57}, {Name: "machine2", Score: 60}},
			name:         "old and new resources requested, old pods scheduled with resources, differently sized machines",
			pods: []*v1.Pod{
				{Spec: cpuOnly},
				{Spec: oldCPUAndMemory},
			},
		},
		{
			// Node1 scores on 0-MaxNodeScore scale
			// CPU Score: ((4000 - 6000) * MaxNodeScore) / 4000 = 0
			// Memory Score: ((10000 - 0) * MaxNodeScore) / 10000 = MaxNodeScore
			// Node1 Score: (0 + MaxNodeScore) / 2 = 50
			// Node2 scores on 0-MaxNodeScore scale
			// CPU Score: ((4000 - 6000) * MaxNodeScore) / 4000 = 0
			// Memory Score: ((10000 - 5000) * MaxNodeScore) / 10000 = 50
			// Node2 Score: (0 + 50) / 2 = 25
			pod:          &v1.Pod{Spec: cpuOnly},
			nodes:        []*v1.Node{makeBENode("machine1", 4000, 10000), makeBENode("machine2", 4000, 10000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 50}, {Name: "machine2", Score: 25}},
			name:         "requested resources exceed node capacity",
			pods: []*v1.Pod{
				{Spec: cpuOnly},
				{Spec: cpuAndMemory},
			},
		},
		{
			// Node1 scores on 0-MaxNodeScore scale
			// CPU Score: ((4000 - 6000) * MaxNodeScore) / 4000 = 0
			// Memory Score: ((10000 - 0) * MaxNodeScore) / 10000 = MaxNodeScore
			// Node1 Score: (0 + MaxNodeScore) / 2 = 50
			// Node2 scores on 0-MaxNodeScore scale
			// CPU Score: ((4000 - 6000) * MaxNodeScore) / 4000 = 0
			// Memory Score: ((10000 - 5000) * MaxNodeScore) / 10000 = 50
			// Node2 Score: (0 + 50) / 2 = 25
			pod:          &v1.Pod{Spec: oldCPUOnly},
			nodes:        []*v1.Node{makeBENode("machine1", 4000, 10000), makeBENode("machine2", 4000, 10000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 50}, {Name: "machine2", Score: 25}},
			name:         "old requested resources exceed node capacity",
			pods: []*v1.Pod{
				{Spec: cpuOnly},
				{Spec: oldCPUAndMemory},
			},
		},
		{
			pod:          &v1.Pod{Spec: noResources},
			nodes:        []*v1.Node{makeBENode("machine1", 0, 0), makeBENode("machine2", 0, 0)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 100}, {Name: "machine2", Score: 100}},
			name:         "zero node resources, pods scheduled with resources",
			pods: []*v1.Pod{
				{Spec: cpuOnly},
				{Spec: cpuAndMemory},
			},
		},
		{
			pod:          &v1.Pod{Spec: normalCPUAndMemory},
			nodes:        []*v1.Node{makeNode("machine1", 4000, 10000), makeNode("machine2", 4000, 10000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 100}, {Name: "machine2", Score: 100}},
			name:         "zero node be resources, normal node resource, pods scheduled with normal resources",
			pods: []*v1.Pod{
				{Spec: normalCPUOnly},
				{Spec: normalCPUAndMemory},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := newTestSharedLister(test.pods, test.nodes)
			fh, _ := runtime.NewFramework(nil, nil, runtime.WithSnapshotSharedLister(snapshot), runtime.WithSnapshotSharedLister(snapshot))
			p, err := NewBELeastAllocated(nil, fh)

			if err != nil && len(test.wantErr) == 0 {
				t.Fatalf("failed to initialize plugin NodeBEResourcesLeastAllocated, got error: %v", err)
			}

			for i := range test.nodes {
				hostResult, err := p.(framework.ScorePlugin).Score(context.Background(), nil, test.pod, test.nodes[i].Name)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if !reflect.DeepEqual(test.expectedList[i].Score, hostResult) {
					t.Errorf("expected %#v, got %#v", test.expectedList[i].Score, hostResult)
				}
			}
		})
	}
}

func makeBENode(node string, beCPU, beMemory int64) *v1.Node {
	resourceList := make(map[v1.ResourceName]resource.Quantity)
	resourceList[BatchCPU] = *resource.NewQuantity(beCPU, resource.DecimalSI)
	resourceList[BatchMemory] = *resource.NewQuantity(beMemory, resource.BinarySI)
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: node},
		Status: v1.NodeStatus{
			Capacity:    resourceList,
			Allocatable: resourceList,
		},
	}
}

func makeOldBENode(node string, beCPU, beMemory int64) *v1.Node {
	resourceList := make(map[v1.ResourceName]resource.Quantity)
	resourceList[uniapiext.AlibabaCloudReclaimedCPU] = *resource.NewQuantity(beCPU, resource.DecimalSI)
	resourceList[uniapiext.AlibabaCloudReclaimedMemory] = *resource.NewQuantity(beMemory, resource.BinarySI)
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: node},
		Status: v1.NodeStatus{
			Capacity:    resourceList,
			Allocatable: resourceList,
		},
	}
}

func makeNode(node string, milliCPU, memory int64) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: node},
		Status: v1.NodeStatus{
			Capacity: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(milliCPU, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(memory, resource.BinarySI),
			},
			Allocatable: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(milliCPU, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(memory, resource.BinarySI),
			},
		},
	}
}
