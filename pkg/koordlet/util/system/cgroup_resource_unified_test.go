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

func TestGetAnolisCgroupResource(t *testing.T) {
	tests := []struct {
		name        string
		arg         ResourceType
		useCgroupV2 bool
		want        Resource
		wantErr     bool
	}{
		{
			name:        "get ht_ratio on v2 successfully",
			arg:         CPUHTRatioName,
			useCgroupV2: true,
			want:        CPUHTRatioV2Anolis,
			wantErr:     false,
		},
		{
			name:        "failed to get ht_ratio on v1",
			arg:         CPUHTRatioName,
			useCgroupV2: false,
			want:        nil,
			wantErr:     true,
		},
		{
			name:        "get cpu.sched_cfs_statistics on v2 successfully",
			arg:         CPUSchedCfsStatisticsName,
			useCgroupV2: true,
			want:        CPUSchedCfsStatisticsV2Anolis,
			wantErr:     false,
		},
		{
			name:        "failed to get cpu.sched_cfs_statistics on v1",
			arg:         CPUSchedCfsStatisticsName,
			useCgroupV2: false,
			want:        nil,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := NewFileTestUtil(t)
			defer helper.Cleanup()
			oldUseCgroupV2 := UseCgroupsV2.Load()
			defer UseCgroupsV2.Store(oldUseCgroupV2)
			helper.SetCgroupsV2(tt.useCgroupV2)
			got, gotErr := GetCgroupResource(tt.arg)
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEnableHTAwareQuotaIfSupported(t *testing.T) {
	type fields struct {
		prepareFn func(helper *FileTestUtil)
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
		want1  string
		wantFn func(t *testing.T, helper *FileTestUtil)
	}{
		{
			name:  "unsupported since no sched features file",
			want:  false,
			want1: "sched_features not supported",
		},
		{
			name: "unsupported when sched features content is unexpected",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					featuresPath := SchedFeatures.Path("")
					helper.WriteFileContents(featuresPath, ``)
				},
			},
			want:  false,
			want1: "ht aware quota not supported",
			wantFn: func(t *testing.T, helper *FileTestUtil) {
				featuresPath := SchedFeatures.Path("")
				got := helper.ReadFileContents(featuresPath)
				assert.Equal(t, ``, got)
			},
		},
		{
			name: "unsupported when sched features content has no ht aware quota",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					featuresPath := SchedFeatures.Path("")
					helper.WriteFileContents(featuresPath, `FEATURE_A FEATURE_B FEATURE_C`)
				},
			},
			want:  false,
			want1: "ht aware quota not supported",
			wantFn: func(t *testing.T, helper *FileTestUtil) {
				featuresPath := SchedFeatures.Path("")
				got := helper.ReadFileContents(featuresPath)
				assert.Equal(t, `FEATURE_A FEATURE_B FEATURE_C`, got)
			},
		},
		{
			name: "supported when ht aware quota shows in the features",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					featuresPath := SchedFeatures.Path("")
					helper.WriteFileContents(featuresPath, `FEATURE_A FEATURE_B FEATURE_C SCHED_CORE_HT_AWARE_QUOTA`)
				},
			},
			want:  true,
			want1: "",
			wantFn: func(t *testing.T, helper *FileTestUtil) {
				featuresPath := SchedFeatures.Path("")
				got := helper.ReadFileContents(featuresPath)
				assert.Equal(t, `FEATURE_A FEATURE_B FEATURE_C SCHED_CORE_HT_AWARE_QUOTA`, got)
			},
		},
		{
			name: "supported when core sched shows in the features 1",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					featuresPath := SchedFeatures.Path("")
					helper.WriteFileContents(featuresPath, `FEATURE_A SCHED_CORE_HT_AWARE_QUOTA FEATURE_B`)
				},
			},
			want:  true,
			want1: "",
			wantFn: func(t *testing.T, helper *FileTestUtil) {
				featuresPath := SchedFeatures.Path("")
				got := helper.ReadFileContents(featuresPath)
				assert.Equal(t, `FEATURE_A SCHED_CORE_HT_AWARE_QUOTA FEATURE_B`, got)
			},
		},
		{
			name: "supported when sched_features disabled but can be enabled",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					featuresPath := SchedFeatures.Path("")
					helper.WriteFileContents(featuresPath, `FEATURE_A FEATURE_B NO_SCHED_CORE_HT_AWARE_QUOTA`)
				},
			},
			want:  true,
			want1: "",
			wantFn: func(t *testing.T, helper *FileTestUtil) {
				featuresPath := SchedFeatures.Path("")
				got := helper.ReadFileContents(featuresPath)
				// content in the real interface should be "FEATURE_A FEATURE_B SCHED_CORE_HT_AWARE_QUOTA"
				assert.Equal(t, "SCHED_CORE_HT_AWARE_QUOTA\n", got)
			},
		},
		{
			name: "supported when sched_features disabled but can be enabled 1",
			fields: fields{
				prepareFn: func(helper *FileTestUtil) {
					featuresPath := SchedFeatures.Path("")
					helper.WriteFileContents(featuresPath, `FEATURE_A NO_SCHED_CORE_HT_AWARE_QUOTA FEATURE_B`)
				},
			},
			want:  true,
			want1: "",
			wantFn: func(t *testing.T, helper *FileTestUtil) {
				featuresPath := SchedFeatures.Path("")
				got := helper.ReadFileContents(featuresPath)
				// content in the real interface should be "FEATURE_A SCHED_CORE_HT_AWARE_QUOTA FEATURE_B"
				assert.Equal(t, "SCHED_CORE_HT_AWARE_QUOTA\n", got)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := NewFileTestUtil(t)
			defer helper.Cleanup()
			if tt.fields.prepareFn != nil {
				tt.fields.prepareFn(helper)
			}
			got, got1 := EnableHTAwareQuotaIfSupported()
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want1, got1)
			if tt.wantFn != nil {
				tt.wantFn(t, helper)
			}
		})
	}
}
