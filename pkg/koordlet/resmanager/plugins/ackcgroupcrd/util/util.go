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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/util"

	uniapiext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

func copyStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, val := range input {
		output[key] = val
	}
	return output
}

func mergeStringMap(src, addon map[string]string) map[string]string {
	output := copyStringMap(src)
	for key, val := range addon {
		output[key] = val
	}
	return output
}

func jsonToStringMap(str string) (map[string]string, error) {
	var tempMap map[string]string
	err := json.Unmarshal([]byte(str), &tempMap)
	return tempMap, err
}

func getQOSClass(pod *corev1.Pod, config *CgroupsControllerConfig) apiext.QoSClass {
	if pod == nil {
		return apiext.QoSNone
	}
	if podQOS := apiext.GetPodQoSClass(pod); podQOS != apiext.QoSNone {
		return podQOS
	}
	if config == nil {
		return apiext.QoSNone
	}
	lowNamespaces := strings.Split(config.LowNamespaces, ",")
	for _, lowNs := range lowNamespaces {
		if pod.Namespace == strings.TrimSpace(lowNs) {
			return apiext.QoSBE
		}
	}
	if defaultQOS := apiext.GetPodQoSClassByName(config.QOSClass); defaultQOS != apiext.QoSNone {
		return defaultQOS
	}
	return apiext.QoSNone
}

func GetPodCgroupsFromNode(pod *corev1.Pod,
	nodeCgroups []*resourcesv1alpha1.NodeCgroupsInfo) *resourcesv1alpha1.PodCgroupsInfo {
	if pod == nil || len(nodeCgroups) == 0 {
		return nil
	}
	for _, nodeCgroup := range nodeCgroups {
		for _, podCgroup := range nodeCgroup.PodCgroups {
			if podCgroup.PodNamespace == pod.Namespace && podCgroup.PodName == pod.Name {
				return podCgroup
			}
		}
	}
	return nil
}

// IsPodCfsQuotaNeedUnset checks if the pod is static cpuset pod. If it is, the pod-level cfs_quota should be unset to
// avoid unnecessary cfs throttles.
// https://aone.alibaba-inc.com/v2/project/1053428/req/43543780#
// https://github.com/koordinator-sh/koordinator/issues/489
func IsPodCfsQuotaNeedUnset(pod *corev1.Pod) bool {
	// does not unset if the pod is not cpuset
	if isPodCPUSet, err := uniapiext.IsPodCPUSet(pod); err != nil {
		klog.Warningf("failed to check if the pod is cpuset, err: %s", err)
		return false
	} else if !isPodCPUSet {
		return false
	}

	// when policy is static-burst, cpu limit <= cpuset cpus, so does not unset cfs quota in this case
	if uniapiext.GetCPUPolicyFromAnnotation(pod) == uniapiext.CPUPolicyStaticBurst {
		// unset the static-burst cpuset pod if it has the same pod cpuset with the specified cpu limit
		allocateResult, err := uniapiext.GetCPUAllocateResultFromAnnotation(pod)
		if err != nil {
			klog.Warningf("failed to get cpu allocate result for the pod %s/%s, err: %s", pod.Namespace, pod.Name, err)
			return false
		}
		if int64(allocateResult.ToCPUSet().Size())*1000 != util.GetPodMilliCPULimit(pod) {
			return false
		}
	}
	return true
}
