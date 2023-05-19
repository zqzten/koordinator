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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func TestNewResourceFlavorInfo(t *testing.T) {
	tests := []struct {
		name                    string
		resourceFlavorCrd       *sev1alpha1.ResourceFlavor
		expectResourceFlavorCrd *sev1alpha1.ResourceFlavor
		expectLocalConfStatus   map[string]*sev1alpha1.ResourceFlavorConfStatus
	}{
		{
			name: "NewResourceFlavorInfo",
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
			expectResourceFlavorCrd: &sev1alpha1.ResourceFlavor{
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
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceFlavorInfo := NewResourceFlavorInfo(tt.resourceFlavorCrd)
			assert.True(t, reflect.DeepEqual(tt.expectResourceFlavorCrd, resourceFlavorInfo.resourceFlavorCrd))
			assert.True(t, reflect.DeepEqual(tt.expectLocalConfStatus, resourceFlavorInfo.localConfStatus))

			resourceFlavorInfo.SetResourceFlavorCrd(tt.resourceFlavorCrd)
			assert.True(t, reflect.DeepEqual(tt.expectResourceFlavorCrd, resourceFlavorInfo.resourceFlavorCrd))
			assert.True(t, reflect.DeepEqual(tt.expectLocalConfStatus, resourceFlavorInfo.localConfStatus))
		})
	}
}

func TestEnable(t *testing.T) {
	tests := []struct {
		name               string
		resourceFlavorInfo *ResourceFlavorInfo
		expectEnable       bool
	}{
		{
			name:         "true",
			expectEnable: true,
			resourceFlavorInfo: &ResourceFlavorInfo{
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
							},
						},
					},
				},
				localConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
					"C1": {
						SelectedNodeNum: 10,
					},
				},
			},
		},
		{
			name:         "true",
			expectEnable: false,
			resourceFlavorInfo: &ResourceFlavorInfo{
				resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
					ObjectMeta: metav1.ObjectMeta{
						Name: "B1",
					},
					Spec: sev1alpha1.ResourceFlavorSpec{
						Enable: false,
					},
					Status: sev1alpha1.ResourceFlavorStatus{
						ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{
							"C1": {
								SelectedNodeNum: 10,
							},
						},
					},
				},
				localConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
					"C1": {
						SelectedNodeNum: 10,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceFlavorInfo := NewResourceFlavorInfo(tt.resourceFlavorInfo.resourceFlavorCrd)
			assert.True(t, reflect.DeepEqual(tt.expectEnable, resourceFlavorInfo.Enable()))
		})
	}
}

func TestGetSelectedNodes(t *testing.T) {
	tests := []struct {
		name               string
		resourceFlavorInfo *ResourceFlavorInfo
		expectSelectNodes  map[string]*NodeBoundInfo
	}{
		{
			name: "GetSelectedNodes",
			resourceFlavorInfo: &ResourceFlavorInfo{
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
			},
			expectSelectNodes: map[string]*NodeBoundInfo{
				"N0": {
					QuotaName: "B1",
					ConfName:  "C1",
				},
				"N1": {
					QuotaName: "B1",
					ConfName:  "C2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceFlavorInfo := NewResourceFlavorInfo(tt.resourceFlavorInfo.resourceFlavorCrd)
			result := resourceFlavorInfo.GetSelectedNodes()
			assert.True(t, reflect.DeepEqual(tt.expectSelectNodes, result))
		})
	}
}

func TestRemoveAllLocalConfStatus(t *testing.T) {
	tests := []struct {
		name                  string
		resourceFlavorCrd     *sev1alpha1.ResourceFlavor
		expectLocalConfStatus map[string]*sev1alpha1.ResourceFlavorConfStatus
		removeFunc            func(confName string, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool
		expectHasRemoved      bool
	}{
		{
			name: "expectHasRemoved:true",
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
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 0,
					SelectedNodes:   map[string]*sev1alpha1.SelectedNodeMeta{},
				},
				"C2": {
					SelectedNodeNum: 0,
					SelectedNodes:   map[string]*sev1alpha1.SelectedNodeMeta{},
				},
			},
			removeFunc: func(confName string, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
				localConfStatus.SelectedNodeNum = 0
				localConfStatus.SelectedNodes = make(map[string]*sev1alpha1.SelectedNodeMeta)
				return true
			},
			expectHasRemoved: true,
		},
		{
			name: "expectHasRemoved:false",
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
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 0,
					SelectedNodes:   map[string]*sev1alpha1.SelectedNodeMeta{},
				},
				"C2": {
					SelectedNodeNum: 0,
					SelectedNodes:   map[string]*sev1alpha1.SelectedNodeMeta{},
				},
			},
			removeFunc: func(confName string, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
				localConfStatus.SelectedNodeNum = 0
				localConfStatus.SelectedNodes = make(map[string]*sev1alpha1.SelectedNodeMeta)
				return false
			},
			expectHasRemoved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceFlavorInfo := NewResourceFlavorInfo(tt.resourceFlavorCrd)
			hasRemoved := resourceFlavorInfo.RemoveAllLocalConfStatus(tt.removeFunc)

			assert.True(t, reflect.DeepEqual(tt.expectLocalConfStatus, resourceFlavorInfo.localConfStatus))
			assert.True(t, reflect.DeepEqual(tt.expectHasRemoved, hasRemoved))
		})
	}
}

