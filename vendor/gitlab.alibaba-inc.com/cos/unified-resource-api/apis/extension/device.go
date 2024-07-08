package extension

import (
	"encoding/json"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"gitlab.alibaba-inc.com/cos/unified-resource-api/apis/scheduling/v1beta1"
)

const (
	// when resource spec needs to be across multiple devices, this annotation indicates how total resources should be proportioned.
	// Example:
	//   alibabacloud.com/device-multi-card-request-hint: |- {
	//	"hints": {
	//		"fpga-device": [{
	//			"ContainerName": "c1",
	//			"hints": [100, 100]
	//		}],
	//		"gpu-device": [{
	//			"ContainerName": "c1",
	//			"hints": [50, 50]
	//		}]
	//	}
	//}
	//
	// This example requires that
	//  1. gpu resources (of any dimension requested) should be divided into two halves, each on a different gpu device;
	AnnotationMultiDeviceRequestHint = "alibabacloud.com/device-multi-card-request-hint"
	// Example:
	//   alibabacloud.com/device-multi-card-alloc-status: |- {
	//	"allocStatus": {
	//		"fpga-device": [{
	//			"containerName": "c1",
	//			"deviceAllocStatus": {
	//				"allocs": [{
	//					"uuid": "uid1",
	//					"minor": 1,
	//					"resources": {
	//						"alibabacloud.com/fpga": "1"
	//					}
	//				}, {
	//					"uuid": "uid2",
	//					"minor": 2,
	//					"resources": {
	//						"alibabacloud.com/fpga": "1"
	//					}
	//				}]
	//			}
	//		}],
	//		"gpu-device": [{
	//			"containerName": "c1",
	//			"deviceAllocStatus": {
	//				"allocs": [{
	//					"uuid": "uid1",
	//					"minor": 1,
	//					"resources": {
	//						"alibabacloud.com/gpu-mem": "1"
	//					}
	//				}, {
	//					"uuid": "uid2",
	//					"minor": 2,
	//					"resources": {
	//						"alibabacloud.com/gpu-mem": "1"
	//					}
	//				}]
	//			}
	//		}]
	//	}
	//}
	AnnotationMultiDeviceAllocStatus = "alibabacloud.com/device-multi-card-alloc-status"
	// AnnotationEnableVGPU indicates whether this pod forces enabling/disabling vGPU configurations.
	//Example:
	// true/True/TRUE/t/T/1: explicitly enables vGPU
	// false/False/FALSE/f/F/0: explicitly mutes vGPU
	// other values: depends on pod configurations, i.e., gpushare or not, BE class or not, etc.
	AnnotationEnableVGPU = "alibabacloud.com/enable-vgpu"

	// AnnotationNVIDIAVisibleDevices specifies the NV devices visible to a container
	AnnotationNVIDIAVisibleDevices = "alibabacloud.com/nvidia-visible-devices"

	// AnnotationVisibleGPUsForTopologyAwareWorker specifies the GPUs a topology-aware container should use.
	// This is used for GPU topology-aware scheduling.
	// @see https://yuque.antfin-inc.com/docs/share/765bd4cd-7452-4a1f-ad36-cfb2ab9c7403?#
	AnnotationVisibleGPUsForTopologyAwareWorker = "alibabacloud.com/visible-gpus-for-topology-aware-worker"

	FpgaResourceXilinx  = "xilinx.com/fpga"       // 表示独占卡
	FpgaResourceAlibaba = "alibabacloud.com/fpga" // 表示比例值，100表示一张卡
	NpuResourceAlibaba  = "alibabacloud.com/npu"  // 表示比例值，100表示一张卡
	RdmaResourceAlibaba = "alibabacloud.com/rdma" // 按照比例售卖，100表示一张卡
	FpgaResourceAliyun  = "aliyun.com/fpga"       // 表示需要多少张独占fpga
	NpuResourceAliyun   = "aliyun.com/npu"        // 表示需要多少张独占npu
	RdmaResourceAliyun  = "aliyun.com/rdma"       // 表示需要多少张独占rdma

	// gpu resource name
	GPUResourceNvidia    = "nvidia.com/gpu"                  // 独占卡分配
	GPUResourceAliyun    = "aliyun.com/gpu"                  // 阿里云表示独占卡分配
	GPUResourceAlibaba   = "alibabacloud.com/gpu"            // 表示需要多少比例的gpu显存和gpu-core
	GPUResourceCore      = "alibabacloud.com/gpu-core"       // 表示需要多少比例的gpu-core，一张卡的gpu-core表示100
	GPUResourceMem       = "alibabacloud.com/gpu-mem"        // 表示需要具体的gpu显存数值
	GPUResourceMemRatio  = "alibabacloud.com/gpu-mem-ratio"  // 表示需要gpu显存比例值，100表示一张卡
	GPUResourceEncode    = "alibabacloud.com/gpu-encode"     // 表示需要gpu的编码能力的比例值，100表示一张卡
	GPUResourceDecode    = "alibabacloud.com/gpu-decode"     // 表示需要gpu的解码能力的比例值，100表示一张卡
	GPUResourceCardRatio = "alibabacloud.com/gpu-card-ratio" // 表示需要多少比例的gpu卡，通常作为gpu quota计算的统一度量维度

	// deprecated gpu resource name
	DeprecatedGpuCountName     = "alibabacloud.com/gpu-count"
	DeprecatedGpuMemName       = "nvidia.com/gpumem"
	DeprecatedGpuMilliCoreName = "alibabacloud.com/gpu-compute"
	FuxiGpu                    = "alibabacloud.com/fuxi-gpu"
)

