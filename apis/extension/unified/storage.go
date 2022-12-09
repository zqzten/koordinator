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

package unified

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	storagelisters "k8s.io/client-go/listers/storage/v1"
)

const (
	// AnnSelectedDisk annotation is added to a PVC that has been triggered by scheduler to
	// be dynamically provisioned. Its value is the mount point of the selected disk.
	AnnSelectedDisk = "volume.kubernetes.io/selected-disk"
	// AnnSelectedStorage likes AnnSelectedDisk. Its value is the target selected disk device.
	AnnSelectedStorage = "volume.kubernetes.io/selected-storage"
	// LocalPVCSIName is built-in localPV class
	LocalPVCSIName = "csi-hostpath-dp"
	// LocalPVCSIProvisioner is the provisioner who provides an extended localPV class
	LocalPVCSIProvisioner = "localplugin.csi.alibabacloud.com"
	// LocalPVCAntRawBlockProvisioner is the provisioner used by ant rund rawfile which enabled iolimit check
	LocalPVCAntRawBlockProvisioner = "rawblock.alibabacloud.csi.alipay.com"
	// AnnotationStorageClassSupportLVMAndQuotaPath is the key of storageclass to specify if it support LVM or QuotaPath, value is true/false
	AnnotationStorageClassSupportLVMAndQuotaPath = "alibabacloud.com/support-lvm-quotapath"
	// AnnotationNodeLocalStorageTopologyKey is the key of node's annotation that records
	// the capacity of local storage , IOPS , BPS and so on.
	AnnotationNodeLocalStorageTopologyKey = "csi.alibabacloud.com/storage-topology"
	// LocalPVSourceIdentity means the StorageClass support Local PV
	LocalPVSourceIdentity = "ephemeral-storage"
	// AnnotationStorageSource declares the storage source be supported
	AnnotationStorageSource = "alibabacloud.com/storage-source"
	// InlineVolumeAttributeStorageClass declares the storage class
	InlineVolumeAttributeStorageClass = "alibabacloud.com/storage-class"
	// InlineVolumeAttributeStorageSize declares the required storage size
	InlineVolumeAttributeStorageSize = "alibabacloud.com/storage-size"
	// IOLimitAnnotationOnPVC declares iolimit values of pvc
	IOLimitAnnotationOnPVC = "alibabacloud.csi.alipay.com/pvc-iolimits"
)

const (
	// LocalStorageType declares the type of local storage
	LocalStorageType = "type"
	// CSILocalVolumeLVM declares the type of LocalVolume in storageClass is "LVM"
	CSILocalVolumeLVM = "LVM"
	// CSILocalVolumeQuotaPath declares the type of LocalVolume in storageClass is "QuotaPath"
	CSILocalVolumeQuotaPath = "QuotaPath"
	// CSILocalVolumeDevice declares the type of LocalVolume in storageClass is "Device"
	CSILocalVolumeDevice = "Device"
	// NodeLocalVolumeLVM declares the type of LocalVolume in node's annotation is "lvm"
	// eg: csi.alibabacloud.com/storage-capacity:
	// [{"type": "volumegroup", "name": "vg1", "capacity": 105151127552}]
	NodeLocalVolumeLVM = "lvm"
	// NodeLocalVolumeQuotaPath declares the type of LocalVolume in node's annotation is "quotapath"
	// eg: csi.alibabacloud.com/storage-capacity:
	//  [{"type": "quotapath", "name": "quotapath1", "capacity": 105151127552, "devicetype": "ssd", mountPoint": "/path1"}]
	NodeLocalVolumeQuotaPath = "quotapath"
	// NodeLocalVolumeDevice declares the type of LocalVolume is "device"
	NodeLocalVolumeDevice = "device"
)

const (
	// CSIVolumeType represents the local storage type (LVM or QuotaPath)
	CSIVolumeType = "volumeType"
	// CSILVMVolumeGroupName represents the name of volumegroup, it must
	// exist when volumeType = "LVM
	CSILVMVolumeGroupName = "vgName"
	// 	LVMCSIFileSystemType represents the type of formal file system
	CSILVMFileSystemType = "fsType"
	// CSILVMType generates the type of LVM, support linear, striping
	CSILVMType = "lvmType"
	// CSIQuotaPathRootPath indicates the name of the directory where QuotaPath resides.
	CSIQuotaPathRootPath = "rootPath"
)

const (
	// AnnotationCSIVolumeTopology indicates the relationship between pv and node
	AnnotationCSIVolumeTopology = "csi.alibabacloud.com/volume-topology"

	// LabelNodeDiskPrefix is the type prefix of disk on node
	LabelNodeDiskPrefix = "node.csi.alibabacloud.com/disktype."

	ACKDiskPlugin = "diskplugin.csi.alibabacloud.com"
	// ACKCloudStorageType declares the type of cloud storage
	ACKCloudStorageType = "type"

	AnnotationUltronDiskUseCapabilities = "alibabacloud.com/ultron-disk-use-capabilities"
	UltronDiskCategory                  = "category"
)

type LocalStorageInfo struct {
	// LocalStorageType indicates the type of local storage
	LocalStorageType string `json:"type"`
	// Name indicates the name of a volumegroup
	Name string `json:"name,omitempty"`
	// Capaicty indicates the capacity of volume
	Capacity int64 `json:"capacity,omitempty"`
	// Devicetype indicates the type of device like ssd
	Devicetype string `json:"deviceType,omitempty"`
	// MountPoint identifies the root directory where the volume resides
	MountPoint string `json:"mountPoint,omitempty"`
	// ReadIOps indicates read io operation count per second of volume
	ReadIops int64 `json:"readIops,omitempty"`
	// ReadBps indicates read io operation byte per second of volume
	ReadBps int64 `json:"readBps,omitempty"`
	// WriteIOps indicates write io operation count per second of volume
	WriteIops int64 `json:"writeIops,omitempty"`
	// WriteBps indicates write io operation byte per second of volume
	WriteBps int64 `json:"writeBps,omitempty"`
}

