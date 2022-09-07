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

package cgroupscrd

import (
	"reflect"
	"testing"
)

func Test_parseCPUSetAllocResult(t *testing.T) {
	type args struct {
		cpuSetAllocStr string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name: "container-cpu-sets",
			args: args{
				cpuSetAllocStr: "{\"container1\":{\"0\":{\"elems\":{\"0\":{},\"1\":{},\"2\":{},\"3\":{}}}},\"container2\":{\"1\":{\"elems\":{\"26\":{},\"27\":{},\"28\":{},\"29\":{}}}}}",
			},
			want: map[string]string{
				"container1": "0-3",
				"container2": "26-29",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCPUSetAllocResult(tt.args.cpuSetAllocStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCPUSetAllocResult() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseCPUSetAllocResult() got = %v, want %v", got, tt.want)
			}
		})
	}
}
