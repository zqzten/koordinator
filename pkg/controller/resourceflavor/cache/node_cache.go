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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// NodeBoundInfo record a node's binding information
type NodeBoundInfo struct {
	QuotaName string
	ConfName  string
}

func (info *NodeBoundInfo) GetQuotaName() string {
	return info.QuotaName
}

// NodeCache store all nodes
type NodeCache struct {
	lock sync.RWMutex

	//all node cr
	nodeInfos map[string]*corev1.Node
	// all nodes in cluster
	allClusterNodes sets.String
	//all unBound nodes in cluster
	allFreePoolNodes sets.String
	//all bound nodes by quotaNodeBinder auto logic.
	allAutoBoundNodes map[string]*NodeBoundInfo
	//all bound nodes by third-party-manual way. there are 2 cases:
	//case1:
	//quotaNodeBinder's enable is false, user bind node to quota by manual-way(edit node label),
	//these nodes can't be recognized as unBound status.
	//case2:
	//user bind node to quota by manual-way(edit node label), then quotaNodeBinder's enable is true,
	//so this is a hot-update case, we first believe these nodes to force-added nodes to let quotaNodeBinder
	//pick them, then we can erase the manual-way(edit node label), the nodes will still stay in quotaNodeBinder.
	thirdPartyBoundNodes map[string]string
}

func NewNodeCache() *NodeCache {
	nodeCache := &NodeCache{
		allClusterNodes:      sets.NewString(),
		allFreePoolNodes:     sets.NewString(),
		allAutoBoundNodes:    make(map[string]*NodeBoundInfo),
		thirdPartyBoundNodes: make(map[string]string),
		nodeInfos:            make(map[string]*corev1.Node),
	}

	return nodeCache
}

func (nodeCache *NodeCache) GetAllAutoBoundNodes4Test() map[string]*NodeBoundInfo {
	nodeCache.lock.Lock()
	defer nodeCache.lock.Unlock()

	return nodeCache.allAutoBoundNodes
}

// UpdateNodes update node cr
func (nodeCache *NodeCache) UpdateNodes(nodes []corev1.Node) {
	nodeCache.lock.Lock()
	defer nodeCache.lock.Unlock()

	nodeCache.nodeInfos = make(map[string]*corev1.Node)

	for _, nodeCr := range nodes {
		nodeCache.nodeInfos[nodeCr.Name] = nodeCr.DeepCopy()
	}
}

// UpdateCache update nodeCache's variable in full-backup way. allAutoBoundNodes collected from each QuotaNodeBinder,
// thirdPartyBoundNodes collected from all node's labels. Here we use full-backup way to make process easier, cause
// there are multiple ways to trigger cache updated, like node\quotaNodeBinder add\update\delete...
func (nodeCache *NodeCache) UpdateCache(allAutoBoundNodes map[string]*NodeBoundInfo, thirdPartyBoundNodes map[string]string) {
	nodeCache.lock.Lock()
	defer nodeCache.lock.Unlock()

	nodeCache.allClusterNodes = sets.NewString()
	nodeCache.allFreePoolNodes = sets.NewString()
	nodeCache.allAutoBoundNodes = make(map[string]*NodeBoundInfo)
	nodeCache.thirdPartyBoundNodes = make(map[string]string)

	for nodeName := range nodeCache.nodeInfos {
		nodeCache.allClusterNodes.Insert(nodeName)
	}

	nodeCache.allAutoBoundNodes = allAutoBoundNodes
	nodeCache.thirdPartyBoundNodes = thirdPartyBoundNodes

	for nodeName := range nodeCache.allClusterNodes {
		_, exist1 := nodeCache.allAutoBoundNodes[nodeName]
		_, exist2 := nodeCache.thirdPartyBoundNodes[nodeName]
		if !exist1 && !exist2 {
			nodeCache.allFreePoolNodes.Insert(nodeName)
		}
	}
}

func (nodeCache *NodeCache) GetAllNodeCr() map[string]*corev1.Node {
	nodeCache.lock.RLock()
	defer nodeCache.lock.RUnlock()

	result := make(map[string]*corev1.Node)
	for nodeName, nodeCr := range nodeCache.nodeInfos {
		result[nodeName] = nodeCr
	}

	return result
}

func (nodeCache *NodeCache) GetNodeCr(nodeName string) *corev1.Node {
	nodeCache.lock.RLock()
	defer nodeCache.lock.RUnlock()

	if nodeCache.nodeInfos[nodeName] == nil {
		return nil
	}

	return nodeCache.nodeInfos[nodeName]
}

func (nodeCache *NodeCache) GetAllFreePoolNodes() sets.String {
	nodeCache.lock.RLock()
	defer nodeCache.lock.RUnlock()

	result := sets.NewString()
	result = result.Union(nodeCache.allFreePoolNodes)

	return result
}

func (nodeCache *NodeCache) GetThirdPartyBoundNodes(_flavorName string) sets.String {
	nodeCache.lock.RLock()
	defer nodeCache.lock.RUnlock()

	result := sets.NewString()
	for nodeName, flavorName := range nodeCache.thirdPartyBoundNodes {
		if flavorName == _flavorName {
			result.Insert(nodeName)
		}
	}

	return result
}
