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

package runtimehooks

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/component-base/featuregate"

	"github.com/koordinator-sh/koordinator/pkg/features"
	unifiedcpushare "github.com/koordinator-sh/koordinator/pkg/koordlet/runtimehooks/hooks/unified/cpushare"
)

const (
	UnifiedCPUShare featuregate.Feature = unifiedcpushare.Name
)

var (
	unifiedRuntimeHooksFG = map[featuregate.Feature]featuregate.FeatureSpec{
		UnifiedCPUShare: {Default: false, PreRelease: featuregate.Alpha},
	}

	unifiedRuntimeHookPlugins = map[featuregate.Feature]HookPlugin{
		UnifiedCPUShare: unifiedcpushare.Object(),
	}
)

func init() {
	runtime.Must(features.DefaultMutableKoordletFeatureGate.Add(unifiedRuntimeHooksFG))
	for k, v := range unifiedRuntimeHookPlugins {
		runtimeHookPlugins[k] = v
	}
}
