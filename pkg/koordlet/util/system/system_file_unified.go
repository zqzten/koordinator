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

	"k8s.io/klog/v2"
)

const (
	KernelSchedSchedStats = "kernel/sched_schedstats"
	KernelSchedAcpu       = "kernel/sched_acpu"
)

func GetSchedSchedStats() (bool, error) {
	s := NewProcSysctl()
	// 0: disabled; 1: enabled
	cur, err := s.GetSysctl(KernelSchedSchedStats)
	if err != nil {
		return false, fmt.Errorf("cannot get sysctl sched schedstats, err: %w", err)
	}
	return cur == 1, nil
}

func SetSchedSchedStats(enable bool) error {
	s := NewProcSysctl()
	cur, err := s.GetSysctl(KernelSchedSchedStats)
	if err != nil {
		return fmt.Errorf("cannot get sysctl sched schedstats, err: %w", err)
	}
	v := 0 // 0: disabled; 1: enabled
	if enable {
		v = 1
	}
	if cur == v {
		klog.V(6).Infof("SetSchedSchedStats skips since current sysctl config is already %v", enable)
		return nil
	}

	err = s.SetSysctl(KernelSchedSchedStats, v)
	if err != nil {
		return fmt.Errorf("cannot set sysctl sched schedstats, err: %w", err)
	}
	klog.V(4).Infof("SetSchedSchedStats set sysctl config successfully, value %v", v)
	return nil
}

func GetSchedAcpu() (bool, error) {
	s := NewProcSysctl()
	// 0: disabled; 1: enabled
	cur, err := s.GetSysctl(KernelSchedAcpu)
	if err != nil {
		return false, fmt.Errorf("cannot get sysctl sched acpu, err: %w", err)
	}
	return cur == 1, nil
}

func SetSchedAcpu(enable bool) error {
	s := NewProcSysctl()
	cur, err := s.GetSysctl(KernelSchedAcpu)
	if err != nil {
		return fmt.Errorf("cannot get sysctl sched acpu, err: %w", err)
	}
	v := 0 // 0: disabled; 1: enabled
	if enable {
		v = 1
	}
	if cur == v {
		klog.V(6).Infof("SetSchedAcpu skips since current sysctl config is already %v", enable)
		return nil
	}

	err = s.SetSysctl(KernelSchedAcpu, v)
	if err != nil {
		return fmt.Errorf("cannot set sysctl sched acpu, err: %w", err)
	}
	klog.V(4).Infof("SetSchedAcpu set sysctl config successfully, value %v", v)
	return nil
}
