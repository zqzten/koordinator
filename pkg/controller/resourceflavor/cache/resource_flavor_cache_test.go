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

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func TestUpdateQuotaNodeBinders(t *testing.T) {
	tests := []struct {
		name                   string
		firstQuotaNodeBinders  []sev1alpha1.ResourceFlavor
		secondQuotaNodeBinders []sev1alpha1.ResourceFlavor
		want                   map[string]*ResourceFlavorInfo
	}{
		{
			name: "normal",
			firstQuotaNodeBinders: []sev1alpha1.ResourceFlavor{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "B1",
					},
					Status: sev1alpha1.ResourceFlavorStatus{
						ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{
							"C1": {
								SelectedNodeNum: 10,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "B2",
					},
				},
			},
			secondQuotaNodeBinders: []sev1alpha1.ResourceFlavor{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "B1",
					},
					Status: sev1alpha1.ResourceFlavorStatus{
						ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{
							"C1": {
								SelectedNodeNum: 20,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "B3",
					},
				},
			},
			want: map[string]*ResourceFlavorInfo{
				"B1": {
					resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
						ObjectMeta: metav1.ObjectMeta{
							Name: "B1",
						},
						Status: sev1alpha1.ResourceFlavorStatus{
							ConfigStatuses: map[string]*sev1alpha1.ResourceFlavorConfStatus{
								"C1": {
									SelectedNodeNum: 20,
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
				"B3": {
					resourceFlavorCrd: &sev1alpha1.ResourceFlavor{
						ObjectMeta: metav1.ObjectMeta{
							Name: "B3",
						},
					},
					localConfStatus: map[string]*sev1alpha1.ResourceFlavorConfStatus{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quotaNodeBinderCache := NewResourceFlavorCache()
			quotaNodeBinderCache.UpdateResourceFlavors(tt.firstQuotaNodeBinders)
			quotaNodeBinderCache.UpdateResourceFlavors(tt.secondQuotaNodeBinders)

			assert.True(t, reflect.DeepEqual(tt.want, quotaNodeBinderCache.GetAllResourceFlavor()))
		})
	}
}
