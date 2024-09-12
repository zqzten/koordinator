package intelligentscheduler

import (
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/intelligentscheduler/CRDs"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"reflect"
	"testing"
)

func Test_unstructuredToVgi(t *testing.T) {
	type args struct {
		utd *unstructured.Unstructured
	}
	tests := []struct {
		name    string
		args    args
		want    *CRDs.VirtualGpuInstance
		wantErr bool
	}{
		{
			name: "testName",
			args: args{
				utd: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"Spec": map[string]interface{}{
							"GPUMemory":               0,
							"VirtualGpuSpecification": "vgs1",
							"GPUUtilization":          1,
							"IsOversell":              false,
						},
					},
				},
			},
			want: &CRDs.VirtualGpuInstance{
				Spec: CRDs.VirtualGpuInstanceSpec{
					GPUMemory:               0,
					VirtualGpuSpecification: "vgs1",
					GPUUtilization:          1,
					IsOversell:              false,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := unstructuredToVgi(tt.args.utd)
			if (err != nil) != tt.wantErr {
				t.Errorf("unstructuredToVgi() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unstructuredToVgi() got = %v, want %v", got, tt.want)
			}
		})
	}
}