// LocalStorageIOLimitInfo indicates iolimit values of pvc
type LocalStorageIOLimitInfo struct {
	// ReadIops indicates read io operation count per second of volume
	ReadIops int64 `json:"readIops,omitempty"`
	// ReadBps indicates read io operation byte per second of volume
	ReadBps int64 `json:"readBps,omitempty"`
	// WriteIops indicates write io operation count per second of volume
	WriteIops int64 `json:"writeIops,omitempty"`
	// WriteBps indicates write io operation byte per second of volume
	WriteBps int64 `json:"writeBps,omitempty"`
}

func (a *LocalStorageIOLimitInfo) Add(b *LocalStorageIOLimitInfo) {
	if a == nil || b == nil {
		return
	}
	a.ReadIops += b.ReadIops
	a.WriteIops += b.WriteIops
	a.ReadBps += b.ReadBps
	a.WriteBps += b.WriteBps
}

func (a *LocalStorageIOLimitInfo) Sub(b *LocalStorageIOLimitInfo) {
	if a == nil || b == nil {
		return
	}
	a.ReadIops = MaxInt64(a.ReadIops-b.ReadIops, 0)
	a.WriteIops = MaxInt64(a.WriteIops-b.WriteIops, 0)
	a.ReadBps = MaxInt64(a.ReadBps-b.ReadBps, 0)
	a.WriteBps = MaxInt64(a.WriteBps-b.WriteBps, 0)
}

// Equal condition: both a and b is nil or all iolimit values are the same
func (a *LocalStorageIOLimitInfo) Equal(b *LocalStorageIOLimitInfo) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.ReadIops == b.ReadIops && a.WriteIops == b.WriteIops &&
		a.ReadBps == b.ReadBps && a.WriteBps == b.WriteBps
}

func (a *LocalStorageIOLimitInfo) Copy() *LocalStorageIOLimitInfo {
	return &LocalStorageIOLimitInfo{
		ReadIops:  a.ReadIops,
		WriteIops: a.WriteIops,
		ReadBps:   a.ReadBps,
		WriteBps:  a.WriteBps,
	}
}

func (a *LocalStorageIOLimitInfo) SetMaxValue(b *LocalStorageIOLimitInfo) {
	if a == nil || b == nil {
		return
	}
	a.ReadIops = MaxInt64(a.ReadIops, b.ReadIops)
	a.WriteIops = MaxInt64(a.WriteIops, b.WriteIops)
	a.ReadBps = MaxInt64(a.ReadBps, b.ReadBps)
	a.WriteBps = MaxInt64(a.WriteBps, b.WriteBps)
}

func (a *LocalStorageIOLimitInfo) IsZero() bool {
	if a == nil {
		return true
	}
	return a.ReadIops == 0 && a.WriteIops == 0 && a.ReadBps == 0 && a.WriteBps == 0
}

func MaxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func CalcLocalInlineVolumeSize(volumes []corev1.Volume, storageClassLister storagelisters.StorageClassLister) int64 {
	if len(volumes) == 0 {
		return 0
	}
	var totalSize int64
	for i := range volumes {
		volume := &volumes[i]
		if volume.CSI == nil {
			continue
		}
		storageClassName := volume.CSI.VolumeAttributes[InlineVolumeAttributeStorageClass]
		if !SupportLocalPV(storageClassLister, storageClassName, nil) {
			continue
		}
		storageSizeText, ok := volume.CSI.VolumeAttributes[InlineVolumeAttributeStorageSize]
		if !ok {
			continue
		}
		quantity, err := apiresource.ParseQuantity(storageSizeText)
		if err == nil {
			totalSize += quantity.Value()
		}
	}
	return totalSize
}

func SupportLocalPV(classLister storagelisters.StorageClassLister, className string, storageClass *storagev1.StorageClass) bool {
	if className == "" {
		return false
	}
	if className == LocalPVCSIName {
		return true
	}
	if storageClass == nil {
		var err error
		storageClass, err = classLister.Get(className)
		if err != nil {
			return false
		}
	}
	return storageClass.Annotations[AnnotationStorageSource] == LocalPVSourceIdentity
}

func SupportLVMOrQuotaPathOrDevice(classLister storagelisters.StorageClassLister, className string, storageClass *storagev1.StorageClass) bool {
	if storageClass == nil {
		var err error
		storageClass, err = classLister.Get(className)
		if err != nil {
			return false
		}
	}
	supportStorageClass := false
	if storageClass.Annotations != nil {
		supportStorageClass = storageClass.Annotations[AnnotationStorageClassSupportLVMAndQuotaPath] == "true"
	}
	return storageClass.Provisioner == LocalPVCSIProvisioner || supportStorageClass
}

func LocalStorageInfoFromNode(node *corev1.Node) ([]LocalStorageInfo, error) {
	localData, ok := node.Annotations[AnnotationNodeLocalStorageTopologyKey]
	if !ok {
		return nil, nil
	}
	var storageInfos []LocalStorageInfo
	if err := json.Unmarshal([]byte(localData), &storageInfos); err != nil {
		return nil, err
	}
	return storageInfos, nil
}
