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
	"gitlab.alibaba-inc.com/cos/scheduling-api/pkg/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/cache"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/flavor"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/paieflops"
)

func TestUpdateNodeCache(t *testing.T) {
	tests := []struct {
		name                       string
		resourceFlavorCrds         []sev1alpha1.ResourceFlavor
		nodes                      []corev1.Node
		expectFreeNodes            sets.String
		expectThirdPartyBoundNodes map[string]sets.String
		expectAllAutoBoundNodes    map[string]*cache.NodeBoundInfo
	}{
		{
			name: "normal1",
			resourceFlavorCrds: []sev1alpha1.ResourceFlavor{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "B1",
					},
					Spec: sev1alpha1.ResourceFlavorSpec{
						Enable: true,
						Configs: map[string]*sev1alpha1.ResourceFlavorConf{
							"C1": {
								NodeNum: 10,
							},
							"C2": {
								NodeNum: 10,
							},
						},
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
								},
							},
							"C2": {
								SelectedNodeNum: 10,
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "B2",
					},
					Spec: sev1alpha1.ResourceFlavorSpec{
						Enable: true,
						Configs: map[string]*sev1alpha1.ResourceFlavorConf{
							"C1": {
								NodeNum: 10,
							},
							"C2": {
								NodeNum: 10,
							},
						},
					},
					Status: sev1alpha1.ResourceFlavorStatus{
						ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{
							"C1": {
								SelectedNodeNum: 10,
								SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
									"N2": {
										NodeMetaInfo: map[string]string{
											"AA": "BB",
										},
									},
								},
							},
							"C2": {
								SelectedNodeNum: 10,
								SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
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
			},
			nodes: []corev1.Node{
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
						Labels: map[string]string{
							"B1": "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N5",
						Labels: map[string]string{
							"B2": "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N6",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N7",
					},
				},
			},
			expectFreeNodes: sets.String{
				"N6": struct{}{},
				"N7": struct{}{},
			},
			expectThirdPartyBoundNodes: map[string]sets.String{
				"B1": {
					"N4": struct{}{},
				},
				"B2": {
					"N5": struct{}{},
				},
			},
			expectAllAutoBoundNodes: map[string]*cache.NodeBoundInfo{
				"N0": {
					QuotaName: "B1",
					ConfName:  "C1",
				},
				"N1": {
					QuotaName: "B1",
					ConfName:  "C2",
				},
				"N2": {
					QuotaName: "B2",
					ConfName:  "C1",
				},
				"N3": {
					QuotaName: "B2",
					ConfName:  "C2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCache := cache.NewNodeCache()
			nodeCache.UpdateNodes(tt.nodes)

			resourceFlavorCache := cache.NewResourceFlavorCache()
			resourceFlavorCache.UpdateResourceFlavors(tt.resourceFlavorCrds)

			paieflops.GetElasticQuotaTreeCr = func(_client client.Client) *v1beta1.ElasticQuotaTree {
				tree := &v1beta1.ElasticQuotaTree{}

				quota1 := &v1beta1.ElasticQuotaSpec{
					Name: "B1",
				}
				quota2 := &v1beta1.ElasticQuotaSpec{
					Name: "B2",
				}
				tree.Spec.Root.Children = append(tree.Spec.Root.Children, *quota1)
				tree.Spec.Root.Children = append(tree.Spec.Root.Children, *quota2)

				return tree
			}

			flavor := flavor.NewResourceFlavor(nil, nodeCache, resourceFlavorCache)
			flavor.UpdateNodeCache()

			assert.True(t, reflect.DeepEqual(tt.expectFreeNodes, nodeCache.GetAllFreePoolNodes()))
			assert.True(t, reflect.DeepEqual(tt.expectThirdPartyBoundNodes["B1"], nodeCache.GetThirdPartyBoundNodes("B1")))
			assert.True(t, reflect.DeepEqual(tt.expectThirdPartyBoundNodes["B2"], nodeCache.GetThirdPartyBoundNodes("B2")))
			assert.True(t, reflect.DeepEqual(tt.expectAllAutoBoundNodes, nodeCache.GetAllAutoBoundNodes4Test()))
		})
	}
}

func TestResourceFlavor(t *testing.T) {
	tests := []struct {
		name                  string
		resourceFlavorCrds    []sev1alpha1.ResourceFlavor
		nodes                 []corev1.Node
		expectLocalConfStatus map[string]*sev1alpha1.ResourceFlavorConfStatus
	}{
		{
			name: "normal1",
			resourceFlavorCrds: []sev1alpha1.ResourceFlavor{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "B1",
					},
					Spec: sev1alpha1.ResourceFlavorSpec{
						Enable: true,
						Configs: map[string]*sev1alpha1.ResourceFlavorConf{
							"C1": {
								NodeNum: 10,
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
						},
					},
				},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"KeyB": "ValueB",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"KeyA": "ValueA",
						},
					},
				},
			},
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 1,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N1": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "",
								paieflops.PointOfDelivery: "",
								paieflops.ASWID:           "",
								flavor.ForceWanted:        "false",
								flavor.ForceWantedType:    "",
							},
						},
					},
				},
			},
		},
		{
			name: "normal2",
			resourceFlavorCrds: []sev1alpha1.ResourceFlavor{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "B1",
					},
					Spec: sev1alpha1.ResourceFlavorSpec{
						Enable: true,
						Configs: map[string]*sev1alpha1.ResourceFlavorConf{
							"C1": {
								NodeNum: 10,
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
						},
					},
					Status: sev1alpha1.ResourceFlavorStatus{
						ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{
							"C1": {
								SelectedNodeNum: 1,
								SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
									"N1": {
										NodeMetaInfo: map[string]string{
											flavor.ForceWanted: "true",
										},
									},
								},
							},
						},
					},
				},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"KeyB": "ValueB",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"KeyA": "ValueA",
						},
					},
				},
			},
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 1,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N1": {
							NodeMetaInfo: map[string]string{
								flavor.ForceWanted:     "false",
								flavor.ForceWantedType: "",
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

			flavorCache := cache.NewResourceFlavorCache()
			flavorCache.UpdateResourceFlavors(tt.resourceFlavorCrds)

			paieflops.GetElasticQuotaTreeCr = func(_client client.Client) *v1beta1.ElasticQuotaTree {
				tree := &v1beta1.ElasticQuotaTree{}

				quota1 := &v1beta1.ElasticQuotaSpec{
					Name: "B1",
				}
				quota2 := &v1beta1.ElasticQuotaSpec{
					Name: "B2",
				}
				tree.Spec.Root.Children = append(tree.Spec.Root.Children, *quota1)
				tree.Spec.Root.Children = append(tree.Spec.Root.Children, *quota2)

				return tree
			}

			flavor := flavor.NewResourceFlavor(nil, nodeCache, flavorCache)
			flavor.ResourceFlavor()

			for _, flavorInfo := range flavorCache.GetAllResourceFlavor() {
				assert.True(t, reflect.DeepEqual(tt.expectLocalConfStatus, flavorInfo.GetLocalConfStatus4Test()))
			}
		})
	}
}

