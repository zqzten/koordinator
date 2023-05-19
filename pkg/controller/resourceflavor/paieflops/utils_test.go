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
	"gitlab.alibaba-inc.com/cos/scheduling-api/pkg/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/cache"
)

func TestGetNodeGpuModel(t *testing.T) {
	tests := []struct {
		name       string
		labelKey   string
		labelValue string
		expect     string
	}{
		{
			name:       "normal1",
			labelKey:   GpuModel,
			labelValue: "A100-SMX-80GB",
			expect:     "A100-SMX-80GB",
		},
		{
			name:       "normal2",
			labelKey:   "aa",
			labelValue: "A100-SMX-80GB",
			expect:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{}
			node.Labels = make(map[string]string)
			node.Labels[tt.labelKey] = tt.labelValue
			assert.True(t, reflect.DeepEqual(tt.expect, GetNodeGpuModel(node)))
		})
	}
}

func TestGetNodePointOfDelivery(t *testing.T) {
	tests := []struct {
		name       string
		labelKey   string
		labelValue string
		expect     string
	}{
		{
			name:       "normal1",
			labelKey:   PointOfDelivery,
			labelValue: "VM-G6-P3",
			expect:     "VM-G6-P3",
		},
		{
			name:       "normal2",
			labelKey:   "aa",
			labelValue: "VM-G6-P3",
			expect:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{}
			node.Labels = make(map[string]string)
			node.Labels[tt.labelKey] = tt.labelValue
			assert.True(t, reflect.DeepEqual(tt.expect, GetNodePointOfDelivery(node)))
		})
	}
}

func TestGetNodeASWID(t *testing.T) {
	tests := []struct {
		name       string
		labelKey   string
		labelValue string
		expect     string
	}{
		{
			name:       "normal1",
			labelKey:   ASWID,
			labelValue: "ASW-VM-G6-P3-S15-2.NA130",
			expect:     "ASW-VM-G6-P3-S15-2.NA130",
		},
		{
			name:       "normal2",
			labelKey:   "aa",
			labelValue: "ASW-VM-G6-P3-S15-2.NA130",
			expect:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{}
			node.Labels = make(map[string]string)
			node.Labels[tt.labelKey] = tt.labelValue
			assert.True(t, reflect.DeepEqual(tt.expect, GetNodeASW(node)))
		})
	}
}

func TestGetNodeASW(t *testing.T) {
	tests := []struct {
		name        string
		labelKey1   string
		labelValue1 string
		labelKey2   string
		labelValue2 string
		expect      string
	}{
		{
			name:        "normal1",
			labelKey1:   PointOfDelivery,
			labelValue1: "VM-G6-P3",
			labelKey2:   ASWID,
			labelValue2: "ASW-VM-G6-P3-S15-2.NA130",
			expect:      "ASW-VM-G6-P3-S15-2.NA130",
		},
		{
			name:        "normal2",
			labelKey1:   PointOfDelivery,
			labelValue1: "VM-G6-P2",
			labelKey2:   ASWID,
			labelValue2: "",
			expect:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{}
			node.Labels = make(map[string]string)
			node.Labels[tt.labelKey1] = tt.labelValue1
			node.Labels[tt.labelKey2] = tt.labelValue2
			assert.True(t, reflect.DeepEqual(tt.expect, GetNodeASW(node)))
		})
	}
}

