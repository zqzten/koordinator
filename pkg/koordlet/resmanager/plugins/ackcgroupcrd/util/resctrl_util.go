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
	corev1 "k8s.io/api/core/v1"

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

const (
	PodCacheQOSLow    string = "low"
	PodCacheQOSMiddle string = "middle"
	PodCacheQOSHigh   string = "high"

	DefaultL3PercentLow    = "10"
	DefaultL3PercentMiddle = "30"
	DefaultL3PercentHigh   = "50"

	DefaultMbPercentLow    = "20"
	DefaultMbPercentMiddle = "50"
	DefaultMbPercentHigh   = "100"

	ResctrlGroupPrefix = "l3"
)

func generateLLCInfo(pod *corev1.Pod, config *CgroupsControllerConfig) *resourcesv1alpha1.LLCinfo {
	var podCacheQOS string
	if podCacheQOS = parseCacheQOS(pod); podCacheQOS == "" {
		return nil
	}

	defaultL3Percent, defaultMbPercent := getDefaultL3MbPercent(podCacheQOS)
	llcinfo := &resourcesv1alpha1.LLCinfo{
		LLCPriority: podCacheQOS,
		MBPercent:   defaultMbPercent,
		L3Percent:   defaultL3Percent,
	}

	if config != nil && config.LowL3Percent != "" {
		llcinfo.L3Percent = DefaultL3PercentLow
	}
	if config != nil && config.LowMbPercent != "" {
		llcinfo.MBPercent = DefaultMbPercentLow
	}
	return llcinfo
}

func parseCacheQOS(pod *corev1.Pod) string {
	if pod == nil || pod.Annotations == nil {
		return ""
	}
	if podCacheQOSStr, exist := pod.Annotations[AnnotationKeyCacheQOS]; exist {
		return podCacheQOSStr
	}
	return ""
}

func getDefaultL3MbPercent(podCacheQOS string) (l3Percent string, mbPercent string) {
	switch podCacheQOS {
	case PodCacheQOSHigh:
		l3Percent = DefaultL3PercentHigh
		mbPercent = DefaultMbPercentHigh
	case PodCacheQOSMiddle:
		l3Percent = DefaultL3PercentMiddle
		mbPercent = DefaultMbPercentMiddle
	default:
		l3Percent = DefaultL3PercentLow
		mbPercent = DefaultMbPercentLow
	}
	return
}

func GenResctrlGroup(cacheQOS string) string {
	return ResctrlGroupPrefix + cacheQOS
}
