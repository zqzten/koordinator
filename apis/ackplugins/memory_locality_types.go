package ackplugins

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	MemoryLocalityExtKey        = "memoryLocality"
	MemoryLocalityConfigKey     = "memory-locality-config"
	AnnotationPodMemoryLocality = extension.DomainPrefix + "memoryLocality"
)

// MemoryLocalityConfig enables memory locality features.
type MemoryLocalityConfig struct {
	// TargetLocalityRatio indicates threshold percentage of memory locality. (default:100)
	TargetLocalityRatio *int64 `json:"targetLocalityRatio,omitempty"`
	// MigrateIntervalMinutes indicates the interval between each migration and the unit is minute. (default:0)
	// MigrateIntervalMinutes = 0, do memory migration once if CurLocalPercent < TargetLocalityRatio.
	MigrateIntervalMinutes *int64 `json:"migrateIntervalMinutes,omitempty"`
}

type MemoryLocalityPolicy string

const (
	// MemoryLocalityPolicBesteffort indicates memory locality in limited migrate times
	MemoryLocalityPolicyBesteffort MemoryLocalityPolicy = "bestEffort"
	// MemoryLocalityPolicyNone indicates pod disables memory locality
	MemoryLocalityPolicyNone MemoryLocalityPolicy = "none"
)

func (policy MemoryLocalityPolicy) Pointer() *MemoryLocalityPolicy {
	new := policy
	return &new
}

type MemoryLocalityStrategy struct {
	// MemoryLocality for LSR pods.
	LSRClass *MemoryLocalityQOS `json:"lsrClass,omitempty"`

	// MemoryLocality for LS pods.
	LSClass *MemoryLocalityQOS `json:"lsClass,omitempty"`

	// MemoryLocalityfor BE pods.
	BEClass *MemoryLocalityQOS `json:"beClass,omitempty"`

	// MemoryLocalityfor system pods
	SystemClass *MemoryLocalityQOS `json:"systemClass,omitempty"`

	// MemoryLocalityfor root cgroup.
	CgroupRoot *MemoryLocalityQOS `json:"cgroupRoot,omitempty"`
}

type MemoryLocalityQOS struct {
	MemoryLocality *MemoryLocality `json:"memoryLocality,omitempty"`
}

type MemoryLocality struct {
	// Policy indicates the way to controll memory locality, default "None"
	Policy               *MemoryLocalityPolicy `json:"policy,omitempty"`
	MemoryLocalityConfig `json:",inline"`
}

type NodeMemoryLocalityCfg struct {
	// an empty label selector matches all objects while a nil label selector matches no objects
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	*MemoryLocalityStrategy
}

type MemoryLocalityCfg struct {
	ClusterStrategy *MemoryLocalityStrategy `json:"clusterStrategy,omitempty"`
	NodeStrategies  []NodeMemoryLocalityCfg `json:"nodeStrategies,omitempty"`
}

func (in *MemoryLocalityConfig) DeepCopy() *MemoryLocalityConfig {
	if in == nil {
		return nil
	}
	out := new(MemoryLocalityConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *MemoryLocalityConfig) DeepCopyInto(out *MemoryLocalityConfig) {
	*out = *in
	if in.TargetLocalityRatio != nil {
		in, out := &in.TargetLocalityRatio, &out.TargetLocalityRatio
		*out = new(int64)
		**out = **in
	}
	if in.MigrateIntervalMinutes != nil {
		in, out := &in.MigrateIntervalMinutes, &out.MigrateIntervalMinutes
		*out = new(int64)
		**out = **in
	}
}

func (in *MemoryLocality) DeepCopy() *MemoryLocality {
	if in == nil {
		return nil
	}
	out := new(MemoryLocality)
	in.DeepCopyInto(out)
	return out
}

func (in *MemoryLocality) DeepCopyInto(out *MemoryLocality) {
	*out = *in
	if in.Policy != nil {
		in, out := &in.Policy, &out.Policy
		*out = new(MemoryLocalityPolicy)
		**out = **in
	}
	in.MemoryLocalityConfig.DeepCopyInto(&out.MemoryLocalityConfig)
}

func (in *MemoryLocalityStrategy) DeepCopyInto(out *MemoryLocalityStrategy) {
	*out = *in
	if in.LSRClass != nil {
		in, out := &in.LSRClass, &out.LSRClass
		*out = new(MemoryLocalityQOS)
		(*in).DeepCopyInto(*out)
	}
	if in.LSClass != nil {
		in, out := &in.LSClass, &out.LSClass
		*out = new(MemoryLocalityQOS)
		(*in).DeepCopyInto(*out)
	}
	if in.BEClass != nil {
		in, out := &in.BEClass, &out.BEClass
		*out = new(MemoryLocalityQOS)
		(*in).DeepCopyInto(*out)
	}
	if in.SystemClass != nil {
		in, out := &in.SystemClass, &out.SystemClass
		*out = new(MemoryLocalityQOS)
		(*in).DeepCopyInto(*out)
	}
	if in.CgroupRoot != nil {
		in, out := &in.CgroupRoot, &out.CgroupRoot
		*out = new(MemoryLocalityQOS)
		(*in).DeepCopyInto(*out)
	}
}

func (in *MemoryLocalityQOS) DeepCopy() *MemoryLocalityQOS {
	if in == nil {
		return nil
	}
	out := new(MemoryLocalityQOS)
	in.DeepCopyInto(out)
	return out
}

func (in *MemoryLocalityQOS) DeepCopyInto(out *MemoryLocalityQOS) {
	*out = *in
	if in.MemoryLocality != nil {
		in, out := &in.MemoryLocality, &out.MemoryLocality
		*out = new(MemoryLocality)
		(*in).DeepCopyInto(*out)
	}
}

func (in *MemoryLocalityStrategy) DeepCopy() *MemoryLocalityStrategy {
	if in == nil {
		return nil
	}
	out := new(MemoryLocalityStrategy)
	in.DeepCopyInto(out)
	return out
}

func (in *MemoryLocalityCfg) DeepCopyInto(out *MemoryLocalityCfg) {
	*out = *in
	if in.ClusterStrategy != nil {
		in, out := &in.ClusterStrategy, &out.ClusterStrategy
		*out = new(MemoryLocalityStrategy)
		(*in).DeepCopyInto(*out)
	}
	if in.NodeStrategies != nil {
		in, out := &in.NodeStrategies, &out.NodeStrategies
		*out = make([]NodeMemoryLocalityCfg, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *MemoryLocalityCfg) DeepCopy() *MemoryLocalityCfg {
	if in == nil {
		return nil
	}
	out := new(MemoryLocalityCfg)
	in.DeepCopyInto(out)
	return out
}

func (in *NodeMemoryLocalityCfg) DeepCopyInto(out *NodeMemoryLocalityCfg) {
	*out = *in
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = new(metav1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.MemoryLocalityStrategy != nil {
		in, out := &in.MemoryLocalityStrategy, &out.MemoryLocalityStrategy
		*out = new(MemoryLocalityStrategy)
		(*in).DeepCopyInto(*out)
	}
}

func (in *NodeMemoryLocalityCfg) DeepCopy() *NodeMemoryLocalityCfg {
	if in == nil {
		return nil
	}
	out := new(NodeMemoryLocalityCfg)
	in.DeepCopyInto(out)
	return out
}
