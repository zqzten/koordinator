package ackplugins

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DsaAccelerateExtKey    = "dsaAccelerate"
	DsaAccelerateConfigKey = "dsa-accelerate-config"
)

type DsaAccelerateConfig struct {
	// polling mode indicates the dma migrate mode. If true, polling mode in use and
	// interrupt mode orelse, default = false
	PollingMode *bool `json:"pollingMode,omitempty"`
}
type DsaAccelerateStrategy struct {
	// enable dsa if true, disable dsa if false, defult = true
	Enable              *bool `json:"enable,omitempty"`
	DsaAccelerateConfig `json:",inline"`
}
type NodeDsaAccelerateCfg struct {
	// an empty label selector matches all objects while a nil label selector matches no objects
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	*DsaAccelerateStrategy
}
type DsaAccelerateCfg struct {
	ClusterStrategy *DsaAccelerateStrategy `json:"clusterStrategy,omitempty"`
	NodeStrategies  []NodeDsaAccelerateCfg `json:"nodeStrategies,omitempty"`
}

func (in *DsaAccelerateConfig) DeepCopyInto(out *DsaAccelerateConfig) {
	*out = *in
	if in.PollingMode != nil {
		in, out := &in.PollingMode, &out.PollingMode
		*out = new(bool)
		**out = **in
	}
}

func (in *DsaAccelerateConfig) DeepCopy() *DsaAccelerateConfig {
	if in == nil {
		return nil
	}
	out := new(DsaAccelerateConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *DsaAccelerateStrategy) DeepCopyInto(out *DsaAccelerateStrategy) {
	*out = *in
	in.DsaAccelerateConfig.DeepCopyInto(&out.DsaAccelerateConfig)
	if in.Enable != nil {
		in, out := &in.Enable, &out.Enable
		*out = new(bool)
		**out = **in
	}
}

func (in *DsaAccelerateStrategy) DeepCopy() *DsaAccelerateStrategy {
	if in == nil {
		return nil
	}
	out := new(DsaAccelerateStrategy)
	in.DeepCopyInto(out)
	return out
}

func (in *DsaAccelerateCfg) DeepCopyInto(out *DsaAccelerateCfg) {
	*out = *in
	if in.ClusterStrategy != nil {
		in, out := &in.ClusterStrategy, &out.ClusterStrategy
		*out = new(DsaAccelerateStrategy)
		(*in).DeepCopyInto(*out)
	}
	if in.NodeStrategies != nil {
		in, out := &in.NodeStrategies, &out.NodeStrategies
		*out = make([]NodeDsaAccelerateCfg, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *DsaAccelerateCfg) DeepCopy() *DsaAccelerateCfg {
	if in == nil {
		return nil
	}
	out := new(DsaAccelerateCfg)
	in.DeepCopyInto(out)
	return out
}

func (in *NodeDsaAccelerateCfg) DeepCopyInto(out *NodeDsaAccelerateCfg) {
	*out = *in
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = new(metav1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.DsaAccelerateStrategy != nil {
		in, out := &in.DsaAccelerateStrategy, &out.DsaAccelerateStrategy
		*out = new(DsaAccelerateStrategy)
		(*in).DeepCopyInto(*out)
	}
}

func (in *NodeDsaAccelerateCfg) DeepCopy() *NodeDsaAccelerateCfg {
	if in == nil {
		return nil
	}
	out := new(NodeDsaAccelerateCfg)
	in.DeepCopyInto(out)
	return out
}
