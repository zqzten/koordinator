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
	"errors"
	"fmt"

	sysutil "github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
)

type CgroupReaderAnolis interface {
	ReadCPUStat(parentDir string) (*sysutil.CPUStatAnolisRaw, error)
	ReadCPUSchedCfsStatistics(parentDir string) (*sysutil.CPUSchedCfsStatisticsAnolisRaw, error)
	ReadCPUHTRatio(parentDir string) (int64, error)
}

var _ CgroupReaderAnolis = (*CgroupV1ReaderAnolis)(nil)

type CgroupV1ReaderAnolis struct{}

func (r *CgroupV1ReaderAnolis) ReadCPUHTRatio(parentDir string) (int64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (r *CgroupV1ReaderAnolis) ReadCPUStat(parentDir string) (*sysutil.CPUStatAnolisRaw, error) {
	return nil, errors.New("not implemented")
}

func (r *CgroupV1ReaderAnolis) ReadCPUSchedCfsStatistics(parentDir string) (*sysutil.CPUSchedCfsStatisticsAnolisRaw, error) {
	return nil, errors.New("not implemented")
}

var _ CgroupReaderAnolis = (*CgroupV2ReaderAnolis)(nil)

type CgroupV2ReaderAnolis struct{}

func (r *CgroupV2ReaderAnolis) ReadCPUStat(parentDir string) (*sysutil.CPUStatAnolisRaw, error) {
	resource, ok := sysutil.DefaultRegistry.Get(sysutil.CgroupVersionV2, sysutil.CPUStatName)
	if !ok {
		return nil, ErrResourceNotRegistered
	}
	s, err := cgroupFileRead(parentDir, resource)
	if err != nil {
		return nil, err
	}
	// content: "...\nsibidle_usec 0\n...\nthrottled_usec 0\n..."
	v, err := sysutil.ParseCPUStatRawV2Anolis(s)
	if err != nil {
		return nil, fmt.Errorf("cannot parse cgroup value %s, err: %v", s, err)
	}
	return v, nil
}

func (r *CgroupV2ReaderAnolis) ReadCPUSchedCfsStatistics(parentDir string) (*sysutil.CPUSchedCfsStatisticsAnolisRaw, error) {
	resource, ok := sysutil.DefaultRegistry.Get(sysutil.CgroupVersionV2, sysutil.CPUSchedCfsStatisticsName)
	if !ok {
		return nil, ErrResourceNotRegistered
	}
	s, err := cgroupFileRead(parentDir, resource)
	if err != nil {
		return nil, err
	}
	// content: "[serve] [oncpu] [queueOther] [queueSibling] [queueMax] [forceIdle]"
	v, err := sysutil.ParseCPUSchedCfsStatisticsV2AnolisRaw(s)
	if err != nil {
		return nil, fmt.Errorf("cannot parse cgroup value %s, err: %v", s, err)
	}
	return v, nil
}

func (r *CgroupV2ReaderAnolis) ReadCPUHTRatio(parentDir string) (int64, error) {
	resource, ok := sysutil.DefaultRegistry.Get(sysutil.CgroupVersionV2, sysutil.CPUHTRatioName)
	if !ok {
		return -1, ErrResourceNotRegistered
	}
	return readCgroupAndParseInt64(parentDir, resource)
}

func NewCgroupReaderAnolis() CgroupReaderAnolis {
	if sysutil.GetCurrentCgroupVersion() == sysutil.CgroupVersionV2 {
		return &CgroupV2ReaderAnolis{}
	}
	return &CgroupV1ReaderAnolis{}
}
