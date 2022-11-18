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

package features

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"

	utilfeature "github.com/koordinator-sh/koordinator/pkg/util/feature"
)

const (
	DefaultEnableACUForLSPod    featuregate.Feature = "DefaultEnableACUForLSPod"
	ResourceSummaryReport       featuregate.Feature = "ResourceSummaryReport"
	ResourceSummaryReportDryRun featuregate.Feature = "ResourceSummaryReportDryRun"
	LocalDeviceVolume           featuregate.Feature = "LocalDeviceVolume"
	EnableLocalVolumeCapacity   featuregate.Feature = "EnableLocalVolumeCapacity"
	EnableLocalVolumeIOLimit    featuregate.Feature = "EnableLocalVolumeIOLimit"
)

const (
	UnifiedDeviceScheduling featuregate.Feature = "UnifiedDeviceScheduling"
)

var defaultUnifiedFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	DefaultEnableACUForLSPod:    {Default: true, PreRelease: featuregate.Beta},
	ResourceSummaryReport:       {Default: true, PreRelease: featuregate.Beta},
	ResourceSummaryReportDryRun: {Default: false, PreRelease: featuregate.Beta},
}

var defaultUnifiedSchedulerFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	UnifiedDeviceScheduling:   {Default: false, PreRelease: featuregate.Beta},
	LocalDeviceVolume:         {Default: false, PreRelease: featuregate.Beta},
	EnableLocalVolumeCapacity: {Default: true, PreRelease: featuregate.Beta},
	EnableLocalVolumeIOLimit:  {Default: false, PreRelease: featuregate.Beta},
}

func init() {
	runtime.Must(utilfeature.DefaultMutableFeatureGate.Add(defaultUnifiedFeatureGates))
	runtime.Must(k8sfeature.DefaultMutableFeatureGate.Add(defaultUnifiedSchedulerFeatureGates))
}
