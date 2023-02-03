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

package cpusetallocator

import (
	"encoding/json"
	"fmt"

	uniapiext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	unifiedextcpuset "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension/cpuset"
	corev1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
	"github.com/koordinator-sh/koordinator/pkg/util/cpuset"
)

const (
	AnnotationCPUExclusivePolicy = "cpu-policy/exclusive"

	// CPUExclusivePolicyPCPULevel represents mutual exclusion in the physical core dimension
	CPUExclusivePolicyPCPULevel = "pcpu-level"
)

const (
	AnnotationCPUBindPolicy = "cpu-policy/bind"
	// CPUBindPolicySpreadByPCPUs favor cpuset allocation that evenly allocate logical cpus across physical cores
	CPUBindPolicySpreadByPCPUs = "spread-by-pcpus"
)

func init() {
	nodenumaresource.GetResourceSpec = GetResourceSpec
	nodenumaresource.GetResourceStatus = GetResourceStatus
	nodenumaresource.SetResourceStatus = SetResourceStatus
	nodenumaresource.GetPodQoSClass = GetPodQoSClass
	nodenumaresource.GetPriorityClass = GetPriorityClass
}

func GetPodQoSClass(pod *corev1.Pod) extension.QoSClass {
	if s, ok := pod.Annotations[uniapiext.AnnotationPodQOSClass]; ok {
		qosClass := uniapiext.GetPodQoSClassByName(s)
		return extension.QoSClass(qosClass)
	}
	return extension.GetPodQoSClass(pod)
}

func GetPriorityClass(pod *corev1.Pod) extension.PriorityClass {
	qosClass := GetPodQoSClass(pod)
	if qosClass == extension.QoSLSR || qosClass == extension.QoSLS {
		return extension.PriorityProd
	}
	return extension.PriorityNone
}

func getCPUSetSchedulerFromAnnotation(annotations map[string]string) uniapiext.CPUSetScheduler {
	if cpusetScheduler, ok := annotations[uniapiext.AnnotationPodCPUSetScheduler]; ok {
		return uniapiext.GetCPUSetSchedulerByValue(cpusetScheduler)
	}
	return uniapiext.CPUSetSchedulerNone
}

func getCPUPolicyFromAnnotation(annotations map[string]string) uniapiext.CPUPolicy {
	if policy, ok := annotations[uniapiext.AnnotationPodCPUPolicy]; ok {
		return uniapiext.GetCPUPolicyByName(policy)
	}
	return uniapiext.CPUPolicyNone
}

func GetResourceSpec(annotations map[string]string) (*extension.ResourceSpec, error) {
	if needCPUSetScheduling(annotations) {
		return getCompatibleResourceSpec(annotations), nil
	}

	return extension.GetResourceSpec(annotations)
}

func getCompatibleResourceSpec(annotations map[string]string) *extension.ResourceSpec {
	resourceSpec := &extension.ResourceSpec{
		PreferredCPUBindPolicy: schedulingconfig.CPUBindPolicyDefault,
	}

	policy := getCPUPolicyFromAnnotation(annotations)
	if policy == uniapiext.CPUPolicyCritical {
		resourceSpec.PreferredCPUBindPolicy = schedulingconfig.CPUBindPolicyFullPCPUs
	}

	if bindPolicy, ok := annotations[AnnotationCPUBindPolicy]; ok {
		if bindPolicy == CPUBindPolicySpreadByPCPUs {
			resourceSpec.PreferredCPUBindPolicy = schedulingconfig.CPUBindPolicySpreadByPCPUs
		}
	}

	if exclusivePolicy, ok := annotations[AnnotationCPUExclusivePolicy]; ok {
		if exclusivePolicy == CPUExclusivePolicyPCPULevel {
			resourceSpec.PreferredCPUExclusivePolicy = schedulingconfig.CPUExclusivePolicyPCPULevel
		}
	}
	return resourceSpec
}

func GetResourceStatus(annotations map[string]string) (*extension.ResourceStatus, error) {
	resourceStatus := &extension.ResourceStatus{}

	if data, ok := annotations[uniapiext.AnnotationPodCPUSet]; ok && data != "" {
		allocateResult := unifiedextcpuset.CPUAllocateResult{}
		err := json.Unmarshal([]byte(data), &allocateResult)
		if err != nil {
			return nil, err
		}
		resourceStatus.CPUSet = allocateResult.ToCPUSet().String()
		return resourceStatus, nil
	}

	return extension.GetResourceStatus(annotations)
}

func SetResourceStatus(pod *corev1.Pod, status *extension.ResourceStatus) error {
	if _, ok := pod.Annotations[extension.AnnotationResourceSpec]; !ok {
		spec := getCompatibleResourceSpec(pod.Annotations)
		data, err := json.Marshal(spec)
		if err != nil {
			return err
		}
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations[extension.AnnotationResourceSpec] = string(data)
	}

	pl, ok := globalPlugin.Load().(*Plugin)
	if !ok {
		return fmt.Errorf("want got Plugin but failed")
	}
	cpuTopologyOptions := pl.Plugin.GetCPUTopologyManager().GetCPUTopologyOptions(pod.Spec.NodeName)
	cpuTopology := cpuTopologyOptions.CPUTopology
	if cpuTopology == nil {
		return fmt.Errorf("not found CPUTopology")
	}

	if status.CPUSet == "" {
		return nil
	}
	cpus, err := cpuset.Parse(status.CPUSet)
	if err != nil {
		return err
	}

	nodeToCPUSet := map[int]unifiedextcpuset.SimpleCPUSet{}
	for _, v := range cpus.ToSliceNoSort() {
		cpuInfo := cpuTopology.CPUDetails[v]
		simpleCPUSet := nodeToCPUSet[cpuInfo.NodeID]
		if simpleCPUSet.Elems == nil {
			simpleCPUSet.Elems = map[int]struct{}{}
		}
		simpleCPUSet.Elems[v] = struct{}{}
		nodeToCPUSet[cpuInfo.NodeID] = simpleCPUSet
	}

	allocateResult := make(unifiedextcpuset.CPUAllocateResult)
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		allocateResult[container.Name] = nodeToCPUSet
	}

	annotations, err := json.Marshal(allocateResult)
	if err != nil {
		return err
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[uniapiext.AnnotationPodCPUSet] = string(annotations)

	return extension.SetResourceStatus(pod, status)
}
