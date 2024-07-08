package v1alpha1

type PodCgroupsInfo struct {
	PodNamespace string               `json:"podNamespace,omitempty"`
	PodName      string               `json:"podName,omitempty"`
	Containers   []*ContainerInfoSpec `json:"containers,omitempty"`
}

type NodeCgroupsInfo struct {
	// namespace of Cgroup CRD
	CgroupsNamespace string `json:"cgroupsNamespace,omitempty"`
	// name of Cgroup CRD
	CgroupsName string `json:"cgroupsName,omitempty"`
	// pod cgroup args to modify on corresponding node
	PodCgroups []*PodCgroupsInfo `json:"podCgroups,omitempty"`
}
