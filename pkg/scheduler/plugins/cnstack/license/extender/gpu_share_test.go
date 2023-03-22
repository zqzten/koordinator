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

package extender

import (
	"testing"
)

func Test_isLicenseValid(t *testing.T) {
	type args struct {
		str string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "OnTrial",
			args: args{
				str: `{"status":"OnTrial","expireTime":"2023-06-15T02:16:02Z","info":{"instances":{"harmonycloud.cn_postgresql":"1"}}}`,
			},
			want: true,
		}, {
			name: "Valid",
			args: args{
				str: `{"status":"Valid","expireTime":"2023-06-15T02:16:02Z","info":{"instances":{"harmonycloud.cn_postgresql":"1"},"gpuShareCard":"123"}}`,
			},
			want: true,
		}, {
			name: "Valid, count is zero",
			args: args{
				str: `{"status":"Valid","expireTime":"2023-06-15T02:16:02Z","info":{"instances":{"harmonycloud.cn_postgresql":"1"},"gpuShareCard":"0"}}`,
			},
			want: false,
		}, {
			name: "inValid",
			args: args{
				str: `{"status":"Invalid","expireTime":"2023-06-15T02:16:02Z","info":{"instances":{"harmonycloud.cn_postgresql":"1"},"gpuShareCard":"123"}}`,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLicenseValid([]byte(tt.args.str)); got != tt.want {
				t.Errorf("isLicenseValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
