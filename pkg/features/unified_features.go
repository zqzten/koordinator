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
	// RecommenderControl determines whether the recommender is enabled.
	// DEPRECATED: This feature-gate will be removed. Please use the command line argument `--controllers` instead.
	RecommenderControl featuregate.Feature = "RecommenderControl"
)

const (
	UnifiedDeviceScheduling                     featuregate.Feature = "UnifiedDeviceScheduling"
	LocalDeviceVolume                           featuregate.Feature = "LocalDeviceVolume"
	EnableLocalVolumeCapacity                   featuregate.Feature = "EnableLocalVolumeCapacity"
	EnableLocalVolumeIOLimit                    featuregate.Feature = "EnableLocalVolumeIOLimit"
	EnableQuotaPathCapacity                     featuregate.Feature = "EnableQuotaPathCapacity"
	EnableDefaultECIProfile                     featuregate.Feature = "EnableDefaultECIProfile"
	EnableEnhanceECIAffinity                    featuregate.Feature = "EnableEnhanceECIAffinity"
	EnableNodeInclusionPolicyInPodConstraint    featuregate.Feature = "EnableNodeInclusionPolicyInPodConstraint"
	DefaultHonorTaintTolerationInPodConstraint  featuregate.Feature = "DefaultHonorTaintTolerationInPodConstraint"
	DefaultHonorTaintTolerationInTopologySpread featuregate.Feature = "DefaultHonorTaintTolerationInTopologySpread"
	EnableMatchLabelKeysInPodTopologySpread     featuregate.Feature = "EnableMatchLabelKeysInPodTopologySpread"
	EnableResourceFlavor                        featuregate.Feature = "EnableResourceFlavor"
	RejectQuotaNotExist                         featuregate.Feature = "RejectQuotaNotExist"
	QuotaRunTime                                featuregate.Feature = "QuotaRunTime"
	SyncTaskQuota                               featuregate.Feature = "SyncTaskQuota"
	DisableCPUSetOversold                       featuregate.Feature = "DisableCPUSetOversold"
	EnableACKGPUShareScheduling                 featuregate.Feature = "EnableACKGPUShareScheduling"
	DisableInterPodAffinityByNamespaces         featuregate.Feature = "DisableInterPodAffinityByNamespaces"
	EnableHugePageAsMemory                      featuregate.Feature = "EnableHugePageAsMemory"
	EnableACSDefaultSpread                      featuregate.Feature = "EnableACSDefaultSpread"
	EnableTransformerDeviceTopology             featuregate.Feature = "EnableTransformerDeviceTopology"
	EnableDefaultDeviceAllocateHint             featuregate.Feature = "EnableDefaultDeviceAllocateHint"
)

const (
	// LRNReport report prometheus metrics info about the LogicalNodeResource object.
	// owner: @saintube
	// alpha: v1.2
	//
	LRNReport featuregate.Feature = "LRNReport"

	// DeadlineEvict evict BE pods once the duration reach the deadline
	// owner: @zwzhang0107
	// alpha: v1.3
	DeadlineEvict featuregate.Feature = "DeadlineEvict"

	// CPUSatisfactionCollector enable pod cpu satisfaction collector
	// owner: @shizun.zx
	// alpha: v1.4
	CPUSatisfactionCollector featuregate.Feature = "CPUSatisfactionCollector"

	// ACSSystemConfig set kernel features in /proc/sys/*
	// owner: @shizun.zx
	// alpha: v1.4
	ACSSystemConfig featuregate.Feature = "ACSSystemConfig"
)

var defaultUnifiedFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	DefaultEnableACUForLSPod:    {Default: true, PreRelease: featuregate.Beta},
	ResourceSummaryReport:       {Default: false, PreRelease: featuregate.Beta},
	ResourceSummaryReportDryRun: {Default: false, PreRelease: featuregate.Beta},
	RecommenderControl:          {Default: false, PreRelease: featuregate.Deprecated},
	EnableResourceFlavor:        {Default: false, PreRelease: featuregate.Beta},
}

var defaultUnifiedSchedulerFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	UnifiedDeviceScheduling:                     {Default: false, PreRelease: featuregate.Beta},
	LocalDeviceVolume:                           {Default: false, PreRelease: featuregate.Beta},
	EnableLocalVolumeCapacity:                   {Default: true, PreRelease: featuregate.Beta},
	EnableLocalVolumeIOLimit:                    {Default: false, PreRelease: featuregate.Beta},
	EnableQuotaPathCapacity:                     {Default: true, PreRelease: featuregate.Beta},
	EnableDefaultECIProfile:                     {Default: false, PreRelease: featuregate.Beta},
	EnableEnhanceECIAffinity:                    {Default: true, PreRelease: featuregate.Beta},
	EnableNodeInclusionPolicyInPodConstraint:    {Default: true, PreRelease: featuregate.Beta},
	DefaultHonorTaintTolerationInPodConstraint:  {Default: false, PreRelease: featuregate.Beta},
	DefaultHonorTaintTolerationInTopologySpread: {Default: false, PreRelease: featuregate.Beta},
	EnableMatchLabelKeysInPodTopologySpread:     {Default: true, PreRelease: featuregate.Beta},
	RejectQuotaNotExist:                         {Default: false, PreRelease: featuregate.Beta},
	QuotaRunTime:                                {Default: false, PreRelease: featuregate.Beta},
	SyncTaskQuota:                               {Default: false, PreRelease: featuregate.Beta},
	DisableCPUSetOversold:                       {Default: true, PreRelease: featuregate.Beta},
	EnableACKGPUShareScheduling:                 {Default: false, PreRelease: featuregate.Beta},
	DisableInterPodAffinityByNamespaces:         {Default: false, PreRelease: featuregate.Beta},
	EnableHugePageAsMemory:                      {Default: false, PreRelease: featuregate.Beta},
	EnableACSDefaultSpread:                      {Default: false, PreRelease: featuregate.Beta},
	EnableTransformerDeviceTopology:             {Default: true, PreRelease: featuregate.Beta},
	EnableDefaultDeviceAllocateHint:             {Default: false, PreRelease: featuregate.Beta},
}

var defaultUnifiedKoordletFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	LRNReport:                {Default: false, PreRelease: featuregate.Alpha},
	DeadlineEvict:            {Default: false, PreRelease: featuregate.Alpha},
	CPUSatisfactionCollector: {Default: false, PreRelease: featuregate.Alpha},
	ACSSystemConfig:          {Default: false, PreRelease: featuregate.Alpha},
}

func init() {
	runtime.Must(utilfeature.DefaultMutableFeatureGate.Add(defaultUnifiedFeatureGates))
	runtime.Must(k8sfeature.DefaultMutableFeatureGate.Add(defaultUnifiedSchedulerFeatureGates))
	runtime.Must(DefaultMutableKoordletFeatureGate.Add(defaultUnifiedKoordletFeatureGates))
}
