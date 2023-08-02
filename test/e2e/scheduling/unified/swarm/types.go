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

package swarm

type GlobalRules struct {
	UpdateTime        string
	CPUAllocatePolicy *CPUAllocatePolicy `json:"cpuAllocatePolicy,omitempty"`
}

// CPUAllocatePolicy configures the policy of CPU Allocate
type CPUAllocatePolicy struct {
	CPUMutexConstraint *CPUMutexConstraint `json:"cpuMutexConstraint,omitempty"`
}

// CPUMutexConstraint configures the CPUSet Mutex
type CPUMutexConstraint struct {
	Apps         []string `json:"apps,omitempty"`
	ServiceUnits []string `json:"serviceUnits,omitempty"`
}
