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
	"k8s.io/klog/v2"
)

// IsSchedSchedStatsSupported checks if the kernel supports the scheduler statistics.
func IsSchedSchedStatsSupported() (bool, string) {
	// kernel supports if sysctl has sched_schedstats
	if FileExists(GetProcSysFilePath(KernelSchedSchedStats)) {
		return true, "sysctl supported"
	}

	return false, "sysctl unsupported"
}

// IsSchedSchedStatsEnable checks if the scheduler statistics feature is enabled in the kernel sched_schedstats.
// The scheduler statistics's kernel feature is known set in `/proc/sys/kernel/sched_schedstats`,
// the value `1` means the feature is enabled while `0` means disabled.
func IsSchedSchedStatsEnabled() (bool, string) {
	isSysctlSupported, err := GetSchedSchedStats()
	if err == nil && isSysctlSupported {
		klog.V(6).Info("scheduler statistics is already enabled by sysctl")
		return true, ""
	}
	if err != nil {
		klog.V(4).Infof("scheduler statistics is not enabled, err: %s", err)
	}
	return false, "scheduler statistics is not enabled"
}

// IsSchedAcpuSupported checks if the kernel supports the acpu statistics.
// The support of "acpu statistics" feature depends on the support of "scheduler statistics".
func IsSchedAcpuSupported() (bool, string) {
	// check if scheduler statistics supported
	schedSchedStatsSupported, msg := IsSchedSchedStatsSupported()
	if !schedSchedStatsSupported {
		klog.V(4).Infof("acpu statistics feature is not supported since scheduler statistics feature not supported, msg: %s", msg)
		return false, "sysctl not supported"
	}

	// kernel supports if sysctl has sched_acpu
	if FileExists(GetProcSysFilePath(KernelSchedAcpu)) {
		return true, "sysctl supported"
	}

	return false, "sysctl not supported"
}

// EnableSchedAcpuIfSupported checks if the acpu statistics feature is enabled in the kernel sched_acpu.
// To enable the "acpu statistics" feature, the "scheduler statistics" feature must also be enabled.
// The "acpu statistics" feature is known set in `/proc/sys/kernel/sched_acpu`,
// the value `1` means the feature is enabled while `0` means disabled.
func IsSchedAcpuEnabled() (bool, string) {
	// 1. check if scheduler statistics enabled
	schedSchedStatsEnabled, msg := IsSchedSchedStatsEnabled()
	if !schedSchedStatsEnabled {
		klog.V(4).Infof("failed to enable acpu statistics since scheduler statistics is not be enabled, err: %s", msg)
		return false, "acpu statistics is not enabled"
	}

	// 2. check if acpu statistics enabled
	isSysctlSupported, err := GetSchedAcpu()
	if err == nil && isSysctlSupported {
		klog.V(6).Info("acpu statistics is already enabled by sysctl")
		return true, ""
	}

	if err != nil {
		klog.V(4).Infof("acpu statistics is not enabled, err: %s", err)

	}

	return false, "acpu statistics is not enabled"
}