func TestRemoveAllInvalidLocalConfStatus(t *testing.T) {
	tests := []struct {
		name                  string
		resourceFlavorCrd     *sev1alpha1.ResourceFlavor
		expectLocalConfStatus map[string]*sev1alpha1.ResourceFlavorConfStatus
		removeFunc            func(confName string, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool
		expectHasRemoved      bool
	}{
		{
			name: "expectHasRemoved:true",
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name: "B1",
				},
				Spec: sev1alpha1.ResourceFlavorSpec{
					Enable: true,
					Configs: map[string]*sev1alpha1.ResourceFlavorConf{
						"C1": {
							NodeNum: 10,
						},
						"C3": {
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
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
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
					SelectedNodeNum: 0,
					SelectedNodes:   map[string]*sev1alpha1.SelectedNodeMeta{},
				},
			},
			removeFunc: func(confName string, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
				localConfStatus.SelectedNodeNum = 0
				localConfStatus.SelectedNodes = make(map[string]*sev1alpha1.SelectedNodeMeta)
				return true
			},
			expectHasRemoved: true,
		},
		{
			name: "expectHasRemoved:false",
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name: "B1",
				},
				Spec: sev1alpha1.ResourceFlavorSpec{
					Enable: true,
					Configs: map[string]*sev1alpha1.ResourceFlavorConf{
						"C1": {
							NodeNum: 10,
						},
						"C3": {
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
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
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
					SelectedNodeNum: 0,
					SelectedNodes:   map[string]*sev1alpha1.SelectedNodeMeta{},
				},
			},
			removeFunc: func(confName string, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
				localConfStatus.SelectedNodeNum = 0
				localConfStatus.SelectedNodes = make(map[string]*sev1alpha1.SelectedNodeMeta)
				return false
			},
			expectHasRemoved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceFlavorInfo := NewResourceFlavorInfo(tt.resourceFlavorCrd)
			hasRemoved := resourceFlavorInfo.RemoveAllInvalidLocalConfStatus(tt.removeFunc)

			assert.True(t, reflect.DeepEqual(tt.expectLocalConfStatus, resourceFlavorInfo.localConfStatus))
			assert.True(t, reflect.DeepEqual(tt.expectHasRemoved, hasRemoved))
		})
	}
}

