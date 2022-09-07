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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"

	uniext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

const (
	// old annotation key of cgroup controller
	AnnotationKeyCPUSet   = "alibabacloud.com/cpuset"
	AnnotationKeyCPUAcct  = "alibabacloud.com/cpuacct"
	AnnotationKeyCacheQOS = "alibabacloud.com/cache-qos"

	ConfigDefaultCpuSetKey  = "default.cpuset"
	ConfigDefaultCpuAcctKey = "default.cpuacct"
)

type CgroupsControllerConfig struct {
	CPUSet            map[string]string
	CPUAcct           map[string]string
	QOSClass          string `json:"default.qosClass,omitempty"`
	LowNamespaces     string `json:"default.cpushare.qos.low.namespaces,omitempty"`
	LowL3Percent      string `json:"default.cpushare.qos.low.l3-percent,omitempty"`
	LowMbPercent      string `json:"default.cpushare.qos.low.mb-percent,omitempty"`
	LowCPUWatermark   string `json:"default.cpushare.qos.low.cpu-watermark,omitempty"`
	HighForceReserved string `json:"default.cpushare.qos.high.force-reserved,omitempty"`
}

func (c *CgroupsControllerConfig) DeepCopy() *CgroupsControllerConfig {
	return &CgroupsControllerConfig{
		CPUSet:            copyStringMap(c.CPUSet),
		CPUAcct:           copyStringMap(c.CPUAcct),
		QOSClass:          c.QOSClass,
		LowNamespaces:     c.LowNamespaces,
		LowL3Percent:      c.LowL3Percent,
		LowMbPercent:      c.LowMbPercent,
		LowCPUWatermark:   c.LowCPUWatermark,
		HighForceReserved: c.HighForceReserved,
	}
}

type PlanType int

const (
	PodType = iota
	CgroupsCrdType
)

type PlanOwner struct {
	Type      PlanType
	Namespace string
	Name      string
}

type PodReconcilePlan struct {
	Owner         PlanOwner
	PodMeta       *statesinformer.PodMeta
	PodPlan       *reconcilePlan
	ContainerPlan map[string]*reconcilePlan
}

type reconcilePlan struct {
	CPULimitMilli    *int64
	MemoryLimitBytes *int64
	Blkio            *resourcesv1alpha1.Blkio
	CPUSet           *string
	LLCInfo          *resourcesv1alpha1.LLCinfo
	CPUSetSpec       map[string]string
	CPUAcctSpec      map[string]string
}

func (r *reconcilePlan) DeepCopy() *reconcilePlan {
	result := &reconcilePlan{}
	if r.CPULimitMilli != nil {
		cpuMilli := *r.CPULimitMilli
		result.CPULimitMilli = pointer.Int64Ptr(cpuMilli)
	}
	if r.MemoryLimitBytes != nil {
		memoryBytes := *r.MemoryLimitBytes
		result.MemoryLimitBytes = pointer.Int64Ptr(memoryBytes)
	}
	if r.Blkio != nil {
		result.Blkio = r.Blkio.DeepCopy()
	}
	if r.CPUSet != nil {
		cpuSet := *r.CPUSet
		result.CPUSet = pointer.StringPtr(cpuSet)
	}
	if r.LLCInfo != nil {
		result.LLCInfo = r.LLCInfo.DeepCopy()
	}
	result.CPUSetSpec = copyStringMap(r.CPUSetSpec)
	result.CPUAcctSpec = copyStringMap(r.CPUAcctSpec)
	return result
}

func NeedReconcileByAnnotation(pod *corev1.Pod, config *CgroupsControllerConfig) bool {
	if pod == nil || len(pod.Annotations) == 0 {
		return false
	}
	if val, exist := pod.Annotations[uniext.AnnotationPodCPUSet]; exist && val != "" {
		return true
	}
	if val, exist := pod.Annotations[AnnotationKeyCPUSet]; exist && val != "" {
		return true
	}
	if val, exist := pod.Annotations[AnnotationKeyCPUAcct]; exist && val != "" {
		return true
	}
	if val, exist := pod.Annotations[uniext.AnnotationPodQOSClass]; exist && val != "" && config != nil {
		return true
	}
	if val, exist := pod.Annotations[AnnotationKeyCacheQOS]; exist && val != "" && config != nil {
		return true
	}
	return false
}

func NeedReconcileByConfigmap(config *CgroupsControllerConfig) bool {
	if config == nil || strings.TrimSpace(config.QOSClass) == "" || strings.TrimSpace(config.LowNamespaces) == "" {
		return false
	}
	return true
}

type ResourceLimitScaleOpt string

const (
	ResourceLimitScaleUp   ResourceLimitScaleOpt = "ResourceLimitScaleUp"
	ResourceLimitScaleDown ResourceLimitScaleOpt = "ResourceLimitScaleDown"
	ResourceLimitRemain    ResourceLimitScaleOpt = "ResourceLimitRemain"
)
