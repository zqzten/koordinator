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

package cache

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestUpdateNodes(t *testing.T) {
	tests := []struct {
		name           string
		firstAllNodes  []corev1.Node
		secondAllNodes []corev1.Node
		want           []corev1.Node
	}{
		{
			name: "normal",
			firstAllNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"NodeName": "N0",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"NodeName": "N1",
						},
					},
				},
			},
			secondAllNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"NodeName": "N0",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							"NodeName": "N2",
						},
					},
				},
			},
			want: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"NodeName": "N0",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							"NodeName": "N2",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := NewNodeCache()
			nodeCache.UpdateNodes(tt.firstAllNodes)
			nodeCache.UpdateNodes(tt.secondAllNodes)

			for _, expectNode := range tt.want {
				assert.True(t, reflect.DeepEqual(expectNode, *nodeCache.nodeInfos[expectNode.Name]))
			}
		})
	}
}

func TestUpdateCache(t *testing.T) {
	tests := []struct {
		name                       string
		allNodes                   []corev1.Node
		allAutoBoundNodes          map[string]*NodeBoundInfo
		thirdPartyBoundNodes       map[string]string
		expectAllClusterNodes      sets.String
		expectAllFreePoolNodes     sets.String
		expectAllAutoBoundNodes    map[string]*NodeBoundInfo
		expectThirdPartyBoundNodes map[string]string
	}{
		{
			name: "normal",
			allNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N4",
					},
				},
			},
			allAutoBoundNodes: map[string]*NodeBoundInfo{
				"N0": {
					QuotaName: "Q1",
					ConfName:  "C1",
				},
				"N1": {
					QuotaName: "Q2",
					ConfName:  "C2",
				},
			},
			thirdPartyBoundNodes: map[string]string{
				"N2": "Q3",
				"N3": "Q4",
			},
			expectAllClusterNodes: sets.String{
				"N0": struct{}{},
				"N1": struct{}{},
				"N2": struct{}{},
				"N3": struct{}{},
				"N4": struct{}{},
			},
			expectAllFreePoolNodes: sets.String{
				"N4": struct{}{},
			},
			expectAllAutoBoundNodes: map[string]*NodeBoundInfo{
				"N0": {
					QuotaName: "Q1",
					ConfName:  "C1",
				},
				"N1": {
					QuotaName: "Q2",
					ConfName:  "C2",
				},
			},
			expectThirdPartyBoundNodes: map[string]string{
				"N2": "Q3",
				"N3": "Q4",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := NewNodeCache()
			nodeCache.UpdateNodes(tt.allNodes)
			nodeCache.UpdateCache(tt.allAutoBoundNodes, tt.thirdPartyBoundNodes)

			assert.True(t, reflect.DeepEqual(tt.expectAllClusterNodes, nodeCache.allClusterNodes))
			assert.True(t, reflect.DeepEqual(tt.expectAllFreePoolNodes, nodeCache.allFreePoolNodes))
			assert.True(t, reflect.DeepEqual(tt.expectAllAutoBoundNodes, nodeCache.allAutoBoundNodes))
			assert.True(t, reflect.DeepEqual(tt.expectThirdPartyBoundNodes, nodeCache.thirdPartyBoundNodes))
		})
	}
}

func TestGetAllNodeCr(t *testing.T) {
	tests := []struct {
		name          string
		firstAllNodes []corev1.Node
		want          []corev1.Node
	}{
		{
			name: "normal",
			firstAllNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"NodeName": "N0",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"NodeName": "N1",
						},
					},
				},
			},
			want: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"NodeName": "N0",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"NodeName": "N1",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := NewNodeCache()
			nodeCache.UpdateNodes(tt.firstAllNodes)

			expects := make(map[string]*corev1.Node)
			for _, node := range tt.want {
				expects[node.Name] = node.DeepCopy()
			}
			assert.True(t, reflect.DeepEqual(expects, nodeCache.GetAllNodeCr()))
		})
	}
}

func TestGetNodeCr(t *testing.T) {
	tests := []struct {
		name          string
		firstAllNodes []corev1.Node
		want          []corev1.Node
	}{
		{
			name: "normal",
			firstAllNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"NodeName": "N0",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"NodeName": "N1",
						},
					},
				},
			},
			want: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"NodeName": "N0",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"NodeName": "N1",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := NewNodeCache()
			nodeCache.UpdateNodes(tt.firstAllNodes)

			for _, node := range tt.want {
				assert.True(t, reflect.DeepEqual(&node, nodeCache.GetNodeCr(node.Name)))
			}
		})
	}
}

func TestGetAllFreePoolNodes(t *testing.T) {
	tests := []struct {
		name                   string
		allNodes               []corev1.Node
		allAutoBoundNodes      map[string]*NodeBoundInfo
		thirdPartyBoundNodes   map[string]string
		expectAllFreePoolNodes sets.String
	}{
		{
			name: "normal",
			allNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N4",
					},
				},
			},
			allAutoBoundNodes: map[string]*NodeBoundInfo{
				"N0": {
					QuotaName: "Q1",
					ConfName:  "C1",
				},
				"N1": {
					QuotaName: "Q2",
					ConfName:  "C2",
				},
			},
			thirdPartyBoundNodes: map[string]string{
				"N2": "Q3",
				"N3": "Q4",
			},
			expectAllFreePoolNodes: sets.String{
				"N4": struct{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := NewNodeCache()
			nodeCache.UpdateNodes(tt.allNodes)
			nodeCache.UpdateCache(tt.allAutoBoundNodes, tt.thirdPartyBoundNodes)

			assert.True(t, reflect.DeepEqual(tt.expectAllFreePoolNodes, nodeCache.GetAllFreePoolNodes()))
		})
	}
}

func TestGetThirdPartyBoundNodes(t *testing.T) {
	tests := []struct {
		name                       string
		allNodes                   []corev1.Node
		allAutoBoundNodes          map[string]*NodeBoundInfo
		thirdPartyBoundNodes       map[string]string
		expectThirdPartyBoundNodes map[string]sets.String
	}{
		{
			name: "normal",
			allNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N4",
					},
				},
			},
			allAutoBoundNodes: map[string]*NodeBoundInfo{
				"N0": {
					QuotaName: "Q1",
					ConfName:  "C1",
				},
				"N1": {
					QuotaName: "Q2",
					ConfName:  "C2",
				},
			},
			thirdPartyBoundNodes: map[string]string{
				"N2": "Q3",
				"N3": "Q4",
			},
			expectThirdPartyBoundNodes: map[string]sets.String{
				"Q3": {
					"N2": struct{}{},
				},
				"Q4": {
					"N3": struct{}{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := NewNodeCache()
			nodeCache.UpdateNodes(tt.allNodes)
			nodeCache.UpdateCache(tt.allAutoBoundNodes, tt.thirdPartyBoundNodes)

			for binderName, expect := range tt.expectThirdPartyBoundNodes {
				assert.True(t, reflect.DeepEqual(expect, nodeCache.GetThirdPartyBoundNodes(binderName)))
			}
		})
	}
}
