package intelligentscheduler

import (
	//CRDs "code.alipay.com/cnstack/intelligent-operator/api/v1"
	"fmt"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/intelligentscheduler/CRDs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"math"
	"strconv"
	"sync"
)

type IntelligentSchedulerArgs struct {
	// GPUMemoryScoreWeight is used to define the gpu memory score weight in score phase
	GPUMemoryScoreWeight *uint `json:"gpuMemoryScoreWeight"`
	// GPUUtilizationScoreWeight is used to define the gpu memory score weight in score phase
	GPUUtilizationScoreWeight *uint `json:"gpuUtilizationScoreWeight"`
	// NodeSelectorPolicy define the policy used to select a node in intelligent scheduler
	NodeSelectorPolicy string `json:"nodeSelectorPolicy"`
	// GpuSelectorPolicy define the policy used to select Gpus in a node in intelligent scheduler
	GpuSelectorPolicy string `json:"gpuSelectorPolicy"`
}

type NodeInfo struct {
	lock         sync.RWMutex
	isOversell   bool
	oversellRate int
	gpuCount     int
	gpuType      string
	gpuMem       int
}

func NewNodeInfo(node *corev1.Node) (*NodeInfo, error) {
	rateVal, ok := node.Labels[OversellRateNodeLabel]
	var isOversell bool
	var oversellRate int
	if !ok {
		isOversell = false
	} else {
		isOversell = true
		rate, err := strconv.Atoi(rateVal)
		if err != nil {
			return nil, err
		}
		oversellRate = rate
	}
	gpuCount, ok := node.Labels[PhysicalGpuCountNodeLabel]
	if !ok {
		return nil, fmt.Errorf("gpu count not found in node %v", node.Name)
	}
	count, err := strconv.Atoi(gpuCount)
	if err != nil {
		return nil, err
	}
	gpuType, ok := node.Labels[PhysicalGpuTypeNodeLabel]
	if !ok {
		return nil, fmt.Errorf("physical gpu type not found in node %v", node.Name)
	}
	memVal, ok := node.Labels[PhysicalGpuMemNodeLabel]
	if !ok {
		return nil, fmt.Errorf("physical gpu mem node not found in node %v", node.Name)
	}
	mem, err := strconv.Atoi(memVal[:len(memVal)-3])
	if err != nil {
		return nil, err
	}
	memGiB := int(math.Round(float64(mem) / float64(1024)))
	return &NodeInfo{
		isOversell:   isOversell,
		oversellRate: oversellRate,
		gpuCount:     count,
		gpuType:      gpuType,
		gpuMem:       memGiB,
	}, nil
}

func (info *NodeInfo) Reset(node *corev1.Node) {
	info.lock.Lock()
	defer info.lock.Unlock()
	rateVal, ok := node.Labels[OversellRateNodeLabel]
	var isOversell bool
	var oversellRate int
	if !ok {
		isOversell = false
	} else {
		isOversell = true
		rate, err := strconv.Atoi(rateVal)
		if err != nil {
			klog.Errorf("failed to parse oversell rate %v, err: %v", rate, err)
			return
		}
		oversellRate = rate
	}
	gpuCount, ok := node.Labels[PhysicalGpuCountNodeLabel]
	if !ok {
		klog.Errorf("gpu count not found in node %v", node.Name)
		return
	}
	count, err := strconv.Atoi(gpuCount)
	if err != nil {
		klog.Errorf("failed to parse gpu count %v, err: %v", gpuCount, err)
		return
	}
	gpuType, ok := node.Labels[PhysicalGpuTypeNodeLabel]
	if !ok {
		klog.Errorf("physical gpu type not found in node %v", node.Name)
		return
	}
	memVal, ok := node.Labels[PhysicalGpuMemNodeLabel]
	if !ok {
		klog.Errorf("physical gpu mem not found in node %v", node.Name)
		return
	}
	mem, err := strconv.Atoi(memVal[:len(memVal)-3])
	if err != nil {
		klog.Errorf("failed to parse physical gpu mem %v, err: %v", memVal, err)
		return
	}
	memGiB := int(float64(mem) / float64(1024))
	info.isOversell = isOversell
	info.oversellRate = oversellRate
	info.gpuCount = count
	info.gpuType = gpuType
	info.gpuMem = memGiB
}

func (info *NodeInfo) Clone() *NodeInfo {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return &NodeInfo{
		isOversell:   info.isOversell,
		oversellRate: info.oversellRate,
		gpuCount:     info.gpuCount,
		gpuType:      info.gpuType,
		gpuMem:       info.gpuMem,
	}
}

func (info *NodeInfo) getIsOversell() bool {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.isOversell
}

func (info *NodeInfo) getOversellRate() int {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.oversellRate
}

func (info *NodeInfo) getGpuCount() int {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.gpuCount
}

func (info *NodeInfo) getGpuType() string {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.gpuType
}

func (info *NodeInfo) getGpuMem() int {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.gpuMem
}

