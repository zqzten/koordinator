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
	"k8s.io/client-go/informers"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
)

type pvEventHandler struct {
	storageCache       *NodeStorageInfoCache
	nodeLister         corelisters.NodeLister
	storageClassLister storagelisters.StorageClassLister
}

func registerPVEventHandler(informerFactory informers.SharedInformerFactory, cache *NodeStorageInfoCache) {
	pvInformer := informerFactory.Core().V1().PersistentVolumes().Informer()
	eventHandler := &pvEventHandler{
		storageCache:       cache,
		nodeLister:         informerFactory.Core().V1().Nodes().Lister(),
		storageClassLister: informerFactory.Storage().V1().StorageClasses().Lister(),
	}
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), informerFactory, pvInformer, eventHandler)
}

func (e *pvEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	pv, ok := obj.(*v1.PersistentVolume)
	if !ok {
		return
	}
	e.setPV(pv)
}

func (e *pvEventHandler) OnUpdate(oldObj, newObj interface{}) {
	pv, ok := newObj.(*v1.PersistentVolume)
	if !ok {
		return
	}
	e.setPV(pv)
}

func (e *pvEventHandler) OnDelete(obj interface{}) {
	var pv *v1.PersistentVolume
	switch t := obj.(type) {
	case *v1.PersistentVolume:
		pv = t
	case cache.DeletedFinalStateUnknown:
		pv, _ = t.Obj.(*v1.PersistentVolume)
	}
	if pv == nil {
		return
	}

	e.deletePV(pv)
}

func (e *pvEventHandler) setPV(pv *v1.PersistentVolume) {
	if pv.Status.Phase != v1.VolumeAvailable && pv.Status.Phase != v1.VolumeBound {
		return
	}
	if !unified.SupportLocalPV(e.storageClassLister, storagehelpers.GetPersistentVolumeClass(pv), nil) {
		return
	}
	selectedNode := GetPVSelectedNode(pv)
	if selectedNode == "" {
		klog.Errorf("miss selectedNode, pv: %s", pv.Name)
		return
	}
	var claim string
	if pv.Spec.ClaimRef != nil {
		claim = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
	}
	capacity := pv.Spec.Capacity.Storage().Value()
	klog.V(4).Infof("update pvAlloc: %s, claimRef: %s, capacity: %d", pv.Name, claim, capacity)
	node, err := e.nodeLister.Get(selectedNode)
	if errors.IsNotFound(err) {
		node, err = e.nodeLister.Get(strings.ToLower(selectedNode))
	}
	if err != nil {
		return
	}
	e.storageCache.UpdateOnNode(node.Name, func(nodeStorageInfo *NodeStorageInfo) {
		nodeStorageInfo.AddLocalPVAlloc(&localPVAlloc{
			Name:     pv.Name,
			Capacity: capacity,
			ClaimRef: claim,
		})
	})
}

func (e *pvEventHandler) deletePV(pv *v1.PersistentVolume) {
	if !unified.SupportLocalPV(e.storageClassLister, storagehelpers.GetPersistentVolumeClass(pv), nil) {
		return
	}
	selectedNode := GetPVSelectedNode(pv)
	if selectedNode == "" {
		klog.Errorf("miss selectedNode, pv: %s", pv.Name)
		return
	}
	var claim string
	if pv.Spec.ClaimRef != nil {
		claim = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
	}
	capacity := pv.Spec.Capacity.Storage().Value()
	klog.V(4).Infof("delete pvAlloc: %s, claimRef: %s, capacity: %d", pv.Name, claim, capacity)
	node, err := e.nodeLister.Get(selectedNode)
	if errors.IsNotFound(err) {
		node, err = e.nodeLister.Get(strings.ToLower(selectedNode))
	}
	if err != nil {
		return
	}

	e.storageCache.UpdateOnNode(node.Name, func(nodeStorageInfo *NodeStorageInfo) {
		nodeStorageInfo.DeleteLocalPVAlloc(&localPVAlloc{
			Name:     pv.Name,
			Capacity: capacity,
			ClaimRef: claim,
		})
	})
}