// for use of (de-)serialization of the AnnotationMultiDeviceRequestHint
type MultiDeviceRequestHint struct {
	// keyed by device name, valued by an int array of the split-proportion across devices.
	Hints map[v1beta1.DeviceType][]ContainerDeviceRequest `json:"hints"`
}

type ContainerDeviceRequest struct {
	ContainerName string  `json:"containerName,omitempty"`
	Hints         []int32 `json:"hints"`
}

type MultiDeviceAllocStatus struct {
	AllocStatus map[v1beta1.DeviceType][]ContainerDeviceAllocStatus `json:"allocStatus"`
}

type ContainerDeviceAllocStatus struct {
	ContainerName     string            `json:"containerName,omitempty"`
	DeviceAllocStatus DeviceAllocStatus `json:"deviceAllocStatus,inline"`
}

// for use of (de-)serialization of device alloc status
// currently containers in a pod must share device alloc status
type DeviceAllocStatus struct {
	Allocs []Alloc `json:"allocs"`
}

type Alloc struct {
	ID        string                       `json:"uuid,omitempty"`
	Minor     int32                        `json:"minor"`
	IsSharing bool                         `json:"isSharing,omitempty"`
	Resources map[string]resource.Quantity `json:"resources,omitempty"` // map of sub-resources
}

func GetMultiDeviceAllocStatus(annotations map[string]string) (*MultiDeviceAllocStatus, error) {
	str := annotations[AnnotationMultiDeviceAllocStatus]
	object := &MultiDeviceAllocStatus{}
	if unmarshalErr := json.Unmarshal([]byte(str), &object); unmarshalErr != nil {
		return nil, unmarshalErr
	}
	return object, nil
}

func SetMultiDeviceAllocStatus(annotations map[string]string, status *MultiDeviceAllocStatus) error {
	bytes, err := json.Marshal(status)
	if err != nil {
		return err
	}
	annotations[AnnotationMultiDeviceAllocStatus] = string(bytes)
	return nil
}

func GetMultiDeviceRequestHint(annotations map[string]string) (*MultiDeviceRequestHint, error) {
	str := annotations[AnnotationMultiDeviceRequestHint]
	object := &MultiDeviceRequestHint{}
	if unmarshalErr := json.Unmarshal([]byte(str), &object); unmarshalErr != nil {
		return nil, unmarshalErr
	}
	return object, nil
}

// IsVGPUForceEnabled only returns true when the pod has AnnotationEnableVGPU with a valid positive value:
// "1", "t", "T", "true", "TRUE", "True"
func IsVGPUForceEnabled(pod *v1.Pod) bool {
	if pod == nil {
		return false
	}
	if pod.Annotations == nil {
		return false
	}
	value, ok := pod.Annotations[AnnotationEnableVGPU]
	if !ok {
		return false
	}
	enabled, _ := strconv.ParseBool(value)
	return enabled
}

// IsVGPUForceDisabled only returns true when the pod has AnnotationEnableVGPU with a valid negative value:
// "0", "f", "F", "false", "FALSE", "False":
func IsVGPUForceDisabled(pod *v1.Pod) bool {
	if pod == nil {
		return false
	}
	if pod.Annotations == nil {
		return false
	}
	value, ok := pod.Annotations[AnnotationEnableVGPU]
	if !ok {
		return false
	}
	enabled, err := strconv.ParseBool(value)
	if enabled {
		// valid positive values
		return false
	}
	if err != nil {
		// invalid bool values
		return false
	}
	return true
}

func SetMultiDeviceRequestHint(annotations map[string]string, request *MultiDeviceRequestHint) error {
	bytes, err := json.Marshal(request)
	if err != nil {
		return err
	}
	annotations[AnnotationMultiDeviceRequestHint] = string(bytes)
	return nil
}
