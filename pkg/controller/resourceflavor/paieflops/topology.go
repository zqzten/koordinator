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

package paieflops

import (
	"sort"

	"k8s.io/apimachinery/pkg/util/sets"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/cache"
)

// NodeTopologyInfo group by nodes by PointOfDelivery and ASW
type NodeTopologyInfo struct {
	nodes           []string
	PointOfDelivery string
	ASW             string
}

// getNodePODAndASWInfo return node's PointOfDelivery and ASW.
func getNodePODAndASWInfo(nodeCache *cache.NodeCache, nodeName string) (string, string) {
	nodeCr := nodeCache.GetNodeCr(nodeName)
	if nodeCr == nil {
		return "", ""
	}

	nodePointOfDelivery := GetNodePointOfDelivery(nodeCr)
	nodeASW := GetNodeASW(nodeCr)

	return nodePointOfDelivery, nodeASW
}

// getSortedNodePODAndASWInfos return sorted classified nodes by PointOfDelivery and ASW.
func getSortedNodePODAndASWInfos(nodeCache *cache.NodeCache, nodes sets.String) []*NodeTopologyInfo {
	result := make(map[string]map[string][]string)
	//1. group by nodes by PointOfDelivery and ASW
	for nodeName := range nodes {
		nodePOD, nodeASW := getNodePODAndASWInfo(nodeCache, nodeName)
		if nodePOD == "" || nodeASW == "" {
			continue
		}

		if result[nodePOD] == nil {
			result[nodePOD] = make(map[string][]string)
		}
		result[nodePOD][nodeASW] = append(result[nodePOD][nodeASW], nodeName)
	}

	//2. turn map to slice, prepare to sort
	resultSlice := make([]*NodeTopologyInfo, 0)
	for podName, podInfo := range result {
		for aswName, _nodes := range podInfo {
			nodeTopologyInfo := &NodeTopologyInfo{
				PointOfDelivery: podName,
				ASW:             aswName,
				nodes:           _nodes,
			}
			sort.Strings(nodeTopologyInfo.nodes)
			resultSlice = append(resultSlice, nodeTopologyInfo)
		}
	}

	//3. sort nodes by nodesNum(small->large), PointOfDelivery, ASW
	sort.Slice(resultSlice, func(i, j int) bool {
		if len(resultSlice[i].nodes) < len(resultSlice[j].nodes) {
			return true
		}
		if len(resultSlice[i].nodes) > len(resultSlice[j].nodes) {
			return false
		}
		if resultSlice[i].ASW < resultSlice[j].ASW {
			return true
		}
		if resultSlice[i].ASW > resultSlice[j].ASW {
			return false
		}
		if resultSlice[i].PointOfDelivery < resultSlice[j].PointOfDelivery {
			return true
		}
		if resultSlice[i].PointOfDelivery > resultSlice[j].PointOfDelivery {
			return false
		}

		return false
	})

	return resultSlice
}

