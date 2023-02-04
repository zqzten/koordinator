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

package reservation

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

func CycleStateMatchReservation(cycleState *framework.CycleState, nodeName string) bool {
	v, err := cycleState.Read(preFilterStateKey)
	if err != nil {
		return false
	}
	cache, ok := v.(*stateData)
	if !ok || cache == nil {
		return false
	}
	if cache.skip || cache.matchedCache == nil || cache.matchedCache.nodeToR == nil {
		return false
	}
	return len(cache.matchedCache.GetOnNode(nodeName)) > 0
}

func CycleStateMatchReservationUID(cycleState *framework.CycleState) types.UID {
	v, err := cycleState.Read(preFilterStateKey)
	if err != nil {
		return ""
	}
	cache, ok := v.(*stateData)
	if !ok || cache == nil || cache.assumed == nil {
		return ""
	}

	return cache.assumed.UID
}
