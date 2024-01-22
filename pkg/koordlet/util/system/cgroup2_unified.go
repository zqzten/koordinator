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
	"fmt"
	"strconv"
	"strings"
)

type CPUStatAnolisRaw struct {
	SibidleUSec int64

	ThrottledUSec int64
}

type CPUSchedCfsStatisticsAnolisRaw struct {
	Serve        int64
	OnCpu        int64
	QueueOther   int64
	QueueSibling int64
	QueueMax     int64
	ForceIdle    int64
}

func ParseCPUStatRawV2Anolis(content string) (*CPUStatAnolisRaw, error) {
	cpuStatRawV2Anolis := &CPUStatAnolisRaw{}

	m := ParseKVMap(content)
	for _, t := range []struct {
		key   string
		value *int64
	}{
		{
			key:   "sibidle_usec",
			value: &cpuStatRawV2Anolis.SibidleUSec,
		},
		{
			key:   "throttled_usec",
			value: &cpuStatRawV2Anolis.ThrottledUSec,
		},
	} {
		valueStr, ok := m[t.key]
		if !ok {
			return nil, fmt.Errorf("parse cpu.stat failed, raw content %s, err: missing field %s", content, t.key)
		}
		v, err := strconv.ParseInt(valueStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse cpu.stat failed, raw content %s, field %s, err: %v", content, t.key, err)
		}
		*t.value = v
	}

	return cpuStatRawV2Anolis, nil
}

func ParseCPUSchedCfsStatisticsV2AnolisRaw(content string) (*CPUSchedCfsStatisticsAnolisRaw, error) {
	cpuSchedCfsStatisticsV2AnolisRaw := &CPUSchedCfsStatisticsAnolisRaw{}
	// content: "[serve] [oncpu] [queueOther] [queueSibling] [queueMax] [forceIdle]"
	ss := strings.Fields(content)

	if len(ss) < 6 {
		return nil, fmt.Errorf("parse cpu.sched_cfs_statistics failed, raw content: %s, err: invalid pattern", content)
	}

	stats := []*int64{
		&cpuSchedCfsStatisticsV2AnolisRaw.Serve,
		&cpuSchedCfsStatisticsV2AnolisRaw.OnCpu,
		&cpuSchedCfsStatisticsV2AnolisRaw.QueueOther,
		&cpuSchedCfsStatisticsV2AnolisRaw.QueueSibling,
		&cpuSchedCfsStatisticsV2AnolisRaw.QueueMax,
		&cpuSchedCfsStatisticsV2AnolisRaw.ForceIdle,
	}

	for i, statPointer := range stats {
		value, err := strconv.ParseInt(ss[i], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse cpu.sched_cfs_statistics failed, content: %s, err: %v", ss[i], err)
		}
		*statPointer = value
	}

	return cpuSchedCfsStatisticsV2AnolisRaw, nil
}