// FilterAndSortNodesByTopology find the best nodes considering topology. there are some rules:
// 1. nodes in one quota can't cross PointOfDelivery.
// 2. nodes in one quota try best gathered in one asw, if can't be satisfied, allow cross different ASW.
// 3. you can find network-topology in https://aliyuque.antfin.com/byyh59/lkqw1k/cpkyy147tggetapp
// so the select rule is:
// a. if quota's node is empty, we first try to find a asw to hold all wanted nodes. If fails, which means
// we have to pick nodes cross different ASW, so we sort each ASW(nodeNum from low->high) in different PointOfDelivery,
// then we think the more ASW crossing, the better to utilize the fragment of each ASW, so we pick the best PointOfDelivery
// and nodes in related-sorted-ASWs from nodeNum low->high. If each PointOfDelivery nodes can't meet the quota's
// configNodeNum, we just pick the PointOfDelivery that can offer the most nodes.
// b. if quota's node is not empty, we simply pick the first node's PointOfDelivery as target PointOfDelivery(we assume
// nodes won't cross PointOfDelivery), then we first try find whether the nodes in exist PointOfDelivery and ASW can match the
// requirement, if not, means we have to pick node across the ASWs. then we sort the ASW(nodeNum from low->high), and pick
// nodes from them.
func FilterAndSortNodesByTopology(wantedNum int, nodeCache *cache.NodeCache,
	selectedNodes map[string]*sev1alpha1.SelectedNodeMeta, allFreePoolNodes sets.String) []string {
	allSelectedNodes := sets.NewString()
	for nodeName := range selectedNodes {
		allSelectedNodes.Insert(nodeName)
	}

	//sort nodes by nodeNum(from low->high), PointOfDelivery and ASW
	sortedSelectedNodeTopology := getSortedNodePODAndASWInfos(nodeCache, allSelectedNodes)
	sortedFreePoolNodesTopology := getSortedNodePODAndASWInfos(nodeCache, allFreePoolNodes)

	if len(sortedSelectedNodeTopology) == 0 {
		//handle empty quota
		return findBestTopologyNodes4Empty(wantedNum, sortedFreePoolNodesTopology)
	} else {
		//handle non-empty quota
		return findBestTopologyNodes4ExistsPod(wantedNum, sortedSelectedNodeTopology, sortedFreePoolNodesTopology)
	}
}

// findBestTopologyNodes4Empty if quota's node is empty, we first try to find a asw to hold all wanted nodes. If fails, which means
// we have to pick nodes cross different ASW, so we sort each ASW(nodeNum from low->high) in different PointOfDelivery,
// then we think the more ASW crossing, the better to utilize the fragment of each ASW, so we pick the best PointOfDelivery
// and nodes in related-sorted-ASWs from nodeNum low->high. If each PointOfDelivery nodes can't meet the quota's
// configNodeNum, we just pick the PointOfDelivery that can offer the most nodes.
func findBestTopologyNodes4Empty(wantedNum int, freeNodeTopologies []*NodeTopologyInfo) []string {
	if len(freeNodeTopologies) <= 0 {
		return nil
	}

	//we first try to find an asw to hold all wanted nodes. notice that freeNodeTopologies is ordered by nodeNum from
	//low->high, so the result's nodeNum is the lowest among all candidates that meet the conditions
	for _, topologyInfo := range freeNodeTopologies {
		if len(topologyInfo.nodes) >= wantedNum {
			return topologyInfo.nodes
		}
	}

	//if fails, we have to pick nodes cross different ASW
	type Candidates struct {
		podName        string
		wantedNodeNum  int
		candidateNodes [][]string
	}

	//group by nodes by PointOfDelivery
	diffPODCandidates := make(map[string]*Candidates)
	for _, topologyInfo := range freeNodeTopologies {
		if diffPODCandidates[topologyInfo.PointOfDelivery] == nil {
			diffPODCandidates[topologyInfo.PointOfDelivery] = &Candidates{
				podName:       topologyInfo.PointOfDelivery,
				wantedNodeNum: wantedNum,
			}
		}
		//we have picked enough nodes to meet conf requirement.
		if diffPODCandidates[topologyInfo.PointOfDelivery].wantedNodeNum <= 0 {
			continue
		}

		//we put the ASW nodes into diffPODCandidates by low->high order. the len(candidateNodes) means
		//how many ASW we cross. Here the candidateNodes.Len() may larger than wantedNum, it's ok, we actually
		//only pick the wantNum-nodes from them.
		diffPODCandidates[topologyInfo.PointOfDelivery].candidateNodes = append(
			diffPODCandidates[topologyInfo.PointOfDelivery].candidateNodes, topologyInfo.nodes)

		diffPODCandidates[topologyInfo.PointOfDelivery].wantedNodeNum -= len(topologyInfo.nodes)
		if diffPODCandidates[topologyInfo.PointOfDelivery].wantedNodeNum < 0 {
			diffPODCandidates[topologyInfo.PointOfDelivery].wantedNodeNum = 0
		}
	}

	//sort the diffPODCandidates, if wantedNodeNum > 0, means the PointOfDelivery nodes can't meet the conf-requirement.
	//we think the more ASW crossing, the better to utilize the fragment of each ASW
	diffPODCandidatesSlice := make([]*Candidates, 0)
	for _, value := range diffPODCandidates {
		diffPODCandidatesSlice = append(diffPODCandidatesSlice, value)
	}
	sort.Slice(diffPODCandidatesSlice, func(i, j int) bool {
		if diffPODCandidatesSlice[i].wantedNodeNum < diffPODCandidatesSlice[j].wantedNodeNum {
			return true
		}
		if diffPODCandidatesSlice[i].wantedNodeNum > diffPODCandidatesSlice[j].wantedNodeNum {
			return false
		}
		if len(diffPODCandidatesSlice[i].candidateNodes) > len(diffPODCandidatesSlice[j].candidateNodes) {
			return true
		}
		if len(diffPODCandidatesSlice[i].candidateNodes) < len(diffPODCandidatesSlice[j].candidateNodes) {
			return false
		}
		return diffPODCandidatesSlice[i].podName < diffPODCandidatesSlice[j].podName
	})

	//we return the best diffPODCandidatesSlice, notice that here we still follow the order of each ASW from low->high.
	returnNodes := make([]string, 0)
	for _, nodes := range diffPODCandidatesSlice[0].candidateNodes {
		returnNodes = append(returnNodes, nodes...)
	}

	return returnNodes
}

