/*
Copyright 2023 The Koordinator Authors.

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

package logicalresourcenode

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetObjectListNames(t *testing.T) {
	testCases := []struct {
		listObj       interface{}
		expectedNames []string
	}{
		{
			listObj: []*corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "n2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
			},
			expectedNames: []string{"n2", "n1"},
		},
		{
			listObj: []corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "p1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "p2"}},
			},
			expectedNames: []string{"p1", "p2"},
		},
	}

	for _, tc := range testCases {
		got := getObjectListNames(tc.listObj)
		if !reflect.DeepEqual(got, tc.expectedNames) {
			t.Fatalf("expected %v, got %v", tc.expectedNames, got)
		}
	}
}
