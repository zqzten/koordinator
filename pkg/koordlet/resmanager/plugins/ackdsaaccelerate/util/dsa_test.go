package dsa

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestManageDsa(t *testing.T) {
	configDsaLock.Lock()
	defer configDsaLock.Unlock()
	nodeReady, err := isNodeSupportDsa()
	assert.NoError(t, err)
	accelReady, err := isAccelConfigReady()
	if err != nil {
		assert.Contains(t, err.Error(), "executable file not found")
	}
	if !nodeReady || !accelReady {
		return
	}
	// run test on instance with DSA
	enabledDsaCache := enabledDsa
	type args struct {
		name      string
		enable    bool
		hasEnable bool
	}
	tests := []args{
		{
			name:      "no need to enable",
			enable:    true,
			hasEnable: true,
		},
		{
			name:      "need to enable",
			enable:    true,
			hasEnable: false,
		},
		{
			name:      "no need to disable",
			enable:    false,
			hasEnable: false,
		},
		{
			name:      "need to disable",
			enable:    false,
			hasEnable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabledDsa = tt.hasEnable
			err := ManageDsa(tt.enable)
			assert.NoError(t, err)
		})
	}
	enabledDsa = enabledDsaCache
}
