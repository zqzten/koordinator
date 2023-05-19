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

package flavor_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/cache"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/flavor"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/paieflops"
)

func TestFillNodeMeta(t *testing.T) {
	tests := []struct {
		name         string
		nodeName     string
		allNodes     []corev1.Node
		forceAdd     bool
		forceType    string
		expectResult map[string]string
	}{
		{
			name:      "normal1",
			nodeName:  "N0",
			forceAdd:  true,
			forceType: flavor.ForceAddTypeManual,
			allNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							flavor.GpuModel:        "A100",
							flavor.PointOfDelivery: "VM-G6-P1",
							flavor.ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
			},
			expectResult: map[string]string{
				flavor.GpuModel:        "A100",
				flavor.PointOfDelivery: "VM-G6-P1",
				flavor.ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
				flavor.ForceWanted:     "true",
				flavor.ForceWantedType: flavor.ForceAddTypeManual,
			},
		},
		{
			name:     "normal2",
			nodeName: "N1",
			forceAdd: true,
			allNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							flavor.GpuModel:        "A100",
							flavor.PointOfDelivery: "VM-G6-P1",
							flavor.ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
			},
			expectResult: nil,
		},
		{
			name:     "normal3",
			nodeName: "N0",
			forceAdd: false,
			allNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							flavor.GpuModel:        "A100",
							flavor.PointOfDelivery: "VM-G6-P1",
							flavor.ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
			},
			expectResult: map[string]string{
				flavor.GpuModel:        "A100",
				flavor.PointOfDelivery: "VM-G6-P1",
				flavor.ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
				flavor.ForceWanted:     "false",
				flavor.ForceWantedType: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := cache.NewNodeCache()
			nodeCache.UpdateNodes(tt.allNodes)

			flavor := flavor.NewResourceFlavor(nil, nodeCache, nil)
			result := flavor.FillNodeMeta(tt.nodeName, tt.forceAdd, tt.forceType)
			assert.True(t, reflect.DeepEqual(result, tt.expectResult))
		})
	}
}

func TestIsNodeShouldAdd(t *testing.T) {
	tests := []struct {
		name         string
		nodeName     string
		nodes        []corev1.Node
		conf         *sev1alpha1.ResourceFlavorConf
		expectResult bool
	}{
		{
			name:     "normal1",
			nodeName: "N0",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"KeyA": "ValueA",
						},
					},
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							{
								Key: "KeyA", Value: "ValueA", Effect: "NoSchedule",
							},
						},
					},
				},
			},
			conf: &sev1alpha1.ResourceFlavorConf{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "KeyA",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"ValueA"},
									},
								},
							},
						},
					},
				},
				Toleration: []corev1.Toleration{
					{
						Key: "KeyA", Value: "ValueA", Effect: "NoSchedule",
					},
				},
			},
			expectResult: true,
		},
		{
			name:     "normal2",
			nodeName: "N0",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"KeyA": "ValueB",
						},
					},
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							{
								Key: "KeyA", Value: "ValueA", Effect: "NoSchedule",
							},
						},
					},
				},
			},
			conf: &sev1alpha1.ResourceFlavorConf{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "KeyA",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"ValueA"},
									},
								},
							},
						},
					},
				},
				Toleration: []corev1.Toleration{
					{
						Key: "KeyA", Value: "ValueA", Effect: "NoSchedule",
					},
				},
			},
			expectResult: false,
		},
		{
			name:     "normal3",
			nodeName: "N0",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"KeyA": "ValueA",
						},
					},
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							{
								Key: "KeyA", Value: "ValueB", Effect: "NoSchedule",
							},
						},
					},
				},
			},
			conf: &sev1alpha1.ResourceFlavorConf{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "KeyA",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"ValueA"},
									},
								},
							},
						},
					},
				},
				Toleration: []corev1.Toleration{
					{
						Key: "KeyA", Value: "ValueA", Effect: "NoSchedule",
					},
				},
			},
			expectResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := cache.NewNodeCache()
			nodeCache.UpdateNodes(tt.nodes)

			flavor := flavor.NewResourceFlavor(nil, nodeCache, nil)
			result := flavor.IsNodeShouldAdd("flavor0", "conf0", tt.nodeName, tt.conf)
			assert.True(t, reflect.DeepEqual(result, tt.expectResult))
		})
	}
}

