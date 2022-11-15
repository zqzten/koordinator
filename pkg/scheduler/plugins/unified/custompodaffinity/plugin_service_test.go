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

package custompodaffinity

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func TestPlugin_RegisterEndpoints(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node",
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

	testApp := "test-app"
	testDeployUnit := "test-du"

	tests := []struct {
		name                           string
		allocSpec                      *extunified.AllocSpec
		assignedPods                   []*corev1.Pod
		queryURL                       string
		expectServiceUnitStatsResponse *serviceUnitStatsResponse
	}{
		{
			name: "app dimension",
			allocSpec: &extunified.AllocSpec{
				Affinity: &extunified.Affinity{
					PodAntiAffinity: &extunified.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []extunified.PodAffinityTerm{
							{
								PodAffinityTerm: corev1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      extunified.SigmaLabelAppName,
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{testApp},
											},
										},
									},
								},
								MaxCount: 2,
							},
						},
					},
				},
			},
			assignedPods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod-1",
						Labels: map[string]string{
							extunified.SigmaLabelAppName: testApp,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod-2",
						Labels: map[string]string{
							extunified.SigmaLabelAppName: testApp,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			queryURL: fmt.Sprintf("/serviceUnitStats/%s/%s", nodes[0].Name, testApp),
			expectServiceUnitStatsResponse: &serviceUnitStatsResponse{
				PodSpreadInfo: &extunified.PodSpreadInfo{
					AppName: testApp,
				},
				AllocCount: 2,
			},
		},
		{
			name: "serviceUnit dimension",
			allocSpec: &extunified.AllocSpec{
				Affinity: &extunified.Affinity{
					PodAntiAffinity: &extunified.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []extunified.PodAffinityTerm{
							{
								PodAffinityTerm: corev1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      extunified.SigmaLabelServiceUnitName,
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{testDeployUnit},
											},
										},
									},
								},
								MaxCount: 2,
							},
						},
					},
				},
			},
			assignedPods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod-1",
						Labels: map[string]string{
							extunified.SigmaLabelServiceUnitName: testDeployUnit,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			queryURL: fmt.Sprintf("/serviceUnitStats/%s//%s", nodes[0].Name, testDeployUnit),
			expectServiceUnitStatsResponse: &serviceUnitStatsResponse{
				PodSpreadInfo: &extunified.PodSpreadInfo{
					ServiceUnit: testDeployUnit,
				},
				AllocCount: 1,
			},
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

			data, err := json.Marshal(tt.allocSpec)
			assert.NoError(t, err)

			for _, v := range tt.assignedPods {
				if v.Annotations == nil {
					v.Annotations = make(map[string]string)
				}
				v.Annotations[extunified.AnnotationPodRequestAllocSpec] = string(data)
				plg.cache.AddPod(v)
			}
			engine := gin.Default()
			plg.RegisterEndpoints(engine.Group("/"))
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", tt.queryURL, nil)
			engine.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Result().StatusCode)
			gotServiceUnitStats := &serviceUnitStatsResponse{}
			err = json.NewDecoder(w.Result().Body).Decode(gotServiceUnitStats)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectServiceUnitStatsResponse, gotServiceUnitStats)
		})
	}
}