func TestGetNewResourceFlavorCrd(t *testing.T) {
	tests := []struct {
		name                    string
		resourceFlavorCrd       *sev1alpha1.ResourceFlavor
		localStatus             map[string]*sev1alpha1.ResourceFlavorConfStatus
		expectResourceFlavorCrd *sev1alpha1.ResourceFlavor
	}{
		{
			name: "expectHasRemoved:true",
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name: "B1",
				},
				Spec: sev1alpha1.ResourceFlavorSpec{
					Enable: true,
					Configs: map[string]*sev1alpha1.ResourceFlavorConf{
						"C1": {
							NodeNum: 10,
						},
						"C3": {
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
			localStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
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
			},
			expectResourceFlavorCrd: &sev1alpha1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name: "B1",
				},
				Spec: sev1alpha1.ResourceFlavorSpec{
					Enable: true,
					Configs: map[string]*sev1alpha1.ResourceFlavorConf{
						"C1": {
							NodeNum: 10,
						},
						"C3": {
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
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceFlavorInfo := NewResourceFlavorInfo(tt.resourceFlavorCrd)
			resourceFlavorInfo.localConfStatus = tt.localStatus

			assert.True(t, reflect.DeepEqual(tt.expectResourceFlavorCrd, resourceFlavorInfo.GetNewResourceFlavorCrd()))
		})
	}
}

func TestTryRemoveNodes(t *testing.T) {
	tests := []struct {
		name                  string
		resourceFlavorCrd     *sev1alpha1.ResourceFlavor
		expectLocalConfStatus map[string]*sev1alpha1.ResourceFlavorConfStatus
		removeFunc            func(confName string, conf *sev1alpha1.ResourceFlavorConf,
			localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool
		expectHasRemoved bool
	}{
		{
			name: "expectHasRemoved:true",
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
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
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 9,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								"AA": "BB",
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 9,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N2": {
							NodeMetaInfo: map[string]string{
								"CC": "DD",
							},
						},
					},
				},
			},
			removeFunc: func(confName string, conf *sev1alpha1.ResourceFlavorConf,
				localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
				RemoveNodeFromLocalConfStatus("N1", localConfStatus)
				RemoveNodeFromLocalConfStatus("N3", localConfStatus)
				return true
			},
			expectHasRemoved: true,
		},
		{
			name: "expectHasRemoved:false",
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
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
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 9,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								"AA": "BB",
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 9,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N2": {
							NodeMetaInfo: map[string]string{
								"CC": "DD",
							},
						},
					},
				},
			},
			removeFunc: func(confName string, conf *sev1alpha1.ResourceFlavorConf,
				localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
				RemoveNodeFromLocalConfStatus("N1", localConfStatus)
				RemoveNodeFromLocalConfStatus("N3", localConfStatus)
				return false
			},
			expectHasRemoved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceFlavorInfo := NewResourceFlavorInfo(tt.resourceFlavorCrd)
			hasRemoved := resourceFlavorInfo.TryRemoveNodes(tt.removeFunc)

			assert.True(t, reflect.DeepEqual(tt.expectLocalConfStatus, resourceFlavorInfo.localConfStatus))
			assert.True(t, reflect.DeepEqual(tt.expectHasRemoved, hasRemoved))
		})
	}
}

func TestGetWantedForceAddNodes(t *testing.T) {
	tests := []struct {
		name                 string
		conf                 *sev1alpha1.ResourceFlavorConf
		thirdPartyBoundNodes sets.String
		expect               sets.String
	}{
		{
			name: "normal",
			conf: &sev1alpha1.ResourceFlavorConf{
				ForceAddNodes: []string{
					"N0", "N1", "N2",
				},
			},
			thirdPartyBoundNodes: sets.String{
				"N2": struct{}{},
				"N3": struct{}{},
			},
			expect: sets.String{
				"N0": struct{}{},
				"N1": struct{}{},
				"N2": struct{}{},
				"N3": struct{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, reflect.DeepEqual(tt.expect, GetWantedForceAddNodes(tt.conf, tt.thirdPartyBoundNodes)))
		})
	}
}

func TestGetGeneralNodeTotalWantedNum(t *testing.T) {
	tests := []struct {
		name                 string
		conf                 *sev1alpha1.ResourceFlavorConf
		thirdPartyBoundNodes sets.String
		expect               int
	}{
		{
			name: "normal1",
			conf: &sev1alpha1.ResourceFlavorConf{
				NodeNum: 10,
				ForceAddNodes: []string{
					"N0", "N1", "N2",
				},
			},
			thirdPartyBoundNodes: sets.String{
				"N2": struct{}{},
				"N3": struct{}{},
			},
			expect: 6,
		},
		{
			name: "normal1",
			conf: &sev1alpha1.ResourceFlavorConf{
				NodeNum: 1,
				ForceAddNodes: []string{
					"N0", "N1", "N2",
				},
			},
			thirdPartyBoundNodes: sets.String{
				"N2": struct{}{},
				"N3": struct{}{},
			},
			expect: -3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, reflect.DeepEqual(tt.expect, GetGeneralNodeTotalWantedNum(tt.conf, tt.thirdPartyBoundNodes)))
		})
	}
}

