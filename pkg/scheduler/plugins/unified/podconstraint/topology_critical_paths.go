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

package podconstraint

import (
	"math"
)

type TopologyCriticalPaths struct {
	Min CriticalPath `json:"min"`
	Max CriticalPath `json:"max"`
}

// CriticalPath traces the minimum matchNum and  matchNum in the Topology
// 1. 需要使用者及时追踪新增的TopologyValue并将其MatchNum更新至0
// 2. 需要使用者及时追踪现有的TopologyValue的MatchNum有变化
// 3. todo 需要使用者确保Pod分配遵循尽量Spread原则
type CriticalPath struct {
	// TopologyValue denotes the topology value mapping to topology key.
	TopologyValue string `json:"topologyValue,omitempty"`
	// MatchNum denotes the number of matching pods.
	MatchNum int `json:"matchNum,omitempty"`
}

func NewTopologyCriticalPaths() *TopologyCriticalPaths {
	return &TopologyCriticalPaths{
		Min: CriticalPath{
			MatchNum: math.MaxInt32,
		},
		Max: CriticalPath{
			MatchNum: math.MaxInt32,
		},
	}
}

func (p *TopologyCriticalPaths) Update(tpVal string, num int) {
	// first verify if `tpVal` exists or not
	var cp *CriticalPath
	if tpVal == p.Min.TopologyValue {
		cp = &p.Min
	} else if tpVal == p.Max.TopologyValue {
		cp = &p.Max
	}
	if cp != nil {
		// `tpVal` exists
		cp.MatchNum = num
		if p.Min.MatchNum > p.Max.MatchNum {
			// swap paths[0] and paths[1]
			p.Min, p.Max = p.Max, p.Min
		}
	} else {
		// `tpVal` doesn't exist
		if num < p.Min.MatchNum {
			// Update paths[1] with paths[0]
			p.Max = p.Min
			// Update paths[0]
			p.Min.TopologyValue, p.Min.MatchNum = tpVal, num
		} else if num < p.Max.MatchNum {
			// Update paths[1]
			p.Max.TopologyValue, p.Max.MatchNum = tpVal, num
		}
	}
}
