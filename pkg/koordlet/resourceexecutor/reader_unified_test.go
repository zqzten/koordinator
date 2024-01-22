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

package resourceexecutor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sysutil "github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
)

func TestNewCgroupReaderAnolis(t *testing.T) {
	type fields struct {
		UseCgroupsV2 bool
	}
	tests := []struct {
		name   string
		fields fields
		want   CgroupReaderAnolis
	}{
		{
			name: "cgroups-v1 reader",
			fields: fields{
				UseCgroupsV2: false,
			},
			want: &CgroupV1ReaderAnolis{},
		},
		{
			name: "anolis cgroups-v2 reader",
			fields: fields{
				UseCgroupsV2: true,
			},
			want: &CgroupV2ReaderAnolis{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := sysutil.NewFileTestUtil(t)
			defer helper.Cleanup()
			helper.SetCgroupsV2(tt.fields.UseCgroupsV2)

			r := NewCgroupReaderAnolis()
			assert.Equal(t, tt.want, r)
		})
	}
}

func TestCgroupReaderAnolis_ReadCPUStat(t *testing.T) {
	type fields struct {
		UseCgroupsV2         bool
		CPUStatV2AnolisValue string
	}
	type args struct {
		parentDir string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *sysutil.CPUStatAnolisRaw
		wantErr bool
	}{
		{
			name: "v2 path not exist",
			fields: fields{
				UseCgroupsV2: true,
			},
			args: args{
				parentDir: "/kubepods.slice",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "parse v2 value successfully",
			fields: fields{
				UseCgroupsV2: true,
				CPUStatV2AnolisValue: `usage_usec 90000
user_usec 20000
system_usec 30000
sibidle_usec 10000
nr_periods 1
nr_throttled 2
throttled_usec 3`,
			},
			args: args{
				parentDir: "/kubepods.slice",
			},
			want: &sysutil.CPUStatAnolisRaw{
				SibidleUSec:   10000,
				ThrottledUSec: 3,
			},
			wantErr: false,
		},
		{
			name: "parse v2 value failed",
			fields: fields{
				UseCgroupsV2: true,
				CPUStatV2AnolisValue: `usage_usec 90000
user_usec 20000
system_usec 30000`,
			},
			args: args{
				parentDir: "/kubepods.slice",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := sysutil.NewFileTestUtil(t)
			defer helper.Cleanup()
			helper.SetCgroupsV2(tt.fields.UseCgroupsV2)
			if tt.fields.CPUStatV2AnolisValue != "" {
				helper.WriteCgroupFileContents(tt.args.parentDir, sysutil.CPUStatV2, tt.fields.CPUStatV2AnolisValue)
			}

			got, gotErr := NewCgroupReaderAnolis().ReadCPUStat(tt.args.parentDir)
			assert.Equal(t, tt.wantErr, gotErr != nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCgroupReaderAnolis_ReadCPUSchedCfsStatistics(t *testing.T) {
	type fields struct {
		UseCgroupsV2                       bool
		CPUSchedCfsStatisticsV2AnolisValue string
	}
	type args struct {
		parentDir string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *sysutil.CPUSchedCfsStatisticsAnolisRaw
		wantErr bool
	}{
		{
			name: "v2 path not exist",
			fields: fields{
				UseCgroupsV2: true,
			},
			args: args{
				parentDir: "/kubepods.slice",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "parse v2 value successfully",
			fields: fields{
				UseCgroupsV2:                       true,
				CPUSchedCfsStatisticsV2AnolisValue: `10000 4000 3000 2000 5000 1000`,
			},
			args: args{
				parentDir: "/kubepods.slice",
			},
			want: &sysutil.CPUSchedCfsStatisticsAnolisRaw{
				Serve:        10000,
				OnCpu:        4000,
				QueueOther:   3000,
				QueueSibling: 2000,
				QueueMax:     5000,
				ForceIdle:    1000,
			},
			wantErr: false,
		},
		{
			name: "parse v2 value failed",
			fields: fields{
				UseCgroupsV2:                       true,
				CPUSchedCfsStatisticsV2AnolisValue: `10000 4000`,
			},
			args: args{
				parentDir: "/kubepods.slice",
			},
			want:    nil,
			wantErr: true,
		},
	}
	sysutil.CPUSchedCfsStatisticsV2Anolis.WithCheckOnce(false)
	defer func() {
		sysutil.CPUSchedCfsStatisticsV2Anolis.WithCheckOnce(true)
	}()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := sysutil.NewFileTestUtil(t)
			defer helper.Cleanup()
			helper.SetCgroupsV2(tt.fields.UseCgroupsV2)
			if tt.fields.CPUSchedCfsStatisticsV2AnolisValue != "" {
				helper.WriteCgroupFileContents(tt.args.parentDir, sysutil.CPUSchedCfsStatisticsV2Anolis, tt.fields.CPUSchedCfsStatisticsV2AnolisValue)
			}

			got, gotErr := NewCgroupReaderAnolis().ReadCPUSchedCfsStatistics(tt.args.parentDir)
			assert.Equal(t, tt.wantErr, gotErr != nil)
			assert.Equal(t, tt.want, got)
		})
	}
}