func TestGetGeneralSelectedNodes(t *testing.T) {
	tests := []struct {
		name                 string
		conf                 *sev1alpha1.ResourceFlavorConf
		localConfStatus      *sev1alpha1.ResourceFlavorConfStatus
		thirdPartyBoundNodes sets.String
		expect               sets.String
	}{
		{
			name: "normal1",
			conf: &sev1alpha1.ResourceFlavorConf{
				NodeNum: 10,
				ForceAddNodes: []string{
					"N0", "N1", "N2",
				},
			},
			localConfStatus: &sev1alpha1.ResourceFlavorConfStatus{
				SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
					"N0": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
					"N4": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
				},
			},
			thirdPartyBoundNodes: sets.String{
				"N2": struct{}{},
				"N3": struct{}{},
			},
			expect: sets.String{
				"N4": struct{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, reflect.DeepEqual(tt.expect, GetGeneralSelectedNodes(
				tt.conf, tt.localConfStatus, tt.thirdPartyBoundNodes)))
		})
	}
}

func TestGetForceSelectedNodes(t *testing.T) {
	tests := []struct {
		name                 string
		conf                 *sev1alpha1.ResourceFlavorConf
		localConfStatus      *sev1alpha1.ResourceFlavorConfStatus
		thirdPartyBoundNodes sets.String
		expect               sets.String
	}{
		{
			name: "normal1",
			conf: &sev1alpha1.ResourceFlavorConf{
				NodeNum: 10,
				ForceAddNodes: []string{
					"N0", "N1", "N2",
				},
			},
			localConfStatus: &sev1alpha1.ResourceFlavorConfStatus{
				SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
					"N0": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
					"N4": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
				},
			},
			thirdPartyBoundNodes: sets.String{
				"N2": struct{}{},
				"N3": struct{}{},
			},
			expect: sets.String{
				"N0": struct{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, reflect.DeepEqual(tt.expect, GetForceSelectedNodes(
				tt.conf, tt.localConfStatus, tt.thirdPartyBoundNodes)))
		})
	}
}

func TestRemoveUnnecessaryNodes(t *testing.T) {
	tests := []struct {
		name                  string
		conf                  *sev1alpha1.ResourceFlavorConf
		localConfStatus       *sev1alpha1.ResourceFlavorConfStatus
		thirdPartyBoundNodes  sets.String
		expectLocalConfStatus *sev1alpha1.ResourceFlavorConfStatus
		expectHasRemove       bool
	}{
		{
			name: "nothing to remove",
			conf: &sev1alpha1.ResourceFlavorConf{
				NodeNum: 10,
				ForceAddNodes: []string{
					"N0", "N1", "N2",
				},
			},
			localConfStatus: &sev1alpha1.ResourceFlavorConfStatus{
				SelectedNodeNum: 2,
				SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
					"N0": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
					"N4": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
				},
			},
			thirdPartyBoundNodes: sets.String{
				"N2": struct{}{},
				"N3": struct{}{},
			},
			expectLocalConfStatus: &sev1alpha1.ResourceFlavorConfStatus{
				SelectedNodeNum: 2,
				SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
					"N0": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
					"N4": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
				},
			},
			expectHasRemove: false,
		},
		{
			name: "remove general",
			conf: &sev1alpha1.ResourceFlavorConf{
				NodeNum: 1,
				ForceAddNodes: []string{
					"N0", "N1", "N2",
				},
			},
			localConfStatus: &sev1alpha1.ResourceFlavorConfStatus{
				SelectedNodeNum: 2,
				SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
					"N0": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
					"N4": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
				},
			},
			thirdPartyBoundNodes: sets.String{
				"N2": struct{}{},
				"N3": struct{}{},
			},
			expectLocalConfStatus: &sev1alpha1.ResourceFlavorConfStatus{
				SelectedNodeNum: 1,
				SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
					"N0": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
				},
			},
			expectHasRemove: true,
		},
		{
			name: "remove all",
			conf: &sev1alpha1.ResourceFlavorConf{
				NodeNum: 0,
				ForceAddNodes: []string{
					"N0", "N1", "N2",
				},
			},
			localConfStatus: &sev1alpha1.ResourceFlavorConfStatus{
				SelectedNodeNum: 2,
				SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
					"N0": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
					"N4": {
						NodeMetaInfo: map[string]string{
							"AA": "BB",
						},
					},
				},
			},
			thirdPartyBoundNodes: sets.String{
				"N2": struct{}{},
				"N3": struct{}{},
			},
			expectLocalConfStatus: &sev1alpha1.ResourceFlavorConfStatus{
				SelectedNodeNum: 0,
				SelectedNodes:   map[string]*sev1alpha1.SelectedNodeMeta{},
			},
			expectHasRemove: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, reflect.DeepEqual(tt.expectHasRemove, RemoveUnnecessaryNodes(
				tt.conf, tt.localConfStatus, tt.thirdPartyBoundNodes)))
			assert.True(t, reflect.DeepEqual(tt.expectLocalConfStatus, tt.localConfStatus))
		})
	}
}

