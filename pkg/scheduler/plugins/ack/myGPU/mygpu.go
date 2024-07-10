package myGPU

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const GPUShareName = "myGPUShare"

type VirtualGpuSpecification struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualGpuSpecificationSpec   `json:"spec"`
	Status            VirtualGpuSpecificationStatus `json:"status"`
}

type VirtualGpuSpecificationSpec struct {
	PhysicalGpuSpecifications []string `json:"physicalGpuSpecifications"` // 虚拟GPU所指定的物理GPU型号
	Description               string   `json:"description"`               // 规格描述
	GPUMemory                 int      `json:"gpuMemory"`                 // 显存大小，TODO 最小步长1GB？
	GPUMemoryIsolation        bool     `json:"gpuMemoryIsolation"`        // 显存隔离
	GPUUtilization            int      `json:"gpuUtilization"`            // 算力比例
	GPUUtilizationIsolation   bool     `json:"gpuUtilizationIsolation"`   // 算力隔离
}

type VirtualGpuSpecificationStatus struct {
	IsActive bool `json:"isActive"` // 规格是否可用, 不可用的情况下将暂停卡分配
}

func (myGpu *VirtualGpuSpecification) Name() string {
	return GPUShareName
}

type VirtualGpuInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualGpuInstanceSpec   `json:"spec"`
	Status            VirtualGpuInstanceStatus `json:"status"`
}

type VirtualGpuInstanceSpec struct {
	VirtualGpuSpecification string `json:"virtualGpuSpecification"` // 虚拟GPU实例的规格
}

type VirtualGpuInstanceStatus struct {
	Pod                      string `json:"pod"`                      // 虚拟GPU所属的Pod
	Node                     string `json:"node"`                     // 虚拟GPU实例所在节点, 只有Running的时候有值
	Status                   string `json:"status"`                   // 状态信息, NoQuota/Pending/Allocated/Running/Releasing
	GPUIndex                 int    `json:"gpuIndex"`                 // 使用哪张物理卡
	GPUDeviceId              string `json:"gpuDeviceId"`              // 唯一标识某张物理卡
	PhysicalGpuSpecification string `json:"physicalGpuSpecification"` // 使用的物理卡型号
}

func (myGpu *VirtualGpuInstance) Name() string {
	return GPUShareName
}

//func (myGpu *VirtualGpuInstance) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
//
//}