func TestQuotaBoundNodeByManual(t *testing.T) {
	tests := []struct {
		name            string
		labels          map[string]string
		quotaNames      sets.String
		expectFind      bool
		expectQuotaName string
	}{
		{
			name: "normal1",
			labels: map[string]string{
				"Q1": "true",
			},
			quotaNames: sets.String{
				"Q1": struct{}{},
				"Q2": struct{}{},
			},
			expectFind:      true,
			expectQuotaName: "Q1",
		},
		{
			name: "normal2",
			labels: map[string]string{
				"Q1": "true",
			},
			quotaNames: sets.String{
				"Q2": struct{}{},
				"Q3": struct{}{},
			},
			expectFind:      false,
			expectQuotaName: "",
		},
		{
			name:   "normal3",
			labels: map[string]string{},
			quotaNames: sets.String{
				"Q2": struct{}{},
				"Q3": struct{}{},
			},
			expectFind:      false,
			expectQuotaName: "",
		},
		{
			name: "normal4",
			labels: map[string]string{
				"Q1":               "true",
				ResourceFlavorType: ResourceFlavorTypeAuto,
			},
			quotaNames: sets.String{
				"Q1": struct{}{},
				"Q2": struct{}{},
			},
			expectFind:      false,
			expectQuotaName: "",
		},
		{
			name: "normal4",
			labels: map[string]string{
				"Q1":               "true",
				ResourceFlavorType: ResourceFlavorTypeManual,
			},
			quotaNames: sets.String{
				"Q1": struct{}{},
				"Q2": struct{}{},
			},
			expectFind:      true,
			expectQuotaName: "Q1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			find, quotaName := quotaBoundNodeByManual(tt.labels, tt.quotaNames)
			assert.True(t, reflect.DeepEqual(tt.expectFind, find))
			assert.True(t, reflect.DeepEqual(tt.expectQuotaName, quotaName))
		})
	}
}

func TestGetAllQuotaNamesFromElasticQuotaTreeRecursive(t *testing.T) {
	tree := &v1beta1.ElasticQuotaTree{}

	quota1 := &v1beta1.ElasticQuotaSpec{
		Name: "Q1",
	}
	quota2 := &v1beta1.ElasticQuotaSpec{
		Name: "Q2",
	}
	quota11 := &v1beta1.ElasticQuotaSpec{
		Name: "Q11",
	}
	quota111 := &v1beta1.ElasticQuotaSpec{
		Name: "Q111",
	}

	quota11.Children = append(quota11.Children, *quota111)
	quota1.Children = append(quota1.Children, *quota11)
	tree.Spec.Root.Children = append(tree.Spec.Root.Children, *quota1)
	tree.Spec.Root.Children = append(tree.Spec.Root.Children, *quota2)

	tests := []struct {
		name         string
		expectQuotas sets.String
	}{
		{
			name: "normal1",
			expectQuotas: sets.String{
				"Q1":   struct{}{},
				"Q2":   struct{}{},
				"Q11":  struct{}{},
				"Q111": struct{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quotas := sets.NewString()
			getAllQuotaNamesFromElasticQuotaTreeRecursive(&tree.Spec.Root, quotas)
			assert.True(t, reflect.DeepEqual(quotas, tt.expectQuotas))
		})
	}
}

func TestGetAllThirdPartyBoundNodes(t *testing.T) {
	tree := &v1beta1.ElasticQuotaTree{}
	quota1 := &v1beta1.ElasticQuotaSpec{
		Name: "Q1",
	}
	tree.Spec.Root.Children = append(tree.Spec.Root.Children, *quota1)

	tests := []struct {
		name     string
		allNodes map[string]*corev1.Node
		expect   map[string]string
	}{
		{
			name: "normal1",
			allNodes: map[string]*corev1.Node{
				"N0": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"Q1": "true",
						},
					},
				},
				"N1": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"Q2": "true",
						},
					},
				},
				"N2": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							"Q1":               "true",
							ResourceFlavorType: ResourceFlavorTypeAuto,
						},
					},
				},
			},
			expect: map[string]string{
				"N0": "Q1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAllThirdPartyBoundNodes(tree, tt.allNodes)
			assert.True(t, reflect.DeepEqual(result, tt.expect))
		})
	}
}

