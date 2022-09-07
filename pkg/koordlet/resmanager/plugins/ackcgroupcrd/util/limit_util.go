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

	"github.com/koordinator-sh/koordinator/pkg/util"
	sysutil "github.com/koordinator-sh/koordinator/pkg/util/system"
)

const (
	MemoryUnlimitValue = -1
)

func getResourceLimitOpt(curVal, targetVal int) ResourceLimitScaleOpt {
	opt := ResourceLimitRemain
	if curVal <= 0 && targetVal > 0 {
		// current unlimited, target limited
		opt = ResourceLimitScaleDown
	} else if curVal >= 0 && targetVal < 0 {
		// current limited, target unlimited
		opt = ResourceLimitScaleUp
	} else if curVal <= 0 && targetVal <= 0 {
		// current and target both unlimited
		opt = ResourceLimitRemain
	} else if curVal < targetVal {
		opt = ResourceLimitScaleUp
	} else if curVal > targetVal {
		opt = ResourceLimitScaleDown
	} // else equal
	return opt
}

func GetPodCPULimitOpt(podPlan *PodReconcilePlan) (ResourceLimitScaleOpt, int, error) {
	opt := ResourceLimitRemain
	curPodCfsQuota, err := util.GetPodCurCFSQuota(podPlan.PodMeta.CgroupDir)
	if err != nil {
		return opt, -1, err
	}
	targetPodCfsQuota := getTargetPodCFSQuota(podPlan)
	return getResourceLimitOpt(int(curPodCfsQuota), targetPodCfsQuota), targetPodCfsQuota, nil
}

func GetPodMemLimitOpt(podPlan *PodReconcilePlan) (ResourceLimitScaleOpt, int, error) {
	opt := ResourceLimitRemain
	curPodMemLimit, err := util.GetPodCurMemLimitBytes(podPlan.PodMeta.CgroupDir)
	if err != nil {
		return opt, -1, err
	}
	targetPodMemLimit := getTargetPodMemLimit(podPlan)
	return getResourceLimitOpt(int(curPodMemLimit), targetPodMemLimit), targetPodMemLimit, nil
}

func getTargetPodCFSQuota(podPlan *PodReconcilePlan) int {
	getContainerMilliCPULimit := func(c *corev1.Container) int64 {
		if containerPlan, exist := podPlan.ContainerPlan[c.Name]; exist && containerPlan.CPULimitMilli != nil {
			return *containerPlan.CPULimitMilli
		}
		return util.GetContainerMilliCPULimit(c)
	}

	podCPUMilliLimit := int64(0)
	for _, container := range podPlan.PodMeta.Pod.Spec.Containers {
		containerCPUMilliLimit := getContainerMilliCPULimit(&container)
		if containerCPUMilliLimit <= 0 {
			return -1
		}
		podCPUMilliLimit += containerCPUMilliLimit
	}
	for _, container := range podPlan.PodMeta.Pod.Spec.InitContainers {
		containerCPUMilliLimit := getContainerMilliCPULimit(&container)
		if containerCPUMilliLimit <= 0 {
			return -1
		}
		podCPUMilliLimit = util.MaxInt64(podCPUMilliLimit, containerCPUMilliLimit)
	}
	if podCPUMilliLimit <= 0 {
		return -1
	}
	return int(podCPUMilliLimit*sysutil.CFSBasePeriodValue) / 1000
}

func getTargetPodMemLimit(podPlan *PodReconcilePlan) int {
	getContainerMemoryLimit := func(c *corev1.Container) int64 {
		if containerPlan, exist := podPlan.ContainerPlan[c.Name]; exist && containerPlan.MemoryLimitBytes != nil {
			return *containerPlan.MemoryLimitBytes
		}
		return util.GetContainerMemoryByteLimit(c)
	}

	podMemLimit := int64(0)
	for _, container := range podPlan.PodMeta.Pod.Spec.Containers {
		containerMemLimit := getContainerMemoryLimit(&container)
		if containerMemLimit <= 0 {
			return int(MemoryUnlimitValue)
		}
		podMemLimit += containerMemLimit
	}
	for _, container := range podPlan.PodMeta.Pod.Spec.InitContainers {
		containerMemLimit := getContainerMemoryLimit(&container)
		if containerMemLimit <= 0 {
			return int(MemoryUnlimitValue)
		}
		podMemLimit = util.MaxInt64(podMemLimit, containerMemLimit)
	}
	if podMemLimit <= 0 {
		return int(MemoryUnlimitValue)
	}
	return int(podMemLimit)
}