func TestTryAddValidNodes(t *testing.T) {
	tests := []struct {
		name              string
		nodes             []corev1.Node
		thirdPartyNodes   map[string]string
		resourceFlavorCrd *sev1alpha1.ResourceFlavor
		expectResult      map[string]*sev1alpha1.ResourceFlavorConfStatus
	}{
		{
			name: "forceAdd1",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"KeyA":                    "ValueA",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"KeyA":                    "ValueA",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
						},
					},
				},
			},
			thirdPartyNodes: map[string]string{
				"N0": "B1",
				"N1": "B1",
			},
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name: "B1",
				},
				Spec: sev1alpha1.ResourceFlavorSpec{
					Enable: true,
					Configs: map[string]*sev1alpha1.ResourceFlavorConf{
						"C1": {
							Name:    "C1",
							NodeNum: 3,
							ForceAddNodes: []string{
								"N1", "N2",
							},
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "KeyA",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"ValueA"},
												},
											},
										},
									},
								},
							},
						},
						"C2": {
							Name:    "C2",
							NodeNum: 3,
							ForceAddNodes: []string{
								"N1", "N2",
							},
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "KeyB",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"ValueB"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Status: sev1alpha1.ResourceFlavorStatus{
					ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{
						"C1": {
							SelectedNodeNum: 1,
							SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
								"N0": {
									NodeMetaInfo: map[string]string{
										"AA": "BB",
									},
								},
							},
						},
						"C2": {
							SelectedNodeNum: 1,
							SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
								"N1": {
									NodeMetaInfo: map[string]string{
										"CC": "DD",
									},
								},
							},
						},
					},
				},
			},
			expectResult: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 3,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								"AA": "BB",
							},
						},
						"N1": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeManual,
							},
						},
						"N2": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeFlavor,
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 3,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeManual,
							},
						},
						"N1": {
							NodeMetaInfo: map[string]string{
								"CC": "DD",
							},
						},
						"N2": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeFlavor,
							},
						},
					},
				},
			},
		},
		{
			name: "forceAdd2",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"KeyA":                    "ValueA",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"KeyA":                    "ValueA",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
						},
					},
				},
			},
			thirdPartyNodes: map[string]string{
				"N0": "B1",
				"N1": "B1",
			},
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name: "B1",
				},
				Spec: sev1alpha1.ResourceFlavorSpec{
					Enable: true,
					Configs: map[string]*sev1alpha1.ResourceFlavorConf{
						"C1": {
							Name:    "C1",
							NodeNum: 6,
							ForceAddNodes: []string{
								"N2", "N3",
							},
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "KeyA",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"ValueA"},
												},
											},
										},
									},
								},
							},
						},
						"C2": {
							Name:    "C2",
							NodeNum: 6,
							ForceAddNodes: []string{
								"N2", "N3",
							},
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "KeyB",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"ValueB"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Status: sev1alpha1.ResourceFlavorStatus{
					ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{
						"C1": {
							SelectedNodeNum: 1,
							SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
								"N0": {
									NodeMetaInfo: map[string]string{
										"AA": "BB",
									},
								},
							},
						},
						"C2": {
							SelectedNodeNum: 1,
							SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
								"N1": {
									NodeMetaInfo: map[string]string{
										"CC": "DD",
									},
								},
							},
						},
					},
				},
			},
			expectResult: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 4,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								"AA": "BB",
							},
						},
						"N1": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeManual,
							},
						},
						"N2": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeFlavor,
							},
						},
						"N3": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeFlavor,
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 4,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeManual,
							},
						},
						"N1": {
							NodeMetaInfo: map[string]string{
								"CC": "DD",
							},
						},
						"N2": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeFlavor,
							},
						},
						"N3": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeFlavor,
							},
						},
					},
				},
			},
		},
		{
			name: "general1",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"KeyA":                    "ValueA",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-0.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-3.NA130",
						},
					},
				},
			},
			thirdPartyNodes: map[string]string{},
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name: "B1",
				},
				Spec: sev1alpha1.ResourceFlavorSpec{
					Enable: true,
					Configs: map[string]*sev1alpha1.ResourceFlavorConf{
						"C1": {
							Name:    "C1",
							NodeNum: 2,
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "KeyB",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"ValueB"},
												},
											},
										},
									},
								},
							},
						},
						"C2": {
							Name:    "C2",
							NodeNum: 2,
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "KeyB",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"ValueB"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Status: sev1alpha1.ResourceFlavorStatus{
					ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{},
				},
			},
			expectResult: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 2,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N1": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-1.NA130",
								flavor.ForceWanted:        "false",
								flavor.ForceWantedType:    "",
							},
						},
						"N2": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
								flavor.ForceWanted:        "false",
								flavor.ForceWantedType:    "",
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 1,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N3": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-3.NA130",
								flavor.ForceWanted:        "false",
								flavor.ForceWantedType:    "",
							},
						},
					},
				},
			},
		},
		{
			name: "general2",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"KeyA":                    "ValueA",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-0.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P3-S15-3.NA130",
						},
					},
				},
			},
			thirdPartyNodes: map[string]string{},
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name: "B1",
				},
				Spec: sev1alpha1.ResourceFlavorSpec{
					Enable: true,
					Configs: map[string]*sev1alpha1.ResourceFlavorConf{
						"C1": {
							Name:    "C1",
							NodeNum: 2,
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "KeyB",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"ValueB"},
												},
											},
										},
									},
								},
							},
						},
						"C2": {
							Name:    "C2",
							NodeNum: 2,
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "KeyB",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"ValueB"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Status: sev1alpha1.ResourceFlavorStatus{
					ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{
						"C1": {
							SelectedNodeNum: 1,
							SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
								"N1": {
									NodeMetaInfo: map[string]string{
										paieflops.GpuModel:        "A100",
										paieflops.PointOfDelivery: "VM-G6-P1",
										paieflops.ASWID:           "ASW-VM-G6-P3-S15-1.NA130",
									},
								},
							},
						},
						"C2": {
							SelectedNodeNum: 1,
							SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
								"N2": {
									NodeMetaInfo: map[string]string{
										paieflops.GpuModel:        "A100",
										paieflops.PointOfDelivery: "VM-G6-P1",
										paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
									},
								},
							},
						},
					},
				},
			},
			expectResult: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 2,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N1": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-1.NA130",
							},
						},
						"N3": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-3.NA130",
								flavor.ForceWanted:        "false",
								flavor.ForceWantedType:    "",
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 1,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N2": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P3-S15-2.NA130",
							},
						},
					},
				},
			},
		},
		{
			name: "general3_sameTopologyFirst",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"KeyA":                    "ValueA",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P1-S15-0.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P1-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P1-S15-2.NA130",
						},
					},
				},
			},
			thirdPartyNodes: map[string]string{},
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name: "B1",
				},
				Spec: sev1alpha1.ResourceFlavorSpec{
					Enable: true,
					Configs: map[string]*sev1alpha1.ResourceFlavorConf{
						"C1": {
							Name:              "C1",
							NodeNum:           2,
							SameTopologyFirst: true,
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "KeyB",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"ValueB"},
												},
											},
										},
									},
								},
							},
						},
						"C2": {
							Name:              "C2",
							SameTopologyFirst: true,
							NodeNum:           2,
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "KeyB",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"ValueB"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Status: sev1alpha1.ResourceFlavorStatus{
					ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{},
				},
			},
			expectResult: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 2,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N2": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P1-S15-2.NA130",
								flavor.ForceWanted:        "false",
								flavor.ForceWantedType:    "",
							},
						},
						"N3": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P1-S15-2.NA130",
								flavor.ForceWanted:        "false",
								flavor.ForceWantedType:    "",
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 1,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N1": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
								flavor.ForceWanted:        "false",
								flavor.ForceWantedType:    "",
							},
						},
					},
				},
			},
		},
		{
			name: "general4_forceAdd",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"KeyA":                    "ValueA",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P1-S15-0.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N2",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P1-S15-2.NA130",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N3",
						Labels: map[string]string{
							"KeyB":                    "ValueB",
							paieflops.GpuModel:        "A100",
							paieflops.PointOfDelivery: "VM-G6-P1",
							paieflops.ASWID:           "ASW-VM-G6-P1-S15-2.NA130",
						},
					},
				},
			},
			thirdPartyNodes: map[string]string{
				"N0": "B1",
			},
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name: "B1",
				},
				Spec: sev1alpha1.ResourceFlavorSpec{
					Enable: true,
					Configs: map[string]*sev1alpha1.ResourceFlavorConf{
						"C1": {
							Name:              "C1",
							NodeNum:           2,
							SameTopologyFirst: true,
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "KeyB",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"ValueB"},
												},
											},
										},
									},
								},
							},
						},
						"C2": {
							Name:              "C2",
							SameTopologyFirst: true,
							NodeNum:           2,
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "KeyB",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"ValueB"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Status: sev1alpha1.ResourceFlavorStatus{
					ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{},
				},
			},
			expectResult: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 2,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P1-S15-0.NA130",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeManual,
							},
						},
						"N1": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P1-S15-1.NA130",
								flavor.ForceWanted:        "false",
								flavor.ForceWantedType:    "",
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 2,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P1-S15-0.NA130",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeManual,
							},
						},
						"N2": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "A100",
								paieflops.PointOfDelivery: "VM-G6-P1",
								paieflops.ASWID:           "ASW-VM-G6-P1-S15-2.NA130",
								flavor.ForceWanted:        "false",
								flavor.ForceWantedType:    "",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := cache.NewNodeCache()
			nodeCache.UpdateNodes(tt.nodes)

			resourceFlavorInfo := cache.NewResourceFlavorInfo(tt.resourceFlavorCrd)
			nodeCache.UpdateCache(resourceFlavorInfo.GetSelectedNodes(), tt.thirdPartyNodes)

			flavor := flavor.NewResourceFlavor(nil, nodeCache, nil)
			flavor.TryAddValidNodes("B1", resourceFlavorInfo)

			assert.True(t, reflect.DeepEqual(tt.expectResult, resourceFlavorInfo.GetLocalConfStatus4Test()))
		})
	}
}
