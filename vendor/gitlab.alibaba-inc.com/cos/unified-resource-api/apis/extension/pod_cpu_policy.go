package extension

import (
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension/cpuset"
)

type CPUPolicy string

const (
	//add by ACK
	CPUPolicyStaticBurst CPUPolicy = "static-burst"
	//add by ACK
	CPUPolicyBurst CPUPolicy = "burst"
	//LSR POD should be critical Policy, so CPUSharePool will exclude this POD cpusets
	CPUPolicyCritical CPUPolicy = "critical"
	//LS POD should be this policy
	CPUPolicySharePool CPUPolicy = "share-pool"

	// none Policy
	CPUPolicyNone CPUPolicy = ""
	// unknown Policy
	CPUPolicyUnknown CPUPolicy = "unknown"
)

func GetCPUPolicyFromAnnotation(pod *corev1.Pod) CPUPolicy {
	if pod == nil || pod.Annotations == nil {
		return CPUPolicyNone
	}
	if policy, ok := pod.Annotations[AnnotationPodCPUPolicy]; ok {
		return GetCPUPolicyByName(policy)
	}
	return CPUPolicyNone
}

func GetCPUPolicyByName(policy string) CPUPolicy {
	cpuPolicy := CPUPolicy(policy)
	switch cpuPolicy {
	case CPUPolicyStaticBurst, CPUPolicyBurst, CPUPolicyCritical, CPUPolicySharePool, CPUPolicyNone:
		return cpuPolicy
	}
	return CPUPolicyUnknown
}

type CPUSetScheduler string

const (
	CPUSetSchedulerTrue  CPUSetScheduler = "true"
	CPUSetSchedulerFalse CPUSetScheduler = "false"
	// none CPUSetScheduler
	CPUSetSchedulerNone CPUSetScheduler = ""
	// CPUSetScheduler unknown
	CPUSetSchedulerUnknown CPUSetScheduler = "unknown"
)

func GetCPUSetSchedulerFromAnnotation(pod *corev1.Pod) CPUSetScheduler {
	if pod == nil || pod.Annotations == nil {
		return CPUSetSchedulerNone
	}
	if cpusetScheduler, ok := pod.Annotations[AnnotationPodCPUSetScheduler]; ok {
		return GetCPUSetSchedulerByValue(cpusetScheduler)
	}
	return CPUSetSchedulerNone
}

func GetCPUSetSchedulerByValue(scheduler string) CPUSetScheduler {
	cpusetScheduler := CPUSetScheduler(scheduler)
	switch cpusetScheduler {
	case CPUSetSchedulerTrue, CPUSetSchedulerFalse, CPUSetSchedulerNone:
		return cpusetScheduler
	}
	return CPUSetSchedulerUnknown
}

func IsPodCPUSet(pod *corev1.Pod) (bool, error) {
	if pod == nil || pod.Annotations == nil {
		return false, fmt.Errorf("pod or pod.Annotations is nil")
	}
	cpusetScheduler := GetCPUSetSchedulerFromAnnotation(pod)
	if cpusetScheduler == CPUSetSchedulerUnknown {
		return false, fmt.Errorf("parse AnnotationPodCPUSetScheduler error! cpusetScheduler unknown! podUID: %v", pod.UID)
	}
	if cpusetScheduler != CPUSetSchedulerTrue {
		return false, nil
	}

	cpusetStr, ok := pod.Annotations[AnnotationPodCPUSet]
	if !ok || strings.TrimSpace(cpusetStr) == "" {
		return false, fmt.Errorf("Pod Is a CPUSet Type, but AnnotationPodCPUSet is empty! podUID: %v ", pod.UID)
	}

	return true, nil
}

func GetCPUAllocateResultFromAnnotation(pod *corev1.Pod) (cpuset.CPUAllocateResult, error) {
	cpuAllocateResult := cpuset.CPUAllocateResult{}
	isCPUSet, err := IsPodCPUSet(pod)
	if err != nil {
		return cpuAllocateResult, err
	}
	if !isCPUSet {
		return cpuAllocateResult, nil
	}

	err = json.Unmarshal([]byte(pod.Annotations[AnnotationPodCPUSet]), &cpuAllocateResult)
	if err != nil {
		return cpuAllocateResult, fmt.Errorf("Parse AnnotationPodCPUSet error, podUID : %v, error: %v ", pod.UID, err)
	}
	return cpuAllocateResult, nil
}
