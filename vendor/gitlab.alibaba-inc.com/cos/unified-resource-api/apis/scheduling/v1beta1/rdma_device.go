package v1beta1

//const (
//	RDMADeviceExtendInfoKey = "rdmaDeviceExtendInfo"
//)

type RDMADeviceInfos []*RDMADeviceInfo

type RDMADeviceInfo struct {
	Name       string   `json:"name,omitempty"`   // e.g., "mlx5_bond_0"
	Minor      int32    `json:"minor,omitempty"`  // e.g., 0
	UVerbs     string   `json:"uVerbs,omitempty"` //e.g., "/dev/infiniband/uverbs1"
	Bond       uint32   `json:"bond,omitempty"`
	BondSlaves []string `json:"bondSlaves,omitempty"` //e.g., ["eth1","eth2"]
}

func NewRDMADeviceInfo(name, uverbs string, bond uint32, bondSlaves []string) *RDMADeviceInfo {
	return &RDMADeviceInfo{
		Name:       name,
		UVerbs:     uverbs,
		Bond:       bond,
		BondSlaves: bondSlaves,
	}
}
