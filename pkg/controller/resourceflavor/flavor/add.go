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
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	v1helper "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/klog"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/cache"
)

const (
	ForceAddTypeManual = "ForceAddTypeManual"
	ForceAddTypeFlavor = "ForceAddTypeFlavor"
)

// TryAddValidNodes add nodes for quotaNodeResourceFlavor
func (flavor *ResourceFlavor) TryAddValidNodes(resourceFlavorName string, resourceFlavorInfo *cache.ResourceFlavorInfo) bool {
	if !resourceFlavorInfo.Enable() {
		return false
	}

	allFreePoolNodes := flavor.nodeCache.GetAllFreePoolNodes()

	hasNewAdded := resourceFlavorInfo.TryAddNodes(func(confName string, conf *sev1alpha1.ResourceFlavorConf,
		localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
		return flavor.addNodes(allFreePoolNodes, resourceFlavorName, confName, conf, localConfStatus)
	})

	return hasNewAdded
}

// addNodes do add node logic for each conf.
func (flavor *ResourceFlavor) addNodes(allFreePoolNodes sets.String, resourceFlavorName string, confName string,
	conf *sev1alpha1.ResourceFlavorConf, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
	hasNewAdded := false

	//first we try to pick force-added nodes, we don't check whether is satisfied cause "force reason".
	thirdPartyBoundNodes := flavor.nodeCache.GetThirdPartyBoundNodes(resourceFlavorName)

	forceSelectedNodes := cache.GetForceSelectedNodes(conf, localConfStatus, thirdPartyBoundNodes)
	forceNodesWanted := cache.GetWantedForceAddNodes(conf, thirdPartyBoundNodes)
	forceNodesTotalWantedNum := len(forceNodesWanted)
	forceNodesWantedNum := forceNodesTotalWantedNum - len(forceSelectedNodes)

	//for test stable
	forceNodesWantedSorted := forceNodesWanted.List()
	sort.Strings(forceNodesWantedSorted)

	for _, node := range forceNodesWantedSorted {
		if forceNodesWantedNum <= 0 {
			break
		}
		if localConfStatus.SelectedNodes[node] == nil {
			forceAddType := ForceAddTypeFlavor
			if thirdPartyBoundNodes.Has(node) {
				forceAddType = ForceAddTypeManual
			}
			cache.AddNode(node, flavor.FillNodeMeta(node, true, forceAddType), localConfStatus)
			allFreePoolNodes.Delete(node)
			hasNewAdded = true
			forceNodesWantedNum -= 1
		}
	}

	//then we try to pick general nodes, no matter whether we can really add the force-added node successfully or not,
	//it indeed holds a node position relative to general nodes.
	generalSelectedNodes := cache.GetGeneralSelectedNodes(conf, localConfStatus, thirdPartyBoundNodes)
	generalNodesTotalWantedNum := cache.GetGeneralNodeTotalWantedNum(conf, thirdPartyBoundNodes)
	generalNodesWantedNum := generalNodesTotalWantedNum - len(generalSelectedNodes)
	if generalNodesWantedNum <= 0 {
		return hasNewAdded
	}

	//we first filter-out valid nodes.
	allValidFreeNodes := sets.NewString()
	for node := range allFreePoolNodes {
		if flavor.IsNodeShouldAdd(resourceFlavorName, confName, node, conf) {
			allValidFreeNodes.Insert(node)
		}
	}

	//then we sort the valid nodes by topology distance if we need sameTopologyFirst
	var allValidFreeNodesSlice []string
	if conf.SameTopologyFirst {
		allValidFreeNodesSlice = FilterAndSortNodesByTopology(generalNodesWantedNum,
			flavor.nodeCache, localConfStatus.SelectedNodes, allValidFreeNodes)
	} else {
		allValidFreeNodesSlice = allValidFreeNodes.List()
	}

	//finally we add nodes.
	for _, node := range allValidFreeNodesSlice {
		if generalNodesWantedNum <= 0 {
			break
		}

		cache.AddNode(node, flavor.FillNodeMeta(node, false, ""), localConfStatus)
		allFreePoolNodes.Delete(node)
		generalNodesWantedNum -= 1
		hasNewAdded = true
	}

	if generalNodesWantedNum > 0 {
		klog.V(3).Infof("still lack nodes after quota node flavor add, "+
			"flavor:%v, conf:%v, total:%v, selected:%v", resourceFlavorName, confName, conf.NodeNum,
			localConfStatus.SelectedNodeNum)
	}

	return hasNewAdded
}

// IsNodeShouldAdd judge one node whether satisfy conf-requirement.
func (flavor *ResourceFlavor) IsNodeShouldAdd(resourceFlavorName string, confName string,
	nodeName string, conf *sev1alpha1.ResourceFlavorConf) bool {
	nodeCr := flavor.nodeCache.GetNodeCr(nodeName)
	//if node not exist
	if nodeCr == nil {
		klog.Errorf("find node add fail due to node has been deleted, "+
			"resourceFlavorName:%v, confName:%v, nodeName:%v", resourceFlavorName, confName, nodeName)
		return false
	}

	//if node affinity not match
	if conf.NodeAffinity != nil {
		affinity := GetNodeAffinity(conf.NodeAffinity)
		if match, err := MatchesNodeSelectorAndAffinityTerms(affinity, nodeCr); !match {
			errMsg := ""
			if err != nil {
				errMsg = err.Error()
			}
			klog.V(3).Infof("find node add fail due to node affinity check failed, "+
				"resourceFlavorName:%v, confName:%v, nodeName:%v, err:%v", resourceFlavorName, confName, nodeName, errMsg)
			return false
		}
	}

	//if node toleration not match
	_, isUnTolerated := v1helper.FindMatchingUntoleratedTaint(nodeCr.Spec.Taints, conf.Toleration, func(t *corev1.Taint) bool {
		return t.Effect == corev1.TaintEffectNoSchedule || t.Effect == corev1.TaintEffectNoExecute
	})
	if isUnTolerated {
		klog.V(3).Infof("find node add fail due to toleration check failed, "+
			"resourceFlavorName:%v, confName:%v, nodeName:%v", resourceFlavorName, confName, nodeName)
		return false
	}

	return true
}

// FillNodeMeta return some important node meta, we use it to update quotaNodeResourceFlavor cr's status.
func (flavor *ResourceFlavor) FillNodeMeta(nodeName string, forceAdd bool, forceType string) map[string]string {
	nodeCr := flavor.nodeCache.GetNodeCr(nodeName)
	if nodeCr == nil {
		return nil
	}

	metaMap := make(map[string]string)
	metaMap[GpuModel] = GetNodeGpuModel(nodeCr)
	metaMap[PointOfDelivery] = GetNodePointOfDelivery(nodeCr)
	metaMap[ASWID] = GetNodeASW(nodeCr)
	metaMap[ForceWanted] = fmt.Sprintf("%t", forceAdd)
	metaMap[ForceWantedType] = forceType

	return metaMap
}
