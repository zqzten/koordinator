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

package podconstraint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/podconstraint/cache"
)

func TestEndpointsQueryConstraintState(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
					corev1.LabelHostname:     "test-node-1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na620",
					corev1.LabelHostname:     "test-node-2",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-3",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-4",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-4",
				},
			},
		},
	}
	suit := newPluginTestSuit(t, nodes, nil)
	p, err := suit.proxyNew(suit.args, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)

	plg := p.(*Plugin)
	suit.start()

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	//
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  2    |  2    |  2    |
	//  +-------+-------+-------+
	//
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-3"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-4"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-5"
	plg.podConstraintCache.AddPod(nodes[2], pod)
	pod.Name = "pod-6"
	plg.podConstraintCache.AddPod(nodes[2], pod)

	expectConstraintStateResponse := &constraintStatesResponse{
		RequiredSpreadConstraints: []*cache.TopologySpreadConstraint{
			{
				TopologyKey:        corev1.LabelTopologyZone,
				MaxSkew:            1,
				NodeAffinityPolicy: v1beta1.NodeInclusionPolicyHonor,
				NodeTaintsPolicy:   v1beta1.NodeInclusionPolicyIgnore,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{
			corev1.LabelTopologyZone: 6,
		},
		TpPairToMatchNum: map[cache.TopologyPair]int{
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 2,
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: 2,
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: 2,
		},
		TpKeyToCriticalPaths: map[string]*cache.TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: cache.CriticalPath{MatchNum: 2, TopologyValue: "na630"},
				Max: cache.CriticalPath{MatchNum: 2, TopologyValue: "na620"},
			},
		},
	}

	engine := gin.Default()
	plg.RegisterEndpoints(engine.Group("/"))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/constraintStates/default/test", nil)
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	gotConstraintStateResponse := &constraintStatesResponse{}
	err = json.NewDecoder(w.Result().Body).Decode(gotConstraintStateResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectConstraintStateResponse, gotConstraintStateResponse)
}

func TestEndpointsQueryAllocSet(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
					corev1.LabelHostname:     "test-node-1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na620",
					corev1.LabelHostname:     "test-node-2",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-3",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-4",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-4",
				},
			},
		},
	}
	suit := newPluginTestSuit(t, nodes, nil)
	p, err := suit.proxyNew(suit.args, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)

	plg := p.(*Plugin)
	suit.start()

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	//
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  2    |  2    |  2    |
	//  +-------+-------+-------+
	//
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-3"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-4"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-5"
	plg.podConstraintCache.AddPod(nodes[2], pod)
	pod.Name = "pod-6"
	plg.podConstraintCache.AddPod(nodes[2], pod)

	expectAllocSet := sets.NewString("default/pod-1", "default/pod-2", "default/pod-3", "default/pod-4", "default/pod-5", "default/pod-6")

	engine := gin.Default()
	plg.RegisterEndpoints(engine.Group("/"))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/allocSet", nil)
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	gotAllocSet := sets.String{}
	err = json.NewDecoder(w.Result().Body).Decode(&gotAllocSet)
	assert.NoError(t, err)
	assert.Equal(t, expectAllocSet, gotAllocSet)
}
