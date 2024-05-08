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

package resourcesummary

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	cosv1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/scheduling/v1beta1"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func Test_statisticsPodUsedResource(t *testing.T) {
	priorities := []int32{
		uniext.PriorityProdValueMax, uniext.PriorityBatchValueMax, uniext.PriorityBatchValueMax,
	}
	podStatistics := []v1beta1.PodStatistics{
		{
			Name: "test",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}
	var pods []*corev1.Pod
	for i := 0; i < 3; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test",
				Labels: map[string]string{
					"app": "test",
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "node-0",
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		pod.Name = fmt.Sprintf("pod-%d-%d", 0, 3)
		pod.Spec.Priority = &priorities[i]
		pods = append(pods, pod)
	}
	type args struct {
		candidateNodes *corev1.NodeList
		nodeOwnedPods  map[string][]*corev1.Pod
		podStatistics  []v1beta1.PodStatistics
	}
	tests := []struct {
		name    string
		args    args
		want    []map[uniext.PriorityClass]corev1.ResourceList
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "normal",
			args: args{
				candidateNodes: &corev1.NodeList{
					Items: []corev1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-0",
							},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Key:    "sigma.ali/resource-pool",
										Value:  "sigma_public",
										Effect: corev1.TaintEffectNoSchedule,
									},
								},
							},
							Status: corev1.NodeStatus{
								Allocatable: corev1.ResourceList{
									corev1.ResourceCPU:              resource.MustParse("110"),
									corev1.ResourceMemory:           resource.MustParse("100Gi"),
									corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
									extension.BatchCPU:              resource.MustParse("50000"),
									extension.BatchMemory:           resource.MustParse("10Gi"),
								},
								Conditions: []corev1.NodeCondition{{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								}},
							},
						},
					},
				},
				nodeOwnedPods: map[string][]*corev1.Pod{
					"node-0": pods,
				},
				podStatistics: podStatistics,
			},
			want: []map[uniext.PriorityClass]corev1.ResourceList{
				{
					uniext.PriorityProd: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					uniext.PriorityBatch: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := statisticsPodUsedResource(tt.args.candidateNodes, tt.args.nodeOwnedPods, tt.args.podStatistics, nil)
			tt.wantErr(t, err)
			for priorityClassType, resourceList := range tt.want[0] {
				assert.True(t, quotav1.Equals(resourceList, got[0][priorityClassType]))
			}
		})
	}
}

