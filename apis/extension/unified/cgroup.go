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

package unified

import (
	"encoding/json"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	AnnotationCustomCgroup = extension.DomainPrefix + "customCgroups"

	// Taken from lmctfy https://github.com/google/lmctfy/blob/master/lmctfy/controllers/cpu_controller.cc
	minShares     = 2
	sharesPerCPU  = 1024
	milliCPUToCPU = 1000
)

type CustomCgroup struct {
	ContainerCgroups []*ContainerCgroup `json:"containerCgroups,omitempty"`
}

type ContainerCgroup struct {
	Name      string `json:"name,omitempty"`
	CPUShares *int64 `json:"cpuShares,omitempty"`
}

func GetCustomCgroup(annotations map[string]string) (*CustomCgroup, error) {
	if annotations == nil {
		return nil, nil
	}
	rawCustomCgroup, ok := annotations[AnnotationCustomCgroup]
	if !ok || len(rawCustomCgroup) == 0 {
		return nil, nil
	}
	var customCgroup CustomCgroup
	if err := json.Unmarshal([]byte(rawCustomCgroup), &customCgroup); err != nil {
		return nil, err
	}
	return &customCgroup, nil
}

func GetCustomCgroupByContainerName(annotations map[string]string, containerName string) (*ContainerCgroup, error) {
	customCgroup, err := GetCustomCgroup(annotations)
	if err != nil {
		return nil, err
	}
	if customCgroup == nil {
		return nil, nil
	}
	for _, containerCgroup := range customCgroup.ContainerCgroups {
		if containerCgroup.Name == containerName {
			return containerCgroup, nil
		}
	}
	return nil, nil
}

// MilliCPUToShares converts milliCPU to CPU shares
func MilliCPUToShares(milliCPU int64) int64 {
	if milliCPU == 0 {
		// Return 2 here to really match kernel default for zero milliCPU.
		return minShares
	}
	// Conceptually (milliCPU / milliCPUToCPU) * sharesPerCPU, but factored to improve rounding.
	shares := (milliCPU * sharesPerCPU) / milliCPUToCPU
	if shares < minShares {
		return minShares
	}
	return shares
}
