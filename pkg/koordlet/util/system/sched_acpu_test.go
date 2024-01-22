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

package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSchedSchedStatsSupported(t *testing.T) {
	type fields struct {
		prepareFn func(helper *FileTestUtil)
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
		want1  string
	}{
		{
			name:  "unsupported since no scheduler statistics features file",
			want:  false,
			want1: "sysctl unsupported",
		},
		{
			name: "supported when scheduler statistics shows in the sysctl",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					sysctlFeaturePath := GetProcSysFilePath(KernelSchedSchedStats)
					helper.WriteFileContents(sysctlFeaturePath, "1\n")
				},
			},
			want:  true,
			want1: "sysctl supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := NewFileTestUtil(t)
			defer helper.Cleanup()
			if tt.fields.prepareFn != nil {
				tt.fields.prepareFn(helper)
			}
			got, got1 := IsSchedSchedStatsSupported()
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want1, got1)
		})
	}
}

func TestIsSchedSchedStatsEnable(t *testing.T) {
	type fields struct {
		prepareFn func(helper *FileTestUtil)
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
		want1  string
	}{
		{
			name:  "not enabled since no scheduler statistics features file",
			want:  false,
			want1: "scheduler statistics is not enabled",
		},
		{
			name: "not enabled when scheduler statistics disabled",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					sysctlFeaturePath := GetProcSysFilePath(KernelSchedSchedStats)
					helper.WriteFileContents(sysctlFeaturePath, "0\n")
				},
			},
			want:  false,
			want1: "scheduler statistics is not enabled",
		},
		{
			name: "enabled when scheduler statistics enabled",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					sysctlFeaturePath := GetProcSysFilePath(KernelSchedSchedStats)
					helper.WriteFileContents(sysctlFeaturePath, "1\n")
				},
			},
			want:  true,
			want1: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := NewFileTestUtil(t)
			defer helper.Cleanup()
			if tt.fields.prepareFn != nil {
				tt.fields.prepareFn(helper)
			}
			got, got1 := IsSchedSchedStatsEnabled()
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want1, got1)
		})
	}
}

func TestIsSchedAcpuSupported(t *testing.T) {
	type fields struct {
		prepareFn func(helper *FileTestUtil)
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
		want1  string
	}{
		{
			name:  "unsupported since no acpu statistics features file and scheduler statistics features file",
			want:  false,
			want1: "sysctl not supported",
		},
		{
			name: "unsupported since acpu statistics enabled but no scheduler statistics features file",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					sysctlFeaturePath := GetProcSysFilePath(KernelSchedAcpu)
					helper.WriteFileContents(sysctlFeaturePath, "1\n")
				},
			},
			want:  false,
			want1: "sysctl not supported",
		},
		{
			name: "supported when acpu statistics  features file and acpu statistics features file exist",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					sysctlFeaturePath := GetProcSysFilePath(KernelSchedSchedStats)
					helper.WriteFileContents(sysctlFeaturePath, "1\n")
					sysctlFeaturePath = GetProcSysFilePath(KernelSchedAcpu)
					helper.WriteFileContents(sysctlFeaturePath, "1\n")
				},
			},
			want:  true,
			want1: "sysctl supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := NewFileTestUtil(t)
			defer helper.Cleanup()
			if tt.fields.prepareFn != nil {
				tt.fields.prepareFn(helper)
			}
			got, got1 := IsSchedAcpuSupported()
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want1, got1)
		})
	}
}

func TestIsSchedAcpuEnabled(t *testing.T) {
	type fields struct {
		prepareFn func(helper *FileTestUtil)
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
		want1  string
	}{
		{
			name:  "not enabled since no acpu statistics features file and scheduler statistics features file",
			want:  false,
			want1: "acpu statistics is not enabled",
		},
		{
			name: "not enabled since acpu statistics enabled but no scheduler statistics features file",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					sysctlFeaturePath := GetProcSysFilePath(KernelSchedAcpu)
					helper.WriteFileContents(sysctlFeaturePath, "1\n")
				},
			},
			want:  false,
			want1: "acpu statistics is not enabled",
		},
		{
			name: "not enabled since acpu statistics enabled but no acpu statistics features file",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					sysctlFeaturePath := GetProcSysFilePath(KernelSchedSchedStats)
					helper.WriteFileContents(sysctlFeaturePath, "1\n")
				},
			},
			want:  false,
			want1: "acpu statistics is not enabled",
		},
		{
			name: "not enabled when acpu statistics disabled",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					sysctlFeaturePath := GetProcSysFilePath(KernelSchedSchedStats)
					helper.WriteFileContents(sysctlFeaturePath, "1\n")
					sysctlFeaturePath = GetProcSysFilePath(KernelSchedAcpu)
					helper.WriteFileContents(sysctlFeaturePath, "0\n")
				},
			},
			want:  false,
			want1: "acpu statistics is not enabled",
		},
		{
			name: "enabled when acpu statistics enabled",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					sysctlFeaturePath := GetProcSysFilePath(KernelSchedSchedStats)
					helper.WriteFileContents(sysctlFeaturePath, "1\n")
					sysctlFeaturePath = GetProcSysFilePath(KernelSchedAcpu)
					helper.WriteFileContents(sysctlFeaturePath, "1\n")
				},
			},
			want:  true,
			want1: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := NewFileTestUtil(t)
			defer helper.Cleanup()
			if tt.fields.prepareFn != nil {
				tt.fields.prepareFn(helper)
			}
			got, got1 := IsSchedAcpuEnabled()
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want1, got1)
		})
	}
}
