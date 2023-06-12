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

package flavor

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/cache"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

// ResourceFlavor try add\remove nodes for each resourceFlavor.
type ResourceFlavor struct {
	client              client.Client
	nodeCache           *cache.NodeCache
	resourceFlavorCache *cache.ResourceFlavorCache
}

var GetAllThirdPartyBoundNodes = func(client client.Client, allNodes map[string]*corev1.Node) map[string]string {
	return nil
}

func NewResourceFlavor(client client.Client, nodeCache *cache.NodeCache, resourceFlavorCache *cache.ResourceFlavorCache) *ResourceFlavor {
	return &ResourceFlavor{
		client:              client,
		nodeCache:           nodeCache,
		resourceFlavorCache: resourceFlavorCache,
	}
}

// ResourceFlavor update nodeCache and execute add\remove nodes for each resourceFlavor
func (flavor *ResourceFlavor) ResourceFlavor() {
	//at the beginning, we should execute UpdateNodeCache to update node cache, to handle node add\update\delete event.
	flavor.UpdateNodeCache()
	flavor.Flavor()
}

// Flavor try add\remove nodes for resourceFlavor
func (flavor *ResourceFlavor) Flavor() {
	allResourceFlavor := flavor.resourceFlavorCache.GetAllResourceFlavor()

	for flavorName, flavorInfo := range allResourceFlavor {
		//first remove useless nodes
		hasNewRemove := flavor.TryRemoveInvalidNodes(flavorName, flavorInfo)
		//then try add new nodes
		hasNewAdd := flavor.TryAddValidNodes(flavorName, flavorInfo)
		//node meta changed, for example force/un-force changed
		hasNodeMetaChanged := flavor.TryUpdateNodeMeta(flavorName, flavorInfo)

		//if updated, try update node cache and resourceFlavorCrd
		if hasNewRemove || hasNewAdd || hasNodeMetaChanged {
			flavor.UpdateNodeCache()
			flavor.tryUpdateResourceFlavorCrd(flavorName, flavorInfo)
		}
		TryUpdateNode(flavor.client, flavorName, flavorInfo, flavor.nodeCache)
	}
}

// UpdateNodeCache collect all bounded nodes and allThirdPartyBoundNodes to update node cache.
func (flavor *ResourceFlavor) UpdateNodeCache() {
	allResourceFlavor := flavor.resourceFlavorCache.GetAllResourceFlavor()

	allSelectedNodes := make(map[string]*cache.NodeBoundInfo)
	for _, flavorInfo := range allResourceFlavor {
		selectedNodes := flavorInfo.GetSelectedNodes()
		for node, nodeInfo := range selectedNodes {
			allSelectedNodes[node] = nodeInfo
		}
	}

	allNodeCr := flavor.nodeCache.GetAllNodeCr()
	allThirdPartyBoundNodes := GetAllThirdPartyBoundNodes(flavor.client, allNodeCr)

	flavor.nodeCache.UpdateCache(allSelectedNodes, allThirdPartyBoundNodes)
}

// tryUpdateResourceFlavorCrd update latest resourceFlavor crd.
func (flavor *ResourceFlavor) tryUpdateResourceFlavorCrd(flavorName string, flavorInfo *cache.ResourceFlavorInfo) {
	if flavor.client == nil {
		return
	}

	err := util.RetryOnConflictOrTooManyRequests(func() error {
		newCrd := flavorInfo.GetNewResourceFlavorCrd()
		err := flavor.client.Update(context.TODO(), newCrd, &client.UpdateOptions{})
		return err
	})
	if err != nil {
		klog.Errorf("Failed Update ResourceFlavor Crd, Name:%v, err:%v", flavorName, err.Error())
	} else {
		klog.V(4).Infof("Success Update ResourceFlavor Crd, Name:%v", flavorName)
	}
}

var TryUpdateNode = func(client client.Client, flavorName string,
	flavorInfo *cache.ResourceFlavorInfo, nodeCache *cache.NodeCache) map[string]*corev1.Node {
	return nil
}
