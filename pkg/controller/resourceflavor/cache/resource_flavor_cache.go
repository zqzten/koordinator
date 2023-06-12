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

package cache

import (
	"sync"

	"k8s.io/gengo/examples/set-gen/sets"
	"k8s.io/klog/v2"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

// ResourceFlavorCache simply store all ResourceFlavorInfo.
type ResourceFlavorCache struct {
	lock                sync.RWMutex
	resourceFlavorInfos map[string]*ResourceFlavorInfo
}

func NewResourceFlavorCache() *ResourceFlavorCache {
	cache := &ResourceFlavorCache{
		resourceFlavorInfos: make(map[string]*ResourceFlavorInfo),
	}

	return cache
}

func (cache *ResourceFlavorCache) UpdateResourceFlavors(resourceFlavors []sev1alpha1.ResourceFlavor) {
	newAllKeys := sets.NewString()

	for _, resourceFlavor := range resourceFlavors {
		cache.updateResourceFlavor(resourceFlavor.DeepCopy())
		newAllKeys.Insert(resourceFlavor.Name)
	}

	for name, resourceFlavor := range cache.resourceFlavorInfos {
		if !newAllKeys.Has(name) {
			cache.deleteResourceFlavor(resourceFlavor.resourceFlavorCrd)
		}
	}
}

// UpdateResourceFlavor update local resourceFlavor from remote api-server.
func (cache *ResourceFlavorCache) updateResourceFlavor(resourceFlavor *sev1alpha1.ResourceFlavor) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	if cache.resourceFlavorInfos[resourceFlavor.Name] == nil {
		//use resourceFlavorCrd' status to initialize local node-bind status. this is for prcocess recover.
		cache.resourceFlavorInfos[resourceFlavor.Name] = NewResourceFlavorInfo(resourceFlavor)
		klog.V(3).Infof("add new resourceFlavor:%v", resourceFlavor.Name)
	} else {
		//only update resourceFlavorCrd, not update local node-bind status. we want local-update-logic
		//to update local node-bind status and then update remote-resourceFlavorCrd's status.
		cache.resourceFlavorInfos[resourceFlavor.Name].SetResourceFlavorCrd(resourceFlavor)
		klog.V(4).Infof("update resourceFlavor:%v", resourceFlavor.Name)
	}
}

// DeleteResourceFlavor delete local resourceFlavor from remote api-server.
func (cache *ResourceFlavorCache) deleteResourceFlavor(resourceFlavor *sev1alpha1.ResourceFlavor) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	delete(cache.resourceFlavorInfos, resourceFlavor.Name)
	klog.V(3).Infof("delete resourceFlavor:%v", resourceFlavor.Name)
}

// GetAllResourceFlavor return all resourceFlavor.
func (cache *ResourceFlavorCache) GetAllResourceFlavor() map[string]*ResourceFlavorInfo {
	cache.lock.RLock()
	defer cache.lock.RUnlock()

	result := make(map[string]*ResourceFlavorInfo)
	for name, info := range cache.resourceFlavorInfos {
		result[name] = info
	}

	return result
}
