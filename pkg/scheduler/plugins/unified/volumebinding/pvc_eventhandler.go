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
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
)

type pvcEventHandler struct {
	storageCache       *NodeStorageInfoCache
	nodeLister         corelisters.NodeLister
	storageClassLister storagelisters.StorageClassLister
}

func registerPVCEventHandler(informerFactory informers.SharedInformerFactory, cache *NodeStorageInfoCache) {
	pvcInformer := informerFactory.Core().V1().PersistentVolumeClaims().Informer()
	eventHandler := &pvcEventHandler{
		storageCache:       cache,
		nodeLister:         informerFactory.Core().V1().Nodes().Lister(),
		storageClassLister: informerFactory.Storage().V1().StorageClasses().Lister(),
	}
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), informerFactory, pvcInformer, eventHandler)
}

func (e *pvcEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	pvc, ok := obj.(*v1.PersistentVolumeClaim)
	if !ok {
		return
	}
	e.setPVC(pvc)
}

func (e *pvcEventHandler) OnUpdate(oldObj, newObj interface{}) {
	pvc, ok := newObj.(*v1.PersistentVolumeClaim)
	if !ok {
		return
	}

	e.setPVC(pvc)
}

func (e *pvcEventHandler) OnDelete(obj interface{}) {
	var pvc *v1.PersistentVolumeClaim
	switch t := obj.(type) {
	case *v1.PersistentVolumeClaim:
		pvc = t
	case cache.DeletedFinalStateUnknown:
		pvc, _ = t.Obj.(*v1.PersistentVolumeClaim)
	}
	if pvc == nil {
		return
	}
	e.deletePVC(pvc)
}

func isPVCFullyBound(pvc *v1.PersistentVolumeClaim) bool {
	return pvc.Spec.VolumeName != "" && metav1.HasAnnotation(pvc.ObjectMeta, storagehelpers.AnnBindCompleted)
}

func (e *pvcEventHandler) setPVC(pvc *v1.PersistentVolumeClaim) {
	if !isPVCFullyBound(pvc) {
		return
	}
	// 只处理多盘LocalPV场景的PVC账本更新。
	// 对于单盘LocalPV的账本更新则由PV链路处理
	selectedStorage, storageClassName, err := e.GetLocalStorage(pvc)
	if len(selectedStorage) == 0 || err != nil {
		return
	}
	ioLimitInfo, err := GetPVCIOLimitInfo(pvc)
	if err != nil {
		return
	}
	selectedNode := pvc.Annotations[storagehelpers.AnnSelectedNode]
	klog.V(4).Infof("update pvc %s/%s, storageClassName: %s, selectNode: %s, selectedDisk: %s",
		pvc.Namespace, pvc.Name, storageClassName, selectedNode, selectedStorage)
	if selectedNode != "" {
		node, err := e.nodeLister.Get(selectedNode)
		if errors.IsNotFound(err) {
			node, err = e.nodeLister.Get(strings.ToLower(selectedNode))
		}
		if err != nil {
			return
		}

		e.storageCache.UpdateOnNode(node.Name, func(nodeStorageInfo *NodeStorageInfo) {
			quantity := pvc.Spec.Resources.Requests[v1.ResourceStorage]
			pvcAlloc := &localPVCAlloc{
				Namespace:   pvc.Namespace,
				Name:        pvc.Name,
				RequestSize: quantity.Value(),
				MountPoint:  selectedStorage,
				IOLimit:     ioLimitInfo,
			}
			nodeStorageInfo.AddLocalPVCAlloc(pvcAlloc)
		})
	}
}

func (e *pvcEventHandler) deletePVC(pvc *v1.PersistentVolumeClaim) {
	selectedStorage, storageClassName, err := e.GetLocalStorage(pvc)
	if len(selectedStorage) == 0 || err != nil {
		return
	}
	ioLimitInfo, err := GetPVCIOLimitInfo(pvc)
	if err != nil {
		return
	}
	selectedNode := pvc.Annotations[storagehelpers.AnnSelectedNode]
	quantity := pvc.Spec.Resources.Requests[v1.ResourceStorage]
	klog.V(4).Infof("delete pvc %s/%s, storageClassName: %s, selectNode: %s, selectedDisk: %s, quantity: %v",
		pvc.Namespace, pvc.Name, storageClassName, selectedNode, selectedStorage, quantity)
	if selectedNode != "" {
		node, err := e.nodeLister.Get(selectedNode)
		if errors.IsNotFound(err) {
			node, err = e.nodeLister.Get(strings.ToLower(selectedNode))
		}
		if err != nil {
			return
		}

		e.storageCache.UpdateOnNode(node.Name, func(nodeStorageInfo *NodeStorageInfo) {
			pvcAlloc := &localPVCAlloc{
				Namespace:   pvc.Namespace,
				Name:        pvc.Name,
				RequestSize: quantity.Value(),
				MountPoint:  selectedStorage,
				IOLimit:     ioLimitInfo,
			}
			nodeStorageInfo.DeleteLocalPVCAlloc(pvcAlloc)
		})
	}
}

func (e *pvcEventHandler) GetLocalStorage(pvc *v1.PersistentVolumeClaim) (string, string, error) {
	storageClassName := storagehelpers.GetPersistentVolumeClaimClass(pvc)
	selectedStorage, ok := pvc.Annotations[unified.AnnSelectedDisk]
	if !ok {
		selectedStorage, ok = pvc.Annotations[unified.AnnSelectedStorage]
	}
	if !ok && unified.SupportLVMOrQuotaPathOrDevice(e.storageClassLister, storageClassName, nil) {
		class, err := e.storageClassLister.Get(storageClassName)
		if err != nil {
			return selectedStorage, storageClassName, err
		}
		volumeType := class.Parameters[unified.CSIVolumeType]
		switch volumeType {
		case unified.CSILocalVolumeLVM:
			selectedStorage = class.Parameters[unified.CSILVMVolumeGroupName]
		case unified.CSILocalVolumeQuotaPath:
			selectedStorage = class.Parameters[unified.CSIQuotaPathRootPath]
		case unified.CSILocalVolumeDevice:
			// do nothing
		case unified.CSILocalLoopDevice:
			selectedStorage = class.Parameters[unified.CSILoopDeviceRootPath]
		default:
			klog.V(4).Infof("unknown volume type %s, pvc: %s/%s, selectedStorage: %s, storageClassName: %s",
				volumeType, pvc.Namespace, pvc.Name, selectedStorage, storageClassName)
			return selectedStorage, storageClassName, fmt.Errorf("unknow volume type %s", volumeType)
		}
	}
	return selectedStorage, storageClassName, nil
}
