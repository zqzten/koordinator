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

package transformer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

func TestGetNodeGPUModel(t *testing.T) {
	type args struct {
		nodeLabels map[string]string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "by GpuModelLabel",
			args: args{
				nodeLabels: map[string]string{
					GpuModelLabel: "model1",
				},
			},
			want: "model1",
		},
		{
			name: "by GpuModelDetailLabel",
			args: args{
				nodeLabels: map[string]string{
					GpuModelLabel:       "model1",
					GpuModelDetailLabel: "model2",
				},
			},
			want: "model2",
		},
		{
			name: "by DeprecatedModel",
			args: args{
				nodeLabels: map[string]string{
					GpuModelLabel:               "model1",
					GpuModelDetailLabel:         "model2",
					DeprecatedLabelNodeGPUModel: "model3",
				},
			},
			want: "model3",
		},
		{
			name: "by koordinator LabelGPUModel",
			args: args{
				nodeLabels: map[string]string{
					GpuModelLabel:               "model1",
					GpuModelDetailLabel:         "model2",
					DeprecatedLabelNodeGPUModel: "model3",
					extension.LabelGPUModel:     "model4",
				},
			},
			want: "model4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, GetNodeGPUModel(tt.args.nodeLabels), "GetNodeGPUModel(%v)", tt.args.nodeLabels)
		})
	}
}
