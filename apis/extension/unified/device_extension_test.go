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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func TestGetGPUPartitionTableFromDevice(t *testing.T) {
	rawH100PartitionTable, err := json.Marshal(H100PartitionTables)
	assert.NoError(t, err)
	tests := []struct {
		name    string
		device  *schedulingv1alpha1.Device
		want    GPUPartitionTable
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "normal flow",
			device: &schedulingv1alpha1.Device{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationGPUPartitions: string(rawH100PartitionTable),
					},
				},
			},
			want:    H100PartitionTables,
			wantErr: assert.NoError,
		},
		{
			name: "null",
			device: &schedulingv1alpha1.Device{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationGPUPartitions: "null",
					},
				},
			},
			want:    nil,
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetGPUPartitionTableFromDevice(tt.device)
			if !tt.wantErr(t, err, fmt.Sprintf("GetGPUPartitionTableFromDevice(%v)", tt.device)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetGPUPartitionTableFromDevice(%v)", tt.device)
		})
	}
}
