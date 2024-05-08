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

package sloconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func TestRegisterDefaultExtensionsMap(t *testing.T) {
	// Test that a new extension can be registered in the default extensions map
	extKey := "test-key"
	extCfg := "test-config"
	RegisterDefaultExtensionsMap(extKey, extCfg)
	defaultExtensions := getDefaultExtensionsMap()
	assert.NotNil(t, defaultExtensions)
	assert.Equal(t, len(defaultExtensions.Object), 4)
	assert.Equal(t, defaultExtensions.Object[extKey], extCfg)

	deadlineEvictStrategy, ok := defaultExtensions.Object[unified.DeadlineEvictExtKey].(*unified.DeadlineEvictStrategy)
	assert.True(t, ok)
	assert.NotNil(t, deadlineEvictStrategy)
	assert.False(t, *deadlineEvictStrategy.Enable)
	assert.NotNil(t, deadlineEvictStrategy.DeadlineEvictConfig)
	assert.Equal(t, deadlineEvictStrategy.DeadlineEvictConfig.DeadlineDuration.Duration, time.Hour*24)

	acsSystemStrategy, ok := defaultExtensions.Object[unified.ACSSystemExtKey].(*unified.ACSSystemStrategy)
	assert.True(t, ok)
	assert.NotNil(t, acsSystemStrategy)
	assert.False(t, *acsSystemStrategy.Enable)
	assert.Equal(t, *DefaultACSSystemStrategy().SchedSchedStats, *acsSystemStrategy.SchedSchedStats)
	assert.Equal(t, *DefaultACSSystemStrategy().SchedAcpu, *acsSystemStrategy.SchedAcpu)

	cpuStableStrategy, ok := defautExtensions.Object[unified.CPUStableExtKey].(*unified.CPUStableStrategy)
	assert.True(t, ok)
	assert.NotNil(t, cpuStableStrategy)
	assert.Equal(t, unified.CPUStablePolicyIgnore, *cpuStableStrategy.Policy)
	assert.Equal(t, DefaultCPUStableStrategy(), cpuStableStrategy)
}
