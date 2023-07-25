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

package limitaware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	st "k8s.io/kubernetes/pkg/scheduler/testing"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/services"
)

func TestPlugin_RegisterEndpoints(t *testing.T) {
	tests := []struct {
		name                        string
		existingPods                []*corev1.Pod
		expectNodeLimitInfoResponse *nodeLimitInfoResponse
	}{
		{
			name: "normal flow",
			existingPods: []*corev1.Pod{
				st.MakePod().Namespace("default").Name("pod-1").Node("node1").Req(map[corev1.ResourceName]string{"cpu": "2000", "memory": "4000"}).Obj(),
			},
			expectNodeLimitInfoResponse: &nodeLimitInfoResponse{
				AllocSet: sets.NewString("default/pod-1"),
				Allocated: &framework.Resource{
					MilliCPU: 2000000,
					Memory:   4000,
				},
				NonZeroAllocated: &framework.Resource{
					MilliCPU: 2000000,
					Memory:   4000,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			suit := newPluginTestSuitWith(t, test.existingPods, nil, defaultLimitToAllocatable)
			p, err := suit.pluginFactory()
			assert.NotNil(t, p)
			assert.NoError(t, err)
			cache := p.(*Plugin).nodeLimitsCache
			for _, existingPod := range test.existingPods {
				cache.AddPod(existingPod.Spec.NodeName, existingPod)
			}

			engine := gin.Default()
			p.(services.APIServiceProvider).RegisterEndpoints(engine.Group("/"))
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/nodeLimitInfo/node1", nil)
			engine.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Result().StatusCode)
			gotNodeLimitInfoResponse := &nodeLimitInfoResponse{}
			err = json.NewDecoder(w.Result().Body).Decode(gotNodeLimitInfoResponse)
			assert.NoError(t, err)
			assert.Equal(t, test.expectNodeLimitInfoResponse, gotNodeLimitInfoResponse)
		})
	}
}
