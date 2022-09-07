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

package cgroupscrd

import (
	"strconv"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	sysutil "github.com/koordinator-sh/koordinator/pkg/util/system"
)

const (
	CPUBvtLSValue = 2
	CPUBvtBEValue = -1
)

func generateCPUIdentity(podQOS apiext.QoSClass) map[string]string {
	bvtValue := "0"
	if podQOS == apiext.QoSLS {
		bvtValue = strconv.Itoa(CPUBvtLSValue)
	} else if podQOS == apiext.QoSBE {
		bvtValue = strconv.Itoa(CPUBvtBEValue)
	}
	return map[string]string{
		sysutil.CPUBVTWarpNsName: bvtValue,
	}
}
