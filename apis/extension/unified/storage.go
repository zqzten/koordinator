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
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/informers"
	storagelisters "k8s.io/client-go/listers/storage/v1"
)

const (
	LocalPVCSIName          string = "csi-hostpath-dp"
	AnnotationStorageSource string = "alibabacloud.com/storage-source"

	LocalPVSourceIdentity string = "ephemeral-storage"

	InlineVolumeAttributeStorageClass string = "alibabacloud.com/storage-class"
	InlineVolumeAttributeStorageSize  string = "alibabacloud.com/storage-size"
)

func GetLocalInlineVolumeSize(volumes []corev1.Volume, factory informers.SharedInformerFactory) int64 {
	if factory == nil {
		return 0
	}
	storageClassLister := factory.Storage().V1().StorageClasses().Lister()
	var totalSize int64
	for i := range volumes {
		volume := &volumes[i]
		if volume.CSI == nil {
			continue
		}
		storageClass := volume.CSI.VolumeAttributes[InlineVolumeAttributeStorageClass]
		if !supportLocalPV(storageClassLister, storageClass, nil) {
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

func supportLocalPV(classLister storagelisters.StorageClassLister, className string, storageClass *storagev1.StorageClass) bool {
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
