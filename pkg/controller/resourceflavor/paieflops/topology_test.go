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

package paieflops

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/cache"
)

func TestGetNodePODAndASWInfo(t *testing.T) {
	tests := []struct {
		name      string
		nodeName  string
		nodes     []corev1.Node
		expectPOD string
		expectASW string
	}{
		{
			name:     "normal1",
			nodeName: "N0",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P3",
							ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
						},
					},
				},
			},
			expectPOD: "VM-G6-P3",
			expectASW: "ASW-VM-G6-P3-S15-2.NA130",
		},
		{
			name:     "normal2",
			nodeName: "N1",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P3",
							ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
						},
					},
				},
			},
			expectPOD: "",
			expectASW: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := cache.NewNodeCache()
			nodeCache.UpdateNodes(tt.nodes)

			pod, asw := getNodePODAndASWInfo(nodeCache, tt.nodeName)
			assert.True(t, reflect.DeepEqual(pod, tt.expectPOD))
			assert.True(t, reflect.DeepEqual(asw, tt.expectASW))
		})
	}
}

func TestGetSortedNodePODAndASWInfos(t *testing.T) {
	tests := []struct {
		name         string
		nodeNames    sets.String
		nodes        []corev1.Node
		expectResult []*NodeTopologyInfo
	}{
		{
			name: "normal1",
			nodeNames: sets.String{
				"N0": struct{}{},
				"N1": struct{}{},
				"N2": struct{}{},
				"N3": struct{}{},
				"N4": struct{}{},
				"N5": struct{}{},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N4",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N5",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P2",
							ASWID:           "ASW-VM-G6-P2-S15-2.NA130",
						},
					},
				},
			},
			expectResult: []*NodeTopologyInfo{
				{
					nodes: []string{
						"N5",
					},
					PointOfDelivery: "VM-G6-P2",
					ASW:             "ASW-VM-G6-P2-S15-2.NA130",
				},
				{
					nodes: []string{
						"N3", "N4",
					},
					PointOfDelivery: "VM-G6-P1",
					ASW:             "ASW-VM-G6-P1-S15-2.NA130",
				},
				{
					nodes: []string{
						"N0", "N1", "N2",
					},
					PointOfDelivery: "VM-G6-P1",
					ASW:             "ASW-VM-G6-P1-S15-1.NA130",
				},
			},
		},
		{
			name: "normal2",
			nodeNames: sets.String{
				"N0": struct{}{},
				"N1": struct{}{},
				"N2": struct{}{},
				"N3": struct{}{},
				"N4": struct{}{},
				"N5": struct{}{},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N4",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P2",
							ASWID:           "ASW-VM-G6-P2-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N5",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P2",
							ASWID:           "ASW-VM-G6-P2-S15-2.NA130",
						},
					},
				},
			},
			expectResult: []*NodeTopologyInfo{
				{
					nodes: []string{
						"N0", "N1",
					},
					PointOfDelivery: "VM-G6-P1",
					ASW:             "ASW-VM-G6-P1-S15-1.NA130",
				},
				{
					nodes: []string{
						"N2", "N3",
					},
					PointOfDelivery: "VM-G6-P1",
					ASW:             "ASW-VM-G6-P1-S15-2.NA130",
				},
				{
					nodes: []string{
						"N4", "N5",
					},
					PointOfDelivery: "VM-G6-P2",
					ASW:             "ASW-VM-G6-P2-S15-2.NA130",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := cache.NewNodeCache()
			nodeCache.UpdateNodes(tt.nodes)

			result := getSortedNodePODAndASWInfos(nodeCache, tt.nodeNames)
			for i := 0; i < len(result); i++ {
				assert.True(t, reflect.DeepEqual(result[i].ASW, tt.expectResult[i].ASW))
				assert.True(t, reflect.DeepEqual(result[i].PointOfDelivery, tt.expectResult[i].PointOfDelivery))
				assert.True(t, reflect.DeepEqual(result[i].nodes, tt.expectResult[i].nodes))
			}
		})
	}
}