type PhysicalGpuSpecInfo struct {
	lock         *sync.RWMutex
	name         string
	vendor       string
	resourceName string
}

func NewPhysicalGpuSpecInfo(pgs *CRDs.PhysicalGpuSpecification) *PhysicalGpuSpecInfo {
	return &PhysicalGpuSpecInfo{
		lock:         new(sync.RWMutex),
		name:         pgs.Name,
		vendor:       pgs.Vendor,
		resourceName: pgs.ResourceName,
	}
}

func (info *PhysicalGpuSpecInfo) getName() string {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.name
}

func (info *PhysicalGpuSpecInfo) getVendor() string {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.vendor
}

func (info *PhysicalGpuSpecInfo) getResourceName() string {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.resourceName
}

func (info *PhysicalGpuSpecInfo) Clone() *PhysicalGpuSpecInfo {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return &PhysicalGpuSpecInfo{
		lock:         new(sync.RWMutex),
		name:         info.name,
		vendor:       info.vendor,
		resourceName: info.resourceName,
	}
}

type VirtualGpuSpecInfo struct {
	lock                      *sync.RWMutex
	name                      string
	nickName                  string
	physicalGpuSpecifications []*PhysicalGpuSpecInfo
	description               string
	gpuMemory                 int32
	gpuMemoryIsolation        bool
	gpuUtilization            int32
	gpuUtilizationIsolation   bool
	isOversell                bool
	isActive                  bool
}

func NewVirtualGpuSpecInfo(vgs *CRDs.VirtualGpuSpecification) *VirtualGpuSpecInfo {
	pgs := vgs.Spec.PhysicalGpuSpecifications
	var physicalGpuSpecs []*PhysicalGpuSpecInfo
	for _, pg := range pgs {
		physicalGpuSpecs = append(physicalGpuSpecs, NewPhysicalGpuSpecInfo(&pg))
	}
	return &VirtualGpuSpecInfo{
		lock:                      new(sync.RWMutex),
		name:                      vgs.Name,
		nickName:                  vgs.Spec.NickName,
		physicalGpuSpecifications: physicalGpuSpecs,
		description:               vgs.Spec.Description,
		gpuMemory:                 vgs.Spec.GPUMemory,
		gpuMemoryIsolation:        vgs.Spec.GPUMemoryIsolation,
		gpuUtilization:            vgs.Spec.GPUUtilization,
		gpuUtilizationIsolation:   vgs.Spec.GPUUtilizationIsolation,
		isOversell:                vgs.Spec.IsOversell,
		isActive:                  vgs.Status.IsActive,
	}
}

func (info *VirtualGpuSpecInfo) Reset(vgs *CRDs.VirtualGpuSpecification) {
	info.lock.Lock()
	defer info.lock.Unlock()
	pgs := vgs.Spec.PhysicalGpuSpecifications
	var physicalGpuSpecs []*PhysicalGpuSpecInfo
	for _, pg := range pgs {
		physicalGpuSpecs = append(physicalGpuSpecs, NewPhysicalGpuSpecInfo(&pg))
	}
	info.name = vgs.Name
	info.nickName = vgs.Spec.NickName
	info.physicalGpuSpecifications = physicalGpuSpecs
	info.description = vgs.Spec.Description
	info.gpuMemory = vgs.Spec.GPUMemory
	info.gpuMemoryIsolation = vgs.Spec.GPUMemoryIsolation
	info.gpuUtilization = vgs.Spec.GPUUtilization
	info.gpuUtilizationIsolation = vgs.Spec.GPUUtilizationIsolation
	info.isOversell = vgs.Spec.IsOversell
	info.isActive = vgs.Status.IsActive
}

func (info *VirtualGpuSpecInfo) Clone() *VirtualGpuSpecInfo {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return &VirtualGpuSpecInfo{
		lock:                      new(sync.RWMutex),
		name:                      info.name,
		nickName:                  info.nickName,
		physicalGpuSpecifications: info.physicalGpuSpecifications,
		description:               info.description,
		gpuMemory:                 info.gpuMemory,
		gpuMemoryIsolation:        info.gpuMemoryIsolation,
		gpuUtilization:            info.gpuUtilization,
		gpuUtilizationIsolation:   info.gpuUtilizationIsolation,
		isOversell:                info.isOversell,
		isActive:                  info.isActive,
	}
}

func (info *VirtualGpuSpecInfo) getPhysicalGpuSpecifications() []*PhysicalGpuSpecInfo {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.physicalGpuSpecifications
}

func (info *VirtualGpuSpecInfo) getDescription() string {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.description
}

func (info *VirtualGpuSpecInfo) getGpuMemoryIsolation() bool {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.gpuMemoryIsolation
}

func (info *VirtualGpuSpecInfo) getGpuUtilizationIsolation() bool {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.gpuUtilizationIsolation
}

func (info *VirtualGpuSpecInfo) getIsOversell() bool {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.isOversell
}