func TestDoUpdateNodeCr(t *testing.T) {
	tree := &v1beta1.ElasticQuotaTree{}
	quota1 := &v1beta1.ElasticQuotaSpec{
		Name: "Q1",
	}
	tree.Spec.Root.Children = append(tree.Spec.Root.Children, *quota1)

	tests := []struct {
		name         string
		quotaName    string
		isManual     bool
		oriNode      *corev1.Node
		expectNode   *corev1.Node
		expectUpdate bool
		add          bool
	}{
		{
			name:      "add1",
			quotaName: "Q1",
			oriNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "N0",
					Labels: map[string]string{
						"Q1": "true",
					},
				},
			},
			expectNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "N0",
					Labels: map[string]string{
						"Q1":               "true",
						ResourceFlavorName: "Q1",
						ResourceFlavorType: ResourceFlavorTypeManual,
					},
				},
			},
			isManual:     true,
			add:          true,
			expectUpdate: true,
		},
		{
			name:      "add2",
			quotaName: "Q1",
			oriNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "N0",
					Labels: map[string]string{
						"Q1": "true",
					},
				},
			},
			expectNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "N0",
					Labels: map[string]string{
						"Q1":               "true",
						ResourceFlavorName: "Q1",
						ResourceFlavorType: ResourceFlavorTypeAuto,
					},
				},
			},
			isManual:     false,
			add:          true,
			expectUpdate: true,
		},
		{
			name:      "add3",
			quotaName: "Q1",
			oriNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "N0",
					Labels: map[string]string{
						"Q1":               "true",
						ResourceFlavorName: "Q1",
						ResourceFlavorType: ResourceFlavorTypeAuto,
					},
				},
			},
			expectNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "N0",
					Labels: map[string]string{
						"Q1":               "true",
						ResourceFlavorName: "Q1",
						ResourceFlavorType: ResourceFlavorTypeAuto,
					},
				},
			},
			isManual:     false,
			add:          true,
			expectUpdate: false,
		},
		{
			name:      "erase1",
			quotaName: "Q1",
			oriNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "N0",
					Labels: map[string]string{
						"Q1":               "true",
						ResourceFlavorName: "Q1",
						ResourceFlavorType: ResourceFlavorTypeManual,
					},
				},
			},
			expectNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "N0",
					Labels: map[string]string{
						"Q1": "true",
					},
				},
			},
			isManual:     true,
			add:          false,
			expectUpdate: true,
		},
		{
			name:      "erase2",
			quotaName: "Q1",
			oriNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "N0",
					Labels: map[string]string{
						"Q1":               "true",
						ResourceFlavorName: "Q1",
						ResourceFlavorType: ResourceFlavorTypeAuto,
					},
				},
			},
			expectNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "N0",
					Labels: map[string]string{},
				},
			},
			isManual:     false,
			add:          false,
			expectUpdate: true,
		},
		{
			name:      "erase3",
			quotaName: "Q1",
			oriNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "N0",
					Labels: map[string]string{},
				},
			},
			expectNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "N0",
					Labels: map[string]string{},
				},
			},
			isManual:     false,
			add:          false,
			expectUpdate: false,
		},
		{
			name:      "erase4",
			quotaName: "Q1",
			oriNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "N0",
					Labels: map[string]string{
						"Q1": "true",
					},
				},
			},
			expectNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "N0",
					Labels: map[string]string{
						"Q1": "true",
					},
				},
			},
			isManual:     true,
			add:          false,
			expectUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update, newNode := doUpdateNodeCr(nil, tt.oriNode, tt.quotaName, tt.isManual, tt.add)
			assert.True(t, reflect.DeepEqual(newNode, tt.expectNode))
			assert.True(t, reflect.DeepEqual(update, tt.expectUpdate))
		})
	}
}

