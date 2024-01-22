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
	"os"
	"strings"

	"k8s.io/klog/v2"
)

func init() {
	DefaultRegistry.Add(CgroupVersionV2, knownCgroupV2AnolisResources...)
}

const (
	CPUSchedCfsStatisticsName = "cpu.sched_cfs_statistics"
	CPUHTRatioName            = "cpu.ht_ratio"

	DefaultHTRatio = 100 // also as minimal
	MaxCPUHTRatio  = 200
)

// for cgroup resources, we use the corresponding cgroups-v1 filename as its resource type
var (
	CPUHTRatioValidator = &RangeValidator{min: DefaultHTRatio, max: MaxCPUHTRatio}

	CPUSchedCfsStatisticsV2Anolis = DefaultFactory.NewV2(CPUSchedCfsStatisticsName, CPUSchedCfsStatisticsName).WithCheckSupported(SupportedIfFileExistsInKubepods).WithCheckOnce(true)
	// CPUHTRatioV2Anolis is the extended cgroup resource named `cpu.ht_ratio`.
	// https://ata.atatech.org/articles/11000235287?#PUGTGmEY
	// Currently, it is only supported by Anolis OS on cgroup-v2.
	// NOTE: it only takes effect on the leaf cgroups which has tasks.
	CPUHTRatioV2Anolis = DefaultFactory.NewV2(CPUHTRatioName, CPUHTRatioName).WithValidator(CPUHTRatioValidator).WithCheckSupported(SupportedIfFileExists)

	knownCgroupV2AnolisResources = []Resource{
		CPUSchedCfsStatisticsV2Anolis,
		CPUHTRatioV2Anolis,
	}
)

const (
	// SchedFeatureHTAwareQuota is the feature name of the HT-aware quota in `/sys/kernel/debug/sched_features`.
	SchedFeatureHTAwareQuota = "SCHED_CORE_HT_AWARE_QUOTA"
	// SchedFeatureNoHTAwareQuota is the feature name that the HT-aware quota supported but disabled.
	SchedFeatureNoHTAwareQuota = "NO_SCHED_CORE_HT_AWARE_QUOTA"
)

// EnableHTAwareQuotaIfSupported checks if the HT-aware quota feature is enabled in the kernel sched_features.
// In `/sys/kernel/debug/sched_features`, the field `SCHED__HT_AWARE_QUOTA` means the feature is enabled while
// `NO_SCHED_CORE_HT_AWARE_QUOTA` means it is disabled.
func EnableHTAwareQuotaIfSupported() (bool, string) {
	isSchedFeaturesSuppported, msg := SchedFeatures.IsSupported("")
	if !isSchedFeaturesSuppported { // sched_features not exist
		klog.V(6).Infof("failed to enable ht aware quota via sched_features, feature unsupported, msg: %s", msg)
		return false, "sched_features not supported"
	}
	isSchedFeatureEnabled, err := IsHTAwareQuotaFeatureEnabled()
	if err == nil && isSchedFeatureEnabled {
		klog.V(6).Info("ht aware quota is already enabled by sched_features")
		return true, ""
	}
	if err == nil {
		klog.V(6).Info("ht aware quota is disabled by sched_features, try to enable it")
		isSchedFeatureEnabled, msg = SetHTAwareQuotaFeatureEnabled()
		if isSchedFeatureEnabled {
			klog.Info("ht aware quota is enabled by sched_features successfully")
			return true, ""
		}
		klog.V(4).Infof("failed to enable ht aware quota via sched_features, msg: %s", msg)
	} else {
		klog.V(5).Infof("failed to enable ht aware quota via sched_features, err: %s", err)
	}

	return false, "ht aware quota not supported"
}

func IsHTAwareQuotaFeatureEnabled() (bool, error) {
	featurePath := SchedFeatures.Path("")
	content, err := os.ReadFile(featurePath)
	if err != nil {
		return false, fmt.Errorf("failed to read sched_features, err: %w", err)
	}

	features := strings.Fields(string(content))
	for _, feature := range features {
		if feature == SchedFeatureHTAwareQuota {
			klog.V(6).Infof("ht aware quota is enabled by sched_features")
			return true, nil
		} else if feature == SchedFeatureNoHTAwareQuota {
			klog.V(6).Infof("ht aware quota is disabled by sched_features")
			return false, nil
		}
	}

	return false, fmt.Errorf("ht aware quota not found in sched_features")
}

func SetHTAwareQuotaFeatureEnabled() (bool, string) {
	featurePath := SchedFeatures.Path("")
	content, err := os.ReadFile(featurePath)
	if err != nil {
		klog.V(5).Infof("ht aware quota is unsupported by sched_features %s, read err: %s", featurePath, err)
		return false, fmt.Sprintf("failed to read sched_features")
	}

	features := strings.Fields(string(content))
	for _, feature := range features {
		if feature == SchedFeatureHTAwareQuota {
			return true, ""
		}
	}

	err = os.WriteFile(featurePath, []byte(fmt.Sprintf("%s\n", SchedFeatureHTAwareQuota)), 0666)
	if err != nil {
		klog.V(5).Infof("ht aware quota is unsupported by sched_features %s, write err: %s", featurePath, err)
		return false, fmt.Sprintf("failed to write sched_features")
	}

	return true, ""
}