func TestFindBestTopologyNodes4Empty(t *testing.T) {
	tests := []struct {
		name               string
		wantedNum          int
		freeNodeTopologies []*NodeTopologyInfo
		expectResult       []string
	}{
		{
			name:      "normal1",
			wantedNum: 2,
			freeNodeTopologies: []*NodeTopologyInfo{
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW1",
					nodes: []string{
						"N5",
					},
				},
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW2",
					nodes: []string{
						"N6",
					},
				},
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW1",
					nodes: []string{
						"N0", "N1",
					},
				},
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW3",
					nodes: []string{
						"N7", "N8",
					},
				},
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW2",
					nodes: []string{
						"N2", "N3", "N4",
					},
				},
			},
			expectResult: []string{
				"N0", "N1",
			},
		},
		{
			name:      "normal2",
			wantedNum: 4,
			freeNodeTopologies: []*NodeTopologyInfo{
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW1",
					nodes: []string{
						"N5",
					},
				},
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW2",
					nodes: []string{
						"N6",
					},
				},
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW1",
					nodes: []string{
						"N0", "N1",
					},
				},
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW3",
					nodes: []string{
						"N7", "N8",
					},
				},
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW2",
					nodes: []string{
						"N2", "N3", "N4",
					},
				},
			},
			expectResult: []string{
				"N5", "N6", "N7", "N8",
			},
		},
		{
			name:      "normal3",
			wantedNum: 6,
			freeNodeTopologies: []*NodeTopologyInfo{
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW1",
					nodes: []string{
						"N5",
					},
				},
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW2",
					nodes: []string{
						"N6",
					},
				},
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW1",
					nodes: []string{
						"N0", "N1",
					},
				},
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW3",
					nodes: []string{
						"N7", "N8",
					},
				},
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW2",
					nodes: []string{
						"N2", "N3", "N4",
					},
				},
			},
			expectResult: []string{
				"N0", "N1", "N2", "N3", "N4",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := findBestTopologyNodes4Empty(tt.wantedNum, tt.freeNodeTopologies)
			assert.True(t, reflect.DeepEqual(nodes, tt.expectResult))
		})
	}
}

func TestFindBestTopologyNodes4ExistsPod(t *testing.T) {
	tests := []struct {
		name                   string
		wantedNum              int
		selectedNodeTopologies []*NodeTopologyInfo
		freeNodeTopologies     []*NodeTopologyInfo
		expectResult           []string
	}{
		{
			name:      "normal1",
			wantedNum: 2,
			selectedNodeTopologies: []*NodeTopologyInfo{
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW1",
					nodes: []string{
						"N-1",
					},
				},
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW2",
					nodes: []string{
						"N-2",
					},
				},
			},
			freeNodeTopologies: []*NodeTopologyInfo{
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW1",
					nodes: []string{
						"N5",
					},
				},
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW2",
					nodes: []string{
						"N6",
					},
				},
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW1",
					nodes: []string{
						"N0", "N1",
					},
				},
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW3",
					nodes: []string{
						"N7", "N8",
					},
				},
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW2",
					nodes: []string{
						"N2", "N3", "N4",
					},
				},
			},
			expectResult: []string{
				"N0", "N1",
			},
		},
		{
			name:      "normal2",
			wantedNum: 3,
			selectedNodeTopologies: []*NodeTopologyInfo{
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW1",
					nodes: []string{
						"N-1",
					},
				},
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW2",
					nodes: []string{
						"N-2",
					},
				},
			},
			freeNodeTopologies: []*NodeTopologyInfo{
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW1",
					nodes: []string{
						"N5",
					},
				},
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW2",
					nodes: []string{
						"N6",
					},
				},
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW0",
					nodes: []string{
						"N-1", "N-2",
					},
				},
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW1",
					nodes: []string{
						"N0", "N1",
					},
				},
				{
					PointOfDelivery: "POD2",
					ASW:             "ASW3",
					nodes: []string{
						"N7", "N8",
					},
				},
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW2",
					nodes: []string{
						"N2", "N3", "N4",
					},
				},
				{
					PointOfDelivery: "POD1",
					ASW:             "ASW3",
					nodes: []string{
						"N9", "N10", "N11",
					},
				},
			},
			expectResult: []string{
				"N0", "N1", "N-1", "N-2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := findBestTopologyNodes4ExistsPod(tt.wantedNum, tt.selectedNodeTopologies, tt.freeNodeTopologies)
			assert.True(t, reflect.DeepEqual(nodes, tt.expectResult))
		})
	}
}

func TestFilterAndSortNodesByTopology(t *testing.T) {
	tests := []struct {
		name          string
		wantedNum     int
		allNodes      []corev1.Node
		selectedNodes map[string]*sev1alpha1.SelectedNodeMeta
		freeNodes     sets.String
		expectResult  []string
	}{
		{
			name:      "normal1",
			wantedNum: 2,
			allNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P2",
							ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
			},
			selectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{},
			freeNodes: sets.String{
				"N0": struct{}{},
				"N1": struct{}{},
				"N2": struct{}{},
			},
			expectResult: []string{
				"N0", "N1",
			},
		},
		{
			name:      "normal2",
			wantedNum: 2,
			allNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P1",
							ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							PointOfDelivery: "VM-G6-P2",
							ASWID:           "ASW-VM-G6-P2-S15-1.NA130",
						},
					},
				},
			},
			selectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
				"N2": {},
			},
			freeNodes: sets.String{
				"N0": struct{}{},
				"N1": struct{}{},
				"N2": struct{}{},
			},
			expectResult: []string{
				"N2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := cache.NewNodeCache()
			nodeCache.UpdateNodes(tt.allNodes)

			nodes := FilterAndSortNodesByTopology(tt.wantedNum, nodeCache, tt.selectedNodes, tt.freeNodes)
			assert.True(t, reflect.DeepEqual(nodes, tt.expectResult))
		})
	}
}
