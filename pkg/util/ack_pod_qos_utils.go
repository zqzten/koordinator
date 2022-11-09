//go:build !github
// +build !github

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

package util

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/apis/core/v1/helper/qos"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"

	uniapiext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
)

// IsPodCfsQuotaNeedUnset checks if the pod-level and container-level cfs_quota should be unset to avoid unnecessary
// throttles.
// https://aone.alibaba-inc.com/v2/project/1053428/req/43543780#
// https://github.com/koordinator-sh/koordinator/issues/489
func IsPodCfsQuotaNeedUnset(annotations map[string]string) (bool, error) {
	if annotations == nil {
		return false, nil
	}
	// 1. check koordinator cpuset protocols
	cpusetVal, err := GetCPUSetFromPod(annotations)
	if err != nil {
		return false, err
	}
	if cpusetVal != "" {
		return true, nil
	}

	// 2. check ack cpuset protocols
	// assert cpuset only check for annotations
	pod := &corev1.Pod{}
	pod.Annotations = annotations
	if isPodCPUSet, err := uniapiext.IsPodCPUSet(pod); err != nil {
		return false, fmt.Errorf("failed to check cpuset, err: %v", err)
	} else if !isPodCPUSet { // does not unset if the pod is not cpuset
		return false, nil
	}

	// when policy is static-burst, cpu limit <= cpuset cpus, so does not unset cfs quota in this case
	if uniapiext.GetCPUPolicyFromAnnotation(pod) == uniapiext.CPUPolicyStaticBurst {
		// FIXME: the static-burst cpuset pod is need to be unset if it has the same pod cpuset size with the specified
		//  cpu limit, but currently pod cpu limit is not parameterized in runtime hook protocols
		return false, nil
	}
	return true, nil
}

// IsPodCPUBurstable checks if cpu burst is allowed for the pod.
func IsPodCPUBurstable(pod *corev1.Pod) bool {
	podQOS := apiext.GetPodQoSClass(pod)
	// LSR, LSE pods are not cfs burstable; BE pods are not allowed to burst
	if podQOS == apiext.QoSLSR || podQOS == apiext.QoSLSE || podQOS == apiext.QoSBE {
		return false
	}

	// check pod with ack cpuset protocols
	if isPodCPUSet, err := uniapiext.IsPodCPUSet(pod); err != nil {
		klog.Warningf("failed to check if the pod is cpuset, err: %s", err)
		return true
	} else if !isPodCPUSet { // pod is cpushare
		return true
	}

	// static-burst pod need cpu burst since its cpuset can be larger than the original cpu limit
	if uniapiext.GetCPUPolicyFromAnnotation(pod) == uniapiext.CPUPolicyStaticBurst {
		return true
	}
	return false
}

func GetKubeQosClass(pod *corev1.Pod) corev1.PodQOSClass {
	qosClass := pod.Status.QOSClass
	if len(qosClass) <= 0 {
		qosClass = qos.GetPodQOS(pod)
	}
	return qosClass
}
