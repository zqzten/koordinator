package extension

import (
	"encoding/json"
	"strconv"

	corev1 "k8s.io/api/core/v1"

	"gitlab.alibaba-inc.com/cos/unified-resource-api/apis/nodes/v1beta1"
)

const (
	AnnotationPodQOSClass = "alibabacloud.com/qosClass"

	AnnotationLoadAwareScheduleEnable = "alibabacloud.com/loadAwareScheduleEnabled"

	AnnotationResourceReclaimConfig = "alibabacloud.com/resourceReclaimConfig"

	AnnotationPodAdvancedQOSConfig = "alibabacloud.com/podAdvancedQOSConfig"

	AnnotationPodCPUBurst = "alibabacloud.com/cpuBurst"

	// AnnotationPodMemoryQOS specifies the pod-level memory-qos config and policy (auto, default, close)
	// @see https://yuque.antfin.com/docs/share/0baad621-16ba-40fa-84f9-2a2fad5c4a8e?#
	AnnotationPodMemoryQOS = "alibabacloud.com/memoryQOS"

	//@see https://yuque.antfin.com/docs/share/aaff2606-a26d-4cfe-a7e7-9d8e7d7e83bd?#
	AnnotationPodBlkioQOS = "alibabacloud.com/blkioQOS"

	//@see https://yuque.antfin-inc.com/docs/share/5dee42e0-8632-4300-8993-a0122b8c16ac?#
	AnnotationPodCPUSetScheduler = "cpuset-scheduler"

	//@see https://yuque.antfin-inc.com/docs/share/5dee42e0-8632-4300-8993-a0122b8c16ac?#
	AnnotationPodCPUPolicy = "cpu-policy"

	//@see https://yuque.antfin-inc.com/docs/share/93096642-7416-47ef-91e0-c7fdf7ebc931?#
	AnnotationPodCPUSet = "cpuset"
)

type ResourceReclaimConfig struct {
	GPUReclaimConfig `json:"gpuReclaimConfig"`
}

type GPUReclaimConfig struct {
	Enable          bool    `json:"enable"`
	ReservationCore float64 `json:"reservationCore"`
	ReservationMem  float64 `json:"reservationMem"`
}

type PodAdvancedQOSConfig struct {
	PodResourceQOS *v1beta1.ResourceQOS `json:"podResourceQOS,omitempty"`
}

var defaultResourceReclaimConfig = ResourceReclaimConfig{
	GPUReclaimConfig: GPUReclaimConfig{
		Enable:          false,
		ReservationCore: 0,
		ReservationMem:  0,
	},
}

func IsLoadAwareScheduleEnabled(pod *corev1.Pod) bool {
	if pod == nil || pod.Annotations == nil {
		return false
	}
	value, exist := pod.Annotations[AnnotationLoadAwareScheduleEnable]
	if !exist {
		return false
	}
	isEnabled, _ := strconv.ParseBool(value)
	return isEnabled
}

func GetPodMemoryQOSConfig(pod *corev1.Pod) (*v1beta1.PodMemoryQOSConfig, error) {
	if pod == nil || pod.Annotations == nil {
		return nil, nil
	}
	value, exist := pod.Annotations[AnnotationPodMemoryQOS]
	if !exist {
		return nil, nil
	}
	cfg := v1beta1.PodMemoryQOSConfig{}
	err := json.Unmarshal([]byte(value), &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func GetResourceReclaimConfig(pod *corev1.Pod) *ResourceReclaimConfig {
	config := defaultResourceReclaimConfig
	if pod == nil || pod.Annotations == nil {
		return &config
	}

	annotation, exist := pod.Annotations[AnnotationResourceReclaimConfig]
	if !exist {
		return &config
	}

	err := json.Unmarshal([]byte(annotation), &config)
	if err != nil {
		return &config
	}

	return &config
}

func IsPodGPUReclaimable(pod *corev1.Pod) bool {
	config := GetResourceReclaimConfig(pod)
	return config.Enable
}

func GetPodAdvancedQOSConfig(pod *corev1.Pod) (*PodAdvancedQOSConfig, error) {
	if pod == nil || pod.Annotations == nil {
		return nil, nil
	}

	annotation, exist := pod.Annotations[AnnotationPodAdvancedQOSConfig]
	if !exist {
		return nil, nil
	}
	resourceQOS := PodAdvancedQOSConfig{}

	err := json.Unmarshal([]byte(annotation), &resourceQOS)
	if err != nil {
		return nil, err
	}

	return &resourceQOS, nil
}

func GetPodCPUBurstConfig(pod *corev1.Pod) (*v1beta1.CPUBurstConfig, error) {
	if pod == nil || pod.Annotations == nil {
		return nil, nil
	}
	annotation, exist := pod.Annotations[AnnotationPodCPUBurst]
	if !exist {
		return nil, nil
	}
	cpuBurst := v1beta1.CPUBurstConfig{}

	err := json.Unmarshal([]byte(annotation), &cpuBurst)
	if err != nil {
		return nil, err
	}
	return &cpuBurst, nil
}