func (info *VirtualGpuSpecInfo) getIsActive() bool {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.isActive
}

type VirtualGpuInstanceInfo struct {
	lock                     *sync.RWMutex
	Name                     string // 虚拟GPU实例对应vgi crd的name
	VirtualGpuSpecification  string // 虚拟GPU实例的规格crd名称
	Pod                      string // 虚拟GPU实例所属的pod
	Node                     string // 虚拟GPU实例所在节点, 只有Running的时候有值
	Status                   string // 状态信息, NoQuota/Pending/Allocated/Running/Releasing
	GPUIndex                 int    // 使用哪张物理卡
	MemAllocated             int32
	PercentageAllocated      int32
	IsOversell               bool
	PhysicalGpuSpecification string // 使用的物理卡型号
}

func NewVirtualGpuInstanceInfo(vgi *CRDs.VirtualGpuInstance) *VirtualGpuInstanceInfo {
	return &VirtualGpuInstanceInfo{
		lock:                     new(sync.RWMutex),
		Name:                     vgi.Name,
		VirtualGpuSpecification:  vgi.Spec.VirtualGpuSpecification,
		Pod:                      vgi.Labels[VgiPodNameLabel] + "/" + vgi.Labels[VgiPodNamespaceLabel],
		Status:                   vgi.Status.Phase,
		Node:                     vgi.Status.Node,
		GPUIndex:                 vgi.Status.GPUIndex,
		PhysicalGpuSpecification: vgi.Status.PhysicalGpuSpecification,
		IsOversell:               vgi.Spec.IsOversell,
		MemAllocated:             vgi.Spec.GPUMemory,
		PercentageAllocated:      vgi.Spec.GPUUtilization,
	}
}

func (info *VirtualGpuInstanceInfo) Reset(vgi *CRDs.VirtualGpuInstance) {
	info.lock.Lock()
	defer info.lock.Unlock()
	info.Name = vgi.Name
	info.VirtualGpuSpecification = vgi.Spec.VirtualGpuSpecification
	info.Pod = vgi.Labels[VgiPodNameLabel] + "/" + vgi.Labels[VgiPodNamespaceLabel]
	info.Status = vgi.Status.Phase
	info.Node = vgi.Status.Node
	info.GPUIndex = vgi.Status.GPUIndex
	info.PhysicalGpuSpecification = vgi.Status.PhysicalGpuSpecification
	info.IsOversell = vgi.Spec.IsOversell
	info.MemAllocated = vgi.Spec.GPUMemory
	info.PercentageAllocated = vgi.Spec.GPUUtilization
}

func (info *VirtualGpuInstanceInfo) Clone() *VirtualGpuInstanceInfo {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return &VirtualGpuInstanceInfo{
		lock:                     new(sync.RWMutex),
		Name:                     info.Name,
		VirtualGpuSpecification:  info.VirtualGpuSpecification,
		Pod:                      info.Pod,
		Node:                     info.Node,
		GPUIndex:                 info.GPUIndex,
		PhysicalGpuSpecification: info.PhysicalGpuSpecification,
		IsOversell:               info.IsOversell,
		Status:                   info.Status,
		MemAllocated:             info.MemAllocated,
		PercentageAllocated:      info.PercentageAllocated,
	}
}

func (info *VirtualGpuInstanceInfo) getGPUIndex() int {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.GPUIndex
}

func (info *VirtualGpuInstanceInfo) getPod() string {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.Pod
}

func (info *VirtualGpuInstanceInfo) getNode() string {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.Node
}

func (info *VirtualGpuInstanceInfo) getName() string {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.Name
}

func (info *VirtualGpuInstanceInfo) getStatus() string {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.Status
}

func (info *VirtualGpuInstanceInfo) getMemAllocated() int {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return int(info.MemAllocated)
}

func (info *VirtualGpuInstanceInfo) getPercentageAllocated() int {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return int(info.PercentageAllocated)
}

func (info *VirtualGpuInstanceInfo) getVgs() string {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.VirtualGpuSpecification
}

func (info *VirtualGpuInstanceInfo) getIsOversell() bool {
	info.lock.RLock()
	defer info.lock.RUnlock()
	return info.IsOversell
}

func (info *VirtualGpuInstanceInfo) setGPUIndex(i int) {
	info.lock.Lock()
	defer info.lock.Unlock()
	info.GPUIndex = i
}

func (info *VirtualGpuInstanceInfo) setNode(nodeName string) {
	info.lock.Lock()
	defer info.lock.Unlock()
	info.Node = nodeName
}

func (info *VirtualGpuInstanceInfo) setStatus(status string) {
	info.lock.Lock()
	defer info.lock.Unlock()
	info.Status = status
}

func (info *VirtualGpuInstanceInfo) setPhysicalGpuSpecification(physicalGpuSpecification string) {
	info.lock.Lock()
	defer info.lock.Unlock()
	info.PhysicalGpuSpecification = physicalGpuSpecification
}
