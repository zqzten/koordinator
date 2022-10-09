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

package unified

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGetResourceRequestByContainerName(t *testing.T) {
	requests := []*ContainerResourceRequest{
		{Name: "container1", ResourceRequest: map[corev1.ResourceName]resource.Quantity{corev1.ResourceCPU: resource.MustParse("1")}},
		{Name: "container2", ResourceRequest: map[corev1.ResourceName]resource.Quantity{corev1.ResourceCPU: resource.MustParse("2")}},
		{Name: "container3", ResourceRequest: map[corev1.ResourceName]resource.Quantity{corev1.ResourceCPU: resource.MustParse("3")}},
	}
	requestsJsonBytes, err := json.Marshal(requests)
	assert.NoError(t, err)

	tests := []struct {
		name          string
		annotations   map[string]string
		containerName string
		want          *ContainerResourceRequest
		wantErr       assert.ErrorAssertionFunc
	}{
		{
			name:          "bat format annotations",
			annotations:   map[string]string{AnnotationContainerResourceRequest: "fds"},
			containerName: "container2",
			want:          nil,
			wantErr:       assert.Error,
		},
		{
			name:          "container exists in annotations",
			annotations:   map[string]string{AnnotationContainerResourceRequest: string(requestsJsonBytes)},
			containerName: "container2",
			want: &ContainerResourceRequest{
				Name:            "container2",
				ResourceRequest: map[corev1.ResourceName]resource.Quantity{corev1.ResourceCPU: resource.MustParse("2")},
			},
			wantErr: assert.NoError,
		},
		{
			name:          "container doesn't exist in annotations",
			annotations:   map[string]string{AnnotationContainerResourceRequest: string(requestsJsonBytes)},
			containerName: "container4",
			want:          nil,
			wantErr:       assert.NoError,
		},
		{
			name:          "container doesn't exist in annotations",
			annotations:   map[string]string{AnnotationContainerResourceRequest: ""},
			containerName: "container4",
			want:          nil,
			wantErr:       assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetResourceRequestByContainerName(tt.annotations, tt.containerName)
			if !tt.wantErr(t, err, fmt.Sprintf("GetResourceRequestByContainerName(%v, %v)", tt.annotations, tt.containerName)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetResourceRequestByContainerName(%v, %v)", tt.annotations, tt.containerName)
		})
	}
}
