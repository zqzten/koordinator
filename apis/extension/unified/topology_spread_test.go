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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetMatchLabelKeysInPodTopologySpread(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        []string
		wantErr     assert.ErrorAssertionFunc
	}{
		{
			name:        "normal flow",
			annotations: map[string]string{AnnotationMatchLabelKeysInPodTopologySpread: "[\"po\",\"ce\"]"},
			want:        []string{"po", "ce"},
			wantErr:     assert.NoError,
		},

		{
			name:        "unmarshal err",
			annotations: map[string]string{AnnotationMatchLabelKeysInPodTopologySpread: "po,ce"},
			want:        nil,
			wantErr:     assert.Error,
		},
		{
			name:        "no key",
			annotations: map[string]string{},
			want:        nil,
			wantErr:     assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetMatchLabelKeysInPodTopologySpread(tt.annotations)
			if !tt.wantErr(t, err, fmt.Sprintf("GetMatchLabelKeysInPodTopologySpread(%v)", tt.annotations)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetMatchLabelKeysInPodTopologySpread(%v)", tt.annotations)
		})
	}
}