// findBestTopologyNodes4ExistsPod if quota's node is not empty, we simply pick the first node's PointOfDelivery as
// target PointOfDelivery and ASW(we assume nodes won't cross PointOfDelivery), then we first try find whether the nodes
// in exist PointOfDelivery and ASW can match the requirement, if not, means we have to pick node across the ASWs.
// then we sort the ASW(nodeNum from low->high), and pick nodes from them.
func findBestTopologyNodes4ExistsPod(wantedNum int,
	selectedNodeTopologies []*NodeTopologyInfo, freeNodeTopologies []*NodeTopologyInfo) []string {
	returnNodes := make([]string, 0)

	//we simply pick the first node's PointOfDelivery as target PointOfDelivery and ASW
	selectedASWName := selectedNodeTopologies[0].ASW
	selectedPODName := selectedNodeTopologies[0].PointOfDelivery

	//we first try find whether the nodes in exist PointOfDelivery and ASW can match the requirement
	for _, topologyInfo := range freeNodeTopologies {
		if topologyInfo.PointOfDelivery != selectedPODName || topologyInfo.ASW != selectedASWName {
			continue
		}
		if len(topologyInfo.nodes) >= wantedNum {
			//satisfied and return
			return topologyInfo.nodes
		} else {
			//not satisfy, but we continue to pick node as much as possible
			wantedNum -= len(topologyInfo.nodes)
			returnNodes = append(returnNodes, topologyInfo.nodes...)
		}
	}

	//pick nodes from different ASW ordered from nodeNum low->high.
	for _, topologyInfo := range freeNodeTopologies {
		//has picked before
		if topologyInfo.PointOfDelivery == selectedPODName && topologyInfo.ASW == selectedASWName {
			continue
		}
		//different PointOfDelivery, ignore
		if topologyInfo.PointOfDelivery != selectedPODName {
			continue
		}

		if wantedNum <= 0 {
			break
		}
		wantedNum -= len(topologyInfo.nodes)
		//Here the candidateNodes.Len() may larger than wantedNum, it's ok,
		//we actually only pick the wantNum-nodes from them.
		returnNodes = append(returnNodes, topologyInfo.nodes...)
	}

	return returnNodes
}
