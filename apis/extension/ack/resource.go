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

package ack

const (
	ResourceAliyunGPU        = "aliyun.com/gpu"
	ResourceAliyunGPUCompute = "aliyun.com/gpu-compute"
	ResourceAliyunGPUMemory  = "aliyun.com/gpu-mem"

	// AnnotationAliyunEnvResourceIndex represents the minor of the gpuAllocation in order to adapt to gpuShare-device-plugin
	AnnotationAliyunEnvResourceIndex       = "ALIYUN_COM_GPU_MEM_IDX"
	AnnotationAliyunEnvAssignedFlag        = "ALIYUN_COM_GPU_ASSIGNED"
	AnnotationAliyunEnvComputeAssignedFlag = "ALIYUN_COM_GPU_COMPUTE_ASSIGNED"
	AnnotationAliyunEnvMemAssignedFlag     = "ALIYUN_COM_GPU_MEM_ASSIGNED"
	AnnotationAliyunEnvResourceAssumeTime  = "ALIYUN_COM_GPU_MEM_ASSUME_TIME"
	AnnotationAliyunEnvComputeDev          = "ALIYUN_COM_GPU_COMPUTE_DEV"
	AnnotationAliyunEnvComputePod          = "ALIYUN_COM_GPU_COMPUTE_POD"
	AnnotationAliyunEnvMemDev              = "ALIYUN_COM_GPU_MEM_DEV"
	AnnotationAliyunEnvMemPod              = "ALIYUN_COM_GPU_MEM_POD"

	AnnotationACKGPUShareAllocation = "scheduler.framework.gpushare.allocation"
	AnnotationACKGPUShareAssigned   = "scheduler.framework.gpushare.assigned"
	AnnotationACKGPUShareAssumeTime = "scheduler.framework.gpushare.assume-time"
)
