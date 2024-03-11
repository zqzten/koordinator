package v1beta1

type GPUDeviceSource struct {
	List     []CommonDeviceSpec `json:"list,omitempty"`
	Topology *GpuTopology       `json:"topology,omitempty"`
}

type GpuTopology struct {
	// 2D array of p2p linkMeta, indexed by device minor
	Links [][]GpuLink `json:"links,omitempty"`
}
type GpuLink struct {
	Type      GpuLinkType `json:"gpuLinkType,omitempty"`
	Bandwidth int64       `json:"bandwidth,omitempty"` // benchmarked with real tests
}

type GpuLinkType string

// Currently supported GPU p2p link types
const (
	FourNVLinks    GpuLinkType = "FourNVLinks"    // Four NVLinks, corresponding to "NV4" in the output of command `nvidia-smi topo -m`.
	ThreeNVLinks   GpuLinkType = "ThreeNVLinks"   // Three NVLinks, corresponding to "NV3"
	TwoNVLinks     GpuLinkType = "TwoNVLinks"     // Two NVLinks, corresponding to "NV2"
	SingleNVLink   GpuLinkType = "SingleNVLink"   // Single NVLink, corresponding to "NV"
	SameBoard      GpuLinkType = "SameBoard"      // On same board, corresponding to "PSB"
	SingleSwitch   GpuLinkType = "SingleSwitch"   // Need to traverse one PCIe switch to talk, corresponding to "PIX"
	MultipleSwitch GpuLinkType = "MultipleSwitch" // Need to traverse multiple PCIe switch to talk, corresponding to "PXB"
	HostBridge     GpuLinkType = "HostBridge"     // Talk over host bridge, corresponding to "PHB"
	SameCPUSocket  GpuLinkType = "SameCPUSocket"  // Connected to same CPU (Same NUMA node), corresponding to "NODE"
	CrossCPUSocket GpuLinkType = "CrossCPUSocket" // Cross CPU through socket-level link (e.g. QPI), corresponding to "SYS"
	UnknownType    GpuLinkType = "Unknown"
)
