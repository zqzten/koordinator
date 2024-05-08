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

package v1beta3

import (
	"k8s.io/utils/pointer"
)

var (
	defaultNetwork = "tcp"
	defaultAddress = ":10261"

	defaultEnableDefaultPodConstraint = pointer.Bool(false)
)

func SetDefaults_UnifiedPodConstraintArgs(obj *UnifiedPodConstraintArgs) {
	if obj.EnableDefaultPodConstraint == nil {
		obj.EnableDefaultPodConstraint = defaultEnableDefaultPodConstraint
	}
}

// SetDefaults_CachedPodArgs sets the default parameters for CachedPod plugin.
func SetDefaults_CachedPodArgs(obj *CachedPodArgs) {
	if obj.Network == "" {
		obj.Network = defaultNetwork
	}
	if obj.Address == "" {
		obj.Address = defaultAddress
	}
}

func SetDefaults_LimitAwareArgs(obj *LimitAwareArgs) {
	if len(obj.ScoringResourceWeights) == 0 {
		obj.ScoringResourceWeights = defaultResourceWeights
	}
	for resourceName, weight := range obj.ScoringResourceWeights {
		if weight == 0 {
			obj.ScoringResourceWeights[resourceName] = 1
		}
	}
}

func SetDefaults_ASIQuotaAdaptorArgs(obj *ASIQuotaAdaptorArgs) {
	if obj.PriorityRangeConfig == nil {
		obj.PriorityRangeConfig = map[string]*PriorityRangeConfig{
			"unified-prod": {
				PriorityStart:     9500,
				PriorityEnd:       9599,
				EnablePreempt:     true,
				EnableBePreempted: false,
			},
			"unified-prod-low": {
				PriorityStart:          9300,
				PriorityEnd:            9399,
				EnablePreempt:          true,
				EnableBePreempted:      true,
				EnableCrossBandPreempt: true,
			},
			"unified-mid": {
				PriorityStart:     7000,
				PriorityEnd:       7099,
				EnablePreempt:     true,
				EnableBePreempted: true,
			},
			"unified-batch-high": {
				PriorityStart:     6700,
				PriorityEnd:       6799,
				EnablePreempt:     true,
				EnableBePreempted: true,
			},
			"unified-batch": {
				PriorityStart:     6500,
				PriorityEnd:       6599,
				EnablePreempt:     true,
				EnableBePreempted: true,
			},
			"unified-batch-low": {
				PriorityStart:     6300,
				PriorityEnd:       6399,
				EnablePreempt:     false,
				EnableBePreempted: true,
			},
			"unified-be": {
				PriorityStart:     3500,
				PriorityEnd:       3599,
				EnablePreempt:     false,
				EnableBePreempted: true,
			},
		}
	}
}
