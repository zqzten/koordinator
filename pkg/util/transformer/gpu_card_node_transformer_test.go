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