func TestTryUpdateNode(t *testing.T) {
	tests := []struct {
		name                 string
		resourceFlavorCrd    *sev1alpha1.ResourceFlavor
		thirdPartyBoundNodes map[string]string
		nodes                []corev1.Node
		expectNodes          map[string]*corev1.Node
	}{
		{
			name: "normal1",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "N0",
						Labels: map[string]string{},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "N1",
						Labels: map[string]string{},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "N2",
						Labels: map[string]string{},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "N3",
						Labels: map[string]string{},
					},
				},
			},
			thirdPartyBoundNodes: map[string]string{
				"N0": "B1",
				"N2": "B1",
			},
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name: "B1",
				},
				Spec: sev1alpha1.ResourceFlavorSpec{
					Enable: true,
				},
				Status: sev1alpha1.ResourceFlavorStatus{
					ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{
						"C1": {
							SelectedNodeNum: 10,
							SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
								"N0": {
									NodeMetaInfo: map[string]string{
										"AA": "BB",
									},
								},
								"N1": {
									NodeMetaInfo: map[string]string{
										"AA": "BB",
									},
								},
							},
						},
						"C2": {
							SelectedNodeNum: 10,
							SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
								"N2": {
									NodeMetaInfo: map[string]string{
										"CC": "DD",
									},
								},
								"N3": {
									NodeMetaInfo: map[string]string{
										"CC": "DD",
									},
								},
							},
						},
					},
				},
			},
			expectNodes: map[string]*corev1.Node{
				"N0": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"B1":               "true",
							ResourceFlavorName: "B1",
							ResourceFlavorType: ResourceFlavorTypeManual,
						},
					},
				},
				"N1": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"B1":               "true",
							ResourceFlavorName: "B1",
							ResourceFlavorType: ResourceFlavorTypeAuto,
						},
					},
				},
				"N2": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							"B1":               "true",
							ResourceFlavorName: "B1",
							ResourceFlavorType: ResourceFlavorTypeManual,
						},
					},
				},
				"N3": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
						Labels: map[string]string{
							"B1":               "true",
							ResourceFlavorName: "B1",
							ResourceFlavorType: ResourceFlavorTypeAuto,
						},
					},
				},
			},
		},
		{
			name: "normal2",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"B1":               "true",
							ResourceFlavorName: "B1",
							ResourceFlavorType: ResourceFlavorTypeManual,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"B1":               "true",
							ResourceFlavorName: "B2",
							ResourceFlavorType: ResourceFlavorTypeAuto,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							"B1":               "true",
							ResourceFlavorName: "B1",
							ResourceFlavorType: ResourceFlavorTypeManual,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
						Labels: map[string]string{
							"B1":               "true",
							ResourceFlavorName: "B1",
							ResourceFlavorType: ResourceFlavorTypeAuto,
						},
					},
				},
			},
			thirdPartyBoundNodes: map[string]string{
				"N0": "B1",
				"N2": "B1",
			},
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name: "B1",
				},
				Spec: sev1alpha1.ResourceFlavorSpec{
					Enable: true,
				},
				Status: sev1alpha1.ResourceFlavorStatus{
					ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{
						"C1": {
							SelectedNodeNum: 10,
							SelectedNodes:   map[string]*sev1alpha1.SelectedNodeMeta{},
						},
						"C2": {
							SelectedNodeNum: 10,
							SelectedNodes:   map[string]*sev1alpha1.SelectedNodeMeta{},
						},
					},
				},
			},
			expectNodes: map[string]*corev1.Node{
				"N0": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"B1": "true",
						},
					},
				},
				"N2": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							"B1": "true",
						},
					},
				},
				"N3": {
					ObjectMeta: metav1.ObjectMeta{
						Name:   "N3",
						Labels: map[string]string{},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResourceFlavorInfo := cache.NewResourceFlavorInfo(tt.resourceFlavorCrd)
			nodeCache := cache.NewNodeCache()
			nodeCache.UpdateNodes(tt.nodes)
			nodeCache.UpdateCache(nil, tt.thirdPartyBoundNodes)

			newNodesMap := TryUpdateNode(nil, "B1", ResourceFlavorInfo, nodeCache)
			assert.True(t, reflect.DeepEqual(newNodesMap, tt.expectNodes))
		})
	}
}
