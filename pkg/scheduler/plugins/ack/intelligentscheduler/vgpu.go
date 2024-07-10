package intelligentscheduler

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:spec
// +kubebuilder:printcolumn:name="PhysicalGpuSpecifications",type="string",JSONPath=".spec.physicalGpuSpecifications",description="Real GPU Specification behind this virtual gpu"
// +kubebuilder:printcolumn:name="GPUMemory",type="string",JSONPath=".spec.gpuMemory",description="Gpu memory"
// +kubebuilder:printcolumn:name="GPUMemoryIsolation",type="string",JSONPath=".spec.gpuMemoryIsolation",description="Gpu memory isolation on"
// +kubebuilder:printcolumn:name="GPUUtilization",type="string",JSONPath=".spec.gpuUtilization.",description="Gpu Utilizaion"
// +kubebuilder:printcolumn:name="GPUUtilizationIsolation",type="string",JSONPath=".spec.gpuUtilizationIsolation.",description="Gpu Utilizaion isolation on"
// +kubebuilder:resource:scope=Namespaced,shortName=vgs
type VirtualGpuSpecification struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualGpuSpecificationSpec   `json:"spec"`
	Status            VirtualGpuSpecificationStatus `json:"status"`
}

type VirtualGpuSpecificationSpec struct {
	NickName                  string   `json:"nickName"`                  //虚拟GPU名称
	PhysicalGpuSpecifications []string `json:"physicalGpuSpecifications"` // 虚拟GPU所指定的物理GPU型号
	Description               string   `json:"description"`               // 规格描述
	GPUMemory                 int      `json:"gpuMemory"`                 // 显存大小
	GPUMemoryIsolation        bool     `json:"gpuMemoryIsolation"`        // 显存隔离
	GPUUtilization            int      `json:"gpuUtilization"`            // 算力比例
	GPUUtilizationIsolation   bool     `json:"gpuUtilizationIsolation"`   // 算力隔离
}

type VirtualGpuSpecificationStatus struct {
	IsActive bool `json:"isActive"` // 规格是否可用, 不可用的情况下将暂停卡分配
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:spec
// +kubebuilder:printcolumn:name="Specification",type="string",JSONPath=".spec.virtualGpuSpecification.",description="VirtualGpuSpecification"
// +kubebuilder:printcolumn:name="GPUIndex",type="string",JSONPath=".status.gpuIndex.",description="The physical gpu index"
// +kubebuilder:printcolumn:name="PhysicalGpuSpecification",type="string",JSONPath=".status.physicalGpuSpecification.",description="PhysicalGpuSpecification"
// +kubebuilder:printcolumn:name="Node",type="string",JSONPath=".status.node.",description="Node"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status.",description="status"
// +kubebuilder:resource:scope=Namespaced,shortName=vgi
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
	Pod                      string `json:"podUid"`                   // 虚拟GPU实例所属的pod
	Node                     string `json:"node"`                     // 虚拟GPU实例所在节点, 只有Running的时候有值
	Status                   string `json:"status"`                   // 状态信息, NoQuota/Pending/Allocated/Running/Releasing
	GPUIndex                 int    `json:"gpuIndex"`                 // 使用哪张物理卡
	GPUDeviceId              string `json:"gpuDeviceId"`              // 唯一标识某张物理卡
	PhysicalGpuSpecification string `json:"physicalGpuSpecification"` // 使用的物理卡型号
}
