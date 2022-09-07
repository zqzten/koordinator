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
	"encoding/json"
	"strconv"

	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"

	uniext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

func GeneratePlanByCgroups(podMeta *statesinformer.PodMeta, podCgroup *resourcesv1alpha1.PodCgroupsInfo) *PodReconcilePlan {
	podPlan := &PodReconcilePlan{
		Owner:         PlanOwner{Type: CgroupsCrdType, Namespace: podCgroup.PodNamespace, Name: podCgroup.PodName},
		PodMeta:       podMeta,
		ContainerPlan: make(map[string]*reconcilePlan, len(podCgroup.Containers)),
	}
	for _, containerCgroups := range podCgroup.Containers {
		containerPlan := genContainerPlanByCgroups(containerCgroups)
		podPlan.ContainerPlan[containerCgroups.Name] = containerPlan
	}
	return podPlan
}

func GeneratePlanByPod(podMeta *statesinformer.PodMeta, config *CgroupsControllerConfig, cpuNum int) *PodReconcilePlan {
	if podMeta == nil || podMeta.Pod == nil || len(podMeta.Pod.Annotations) == 0 {
		return nil
	}
	pod := podMeta.Pod
	podPlan := &reconcilePlan{}
	containerPlanTemplate := &reconcilePlan{}

	// parse numa-aware cpuset allocate result
	var containerCPUSetMap map[string]string
	var err error
	if cpuSetAllocResult, exist := pod.Annotations[uniext.AnnotationPodCPUSet]; exist && cpuSetAllocResult != "" {
		if containerCPUSetMap, err = parseCPUSetAllocResult(cpuSetAllocResult); err != nil {
			klog.Warningf("unmarshal pod annotation %v failed, error %v", uniext.AnnotationPodCPUSet, err)
		}
		klog.V(6).Infof("pod %v/%v cpu set alloc resource %v", pod.Namespace, pod.Name, cpuSetAllocResult)
	}

	needUnsetPodCfsQuota := IsPodCfsQuotaNeedUnset(pod)
	if needUnsetPodCfsQuota && len(containerCPUSetMap) > 0 {
		podPlan.CPULimitMilli = pointer.Int64Ptr(-1) // set -1 to unset cfs quota
	}

	// parse pod cpuset map, merge with default value in config-map, set as container plan
	var cpuSetMap map[string]string
	if config != nil && len(config.CPUSet) > 0 {
		cpuSetMap = copyStringMap(config.CPUSet)
	}
	if cpuAnnCPUSet, exist := pod.Annotations[AnnotationKeyCPUSet]; exist && cpuAnnCPUSet != "" {
		podCpuSet := make(map[string]string)
		err := json.Unmarshal([]byte(cpuAnnCPUSet), &podCpuSet)
		if err != nil {
			klog.Warningf("unmarshal pod annotation %v failed, error %v", AnnotationKeyCPUSet, err)
		} else {
			cpuSetMap = mergeStringMap(cpuSetMap, podCpuSet)
		}
	}
	if len(cpuSetMap) > 0 {
		containerPlanTemplate.CPUSetSpec = cpuSetMap
		klog.V(6).Infof("set pod %v/%v cpu set map %v", pod.Namespace, pod.Name, cpuSetMap)
	}

	// parse pod cpu acct map, merge with default value in config-map, set as container plan
	var cpuAcctMap map[string]string
	if config != nil && len(config.CPUAcct) > 0 {
		cpuAcctMap = copyStringMap(config.CPUAcct)
	}
	if cpuAnnCPUAcct, exist := pod.Annotations[AnnotationKeyCPUAcct]; exist && cpuAnnCPUAcct != "" {
		podCpuAcct := make(map[string]string)
		err := json.Unmarshal([]byte(cpuAnnCPUAcct), &podCpuAcct)
		if err != nil {
			klog.Warningf("unmarshal pod annotation %v failed, error %v", AnnotationKeyCPUAcct, err)
		} else {
			cpuAcctMap = mergeStringMap(cpuAcctMap, podCpuAcct)
		}
	}
	if len(cpuAcctMap) > 0 {
		containerPlanTemplate.CPUAcctSpec = cpuAcctMap
		klog.V(6).Infof("set pod %v/%v cpu acct map %v", pod.Namespace, pod.Name, cpuAcctMap)
	}

	// parse pod qos, set cpu identity of pod and container
	podQOS := getQOSClass(pod, config)
	klog.V(6).Infof("parse pod %v/%v qos class as %v", pod.Namespace, pod.Name, podQOS)
	if podQOS != apiext.QoSNone {
		cpuIdentityMap := generateCPUIdentity(podQOS)
		podPlan.CPUAcctSpec = mergeStringMap(podPlan.CPUAcctSpec, cpuIdentityMap)
		containerPlanTemplate.CPUAcctSpec = mergeStringMap(containerPlanTemplate.CPUAcctSpec, cpuIdentityMap)
	}

	// set llc plan for BE pod by config
	if podQOS == apiext.QoSBE {
		containerPlanTemplate.LLCInfo = generateLLCInfo(pod, config)
	}

	// reset cpuset for container if pod specified qos
	if config != nil && config.LowCPUWatermark != "" {
		lowCPUPercent, err := strconv.Atoi(config.LowCPUWatermark)
		if err != nil || lowCPUPercent <= 0 || lowCPUPercent > 100 {
			lowCPUPercent = 100
			klog.Warningf("default.cpushare.qos.low.cpu-watermark %v is illegal, set as 100", lowCPUPercent)
		}

		var lsCPUSet, beCPUSet string
		if config.HighForceReserved == "true" {
			lsCPUSet, beCPUSet = getIsolateCpuset(lowCPUPercent)
		} else {
			lsCPUSet = getTaskCpuset(100, cpuNum)
			beCPUSet = getTaskCpuset(lowCPUPercent, cpuNum)
		}
		if podQOS == apiext.QoSLS {
			containerPlanTemplate.CPUSet = &lsCPUSet
			klog.V(6).Infof("set LS pod %v/%v cpu by config %v", pod.Namespace, pod.Name, lsCPUSet)
		} else if podQOS == apiext.QoSBE {
			containerPlanTemplate.CPUSet = &beCPUSet
			klog.V(6).Infof("set BE pod %v/%v cpu by config %v", pod.Namespace, pod.Name, beCPUSet)
		}
	}

	// merge pod and container plan
	podReconcilePlan := &PodReconcilePlan{
		Owner:         PlanOwner{Type: PodType, Namespace: podMeta.Pod.Namespace, Name: podMeta.Pod.Name},
		PodMeta:       podMeta,
		PodPlan:       podPlan,
		ContainerPlan: make(map[string]*reconcilePlan, len(pod.Spec.Containers)),
	}
	for i := range pod.Spec.Containers {
		containerName := pod.Spec.Containers[i].Name
		containerPlan := containerPlanTemplate.DeepCopy()
		if containerCPUSetStr, exist := containerCPUSetMap[containerName]; exist && containerCPUSetStr != "" {
			klog.V(6).Infof("add container %v cpuset %v", containerName, containerCPUSetStr)
			containerPlan.CPUSet = pointer.StringPtr(containerCPUSetStr)
			if needUnsetPodCfsQuota { // containers of policy=static-burst cpuset pod should not unset cfs quota
				containerPlan.CPULimitMilli = pointer.Int64Ptr(-1) // set -1 to unset cfs quota
			}
		}
		podReconcilePlan.ContainerPlan[containerName] = containerPlan
		klog.V(6).Infof("add container plan %v to pod %v/%v, detail %v",
			containerName, pod.Namespace, pod.Name, containerPlan)
	}
	return podReconcilePlan
}

