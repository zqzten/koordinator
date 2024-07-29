package CRDs

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
	GPUMemory                 int32    `json:"gpuMemory"`                 // 显存大小
	GPUMemoryIsolation        bool     `json:"gpuMemoryIsolation"`        // 显存隔离
	GPUUtilization            int32    `json:"gpuUtilization"`            // 算力比例
	GPUUtilizationIsolation   bool     `json:"gpuUtilizationIsolation"`   // 算力隔离
	IsOversell                bool     `json:"isOversell"`                // 是否超卖
}

type VirtualGpuSpecificationStatus struct {
	IsActive bool `json:"isActive"` // 规格是否可用, 不可用的情况下将暂停卡分配
}

// +kubebuilder:object:root=true

// VirtualGpuSpecificationList contains a list of VirtualGpuSpecification
type VirtualGpuSpecificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualGpuSpecification `json:"items"`
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
	Spec              VirtualGpuInstanceSpec   `json:"spec,omitempty"`
	Status            VirtualGpuInstanceStatus `json:"status,omitempty"`
}

type VirtualGpuInstanceSpec struct {
	VirtualGpuSpecification string `json:"virtualGpuSpecification"` // 虚拟GPU实例的规格
	GPUMemory               int32  `json:"gpuMemory,omitempty"`
	GPUUtilization          int32  `json:"gpuUtilization,omitempty"`
	IsOversell              bool   `json:"isOversell,omitempty"`
}

type VirtualGpuInstanceStatus struct {
	Pod                      string `json:"podUid,omitempty"`                   // 虚拟GPU实例所属的pod
	Node                     string `json:"node,omitempty"`                     // 虚拟GPU实例所在节点, 只有Running的时候有值
	Phase                    string `json:"phase"`                              // 状态信息, Pending/PreAllocated/Allocated/Running 后端设置
	GPUIndex                 int    `json:"gpuIndex,omitempty"`                 // 使用哪张物理卡
	PhysicalGpuSpecification string `json:"physicalGpuSpecification,omitempty"` // 使用的物理卡型号
	ContainerIndex           int    `json:"countainerIndex,omitempty"`
}

type VirtualGpuInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualGpuInstance `json:"items"`
}

//type PhysicalGpuInstance struct {
//	metav1.TypeMeta   `json:",inline"`
//	metav1.ObjectMeta `json:"metadata,omitempty"`
//	Spec              PhysicalGpuInstanceSpec   `json:"spec"`
//	Status            PhysicalGpuInstanceStatus `json:"status"`
//}
//
//type PhysicalGpuInstanceSpec struct {
//	PhysicalGpuSpecification string `json:"physicalGpuSpecification"` // 物理GPU型号
//	TotalMemory              int    `json:"gpuMem"`
//}
//
//type PhysicalGpuInstanceStatus struct {
//	Node                 string `json:"node"`                 // 所在node
//	Index                int    `json:"index"`                // 在所在node上的index
//	UsedMemory           int    `json:"usedMemory"`           // 已分配显存
//	UsedUtilization      int    `json:"usedUtilization"`      // 已分配算力
//	MemoryIsolation      bool   `json:"memoryIsolation"`      // 是否显存隔离
//	UtilizationIsolation bool   `json:"utilizationIsolation"` // 是否算力隔离
//}
//
//type PhysicalGpuInstanceList struct {
//	metav1.TypeMeta `json:",inline"`
//	metav1.ListMeta `json:"metadata,omitempty"`
//	Items           []PhysicalGpuInstance `json:"items"`
//}
