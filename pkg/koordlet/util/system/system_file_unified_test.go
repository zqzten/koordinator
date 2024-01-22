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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetSchedSchedStats(t *testing.T) {
	t.Run("test", func(t *testing.T) {
		helper := NewFileTestUtil(t)

		// system not supported
		err := SetSchedSchedStats(false)
		assert.Error(t, err)

		// system supported, already disabled
		testProcSysFile := KernelSchedSchedStats
		testProcSysFilepath := filepath.Join(SysctlSubDir, testProcSysFile)
		testContent := "0"
		assert.False(t, FileExists(GetProcSysFilePath(testProcSysFile)))
		helper.WriteProcSubFileContents(testProcSysFilepath, testContent)
		err = SetSchedSchedStats(false)
		assert.NoError(t, err)
		got := helper.ReadProcSubFileContents(testProcSysFilepath)
		assert.Equal(t, got, testContent)

		// system supported, set enabled
		testContent = "1"
		err = SetSchedSchedStats(true)
		assert.NoError(t, err)
		got = helper.ReadProcSubFileContents(testProcSysFilepath)
		assert.Equal(t, got, testContent)
	})
}

func TestSetSchedAcpu(t *testing.T) {
	t.Run("test", func(t *testing.T) {
		helper := NewFileTestUtil(t)

		// system not supported
		err := SetSchedAcpu(false)
		assert.Error(t, err)

		// system supported, already disabled
		testProcSysFile := KernelSchedAcpu
		testProcSysFilepath := filepath.Join(SysctlSubDir, testProcSysFile)
		testContent := "0"
		assert.False(t, FileExists(GetProcSysFilePath(testProcSysFile)))
		helper.WriteProcSubFileContents(testProcSysFilepath, testContent)
		err = SetSchedAcpu(false)
		assert.NoError(t, err)
		got := helper.ReadProcSubFileContents(testProcSysFilepath)
		assert.Equal(t, got, testContent)

		// system supported, set enabled
		testContent = "1"
		err = SetSchedAcpu(true)
		assert.NoError(t, err)
		got = helper.ReadProcSubFileContents(testProcSysFilepath)
		assert.Equal(t, got, testContent)
	})
}
