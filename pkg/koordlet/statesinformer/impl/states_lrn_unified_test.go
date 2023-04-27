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

package impl

import (
	"fmt"
	"testing"

	faketopologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	fakekoordclientset "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned/fake"
	listerschedulingv1alpha1 "github.com/koordinator-sh/koordinator/pkg/client/listers/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metrics"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
)

func Test_lrnInformer(t *testing.T) {
	t.Run("test", func(t *testing.T) {
		nodeName := "test-node"
		opt := &PluginOption{
			config:      NewDefaultConfig(),
			KubeClient:  fakeclientset.NewSimpleClientset(),
			KoordClient: fakekoordclientset.NewSimpleClientset(),
			TopoClient:  faketopologyclientset.NewSimpleClientset(),
			NodeName:    nodeName,
		}

		ni := NewNodeInformer()
		pi := NewPodsInformer()
		informer := newLRNInformer()
		state := &PluginState{
			informerPlugins: map[PluginName]informerPlugin{
				nodeInformerName: ni,
				podsInformerName: pi,
				lrnInformerName:  informer,
			},
			callbackRunner: NewCallbackRunner(),
		}
		assert.NotPanics(t, func() {
			ni.Setup(opt, state)
			pi.Setup(opt, state)
			informer.Setup(opt, state)
			informer.GetLRN("test-lrn")
		})
	})
}

var _ listerschedulingv1alpha1.LogicalResourceNodeLister = &fakeLRNLister{}

type fakeLRNLister struct {
	objects   []*schedulingv1alpha1.LogicalResourceNode
	objectMap map[string]*schedulingv1alpha1.LogicalResourceNode
	listErr   bool
	getErr    map[string]bool
}

func newFakeLRNLister(listErr bool, objects ...*schedulingv1alpha1.LogicalResourceNode) *fakeLRNLister {
	m := map[string]*schedulingv1alpha1.LogicalResourceNode{}
	for i := range objects {
		m[objects[i].Name] = objects[i]
	}
	return &fakeLRNLister{
		objects:   objects,
		objectMap: m,
		listErr:   listErr,
		getErr:    map[string]bool{},
	}
}

func (f *fakeLRNLister) List(selector labels.Selector) (ret []*schedulingv1alpha1.LogicalResourceNode, err error) {
	if f.listErr {
		return nil, fmt.Errorf("got fake List error")
	}
	return f.objects, nil
}

func (f *fakeLRNLister) Get(name string) (*schedulingv1alpha1.LogicalResourceNode, error) {
	if f.getErr[name] {
		return nil, fmt.Errorf("got fake Get error")
	}
	return f.objectMap[name], nil
}

func Test_syncLRN(t *testing.T) {
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:       resource.MustParse("10"),
				corev1.ResourceMemory:    resource.MustParse("10Gi"),
				apiext.ResourceNvidiaGPU: resource.MustParse("2"),
			},
			Capacity: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:       resource.MustParse("10"),
				corev1.ResourceMemory:    resource.MustParse("12Gi"),
				apiext.ResourceNvidiaGPU: resource.MustParse("2"),
			},
		},
	}
	testLRN := &schedulingv1alpha1.LogicalResourceNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-lrn",
			Labels: map[string]string{
				schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode: testNode.Name,
			},
		},
		Status: schedulingv1alpha1.LogicalResourceNodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:       resource.MustParse("6"),
				corev1.ResourceMemory:    resource.MustParse("6Gi"),
				apiext.ResourceNvidiaGPU: resource.MustParse("2"),
			},
		},
	}
	testLRN1 := &schedulingv1alpha1.LogicalResourceNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-lrn-1",
			Labels: map[string]string{
				schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode: "other-node",
			},
		},
		Status: schedulingv1alpha1.LogicalResourceNodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:       resource.MustParse("4"),
				corev1.ResourceMemory:    resource.MustParse("4Gi"),
				apiext.ResourceNvidiaGPU: resource.MustParse("0"),
			},
		},
	}
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-ns",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test-container",
					Resources: corev1.ResourceRequirements{
						Requests: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU:       resource.MustParse("2"),
							corev1.ResourceMemory:    resource.MustParse("4Gi"),
							apiext.ResourceNvidiaGPU: resource.MustParse("1"),
						},
						Limits: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU:       resource.MustParse("2"),
							corev1.ResourceMemory:    resource.MustParse("4Gi"),
							apiext.ResourceNvidiaGPU: resource.MustParse("1"),
						},
					},
				},
			},
			NodeName: testNode.Name,
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:    "test-container",
					Started: pointer.Bool(true),
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	testPodonLRN := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-on-lrn",
			Namespace: "test-ns",
			Labels: map[string]string{
				schedulingv1alpha1.LabelLogicalResourceNodePodAssign: testLRN.Name,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test-container",
					Resources: corev1.ResourceRequirements{
						Requests: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU:       resource.MustParse("2"),
							corev1.ResourceMemory:    resource.MustParse("4Gi"),
							apiext.ResourceNvidiaGPU: resource.MustParse("1"),
						},
						Limits: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU:       resource.MustParse("2"),
							corev1.ResourceMemory:    resource.MustParse("4Gi"),
							apiext.ResourceNvidiaGPU: resource.MustParse("1"),
						},
					},
				},
			},
			NodeName: testNode.Name,
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:    "test-container",
					Started: pointer.Bool(true),
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	type fields struct {
		node      *corev1.Node
		podMap    map[string]*statesinformer.PodMeta
		lrnLister listerschedulingv1alpha1.LogicalResourceNodeLister
	}
	tests := []struct {
		name   string
		fields fields
	}{
		{
			name: "no lrn to sync",
			fields: fields{
				node: testNode,
				podMap: map[string]*statesinformer.PodMeta{
					"test-ns/test-pod": {
						Pod: testPod,
					},
				},
				lrnLister: newFakeLRNLister(false),
			},
		},
		{
			name: "sync lrn successfully",
			fields: fields{
				node: testNode,
				podMap: map[string]*statesinformer.PodMeta{
					"test-ns/test-pod-on-lrn": {
						Pod: testPodonLRN,
					},
				},
				lrnLister: newFakeLRNLister(false, testLRN),
			},
		},
		{
			name: "sync lrn successfully 1",
			fields: fields{
				node: testNode,
				podMap: map[string]*statesinformer.PodMeta{
					"test-ns/test-pod-on-lrn": {
						Pod: testPodonLRN,
					},
				},
				lrnLister: newFakeLRNLister(false, testLRN1),
			},
		},
		{
			name: "failed to list lrn ",
			fields: fields{
				node: testNode,
				podMap: map[string]*statesinformer.PodMeta{
					"test-ns/test-pod-on-lrn": {
						Pod: testPodonLRN,
					},
				},
				lrnLister: newFakeLRNLister(true, testLRN),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics.Register(tt.fields.node)
			defer metrics.Register(nil)

			ni := NewNodeInformer()
			pi := NewPodsInformer()
			ni.node = tt.fields.node
			pi.podMap = tt.fields.podMap
			li := newLRNInformer()
			li.nodeInformer = ni
			li.podsInformer = pi
			li.lrnLister = tt.fields.lrnLister

			assert.NotPanics(t, li.syncLRN)
		})
	}
}
