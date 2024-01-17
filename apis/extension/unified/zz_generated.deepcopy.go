//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by controller-gen. DO NOT EDIT.

package unified

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeadlineEvictCfg) DeepCopyInto(out *DeadlineEvictCfg) {
	*out = *in
	if in.ClusterStrategy != nil {
		in, out := &in.ClusterStrategy, &out.ClusterStrategy
		*out = new(DeadlineEvictStrategy)
		(*in).DeepCopyInto(*out)
	}
	if in.NodeStrategies != nil {
		in, out := &in.NodeStrategies, &out.NodeStrategies
		*out = make([]NodeDeadlineEvictStrategy, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeadlineEvictCfg.
func (in *DeadlineEvictCfg) DeepCopy() *DeadlineEvictCfg {
	if in == nil {
		return nil
	}
	out := new(DeadlineEvictCfg)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeadlineEvictConfig) DeepCopyInto(out *DeadlineEvictConfig) {
	*out = *in
	if in.DeadlineDuration != nil {
		in, out := &in.DeadlineDuration, &out.DeadlineDuration
		*out = new(v1.Duration)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeadlineEvictConfig.
func (in *DeadlineEvictConfig) DeepCopy() *DeadlineEvictConfig {
	if in == nil {
		return nil
	}
	out := new(DeadlineEvictConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeadlineEvictStrategy) DeepCopyInto(out *DeadlineEvictStrategy) {
	*out = *in
	if in.Enable != nil {
		in, out := &in.Enable, &out.Enable
		*out = new(bool)
		**out = **in
	}
	in.DeadlineEvictConfig.DeepCopyInto(&out.DeadlineEvictConfig)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeadlineEvictStrategy.
func (in *DeadlineEvictStrategy) DeepCopy() *DeadlineEvictStrategy {
	if in == nil {
		return nil
	}
	out := new(DeadlineEvictStrategy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DynamicProdResourceConfig) DeepCopyInto(out *DynamicProdResourceConfig) {
	*out = *in
	if in.ProdOvercommitPolicy != nil {
		in, out := &in.ProdOvercommitPolicy, &out.ProdOvercommitPolicy
		*out = new(ProdOvercommitPolicy)
		**out = **in
	}
	if in.ProdCPUOvercommitDefaultPercent != nil {
		in, out := &in.ProdCPUOvercommitDefaultPercent, &out.ProdCPUOvercommitDefaultPercent
		*out = new(int64)
		**out = **in
	}
	if in.ProdCPUOvercommitMaxPercent != nil {
		in, out := &in.ProdCPUOvercommitMaxPercent, &out.ProdCPUOvercommitMaxPercent
		*out = new(int64)
		**out = **in
	}
	if in.ProdCPUOvercommitMinPercent != nil {
		in, out := &in.ProdCPUOvercommitMinPercent, &out.ProdCPUOvercommitMinPercent
		*out = new(int64)
		**out = **in
	}
	if in.ProdMemoryOvercommitDefaultPercent != nil {
		in, out := &in.ProdMemoryOvercommitDefaultPercent, &out.ProdMemoryOvercommitDefaultPercent
		*out = new(int64)
		**out = **in
	}
	if in.ProdMemoryOvercommitMaxPercent != nil {
		in, out := &in.ProdMemoryOvercommitMaxPercent, &out.ProdMemoryOvercommitMaxPercent
		*out = new(int64)
		**out = **in
	}
	if in.ProdMemoryOvercommitMinPercent != nil {
		in, out := &in.ProdMemoryOvercommitMinPercent, &out.ProdMemoryOvercommitMinPercent
		*out = new(int64)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DynamicProdResourceConfig.
func (in *DynamicProdResourceConfig) DeepCopy() *DynamicProdResourceConfig {
	if in == nil {
		return nil
	}
	out := new(DynamicProdResourceConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeDeadlineEvictStrategy) DeepCopyInto(out *NodeDeadlineEvictStrategy) {
	*out = *in
	in.NodeCfgProfile.DeepCopyInto(&out.NodeCfgProfile)
	if in.DeadlineEvictStrategy != nil {
		in, out := &in.DeadlineEvictStrategy, &out.DeadlineEvictStrategy
		*out = new(DeadlineEvictStrategy)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeDeadlineEvictStrategy.
func (in *NodeDeadlineEvictStrategy) DeepCopy() *NodeDeadlineEvictStrategy {
	if in == nil {
		return nil
	}
	out := new(NodeDeadlineEvictStrategy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeDynamicProdConfig) DeepCopyInto(out *NodeDynamicProdConfig) {
	*out = *in
	in.DynamicProdResourceConfig.DeepCopyInto(&out.DynamicProdResourceConfig)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeDynamicProdConfig.
func (in *NodeDynamicProdConfig) DeepCopy() *NodeDynamicProdConfig {
	if in == nil {
		return nil
	}
	out := new(NodeDynamicProdConfig)
	in.DeepCopyInto(out)
	return out
}
