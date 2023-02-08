package gpushare

import (
	v1 "k8s.io/api/core/v1"
)

const (
	GPUShareMaxPodPerDeviceLabelKey = "ack.node.gpu.max-pod"
)

const (
	//GPUShareResourceName defines the resource name
	GPUShareResourceMemoryName = "aliyun.com/gpu-mem"
	// GPUShareResourceCountName  defines the resource name  of gpu count
	GPUShareResourceCountName = "aliyun.com/gpu-count"
)

const (
	GPUShareV3DeviceCountFlag = "gpushare.alibabacloud.com/device-resource"
	//GPUShareAllocationFlag defines the allocation of pod
	GPUShareV3AllocationFlag = "gpushare.alibabacloud.com/gpumemory"
	//CGPUPodAssignFlag represents the pod is assigned or not
	GPUShareV3PodAnnoAssignFlag = "gpushare.alibabacloud.com/assigned"
	//CGPUPodAssumeTimeFlag represents the assume time of pod
	GPUShareV3PodAnnoAssumeTimeFlag = "gpushare.alibabacloud.com/assume-time"
)

const (
	//GPUShareAllocationFlag defines the allocation of pod
	GPUShareV2PodAllocationFlag = "scheduler.framework.gpushare.allocation"
	//CGPUPodAssignFlag represents the pod is assigned or not
	GPUShareV2PodAnnoAssignFlag = "scheduler.framework.gpushare.assigned"
	//CGPUPodAssumeTimeFlag represents the assume time of pod
	GPUShareV2PodAnnoAssumeTimeFlag = "scheduler.framework.gpushare.assume-time"
)

const (
	GPUShareV1AllocationFlag     = "ALIYUN_COM_GPU_MEM_POD"
	GPUShareV1DeviceIndexFlag    = "ALIYUN_COM_GPU_MEM_IDX"
	GPUShareV1TotalGPUMemoryFlag = "ALIYUN_COM_GPU_MEM_DEV"
)

type ResourceInfo struct {
	Name             string
	PodAnnotationKey string
}

func GetResourceInfoList() ResourceInfoList {
	return ResourceInfoList([]ResourceInfo{
		{
			Name:             GPUShareResourceMemoryName,
			PodAnnotationKey: "scheduler.framework.gpushare.allocation",
		},
		{
			Name:             "aliyun.com/gpu-core.percentage",
			PodAnnotationKey: "gpushare.alibabacloud.com/core-percentage",
		},
		{
			Name:             "aliyun.com/gpu-core.tflops",
			PodAnnotationKey: "gpushare.alibabacloud.com/core-tflops",
		},
	})
}

type ResourceInfoList []ResourceInfo

func (g ResourceInfoList) GetNames() []v1.ResourceName {
	resourceNames := []v1.ResourceName{}
	for _, r := range g {
		resourceNames = append(resourceNames, v1.ResourceName(r.Name))
	}
	return resourceNames
}

func (g ResourceInfoList) GetPodAnnotationKey(resourceName v1.ResourceName) string {
	for _, r := range g {
		if r.Name == string(resourceName) {
			return r.PodAnnotationKey
		}
	}
	return ""
}