func genContainerPlanByCgroups(containerCgroups *resourcesv1alpha1.ContainerInfoSpec) *reconcilePlan {
	if containerCgroups == nil {
		return nil
	}
	result := &reconcilePlan{}
	if !containerCgroups.Cpu.IsZero() {
		result.CPULimitMilli = pointer.Int64Ptr(containerCgroups.Cpu.MilliValue())
	}
	if !containerCgroups.Memory.IsZero() {
		result.MemoryLimitBytes = pointer.Int64Ptr(containerCgroups.Memory.Value())
	}
	if !containerCgroups.BlkioSpec.IsEmpty() {
		result.Blkio = &containerCgroups.BlkioSpec
	}
	if containerCgroups.CPUSet != "" {
		result.CPUSet = &containerCgroups.CPUSet
	}
	if !containerCgroups.LLCSpec.IsEmpty() {
		result.LLCInfo = &containerCgroups.LLCSpec
	}
	if containerCgroups.CpuSetSpec != "" {
		if cpuSetMap, err := jsonToStringMap(containerCgroups.CpuSetSpec); err == nil {
			result.CPUSetSpec = cpuSetMap
		}
	}
	if containerCgroups.CpuAcctSpec != "" {
		if cpuAcctMap, err := jsonToStringMap(containerCgroups.CpuAcctSpec); err == nil {
			result.CPUAcctSpec = cpuAcctMap
		}
	}
	return result
}
