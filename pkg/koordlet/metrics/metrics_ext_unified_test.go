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

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func TestLRNCollectors(t *testing.T) {
	testingNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-node",
			Labels: map[string]string{},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("96"),
				corev1.ResourceMemory: resource.MustParse("180Gi"),
				apiext.BatchCPU:       resource.MustParse("50000"),
				apiext.BatchMemory:    resource.MustParse("80Gi"),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
				apiext.BatchCPU:       resource.MustParse("50000"),
				apiext.BatchMemory:    resource.MustParse("80Gi"),
			},
		},
	}
	testingLRN := &schedulingv1alpha1.LogicalResourceNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-lrn",
			Labels: map[string]string{
				schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode: "test-node",
				"ali/metric-node-bound-quotas":                        "xxx",
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
	testingLRN1 := &schedulingv1alpha1.LogicalResourceNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-lrn-3",
			Labels: map[string]string{
				schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode: "test-node",
				"ali/metric-node-bound-quotas":                        "xxx",
				"ali/metric-node-bound-quotas-two":                    "yyy",
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
	testingLRN2 := &schedulingv1alpha1.LogicalResourceNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-lrn-1",
			Labels: map[string]string{
				schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode: "test-node",
				"ali/metric-node-bound-quotas":                        "zzz",
			},
		},
		Status: schedulingv1alpha1.LogicalResourceNodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:       resource.MustParse("4"),
				corev1.ResourceMemory:    resource.MustParse("8Gi"),
				apiext.ResourceNvidiaGPU: resource.MustParse("2"),
			},
		},
	}
	testingLRNLabels := map[string]string{
		schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode: "test-node",
		"ali/metric-node-bound-quotas":                        "xxx",
		"ali/metric-node-bound-quotas-two":                    "yyy",
	}
	testingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test_pod",
			Namespace: "test_pod_namespace",
			UID:       "test01",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test_container",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "test_container",
					ContainerID: "containerd://testxxx",
				},
			},
		},
	}
	t.Run("test", func(t *testing.T) {
		Register(testingNode)
		defer Register(nil)

		RecordNodeResourceAllocatable(string(corev1.ResourceCPU), UnitCore, float64(testingNode.Status.Allocatable.Cpu().MilliValue())/1000)
		RecordNodeResourceAllocatable(string(corev1.ResourceMemory), UnitByte, float64(testingNode.Status.Allocatable.Memory().Value()))
		RecordNodeResourceAllocatable(AcceleratorResource, UnitInteger, float64(testingNode.Status.Allocatable.Name(apiext.ResourceNvidiaGPU, resource.DecimalSI).Value()))
		RecordLRNAllocatableCPUCores(testingLRN.Name, float64(testingLRN.Status.Allocatable.Cpu().MilliValue())/1000)
		RecordLRNAllocatableMemoryTotalBytes(testingLRN.Name, float64(testingLRN.Status.Allocatable.Memory().Value()))
		RecordLRNAllocatableAcceleratorTotal(testingLRN.Name, float64(testingLRN.Status.Allocatable.Name(apiext.ResourceNvidiaGPU, resource.DecimalSI).Value()))
		RecordLRNPods(testingLRN.Name, testingPod)
		RecordLRNContainers(testingLRN.Name, &testingPod.Status.ContainerStatuses[0], testingPod)
		RecordLRNAccelerators(testingLRN.Name, "gpu", "4")
		ResetNodeLRNs()
		RefreshNodeLRNsLabels(testingLRNLabels)
		RecordNodeLRNs(testingLRN.Name, testingLRN.Labels)
		RecordNodeLRNs(testingLRN1.Name, testingLRN1.Labels)
		RecordNodeLRNs(testingLRN2.Name, testingLRN2.Labels)
		RefreshNodeLRNsLabels(testingLRN2.Labels)
		RecordNodeLRNs(testingLRN2.Name, testingLRN2.Labels)
	})
}

func TestUnifiedCollectors(t *testing.T) {
	testingNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-node",
			Labels: map[string]string{},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:       resource.MustParse("96"),
				corev1.ResourceMemory:    resource.MustParse("180Gi"),
				apiext.BatchCPU:          resource.MustParse("50000"),
				apiext.BatchMemory:       resource.MustParse("80Gi"),
				apiext.ResourceNvidiaGPU: resource.MustParse("4"),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:       resource.MustParse("100"),
				corev1.ResourceMemory:    resource.MustParse("200Gi"),
				apiext.BatchCPU:          resource.MustParse("50000"),
				apiext.BatchMemory:       resource.MustParse("80Gi"),
				apiext.ResourceNvidiaGPU: resource.MustParse("4"),
			},
		},
	}
	t.Run("test", func(t *testing.T) {
		Register(testingNode)
		defer Register(nil)

		assert.NotPanics(t, func() {
			RecordNodeResourceAllocatableCPUCores(float64(testingNode.Status.Allocatable.Cpu().MilliValue()) / 1000)
			RecordNodeResourceAllocatableMemoryTotalBytes(float64(testingNode.Status.Allocatable.Memory().Value()))
			RecordNodeResourceAllocatableAcceleratorTotal(float64(testingNode.Status.Allocatable.Name(apiext.ResourceNvidiaGPU, resource.DecimalSI).Value()))
			RecordNodeResourceCapacityCPUCores(float64(testingNode.Status.Capacity.Cpu().MilliValue()) / 1000)
			RecordNodeResourceCapacityMemoryTotalBytes(float64(testingNode.Status.Capacity.Memory().Value()))
			RecordNodeResourceCapacityAcceleratorTotal(float64(testingNode.Status.Capacity.Name(apiext.ResourceNvidiaGPU, resource.DecimalSI).Value()))
		})
	})
}

func TestCPUStableCollectors(t *testing.T) {
	testingNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-node",
			Labels: map[string]string{},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:       resource.MustParse("96"),
				corev1.ResourceMemory:    resource.MustParse("180Gi"),
				apiext.BatchCPU:          resource.MustParse("50000"),
				apiext.BatchMemory:       resource.MustParse("80Gi"),
				apiext.ResourceNvidiaGPU: resource.MustParse("4"),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:       resource.MustParse("100"),
				corev1.ResourceMemory:    resource.MustParse("200Gi"),
				apiext.BatchCPU:          resource.MustParse("50000"),
				apiext.BatchMemory:       resource.MustParse("80Gi"),
				apiext.ResourceNvidiaGPU: resource.MustParse("4"),
			},
		},
	}
	t.Run("test", func(t *testing.T) {
		Register(testingNode)
		defer Register(nil)

		assert.NotPanics(t, func() {
			ResetPodHTRatio()
			RecordPodHTRatio("test-ns", "test", "test-uid", "none", 100.0)
		})
	})
}
