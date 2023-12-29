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
