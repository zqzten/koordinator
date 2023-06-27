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

package deviceshare

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/deviceshare"
)

func TestValidateDeviceRequest(t *testing.T) {
	tests := []struct {
		name       string
		podRequest corev1.ResourceList
		want       uint
		wantErr    bool
	}{
		{
			name: "valid PPU request 2",
			podRequest: corev1.ResourceList{
				unified.ResourcePPU: resource.MustParse("2"),
			},
			want:    aliyunPPU,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := deviceshare.ValidateDeviceRequest(tt.podRequest)
			assert.Equal(t, tt.wantErr, err != nil)
			if !tt.wantErr {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestConvertDeviceRequest(t *testing.T) {
	type args struct {
		podRequest  corev1.ResourceList
		combination uint
	}
	tests := []struct {
		name string
		args args
		want corev1.ResourceList
	}{
		{
			name: "aliyunPPU",
			args: args{
				podRequest: corev1.ResourceList{
					unified.ResourcePPU: resource.MustParse("2"),
				},
				combination: aliyunPPU,
			},
			want: corev1.ResourceList{
				apiext.ResourceGPUCore:        *resource.NewQuantity(200, resource.DecimalSI),
				apiext.ResourceGPUMemoryRatio: *resource.NewQuantity(200, resource.DecimalSI),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deviceshare.ConvertDeviceRequest(tt.args.podRequest, tt.args.combination)
			assert.Equal(t, tt.want, got)
		})
	}
}
