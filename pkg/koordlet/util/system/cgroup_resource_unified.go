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

func init() {
	DefaultRegistry.Add(CgroupVersionV2, knownCgroupV2AnolisResources...)
}

const (
	CPUSchedCfsStatisticsName = "cpu.sched_cfs_statistics"
)

// for cgroup resources, we use the corresponding cgroups-v1 filename as its resource type
var (
	CPUSchedCfsStatisticsV2Anolis = DefaultFactory.NewV2(CPUSchedCfsStatisticsName, CPUSchedCfsStatisticsName).WithCheckSupported(SupportedIfFileExistsInKubepods).WithCheckOnce(true)

	knownCgroupV2AnolisResources = []Resource{
		CPUSchedCfsStatisticsV2Anolis,
	}
)
