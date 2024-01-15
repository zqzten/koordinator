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
	"strings"

	v1 "k8s.io/api/core/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/features"
)

type NodeLocalVolumeInfo struct {
	// SystemDisk is a local volume that also provides ephemeral storage for containers.
	SystemDisk *LocalVolume

	// LocalVolumes tells the information of the local volumes.
	// Key is host path.
	// This excludes system disk.
	LocalVolumes map[string]*LocalVolume
}

// LocalVolume tells the information of a local volume.
type LocalVolume struct {
	DiskType string // SSD, HDD
	HostPath string
	Capacity int64 // The size of volume capacity.
	// Labels is the volume's labels Used to select volumes.
	// As a note, in sigma, this is from LogicInfo, while capacity is from LocalInfo.
	Labels map[string]string
	// iolimit of volume
	IOLimit *unified.LocalStorageIOLimitInfo
}

func newNodeLocalVolumeInfo(node *v1.Node) (*NodeLocalVolumeInfo, error) {
	localInfo, err := unified.LocalInfoFromNode(node)
	if err != nil {
		return nil, err
	}

	ephemeralStorageQuantity := node.Status.Allocatable[v1.ResourceEphemeralStorage]
	localVolumes := make(map[string]*LocalVolume)
	graphVolume := &LocalVolume{
		Capacity: ephemeralStorageQuantity.Value(),
	}
	if localInfo != nil {
		for _, diskInfo := range localInfo.DiskInfos {
			if diskInfo.IsGraphDisk {
				if diskInfo.Size != ephemeralStorageQuantity.Value() {
					klog.V(5).Infof("node %s graph disk size diff, [diskInfo Size = %d, nodeAllocatable Size = %d]", node.Name, diskInfo.Size, ephemeralStorageQuantity.Value())
				}
				graphVolume.DiskType = string(diskInfo.DiskType)
				graphVolume.HostPath = diskInfo.MountPoint
				continue
			}
			if strings.HasPrefix(diskInfo.Device, "/dev/nbd") ||
				strings.HasPrefix(diskInfo.Device, "/dev/vrbd") ||
				strings.HasPrefix(diskInfo.Device, "/dev/loop") {
				continue
			}
			if diskInfo.FileSystemType == "tmpfs" ||
				diskInfo.FileSystemType == "devtmpfs" ||
				diskInfo.FileSystemType == "overlay" {
				continue
			}
			vol := &LocalVolume{
				DiskType: string(diskInfo.DiskType),
				HostPath: diskInfo.MountPoint,
				Capacity: diskInfo.Size,
			}
			localVolumes[vol.HostPath] = vol
		}
	}
	localVolumes[graphVolume.HostPath] = graphVolume

	localStorageInfo, err := unified.LocalStorageInfoFromNode(node)
	if err != nil {
		return nil, err
	}
	for _, storage := range localStorageInfo {
		switch storage.LocalStorageType {
		case unified.NodeLocalVolumeLVM, unified.NodeLocalLoopDevice:
			localVolume := &LocalVolume{
				HostPath: storage.Name, // LVM 类型无目录名称，直接使用vgName表示
				Capacity: storage.Capacity,
				Labels:   map[string]string{unified.LocalStorageType: storage.LocalStorageType},
				IOLimit:  &unified.LocalStorageIOLimitInfo{},
			}
			if !utilfeature.DefaultFeatureGate.Enabled(features.EnableLocalVolumeCapacity) {
				if _, ok := localVolumes[storage.Name]; ok {
					localVolume.Capacity = localVolumes[storage.Name].Capacity
				}
			}
			if utilfeature.DefaultFeatureGate.Enabled(features.EnableLocalVolumeIOLimit) {
				localVolume.IOLimit = &unified.LocalStorageIOLimitInfo{
					ReadIops:  storage.ReadIops,
					WriteIops: storage.WriteIops,
					ReadBps:   storage.ReadBps,
					WriteBps:  storage.WriteBps,
				}
			}
			localVolumes[storage.Name] = localVolume
		case unified.NodeLocalVolumeQuotaPath:
			localVolume := &LocalVolume{
				Capacity: storage.Capacity,
				DiskType: storage.Devicetype,
				HostPath: storage.MountPoint,
				Labels:   map[string]string{unified.LocalStorageType: storage.LocalStorageType},
				IOLimit:  &unified.LocalStorageIOLimitInfo{},
			}
			if !utilfeature.DefaultFeatureGate.Enabled(features.EnableLocalVolumeCapacity) {
				if _, ok := localVolumes[storage.MountPoint]; ok {
					localVolume.Capacity = localVolumes[storage.MountPoint].Capacity
				}
			}
			if utilfeature.DefaultFeatureGate.Enabled(features.EnableLocalVolumeIOLimit) {
				localVolume.IOLimit = &unified.LocalStorageIOLimitInfo{
					ReadIops:  storage.ReadIops,
					WriteIops: storage.WriteIops,
					ReadBps:   storage.ReadBps,
					WriteBps:  storage.WriteBps,
				}
			}
			localVolumes[storage.MountPoint] = localVolume
		case unified.NodeLocalVolumeDevice:
			localVolume := &LocalVolume{
				Capacity: storage.Capacity,
				DiskType: storage.Devicetype,
				HostPath: storage.Name,
				Labels:   map[string]string{unified.LocalStorageType: storage.LocalStorageType},
				IOLimit:  &unified.LocalStorageIOLimitInfo{},
			}
			if !utilfeature.DefaultFeatureGate.Enabled(features.EnableLocalVolumeCapacity) {
				if _, ok := localVolumes[storage.Name]; ok {
					localVolume.Capacity = localVolumes[storage.Name].Capacity
				}
			}
			if utilfeature.DefaultFeatureGate.Enabled(features.EnableLocalVolumeIOLimit) {
				localVolume.IOLimit = &unified.LocalStorageIOLimitInfo{
					ReadIops:  storage.ReadIops,
					WriteIops: storage.WriteIops,
					ReadBps:   storage.ReadBps,
					WriteBps:  storage.WriteBps,
				}
			}
			localVolumes[storage.Name] = localVolume
		}
	}

	return &NodeLocalVolumeInfo{
		SystemDisk:   graphVolume,
		LocalVolumes: localVolumes,
	}, nil
}
