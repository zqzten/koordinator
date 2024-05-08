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

package nodeports

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
)

func newPod(host string, hostPortInfos ...string) *corev1.Pod {
	var networkPorts []corev1.ContainerPort
	for _, portInfo := range hostPortInfos {
		splited := strings.Split(portInfo, "/")
		hostPort, _ := strconv.Atoi(splited[2])

		networkPorts = append(networkPorts, corev1.ContainerPort{
			HostIP:   splited[1],
			HostPort: int32(hostPort),
			Protocol: corev1.Protocol(splited[0]),
		})
	}
	return &corev1.Pod{
		Spec: corev1.PodSpec{
			NodeName: host,
			Containers: []corev1.Container{
				{
					Ports: networkPorts,
				},
			},
		},
	}
}

func vkNode(nodeName string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				uniext.LabelNodeType: uniext.VKType,
			},
		},
	}
}

func TestNodePorts(t *testing.T) {

	tests := []struct {
		pod        *corev1.Pod
		nodeInfo   *framework.NodeInfo
		node       *corev1.Node
		name       string
		wantStatus *framework.Status
	}{
		{
			pod: newPod("m1", "UDP/127.0.0.1/8080"),
			nodeInfo: framework.NewNodeInfo(
				newPod("m1", "UDP/127.0.0.1/9090")),
			node: vkNode("test-node"),
			name: "other port",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p, _ := New(nil, nil)
			if test.nodeInfo != nil && test.node != nil {
				test.nodeInfo.SetNode(test.node)
			}
			cycleState := framework.NewCycleState()
			_, preFilterStatus := p.(framework.PreFilterPlugin).PreFilter(context.Background(), cycleState, test.pod)
			if !preFilterStatus.IsSuccess() {
				t.Errorf("prefilter failed with status: %v", preFilterStatus)
			}
			gotStatus := p.(framework.FilterPlugin).Filter(context.Background(), cycleState, test.pod, test.nodeInfo)
			if !reflect.DeepEqual(gotStatus, test.wantStatus) {
				t.Errorf("status does not match: %v, want: %v", gotStatus, test.wantStatus)
			}
		})
	}
}
