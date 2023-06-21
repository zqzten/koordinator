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

import ()

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