func Test_statisticsNodesResource(t *testing.T) {
	priorities := []int32{
		uniext.PriorityProdValueMax, uniext.PriorityBatchValueMax, uniext.PriorityBatchValueMax,
	}
	var node0Pods []*corev1.Pod
	for i := 0; i < 3; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test",
				Labels: map[string]string{
					"app": "test",
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "node-0",
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:               resource.MustParse("1.5"),
								corev1.ResourceMemory:            resource.MustParse("4Gi"),
								extension.ResourceGPUMemoryRatio: resource.MustParse("100"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		pod.Name = fmt.Sprintf("pod-%s-%d", "node-0", i)
		pod.Spec.Priority = &priorities[i]
		node0Pods = append(node0Pods, pod)
	}
	var node0Reservations []*schedulingv1alpha1.Reservation
	for i := 0; i < 3; i++ {
		reservation := &schedulingv1alpha1.Reservation{
			Spec: schedulingv1alpha1.ReservationSpec{
				Template: &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:                     resource.MustParse("4000m"),
										corev1.ResourceMemory:                  resource.MustParse("4Gi"),
										unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
									},
								},
							},
						},
					},
				},
			},
			Status: schedulingv1alpha1.ReservationStatus{
				Phase: schedulingv1alpha1.ReservationAvailable,
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("4000m"),
					corev1.ResourceMemory:                  resource.MustParse("4Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
				},
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("3000m"),
					corev1.ResourceMemory:                  resource.MustParse("2Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
				},
				CurrentOwners: []corev1.ObjectReference{{}},
			},
		}
		reservation.Name = fmt.Sprintf("reservation-%s-%d", "node-0", i)
		reservation.Spec.Template.Spec.Priority = &priorities[i]
		if i == 2 {
			reservation.Status.Allocated = corev1.ResourceList{}
			reservation.Status.CurrentOwners = nil
		}
		node0Reservations = append(node0Reservations, reservation)

	}
	var node1Pods []*corev1.Pod
	for i := 0; i < 3; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test",
				Labels: map[string]string{
					"app": "test",
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:          resource.MustParse("1.5"),
								corev1.ResourceMemory:       resource.MustParse("4Gi"),
								extension.ResourceNvidiaGPU: resource.MustParse("1"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		pod.Name = fmt.Sprintf("pod-%s-%d", "node-1", i)
		pod.Spec.Priority = &priorities[i]
		node1Pods = append(node1Pods, pod)
	}
	var node1Reservations []*schedulingv1alpha1.Reservation
	for i := 0; i < 3; i++ {
		reservation := &schedulingv1alpha1.Reservation{
			Spec: schedulingv1alpha1.ReservationSpec{
				Template: &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:                     resource.MustParse("4000m"),
										corev1.ResourceMemory:                  resource.MustParse("4Gi"),
										unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
									},
								},
							},
						},
					},
				},
			},
			Status: schedulingv1alpha1.ReservationStatus{
				Phase: schedulingv1alpha1.ReservationAvailable,
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("4000m"),
					corev1.ResourceMemory:                  resource.MustParse("4Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
				},
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:                     resource.MustParse("3000m"),
					corev1.ResourceMemory:                  resource.MustParse("2Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
				},
				CurrentOwners: []corev1.ObjectReference{{}},
			},
		}
		reservation.Name = fmt.Sprintf("reservation-%s-%d", "node-1", i)
		reservation.Spec.Template.Spec.Priority = &priorities[i]
		if i == 0 {
			reservation.Status.Allocated = corev1.ResourceList{}
			reservation.Status.CurrentOwners = nil
		}
		node1Reservations = append(node1Reservations, reservation)

	}
	type args struct {
		candidateNodes        *corev1.NodeList
		nodeOwnedPods         map[string][]*corev1.Pod
		nodeOwnedReservations map[string][]*schedulingv1alpha1.Reservation
		resourceSpecs         []v1beta1.ResourceSpec
		nodeGPUCapacity       map[string]corev1.ResourceList
	}
	tests := []struct {
		name                     string
		args                     args
		wantCapacity             map[uniext.PriorityClass]corev1.ResourceList
		wantRequested            map[uniext.PriorityClass]corev1.ResourceList
		wantFree                 map[uniext.PriorityClass]corev1.ResourceList
		wantReservationCapacity  map[uniext.PriorityClass]corev1.ResourceList
		wantReservationRequested map[uniext.PriorityClass]corev1.ResourceList
		wantReservationFree      map[uniext.PriorityClass]corev1.ResourceList
		wantAllocatableNums      map[string]map[uniext.PriorityClass]int32
	}{
		{
			name: "normal",
			args: args{
				candidateNodes: &corev1.NodeList{
					Items: []corev1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-0",
							},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Key:    "sigma.ali/resource-pool",
										Value:  "sigma_public",
										Effect: corev1.TaintEffectNoSchedule,
									},
								},
							},
							Status: corev1.NodeStatus{
								Allocatable: corev1.ResourceList{
									corev1.ResourceCPU:              resource.MustParse("110"),
									corev1.ResourceMemory:           resource.MustParse("100Gi"),
									corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
									extension.BatchCPU:              resource.MustParse("50000"),
									extension.BatchMemory:           resource.MustParse("10Gi"),
								},
								Conditions: []corev1.NodeCondition{{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								}},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
							},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Key:    "sigma.ali/resource-pool",
										Value:  "sigma_public",
										Effect: corev1.TaintEffectNoSchedule,
									},
								},
							},
							Status: corev1.NodeStatus{
								Allocatable: corev1.ResourceList{
									corev1.ResourceCPU:              resource.MustParse("110"),
									corev1.ResourceMemory:           resource.MustParse("100Gi"),
									corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
									extension.BatchCPU:              resource.MustParse("50000"),
									extension.BatchMemory:           resource.MustParse("10Gi"),
								},
								Conditions: []corev1.NodeCondition{{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								}},
							},
						},
					},
				},
				nodeOwnedPods: map[string][]*corev1.Pod{
					"node-0": node0Pods,
					"node-1": node1Pods,
				},
				nodeGPUCapacity: map[string]corev1.ResourceList{
					"node-0": nil,
					"node-1": nil,
				},
				resourceSpecs: []v1beta1.ResourceSpec{
					{
						Name:      "test",
						Resources: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
					},
				},
			},
			wantCapacity: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:              resource.MustParse("220"),
					corev1.ResourceMemory:           resource.MustParse("200Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:              resource.MustParse("100"),
					corev1.ResourceMemory:           resource.MustParse("20Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
				},
				uniext.PriorityMid: {
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
				},
				uniext.PriorityFree: {
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
				},
			},
			wantRequested: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:    resource.MustParse("3"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:    resource.MustParse("6"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
				uniext.PriorityMid:  {},
				uniext.PriorityFree: {},
			},
			wantFree: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:              resource.MustParse("217"),
					corev1.ResourceMemory:           resource.MustParse("176Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:              resource.MustParse("94"),
					corev1.ResourceMemory:           resource.MustParse("4Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
				},
				uniext.PriorityMid: {
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
				},
				uniext.PriorityFree: {
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
				},
			},
			wantAllocatableNums: map[string]map[uniext.PriorityClass]int32{
				"test": {
					uniext.PriorityProd:  216,
					uniext.PriorityBatch: 94,
					uniext.PriorityMid:   0,
					uniext.PriorityFree:  0,
				},
			},
		},
		{
			name: "normal; gpu",
			args: args{
				candidateNodes: &corev1.NodeList{
					Items: []corev1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-0",
							},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Key:    "sigma.ali/resource-pool",
										Value:  "sigma_public",
										Effect: corev1.TaintEffectNoSchedule,
									},
								},
							},
							Status: corev1.NodeStatus{
								Allocatable: corev1.ResourceList{
									corev1.ResourceCPU:              resource.MustParse("110"),
									corev1.ResourceMemory:           resource.MustParse("100Gi"),
									corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
									extension.BatchCPU:              resource.MustParse("50000"),
									extension.BatchMemory:           resource.MustParse("10Gi"),
								},
								Conditions: []corev1.NodeCondition{{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								}},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
							},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Key:    "sigma.ali/resource-pool",
										Value:  "sigma_public",
										Effect: corev1.TaintEffectNoSchedule,
									},
								},
							},
							Status: corev1.NodeStatus{
								Allocatable: corev1.ResourceList{
									corev1.ResourceCPU:              resource.MustParse("110"),
									corev1.ResourceMemory:           resource.MustParse("100Gi"),
									corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
									extension.BatchCPU:              resource.MustParse("50000"),
									extension.BatchMemory:           resource.MustParse("10Gi"),
								},
								Conditions: []corev1.NodeCondition{{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								}},
							},
						},
					},
				},
				nodeOwnedPods: map[string][]*corev1.Pod{
					"node-0": node0Pods,
					"node-1": node1Pods,
				},
				nodeGPUCapacity: map[string]corev1.ResourceList{
					"node-0": map[corev1.ResourceName]resource.Quantity{
						unifiedresourceext.GPUResourceCore:     resource.MustParse("800"),
						unifiedresourceext.GPUResourceMem:      resource.MustParse("800Gi"),
						unifiedresourceext.GPUResourceMemRatio: resource.MustParse("800"),
					},
					"node-1": map[corev1.ResourceName]resource.Quantity{
						unifiedresourceext.GPUResourceCore:     resource.MustParse("800"),
						unifiedresourceext.GPUResourceMem:      resource.MustParse("800Gi"),
						unifiedresourceext.GPUResourceMemRatio: resource.MustParse("800"),
					},
				},
				resourceSpecs: []v1beta1.ResourceSpec{
					{
						Name:      "test",
						Resources: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
					},
				},
			},
			wantCapacity: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:                     resource.MustParse("220"),
					corev1.ResourceMemory:                  resource.MustParse("200Gi"),
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("1600"),
					extension.ResourceGPUCore:              resource.MustParse("1600"),
					extension.ResourceGPUMemory:            resource.MustParse("1600Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("1600"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("1600"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("1600"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("1600Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("1600"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:                     resource.MustParse("100"),
					corev1.ResourceMemory:                  resource.MustParse("20Gi"),
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("1600"),
					extension.ResourceGPUCore:              resource.MustParse("1600"),
					extension.ResourceGPUMemory:            resource.MustParse("1600Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("1600"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("1600"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("1600"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("1600Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("1600"),
				},
				uniext.PriorityMid: {
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("1600"),
					extension.ResourceGPUCore:              resource.MustParse("1600"),
					extension.ResourceGPUMemory:            resource.MustParse("1600Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("1600"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("1600"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("1600"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("1600Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("1600"),
				},
				uniext.PriorityFree: {
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("1600"),
					extension.ResourceGPUCore:              resource.MustParse("1600"),
					extension.ResourceGPUMemory:            resource.MustParse("1600Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("1600"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("1600"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("1600"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("1600Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("1600"),
				},
			},
			wantRequested: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:                     resource.MustParse("3"),
					corev1.ResourceMemory:                  resource.MustParse("8Gi"),
					extension.ResourceGPU:                  resource.MustParse("200"),
					extension.ResourceGPUCore:              resource.MustParse("200"),
					extension.ResourceGPUMemory:            resource.MustParse("200Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("200"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("200"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("200"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("200Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:                     resource.MustParse("6"),
					corev1.ResourceMemory:                  resource.MustParse("16Gi"),
					extension.ResourceGPU:                  resource.MustParse("400"),
					extension.ResourceGPUCore:              resource.MustParse("400"),
					extension.ResourceGPUMemory:            resource.MustParse("400Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("400"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("400"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("400"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("400Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("400"),
				},
				uniext.PriorityMid:  {},
				uniext.PriorityFree: {},
			},
			wantFree: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:                     resource.MustParse("217"),
					corev1.ResourceMemory:                  resource.MustParse("176Gi"),
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("1000"),
					extension.ResourceGPUCore:              resource.MustParse("1000"),
					extension.ResourceGPUMemory:            resource.MustParse("1000Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("1000"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("1000"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("1000"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("1000Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("1000"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:                     resource.MustParse("94"),
					corev1.ResourceMemory:                  resource.MustParse("4Gi"),
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("1000"),
					extension.ResourceGPUCore:              resource.MustParse("1000"),
					extension.ResourceGPUMemory:            resource.MustParse("1000Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("1000"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("1000"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("1000"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("1000Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("1000"),
				},
				uniext.PriorityMid: {
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("1000"),
					extension.ResourceGPUCore:              resource.MustParse("1000"),
					extension.ResourceGPUMemory:            resource.MustParse("1000Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("1000"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("1000"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("1000"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("1000Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("1000"),
				},
				uniext.PriorityFree: {
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("1000"),
					extension.ResourceGPUCore:              resource.MustParse("1000"),
					extension.ResourceGPUMemory:            resource.MustParse("1000Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("1000"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("1000"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("1000"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("1000Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("1000"),
				},
			},
			wantAllocatableNums: map[string]map[uniext.PriorityClass]int32{
				"test": {
					uniext.PriorityProd:  216,
					uniext.PriorityBatch: 94,
					uniext.PriorityMid:   0,
					uniext.PriorityFree:  0,
				},
			},
		},
		{
			name: "normal; gpu; reservation",
			args: args{
				candidateNodes: &corev1.NodeList{
					Items: []corev1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-0",
							},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Key:    "sigma.ali/resource-pool",
										Value:  "sigma_public",
										Effect: corev1.TaintEffectNoSchedule,
									},
								},
							},
							Status: corev1.NodeStatus{
								Allocatable: corev1.ResourceList{
									corev1.ResourceCPU:              resource.MustParse("110"),
									corev1.ResourceMemory:           resource.MustParse("100Gi"),
									corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
									extension.BatchCPU:              resource.MustParse("50000"),
									extension.BatchMemory:           resource.MustParse("30Gi"),
									corev1.ResourcePods:             resource.MustParse("200"),
								},
								Conditions: []corev1.NodeCondition{{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								}},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
							},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Key:    "sigma.ali/resource-pool",
										Value:  "sigma_public",
										Effect: corev1.TaintEffectNoSchedule,
									},
								},
							},
							Status: corev1.NodeStatus{
								Allocatable: corev1.ResourceList{
									corev1.ResourceCPU:              resource.MustParse("110"),
									corev1.ResourceMemory:           resource.MustParse("100Gi"),
									corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
									extension.BatchCPU:              resource.MustParse("50000"),
									extension.BatchMemory:           resource.MustParse("30Gi"),
									corev1.ResourcePods:             resource.MustParse("200"),
								},
								Conditions: []corev1.NodeCondition{{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								}},
							},
						},
					},
				},
				nodeOwnedPods: map[string][]*corev1.Pod{
					"node-0": node0Pods,
					"node-1": node1Pods,
				},
				nodeOwnedReservations: map[string][]*schedulingv1alpha1.Reservation{
					"node-0": node0Reservations,
					"node-1": node1Reservations,
				},
				nodeGPUCapacity: map[string]corev1.ResourceList{
					"node-0": map[corev1.ResourceName]resource.Quantity{
						unifiedresourceext.GPUResourceCore:     resource.MustParse("800"),
						unifiedresourceext.GPUResourceMem:      resource.MustParse("800Gi"),
						unifiedresourceext.GPUResourceMemRatio: resource.MustParse("800"),
					},
					"node-1": map[corev1.ResourceName]resource.Quantity{
						unifiedresourceext.GPUResourceCore:     resource.MustParse("800"),
						unifiedresourceext.GPUResourceMem:      resource.MustParse("800Gi"),
						unifiedresourceext.GPUResourceMemRatio: resource.MustParse("800"),
					},
				},
				resourceSpecs: []v1beta1.ResourceSpec{
					{
						Name: "test",
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
			wantCapacity: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:                     resource.MustParse("220"),
					corev1.ResourceMemory:                  resource.MustParse("200Gi"),
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("1600"),
					extension.ResourceGPUCore:              resource.MustParse("1600"),
					extension.ResourceGPUMemory:            resource.MustParse("1600Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("1600"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("1600"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("1600"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("1600Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("1600"),
					corev1.ResourcePods:                    resource.MustParse("400"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:                     resource.MustParse("100"),
					corev1.ResourceMemory:                  resource.MustParse("60Gi"),
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("1600"),
					extension.ResourceGPUCore:              resource.MustParse("1600"),
					extension.ResourceGPUMemory:            resource.MustParse("1600Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("1600"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("1600"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("1600"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("1600Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("1600"),
					corev1.ResourcePods:                    resource.MustParse("400"),
				},
				uniext.PriorityMid: {
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("1600"),
					extension.ResourceGPUCore:              resource.MustParse("1600"),
					extension.ResourceGPUMemory:            resource.MustParse("1600Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("1600"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("1600"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("1600"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("1600Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("1600"),
					corev1.ResourcePods:                    resource.MustParse("400"),
				},
				uniext.PriorityFree: {
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("1600"),
					extension.ResourceGPUCore:              resource.MustParse("1600"),
					extension.ResourceGPUMemory:            resource.MustParse("1600Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("1600"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("1600"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("1600"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("1600Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("1600"),
					corev1.ResourcePods:                    resource.MustParse("400"),
				},
			},
			wantRequested: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:                     resource.MustParse("8"),
					corev1.ResourceMemory:                  resource.MustParse("14Gi"),
					extension.ResourceGPU:                  resource.MustParse("500"),
					extension.ResourceGPUCore:              resource.MustParse("500"),
					extension.ResourceGPUMemory:            resource.MustParse("500Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("500"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("500"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("500"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("500Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("500"),
					corev1.ResourcePods:                    resource.MustParse("4"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:                     resource.MustParse("13"),
					corev1.ResourceMemory:                  resource.MustParse("26Gi"),
					extension.ResourceGPU:                  resource.MustParse("900"),
					extension.ResourceGPUCore:              resource.MustParse("900"),
					extension.ResourceGPUMemory:            resource.MustParse("900Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("900"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("900"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("900"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("900Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("900"),
					corev1.ResourcePods:                    resource.MustParse("8"),
				},
				uniext.PriorityMid:  {},
				uniext.PriorityFree: {},
			},
			wantFree: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:                     resource.MustParse("212"),
					corev1.ResourceMemory:                  resource.MustParse("160Gi"),
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("200"),
					extension.ResourceGPUCore:              resource.MustParse("200"),
					extension.ResourceGPUMemory:            resource.MustParse("200Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("200"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("200"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("200"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("200Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
					corev1.ResourcePods:                    resource.MustParse("388"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:                     resource.MustParse("87"),
					corev1.ResourceMemory:                  resource.MustParse("34Gi"),
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("200"),
					extension.ResourceGPUCore:              resource.MustParse("200"),
					extension.ResourceGPUMemory:            resource.MustParse("200Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("200"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("200"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("200"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("200Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
					corev1.ResourcePods:                    resource.MustParse("388"),
				},
				uniext.PriorityMid: {
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("200"),
					extension.ResourceGPUCore:              resource.MustParse("200"),
					extension.ResourceGPUMemory:            resource.MustParse("200Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("200"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("200"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("200"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("200Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
					corev1.ResourcePods:                    resource.MustParse("388"),
				},
				uniext.PriorityFree: {
					corev1.ResourceEphemeralStorage:        resource.MustParse("400Gi"),
					extension.ResourceGPU:                  resource.MustParse("200"),
					extension.ResourceGPUCore:              resource.MustParse("200"),
					extension.ResourceGPUMemory:            resource.MustParse("200Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("200"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("200"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("200"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("200Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("200"),
					corev1.ResourcePods:                    resource.MustParse("388"),
				},
			},
			wantReservationCapacity: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:                     resource.MustParse("8000m"),
					corev1.ResourceMemory:                  resource.MustParse("8Gi"),
					extension.ResourceGPU:                  resource.MustParse("400"),
					extension.ResourceGPUCore:              resource.MustParse("400"),
					extension.ResourceGPUMemory:            resource.MustParse("400Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("400"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("400"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("400"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("400Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("400"),
					corev1.ResourcePods:                    resource.MustParse("3"),
				},
				uniext.PriorityBatch: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:                     resource.MustParse("16000m"),
					corev1.ResourceMemory:                  resource.MustParse("16Gi"),
					extension.ResourceGPU:                  resource.MustParse("800"),
					extension.ResourceGPUCore:              resource.MustParse("800"),
					extension.ResourceGPUMemory:            resource.MustParse("800Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("800"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("800"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("800"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("800Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("800"),
					corev1.ResourcePods:                    resource.MustParse("7"),
				},
				uniext.PriorityMid:  {},
				uniext.PriorityFree: {},
			},
			wantReservationRequested: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:                     resource.MustParse("3000m"),
					corev1.ResourceMemory:                  resource.MustParse("2Gi"),
					extension.ResourceGPU:                  resource.MustParse("100"),
					extension.ResourceGPUCore:              resource.MustParse("100"),
					extension.ResourceGPUMemory:            resource.MustParse("100Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("100"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("100"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("100"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("100Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("100"),
					corev1.ResourcePods:                    resource.MustParse("1"),
				},
				uniext.PriorityBatch: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:                     resource.MustParse("9000m"),
					corev1.ResourceMemory:                  resource.MustParse("6Gi"),
					extension.ResourceGPU:                  resource.MustParse("300"),
					extension.ResourceGPUCore:              resource.MustParse("300"),
					extension.ResourceGPUMemory:            resource.MustParse("300Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("300"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("300"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("300"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("300Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("300"),
					corev1.ResourcePods:                    resource.MustParse("3"),
				},
				uniext.PriorityMid:  {},
				uniext.PriorityFree: {},
			},
			wantReservationFree: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:                     resource.MustParse("5000m"),
					corev1.ResourceMemory:                  resource.MustParse("6Gi"),
					extension.ResourceGPU:                  resource.MustParse("300"),
					extension.ResourceGPUCore:              resource.MustParse("300"),
					extension.ResourceGPUMemory:            resource.MustParse("300Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("300"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("300"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("300"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("300Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("300"),
					corev1.ResourcePods:                    resource.MustParse("2"),
				},
				uniext.PriorityBatch: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:                     resource.MustParse("7000m"),
					corev1.ResourceMemory:                  resource.MustParse("10Gi"),
					extension.ResourceGPU:                  resource.MustParse("500"),
					extension.ResourceGPUCore:              resource.MustParse("500"),
					extension.ResourceGPUMemory:            resource.MustParse("500Gi"),
					extension.ResourceGPUMemoryRatio:       resource.MustParse("500"),
					unifiedresourceext.GPUResourceAlibaba:  resource.MustParse("500"),
					unifiedresourceext.GPUResourceCore:     resource.MustParse("500"),
					unifiedresourceext.GPUResourceMem:      resource.MustParse("500Gi"),
					unifiedresourceext.GPUResourceMemRatio: resource.MustParse("500"),
					corev1.ResourcePods:                    resource.MustParse("4"),
				},
				uniext.PriorityMid:  {},
				uniext.PriorityFree: {},
			},
			wantAllocatableNums: map[string]map[uniext.PriorityClass]int32{
				"test": {
					uniext.PriorityProd:  211,
					uniext.PriorityBatch: 87,
					uniext.PriorityMid:   0,
					uniext.PriorityFree:  0,
				},
			},
		},
		{
			name: "allocatablePodNums",
			args: args{
				candidateNodes: &corev1.NodeList{
					Items: []corev1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-0",
							},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Key:    "sigma.ali/resource-pool",
										Value:  "sigma_public",
										Effect: corev1.TaintEffectNoSchedule,
									},
								},
							},
							Status: corev1.NodeStatus{
								Allocatable: corev1.ResourceList{
									corev1.ResourceCPU:              resource.MustParse("110"),
									corev1.ResourceMemory:           resource.MustParse("100Gi"),
									corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
									corev1.ResourcePods:             resource.MustParse("50"),
									extension.BatchCPU:              resource.MustParse("50000"),
									extension.BatchMemory:           resource.MustParse("10Gi"),
								},
								Conditions: []corev1.NodeCondition{{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								}},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
							},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Key:    "sigma.ali/resource-pool",
										Value:  "sigma_public",
										Effect: corev1.TaintEffectNoSchedule,
									},
								},
							},
							Status: corev1.NodeStatus{
								Allocatable: corev1.ResourceList{
									corev1.ResourceCPU:              resource.MustParse("110"),
									corev1.ResourceMemory:           resource.MustParse("100Gi"),
									corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
									corev1.ResourcePods:             resource.MustParse("50"),
									extension.BatchCPU:              resource.MustParse("50000"),
									extension.BatchMemory:           resource.MustParse("10Gi"),
								},
								Conditions: []corev1.NodeCondition{{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								}},
							},
						},
					},
				},
				nodeOwnedPods: map[string][]*corev1.Pod{
					"node-0": node0Pods,
					"node-1": node1Pods,
				},
				resourceSpecs: []v1beta1.ResourceSpec{
					{
						Name:      "test",
						Resources: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
					},
				},
			},
			wantCapacity: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:              resource.MustParse("220"),
					corev1.ResourceMemory:           resource.MustParse("200Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
					corev1.ResourcePods:             resource.MustParse("100"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:              resource.MustParse("100"),
					corev1.ResourceMemory:           resource.MustParse("20Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
					corev1.ResourcePods:             resource.MustParse("100"),
				},
				uniext.PriorityMid: {
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
					corev1.ResourcePods:             resource.MustParse("100"),
				},
				uniext.PriorityFree: {
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
					corev1.ResourcePods:             resource.MustParse("100"),
				},
			},
			wantRequested: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:    resource.MustParse("3"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					corev1.ResourcePods:   resource.MustParse("2"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:    resource.MustParse("6"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
					corev1.ResourcePods:   resource.MustParse("4"),
				},
				uniext.PriorityMid:  {},
				uniext.PriorityFree: {},
			},
			wantFree: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:              resource.MustParse("217"),
					corev1.ResourceMemory:           resource.MustParse("176Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
					corev1.ResourcePods:             resource.MustParse("94"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:              resource.MustParse("94"),
					corev1.ResourceMemory:           resource.MustParse("4Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
					corev1.ResourcePods:             resource.MustParse("94"),
				},
				uniext.PriorityMid: {
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
					corev1.ResourcePods:             resource.MustParse("94"),
				},
				uniext.PriorityFree: {
					corev1.ResourceEphemeralStorage: resource.MustParse("400Gi"),
					corev1.ResourcePods:             resource.MustParse("94"),
				},
			},
			wantAllocatableNums: map[string]map[uniext.PriorityClass]int32{
				"test": {
					uniext.PriorityProd:  94,
					uniext.PriorityBatch: 94,
					uniext.PriorityMid:   0,
					uniext.PriorityFree:  0,
				},
			},
		},
		{
			name: "allocatablePodNums when candidate node is nil",
			args: args{
				candidateNodes: &corev1.NodeList{
					Items: nil,
				},
				nodeOwnedPods: map[string][]*corev1.Pod{},
				resourceSpecs: []v1beta1.ResourceSpec{
					{
						Name:      "test",
						Resources: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
					},
				},
			},
			wantCapacity: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd:  {},
				uniext.PriorityBatch: {},
				uniext.PriorityMid:   {},
				uniext.PriorityFree:  {},
			},
			wantRequested: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd:  {},
				uniext.PriorityBatch: {},
				uniext.PriorityMid:   {},
				uniext.PriorityFree:  {},
			},
			wantFree: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd:  {},
				uniext.PriorityBatch: {},
				uniext.PriorityMid:   {},
				uniext.PriorityFree:  {},
			},
			wantAllocatableNums: map[string]map[uniext.PriorityClass]int32{
				"test": {
					uniext.PriorityProd:  0,
					uniext.PriorityBatch: 0,
					uniext.PriorityMid:   0,
					uniext.PriorityFree:  0,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCapacity, gotRequested, gotFree, gotReservationCapacity, gotReservationRequested, gotReservationFree, gotAllocatableNums := statisticsNodeRelated(tt.args.candidateNodes, tt.args.nodeOwnedPods, tt.args.nodeOwnedReservations, tt.args.resourceSpecs, tt.args.nodeGPUCapacity)
			for _, priorityClassType := range priorityClassTypes {
				klog.Info(priorityClassType)
				klog.Info(tt.wantFree[priorityClassType])
				klog.Info(gotFree[priorityClassType])
				assert.True(t, quotav1.Equals(tt.wantReservationCapacity[priorityClassType], gotReservationCapacity[priorityClassType]))
				assert.True(t, quotav1.Equals(tt.wantReservationRequested[priorityClassType], gotReservationRequested[priorityClassType]))
				assert.True(t, quotav1.Equals(tt.wantReservationFree[priorityClassType], gotReservationFree[priorityClassType]))
				assert.True(t, quotav1.Equals(tt.wantCapacity[priorityClassType], gotCapacity[priorityClassType]))
				assert.True(t, quotav1.Equals(tt.wantRequested[priorityClassType], gotRequested[priorityClassType]))
				assert.True(t, quotav1.Equals(tt.wantFree[priorityClassType], gotFree[priorityClassType]))
				for _, resourceSpec := range tt.args.resourceSpecs {
					assert.Equal(t, tt.wantAllocatableNums[resourceSpec.Name][priorityClassType], gotAllocatableNums[resourceSpec.Name][priorityClassType])
				}
			}
		})
	}
}

func TestResourceSummaryReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)
	err = v1beta1.AddToScheme(scheme)
	assert.NoError(t, err)
	err = schedulingv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)
	scheme.AddKnownTypes(cosv1beta1.GroupVersion, &cosv1beta1.Device{})
	metav1.AddToGroupVersion(scheme, cosv1beta1.GroupVersion)

	assert.NoError(t, err)
	client := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&v1beta1.ResourceSummary{}).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(object client.Object) []string {
			return []string{object.(*corev1.Pod).Spec.NodeName}
		}).
		WithIndex(&schedulingv1alpha1.Reservation{}, "status.nodeName", func(object client.Object) []string {
			return []string{object.(*schedulingv1alpha1.Reservation).Status.NodeName}
		}).
		Build()
	r := &Reconciler{
		Client: client,
	}

	nodeName := fmt.Sprintf("node-%d", 0)
	err = r.Client.Create(context.Background(), &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{
					Key:    "sigma.ali/resource-pool",
					Value:  "sigma_public",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse("110"),
				corev1.ResourceMemory:           resource.MustParse("100Gi"),
				corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				extension.BatchCPU:              resource.MustParse("50000"),
				extension.BatchMemory:           resource.MustParse("10Gi"),
			},
			Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			}},
		},
	})
	assert.NoError(t, err)

	reservation := &schedulingv1alpha1.Reservation{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("reservation-%s-%d", "node-0", 0),
		},
		Spec: schedulingv1alpha1.ReservationSpec{
			Template: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4000m"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
						},
					},
					Priority: pointer.Int32(uniext.PriorityProdValueMax),
				},
			},
		},
		Status: schedulingv1alpha1.ReservationStatus{
			Phase: schedulingv1alpha1.ReservationAvailable,
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4000m"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
			Allocated: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1000m"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
			CurrentOwners: []corev1.ObjectReference{
				{
					Namespace: "default",
					Name:      fmt.Sprintf("pod-%d-%d", 0, 1),
				},
			},
			NodeName: nodeName,
		},
	}
	assert.NoError(t, r.Client.Create(context.Background(), reservation))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	pod.Name = fmt.Sprintf("pod-%d-%d", 0, 1)
	priority := uniext.PriorityProdValueMax
	pod.Spec.Priority = &priority
	extension.SetReservationAllocated(pod, reservation)
	err = r.Client.Create(context.Background(), pod)
	assert.NoError(t, err)

	pod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	pod.Name = fmt.Sprintf("pod-%d-%d", 0, 2)
	priority = uniext.PriorityBatchValueMax
	pod.Spec.Priority = &priority
	err = r.Client.Create(context.Background(), pod)
	assert.NoError(t, err)

	pod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	pod.Name = fmt.Sprintf("pod-%d-%d", 0, 3)
	priority = uniext.PriorityBatchValueMax
	pod.Spec.Priority = &priority
	err = r.Client.Create(context.Background(), pod)
	assert.NoError(t, err)

	resourceSummary := &v1beta1.ResourceSummary{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-summary",
			Namespace: "test",
		},
		Spec: v1beta1.ResourceSummarySpec{
			Tolerations: []corev1.Toleration{
				{
					Key:      "sigma.ali/resource-pool",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			PodStatistics: []v1beta1.PodStatistics{
				{
					Name: "test",
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			},
			ResourceSpecs: []v1beta1.ResourceSpec{
				{
					Name: "50C50Gi",
					Resources: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50"),
						corev1.ResourceMemory: resource.MustParse("50Gi"),
					},
				},
				{
					Name: "500C500Gi",
					Resources: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500"),
						corev1.ResourceMemory: resource.MustParse("500Gi"),
					},
				},
				{
					Name: "10C1Gi",
					Resources: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			},
		},
	}
	key := types.NamespacedName{
		Namespace: resourceSummary.Namespace,
		Name:      resourceSummary.Name,
	}
	err = r.Create(context.Background(), resourceSummary)
	assert.NoError(t, err)

	_, err = r.Reconcile(context.TODO(), ctrl.Request{NamespacedName: key})
	assert.NoError(t, err)

	newSummary := &v1beta1.ResourceSummary{}
	err = r.Client.Get(context.Background(), key, newSummary)
	assert.NoError(t, err)
	sort.Slice(newSummary.Status.Resources, func(i, j int) bool {
		return newSummary.Status.Resources[i].PriorityClass < newSummary.Status.Resources[j].PriorityClass
	})
	sort.Slice(newSummary.Status.ResourceSpecStats, func(i, j int) bool {
		return newSummary.Status.ResourceSpecStats[i].Name < newSummary.Status.ResourceSpecStats[j].Name
	})
	for _, resourceSpecStat := range newSummary.Status.ResourceSpecStats {
		sort.Slice(resourceSpecStat.Allocatable, func(i, j int) bool {
			return resourceSpecStat.Allocatable[i].PriorityClass < resourceSpecStat.Allocatable[j].PriorityClass
		})
	}
	newPodUsedStatistics := newSummary.Status.PodUsedStatistics[0]
	sort.Slice(newPodUsedStatistics.Allocated, func(i, j int) bool {
		return newPodUsedStatistics.Allocated[i].PriorityClass < newPodUsedStatistics.Allocated[j].PriorityClass
	})

	expectedSummary := newSummary.DeepCopy()
	expectedSummary.Status = v1beta1.ResourceSummaryStatus{
		UpdateTimestamp: newSummary.Status.UpdateTimestamp,
		Phase:           v1beta1.ResourceSummarySucceeded,
		NumNodes:        1,
		Resources: []*v1beta1.NodeResourceSummary{
			{
				PriorityClass: uniext.PriorityBatch,
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("50"),
					corev1.ResourceMemory:           resource.MustParse("10Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("48"),
					corev1.ResourceMemory:           resource.MustParse("2Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			{
				PriorityClass: uniext.PriorityFree,
				Capacity: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				Allocated: corev1.ResourceList{},
				Allocatable: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
			},
			{
				PriorityClass: uniext.PriorityMid,
				Capacity: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				Allocated: corev1.ResourceList{},
				Allocatable: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
			},
			{
				PriorityClass: uniext.PriorityProd,
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("110"),
					corev1.ResourceMemory:           resource.MustParse("100Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("106"),
					corev1.ResourceMemory:           resource.MustParse("88Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
				ReserveResourceCapacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
				ReserveResourceAllocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("3"),
					corev1.ResourceMemory: resource.MustParse("0Gi"),
				},
				ReserveResourceAllocated: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
		},
		ResourceSpecStats: []*v1beta1.ResourceSpecStat{
			{
				Name: "10C1Gi",
				Allocatable: []*v1beta1.ResourceSpecStateAllocatable{

					{
						PriorityClass: uniext.PriorityBatch,
						Count:         int32(2),
					},
					{
						PriorityClass: uniext.PriorityFree,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityMid,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityProd,
						Count:         int32(10),
					},
				},
			},
			{
				Name: "500C500Gi",
				Allocatable: []*v1beta1.ResourceSpecStateAllocatable{
					{
						PriorityClass: uniext.PriorityBatch,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityFree,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityMid,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityProd,
						Count:         int32(0),
					},
				},
			},
			{
				Name: "50C50Gi",
				Allocatable: []*v1beta1.ResourceSpecStateAllocatable{

					{
						PriorityClass: uniext.PriorityBatch,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityFree,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityMid,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityProd,
						Count:         int32(1),
					},
				},
			},
		},
		PodUsedStatistics: []*v1beta1.PodUsedStatistics{
			{
				Name: "test",
				Allocated: []*v1beta1.PodPriorityUsed{
					{
						PriorityClass: uniext.PriorityBatch,
						Allocated: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("8Gi"),
						},
					},
					{
						PriorityClass: uniext.PriorityFree,
						Allocated:     corev1.ResourceList{},
					},
					{
						PriorityClass: uniext.PriorityMid,
						Allocated:     corev1.ResourceList{},
					},
					{
						PriorityClass: uniext.PriorityProd,
						Allocated: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
	}

	assert.Equal(t, expectedSummary.Status.Phase, newSummary.Status.Phase)
	assert.Equal(t, expectedSummary.Status.NumNodes, newSummary.Status.NumNodes)
	assert.Len(t, newSummary.Status.Resources, len(expectedSummary.Status.Resources))
	for i := 0; i < len(expectedSummary.Status.Resources); i++ {
		left := expectedSummary.Status.Resources[i]
		right := newSummary.Status.Resources[i]
		assert.True(t, quotav1.Equals(left.Capacity, right.Capacity))
		assert.True(t, quotav1.Equals(left.Allocatable, right.Allocatable))
		assert.True(t, quotav1.Equals(left.Allocated, right.Allocated))
		assert.True(t, quotav1.Equals(left.ReserveResourceCapacity, right.ReserveResourceCapacity))
		assert.True(t, quotav1.Equals(left.ReserveResourceAllocatable, right.ReserveResourceAllocatable))
		assert.True(t, quotav1.Equals(left.ReserveResourceAllocated, right.ReserveResourceAllocated))
	}
	for i := 0; i < len(expectedSummary.Status.ResourceSpecStats); i++ {
		left := expectedSummary.Status.ResourceSpecStats[i]
		right := newSummary.Status.ResourceSpecStats[i]
		assert.Equal(t, left.Name, right.Name)
		for k, v := range left.Allocatable {
			assert.Equal(t, v.PriorityClass, right.Allocatable[k].PriorityClass)
			assert.Equal(t, v.Count, right.Allocatable[k].Count)
		}
	}
	expectedPodUsedStatistics := expectedSummary.Status.PodUsedStatistics[0]
	assert.Equal(t, expectedPodUsedStatistics.Name, newPodUsedStatistics.Name)
	for i := 0; i < len(expectedPodUsedStatistics.Allocated); i++ {
		left := expectedPodUsedStatistics.Allocated[i]
		right := newPodUsedStatistics.Allocated[i]
		for k, v := range left.Allocated {
			assert.True(t, v.Equal(right.Allocated[k]))
		}
	}
}
