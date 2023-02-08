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
	"fmt"
	"reflect"
	"testing"

	uniapiext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

func newBEPod(beCPU, beMemory int64) *v1.Pod {
	return &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							BatchCPU:    *resource.NewQuantity(beCPU, resource.DecimalSI),
							BatchMemory: *resource.NewQuantity(beMemory, resource.BinarySI),
						},
					},
				},
			},
		},
	}
}

func newOldBEPod(beCPU, beMemory int64) *v1.Pod {
	return &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							uniapiext.AlibabaCloudReclaimedCPU:    *resource.NewQuantity(beCPU, resource.DecimalSI),
							uniapiext.AlibabaCloudReclaimedMemory: *resource.NewQuantity(beMemory, resource.BinarySI),
						},
					},
				},
			},
		},
	}
}

func getErrReason(rn v1.ResourceName) string {
	return fmt.Sprintf("Insufficient %v", rn)
}

func TestEnoughRequests(t *testing.T) {
	enoughPodsTests := []struct {
		pod                       *v1.Pod
		nodeInfo                  *framework.NodeInfo
		name                      string
		wantInsufficientResources []InsufficientResource
		wantStatus                *framework.Status
	}{
		{
			pod: &v1.Pod{},
			nodeInfo: framework.NewNodeInfo(
				newBEPod(10, 20),
			),
			name:                      "no resources requested always fits",
			wantInsufficientResources: []InsufficientResource{},
		},
		{
			pod: newBEPod(1, 1),
			nodeInfo: framework.NewNodeInfo(
				newBEPod(10, 20),
			),
			name:                      "too many resources fails",
			wantStatus:                framework.NewStatus(framework.Unschedulable, getErrReason(BatchCPU), getErrReason(BatchMemory)),
			wantInsufficientResources: []InsufficientResource{{BatchCPU, getErrReason(BatchCPU), 1, 10, 10}, {BatchMemory, getErrReason(BatchMemory), 1, 20, 20}},
		},
		{
			pod: newOldBEPod(1, 1),
			nodeInfo: framework.NewNodeInfo(
				newBEPod(5, 10),
				newOldBEPod(5, 10),
			),
			name:                      "too many resources fails with old pod",
			wantStatus:                framework.NewStatus(framework.Unschedulable, getErrReason(BatchCPU), getErrReason(BatchMemory)),
			wantInsufficientResources: []InsufficientResource{{BatchCPU, getErrReason(BatchCPU), 1, 10, 10}, {BatchMemory, getErrReason(BatchMemory), 1, 20, 20}},
		},
		{
			pod: newBEPod(1, 1),
			nodeInfo: framework.NewNodeInfo(
				newBEPod(5, 5),
			),
			name:                      "both resources fit",
			wantInsufficientResources: []InsufficientResource{},
		},
		{
			pod: newOldBEPod(1, 1),
			nodeInfo: framework.NewNodeInfo(
				newBEPod(2, 2),
				newOldBEPod(3, 3),
			),
			name:                      "both resources fit with old pod",
			wantInsufficientResources: []InsufficientResource{},
		},
		{
			pod: newBEPod(2, 1),
			nodeInfo: framework.NewNodeInfo(
				newBEPod(9, 5),
			),
			name:                      "one resource memory fits",
			wantStatus:                framework.NewStatus(framework.Unschedulable, getErrReason(BatchCPU)),
			wantInsufficientResources: []InsufficientResource{{BatchCPU, getErrReason(BatchCPU), 2, 9, 10}},
		},
		{
			pod: newOldBEPod(2, 1),
			nodeInfo: framework.NewNodeInfo(
				newBEPod(4, 2),
				newOldBEPod(5, 3),
			),
			name:                      "one resource memory fits with old pod",
			wantStatus:                framework.NewStatus(framework.Unschedulable, getErrReason(BatchCPU)),
			wantInsufficientResources: []InsufficientResource{{BatchCPU, getErrReason(BatchCPU), 2, 9, 10}},
		},
		{
			pod: newBEPod(1, 2),
			nodeInfo: framework.NewNodeInfo(
				newBEPod(5, 19)),
			name:                      "one resource cpu fits",
			wantStatus:                framework.NewStatus(framework.Unschedulable, getErrReason(BatchMemory)),
			wantInsufficientResources: []InsufficientResource{{BatchMemory, getErrReason(BatchMemory), 2, 19, 20}},
		},
		{
			pod: newOldBEPod(1, 2),
			nodeInfo: framework.NewNodeInfo(
				newBEPod(3, 9),
				newOldBEPod(2, 10),
			),
			name:                      "one resource cpu fits with old and new pod",
			wantStatus:                framework.NewStatus(framework.Unschedulable, getErrReason(BatchMemory)),
			wantInsufficientResources: []InsufficientResource{{BatchMemory, getErrReason(BatchMemory), 2, 19, 20}},
		},
		{
			pod: newBEPod(5, 1),
			nodeInfo: framework.NewNodeInfo(
				newBEPod(5, 19)),
			name:                      "equal edge case",
			wantInsufficientResources: []InsufficientResource{},
		},
		{
			pod: newOldBEPod(5, 1),
			nodeInfo: framework.NewNodeInfo(
				newBEPod(3, 9),
				newOldBEPod(2, 10),
			),
			name:                      "equal edge case with old and new pod",
			wantInsufficientResources: []InsufficientResource{},
		},
	}

	for _, test := range enoughPodsTests {
		t.Run(test.name, func(t *testing.T) {
			node := makeBENode("test-node", 10, 20)
			test.nodeInfo.SetNode(node)

			p, err := NewFit(nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			cycleState := framework.NewCycleState()
			preFilterStatus := p.(framework.PreFilterPlugin).PreFilter(context.Background(), cycleState, test.pod)
			if !preFilterStatus.IsSuccess() {
				t.Errorf("prefilter failed with status: %v", preFilterStatus)
			}

			gotStatus := p.(framework.FilterPlugin).Filter(context.Background(), cycleState, test.pod, test.nodeInfo)
			if !reflect.DeepEqual(gotStatus, test.wantStatus) {
				t.Errorf("status does not match.\n got: %v\nwant: %v", gotStatus, test.wantStatus)
			}

			prefilterStat := &preFilterState{
				computePodBatchRequest(test.pod, true),
			}
			gotInsufficientResources := fitsRequest(prefilterStat, test.nodeInfo)
			if !reflect.DeepEqual(gotInsufficientResources, test.wantInsufficientResources) {
				t.Errorf("insufficient resources do not match.\n got: %+v\nwant: %v", gotInsufficientResources, test.wantInsufficientResources)
			}
		})
	}
}

func TestPreFilterDisabled(t *testing.T) {
	pod := &v1.Pod{}
	nodeInfo := framework.NewNodeInfo()
	node := v1.Node{}
	nodeInfo.SetNode(&node)
	p, err := NewFit(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cycleState := framework.NewCycleState()
	gotStatus := p.(framework.FilterPlugin).Filter(context.Background(), cycleState, pod, nodeInfo)
	wantStatus := framework.AsStatus(fmt.Errorf(`error reading "%v" from cycleState: %w`, preFilterStateKey, framework.ErrNotFound))
	if !reflect.DeepEqual(gotStatus, wantStatus) {
		t.Errorf("status does not match: %v, want: %v", gotStatus, wantStatus)
	}
}
