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

func TestTryUpdateNodeMeta(t *testing.T) {
	tests := []struct {
		name               string
		nodes              []corev1.Node
		thirdPartyNodes    map[string]string
		quotaNodeBinderCrd *sev1alpha1.ResourceFlavor
		expectResult       map[string]*sev1alpha1.ResourceFlavorConfStatus
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
			quotaNodeBinderCrd: &sev1alpha1.ResourceFlavor{
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
							SelectedNodeNum: 4,
							SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
								"N0": {
									NodeMetaInfo: map[string]string{
										flavor.ForceWanted: "true",
									},
								},
								"N1": {
									NodeMetaInfo: map[string]string{
										flavor.ForceWanted: "false",
									},
								},
								"N2": {
									NodeMetaInfo: map[string]string{
										flavor.ForceWanted: "false",
									},
								},
								"N3": {
									NodeMetaInfo: map[string]string{
										flavor.ForceWanted: "true",
									},
								},
							},
						},
						"C2": {
							SelectedNodeNum: 4,
							SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
								"N0": {
									NodeMetaInfo: map[string]string{
										flavor.ForceWanted: "true",
									},
								},
								"N1": {
									NodeMetaInfo: map[string]string{
										flavor.ForceWanted: "false",
									},
								},
								"N2": {
									NodeMetaInfo: map[string]string{
										flavor.ForceWanted: "false",
									},
								},
								"N3": {
									NodeMetaInfo: map[string]string{
										flavor.ForceWanted: "true",
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
								flavor.ForceWanted:     "true",
								flavor.ForceWantedType: flavor.ForceAddTypeManual,
							},
						},
						"N1": {
							NodeMetaInfo: map[string]string{
								flavor.ForceWanted:     "true",
								flavor.ForceWantedType: flavor.ForceAddTypeManual,
							},
						},
						"N2": {
							NodeMetaInfo: map[string]string{
								flavor.ForceWanted:     "true",
								flavor.ForceWantedType: flavor.ForceAddTypeFlavor,
							},
						},
						"N3": {
							NodeMetaInfo: map[string]string{
								flavor.ForceWanted:     "false",
								flavor.ForceWantedType: "",
							},
						},
					},
				},
				"C2": {
					SelectedNodeNum: 4,
					SelectedNodes: map[string]*sev1alpha1.SelectedNodeMeta{
						"N0": {
							NodeMetaInfo: map[string]string{
								flavor.ForceWanted:     "true",
								flavor.ForceWantedType: flavor.ForceAddTypeManual,
							},
						},
						"N1": {
							NodeMetaInfo: map[string]string{
								flavor.ForceWanted:     "true",
								flavor.ForceWantedType: flavor.ForceAddTypeManual,
							},
						},
						"N2": {
							NodeMetaInfo: map[string]string{
								flavor.ForceWanted:     "true",
								flavor.ForceWantedType: flavor.ForceAddTypeFlavor,
							},
						},
						"N3": {
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

			quotaNodeBinderInfo := cache.NewResourceFlavorInfo(tt.quotaNodeBinderCrd)
			nodeCache.UpdateCache(quotaNodeBinderInfo.GetSelectedNodes(), tt.thirdPartyNodes)

			binder := flavor.NewResourceFlavor(nil, nodeCache, nil)
			binder.TryUpdateNodeMeta("B1", quotaNodeBinderInfo)

			assert.True(t, reflect.DeepEqual(tt.expectResult, quotaNodeBinderInfo.GetLocalConfStatus4Test()))
		})
	}
}
