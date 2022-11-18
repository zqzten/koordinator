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

package volumebinding

import (
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

type LocalVolumeResource struct {
	VolumeSize map[string]int64
	IOLimits   map[string]*unified.LocalStorageIOLimitInfo
}

func newLocalVolumeResource() *LocalVolumeResource {
	return &LocalVolumeResource{
		VolumeSize: make(map[string]int64),
		IOLimits:   make(map[string]*unified.LocalStorageIOLimitInfo),
	}
}

func (r *LocalVolumeResource) UpdateLocalVolumeCapacity(volumeInfos map[string]*LocalVolume) {
	volumeSize := make(map[string]int64)
	ioLimits := make(map[string]*unified.LocalStorageIOLimitInfo)
	for _, info := range volumeInfos {
		volumeSize[info.HostPath] = info.Capacity
		if info.IOLimit == nil {
			continue
		}
		ioLimits[info.HostPath] = &unified.LocalStorageIOLimitInfo{
			ReadIops:  info.IOLimit.ReadIops,
			WriteIops: info.IOLimit.WriteIops,
			ReadBps:   info.IOLimit.ReadBps,
			WriteBps:  info.IOLimit.WriteBps,
		}
	}
	r.VolumeSize = volumeSize
	r.IOLimits = ioLimits
}

func (r *LocalVolumeResource) addLocalVolumeAlloc(systemDisk string, ephemeralStorageSize int64, localInlineVolumeSize int64) {
	total := ephemeralStorageSize + localInlineVolumeSize
	r.VolumeSize[systemDisk] += total
}

func (r *LocalVolumeResource) deleteLocalVolumeAlloc(systemDisk string, ephemeralStorageSize int64, localInlineVolumeSize int64) {
	total := ephemeralStorageSize + localInlineVolumeSize
	r.VolumeSize[systemDisk] -= total
}

func (r *LocalVolumeResource) CopyFrom(src *LocalVolumeResource) {
	for key := range r.VolumeSize {
		if _, ok := src.VolumeSize[key]; !ok {
			delete(r.VolumeSize, key)
		}
	}
	for key, value := range src.VolumeSize {
		r.VolumeSize[key] = value
	}
	for key := range r.IOLimits {
		if _, ok := src.IOLimits[key]; !ok {
			delete(r.IOLimits, key)
		}
	}
	for key, value := range src.IOLimits {
		r.IOLimits[key] = value.Copy()
	}
}

func (r *LocalVolumeResource) Copy() *LocalVolumeResource {
	newRes := newLocalVolumeResource()
	for k, v := range r.VolumeSize {
		newRes.VolumeSize[k] = v
	}
	for k, v := range r.IOLimits {
		newRes.IOLimits[k] = v.Copy()
	}
	return newRes
}

func (r *LocalVolumeResource) RealSubtract(rhs *LocalVolumeResource) {
	for hostPath, size := range rhs.VolumeSize {
		r.VolumeSize[hostPath] -= size
	}
	for hostPath, ioLimit := range rhs.IOLimits {
		r.IOLimits[hostPath].Sub(ioLimit)
	}
}