func TestResourceFlavor_TryUpdateNodeMeta(t *testing.T) {
	tests := []struct {
		name                   string
		resourceFlavorCrds     []sev1alpha1.ResourceFlavor
		nodes                  []corev1.Node
		expectLocalConfStatus1 map[string]*sev1alpha1.ResourceFlavorConfStatus
		expectLocalConfStatus2 map[string]*sev1alpha1.ResourceFlavorConfStatus
	}{
		{
			name: "normal1",
			resourceFlavorCrds: []sev1alpha1.ResourceFlavor{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "B1",
					},
					Spec: sev1alpha1.ResourceFlavorSpec{
						Enable: true,
						Configs: map[string]*sev1alpha1.ResourceFlavorConf{
							"C1": {
								NodeNum: 10,
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
						},
					},
				},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N0",
						Labels: map[string]string{
							"KeyB": "ValueB",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "N1",
						Labels: map[string]string{
							"KeyA": "ValueA",
							"B1":   "true",
						},
					},
				},
			},
			expectLocalConfStatus1: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 1,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N1": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "",
								paieflops.PointOfDelivery: "",
								paieflops.ASWID:           "",
								flavor.ForceWanted:        "true",
								flavor.ForceWantedType:    flavor.ForceAddTypeManual,
							},
						},
					},
				},
			},
			expectLocalConfStatus2: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 1,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N1": {
							NodeMetaInfo: map[string]string{
								paieflops.GpuModel:        "",
								paieflops.PointOfDelivery: "",
								paieflops.ASWID:           "",
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

			flavorCache := cache.NewResourceFlavorCache()
			flavorCache.UpdateResourceFlavors(tt.resourceFlavorCrds)

			paieflops.GetElasticQuotaTreeCr = func(_client client.Client) *v1beta1.ElasticQuotaTree {
				tree := &v1beta1.ElasticQuotaTree{}

				quota1 := &v1beta1.ElasticQuotaSpec{
					Name: "B1",
				}
				quota2 := &v1beta1.ElasticQuotaSpec{
					Name: "B2",
				}
				tree.Spec.Root.Children = append(tree.Spec.Root.Children, *quota1)
				tree.Spec.Root.Children = append(tree.Spec.Root.Children, *quota2)

				return tree
			}

			flavor := flavor.NewResourceFlavor(nil, nodeCache, flavorCache)
			flavor.ResourceFlavor()

			for _, flavorInfo := range flavorCache.GetAllResourceFlavor() {
				assert.True(t, reflect.DeepEqual(tt.expectLocalConfStatus1, flavorInfo.GetLocalConfStatus4Test()))
			}

			for _, node := range tt.nodes {
				delete(node.Labels, "B1")
			}
			nodeCache.UpdateNodes(tt.nodes)
			flavor.ResourceFlavor()
			for _, flavorInfo := range flavorCache.GetAllResourceFlavor() {
				assert.True(t, reflect.DeepEqual(tt.expectLocalConfStatus2, flavorInfo.GetLocalConfStatus4Test()))
			}

		})
	}
}