func TestTryAddNodes(t *testing.T) {
	tests := []struct {
		name                  string
		resourceFlavorCrd     *sev1alpha1.ResourceFlavor
		expectLocalConfStatus map[string]*sev1alpha1.ResourceFlavorConfStatus
		addFunc               func(confName string, conf *sev1alpha1.ResourceFlavorConf,
			localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool
		expectHasAdd bool
	}{
		{
			name: "case1",
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
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
			addFunc: func(confName string, conf *sev1alpha1.ResourceFlavorConf,
				localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
				metaMap := make(map[string]string)
				metaMap["AAA"] = "BBB"
				AddNode("N0", metaMap, localConfStatus)
				AddNode("N4", metaMap, localConfStatus)
				return true
			},
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 11,
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
						"N4": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 12,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
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
						"N4": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
					},
				},
			},
			expectHasAdd: true,
		},
		{
			name: "case2",
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
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
				Status: sev1alpha1.ResourceFlavorStatus{},
			},
			addFunc: func(confName string, conf *sev1alpha1.ResourceFlavorConf,
				localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
				metaMap := make(map[string]string)
				metaMap["AAA"] = "BBB"
				AddNode("N0", metaMap, localConfStatus)
				AddNode("N4", metaMap, localConfStatus)
				return true
			},
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 2,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
						"N4": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 2,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
						"N4": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
					},
				},
			},
			expectHasAdd: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceFlavorInfo := NewResourceFlavorInfo(tt.resourceFlavorCrd)
			hasAdd := resourceFlavorInfo.TryAddNodes(tt.addFunc)

			assert.True(t, reflect.DeepEqual(tt.expectLocalConfStatus, resourceFlavorInfo.localConfStatus))
			assert.True(t, reflect.DeepEqual(tt.expectHasAdd, hasAdd))
		})
	}
}

func TestTryUpdateNodeMeta(t *testing.T) {
	tests := []struct {
		name                  string
		resourceFlavorCrd     *sev1alpha1.ResourceFlavor
		expectLocalConfStatus map[string]*sev1alpha1.ResourceFlavorConfStatus
		updateFunc            func(confName string, conf *sev1alpha1.ResourceFlavorConf,
			localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool
		expectHasUpdate bool
	}{
		{
			name: "case1",
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
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
			updateFunc: func(confName string, conf *sev1alpha1.ResourceFlavorConf,
				localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
				metaMap := make(map[string]string)
				metaMap["AAA"] = "BBB"
				AddNode("N0", metaMap, localConfStatus)
				AddNode("N4", metaMap, localConfStatus)
				return true
			},
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 11,
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
						"N4": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 12,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
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
						"N4": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
					},
				},
			},
			expectHasUpdate: true,
		},
		{
			name: "case2",
			resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
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
				Status: sev1alpha1.ResourceFlavorStatus{},
			},
			updateFunc: func(confName string, conf *sev1alpha1.ResourceFlavorConf,
				localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
				metaMap := make(map[string]string)
				metaMap["AAA"] = "BBB"
				AddNode("N0", metaMap, localConfStatus)
				AddNode("N4", metaMap, localConfStatus)
				return true
			},
			expectLocalConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{
				"C1": {
					SelectedNodeNum: 2,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
						"N4": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 2,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
						"N4": {
							NodeMetaInfo: map[string]string{
								"AAA": "BBB",
							},
						},
					},
				},
			},
			expectHasUpdate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceFlavorInfo := NewResourceFlavorInfo(tt.resourceFlavorCrd)
			hasAdd := resourceFlavorInfo.TryUpdateNodeMeta(tt.updateFunc)

			assert.True(t, reflect.DeepEqual(tt.expectLocalConfStatus, resourceFlavorInfo.localConfStatus))
			assert.True(t, reflect.DeepEqual(tt.expectHasUpdate, hasAdd))
		})
	}
}
