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
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

// ResourceFlavorInfo is the local reflection of each real resourceFlavor cr.
type ResourceFlavorInfo struct {
	lock sync.Mutex
	//resourceFlavorCrd is the received cr from api-server
	resourceFlavorCrd *sev1alpha1.ResourceFlavor
	//when local logic update the bound nodes of quota, we use localConfStatus to store the local-update result, and
	//then update it to the remote cr's status.
	localConfStatus map[string]*sev1alpha1.ResourceFlavorConfStatus
}

func NewResourceFlavorInfo(resourceFlavorCrd *sev1alpha1.ResourceFlavor) *ResourceFlavorInfo {
	//use resourceFlavorCrd' status to initialize local node-bind status. this is for prcocess recover.
	info := &ResourceFlavorInfo{
		resourceFlavorCrd: resourceFlavorCrd,
		localConfStatus:   resourceFlavorCrd.Status.ConfigStatuses,
	}
	if info.localConfStatus == nil {
		info.localConfStatus = make(map[string]*sev1alpha1.ResourceFlavorConfStatus)
	}

	return info
}

func (info *ResourceFlavorInfo) GetLocalConfStatus4Test() map[string]*sev1alpha1.ResourceFlavorConfStatus {
	info.lock.Lock()
	defer info.lock.Unlock()

	return info.localConfStatus
}

// Enable return resourceFlavor is enabled, this is a fast-way to turn on\off the auto-bind function for one quota,
// and no need to delete the resourceFlavor to turn function off.
func (info *ResourceFlavorInfo) Enable() bool {
	info.lock.Lock()
	defer info.lock.Unlock()

	return info.resourceFlavorCrd.Spec.Enable
}

// SetResourceFlavorCrd update resourceFlavorCrd from api-server.
func (info *ResourceFlavorInfo) SetResourceFlavorCrd(resourceFlavorCrd *sev1alpha1.ResourceFlavor) {
	info.lock.Lock()
	defer info.lock.Unlock()

	info.resourceFlavorCrd = resourceFlavorCrd
}

// GetSelectedNodes get local selected nodes.
func (info *ResourceFlavorInfo) GetSelectedNodes() map[string]*NodeBoundInfo {
	info.lock.Lock()
	defer info.lock.Unlock()

	result := make(map[string]*NodeBoundInfo)
	for confName, confStatus := range info.localConfStatus {
		for node := range confStatus.SelectedNodes {
			result[node] = &NodeBoundInfo{
				QuotaName: info.resourceFlavorCrd.Name,
				ConfName:  confName,
			}
		}
	}

	return result
}

// RemoveAllLocalConfStatus foreach all conf and remove all nodes, this is used for enable from true->false.
func (info *ResourceFlavorInfo) RemoveAllLocalConfStatus(removeFunc func(confName string,
	localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool) bool {
	info.lock.Lock()
	defer info.lock.Unlock()

	hasNewRemoved := false

	for confName, localConfStatus := range info.localConfStatus {
		if removeFunc(confName, localConfStatus) {
			hasNewRemoved = true
		}
	}

	return hasNewRemoved
}

// RemoveAllInvalidLocalConfStatus when resourceFlavor add some new conf and delete some conf,
// we should release the nodes from the deleted-conf.
func (info *ResourceFlavorInfo) RemoveAllInvalidLocalConfStatus(removeFunc func(confName string,
	localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool) bool {
	info.lock.Lock()
	defer info.lock.Unlock()

	hasNewRemoved := false

	for confName, localConfStatus := range info.localConfStatus {
		if info.resourceFlavorCrd.Spec.Configs[confName] == nil {
			if removeFunc(confName, localConfStatus) {
				hasNewRemoved = true
			}
		}
	}

	return hasNewRemoved
}

// TryRemoveNodes foreach all conf, when some selected nodes no longer satisfy the conf-requirement, we should remove them.
func (info *ResourceFlavorInfo) TryRemoveNodes(removeFunc func(confName string, conf *sev1alpha1.ResourceFlavorConf,
	localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool) bool {
	info.lock.Lock()
	defer info.lock.Unlock()

	hasNewRemoved := false

	for confName, conf := range info.resourceFlavorCrd.Spec.Configs {
		//to make localConfStatus has same config dimension with resourceFlavorCrd.Spec.Configs
		if info.localConfStatus[confName] == nil {
			info.localConfStatus[confName] = &sev1alpha1.ResourceFlavorConfStatus{
				SelectedNodes: make(map[string]*sev1alpha1.SelectedNodeMeta),
			}
		}

		if removeFunc(confName, conf, info.localConfStatus[confName]) {
			hasNewRemoved = true
		}
	}

	return hasNewRemoved
}

// RemoveNodeFromLocalConfStatus remove one node from localConfStatus
func RemoveNodeFromLocalConfStatus(nodeName string, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) {
	if localConfStatus.SelectedNodes[nodeName] != nil {
		localConfStatus.SelectedNodeNum -= 1
		delete(localConfStatus.SelectedNodes, nodeName)
	}
}

// RemoveUnnecessaryNodes describes remove unwanted node when configNodeNum is smaller than selected node num.
// if nodes are in forceAddNodes or thirdPartyBoundNodes, we should remove them as late as possible.
func RemoveUnnecessaryNodes(conf *sev1alpha1.ResourceFlavorConf,
	localConfStatus *sev1alpha1.ResourceFlavorConfStatus, thirdPartyBoundNodes sets.String) bool {
	//there are two cases to remove nodes by num:
	//case1:
	//configNodeNum=10, force-added nodes.Len() = 6, selected general nodes.Len() = 4, selected force-added nodes.Len() = 6,
	//then configNodeNum 10->5, we should remove 5 general nodes and 1 force-added node.

	//case2:
	//configNodeNum=10, force-added nodes.Len() = 6, selected general nodes.Len() = 4, selected force-added nodes.Len() = 6,
	//then force-added nodes.Len() 6->7, so we should remove 1 general node.

	//case3:
	//configNodeNum=10, force-added nodes.Len() = 6, selected general nodes.Len() = 4, selected force-added nodes.Len() = 6,
	//then force-added nodes.Len() 6->7, configNodeNum 10->7, and so we should remove 4 general nodes, after remove only
	//keep 6 force-added nodes, which is smaller than 7.(we don't keep 1 general node to satisfy configNum:7!).

	//case2&&3 tell us that no matter whether we can really add the force-added node successfully or not, it indeed holds a
	//node position relative to general nodes.

	generalSelectedNodes := GetGeneralSelectedNodes(conf, localConfStatus, thirdPartyBoundNodes)
	generalNodeTotalWantedNum := GetGeneralNodeTotalWantedNum(conf, thirdPartyBoundNodes)

	hasNewRemove := false
	overFlowNodeNum := generalSelectedNodes.Len() - generalNodeTotalWantedNum

	//for test stable
	generalSelectedNodesSlice := generalSelectedNodes.List()
	sort.Strings(generalSelectedNodesSlice)

	for _, nodeName := range generalSelectedNodesSlice {
		if overFlowNodeNum <= 0 {
			break
		}

		RemoveNodeFromLocalConfStatus(nodeName, localConfStatus)
		overFlowNodeNum -= 1
		hasNewRemove = true
	}

	//if configNum is still smaller than len(localConfStatus.SelectedNodes), we have to keep remove-action.
	//for test stable
	selectedNodesSlice := make([]string, 0)
	for nodeName := range localConfStatus.SelectedNodes {
		selectedNodesSlice = append(selectedNodesSlice, nodeName)
	}
	sort.Strings(selectedNodesSlice)

	overFlowNodeNum = len(selectedNodesSlice) - conf.NodeNum
	for _, nodeName := range selectedNodesSlice {
		if overFlowNodeNum <= 0 {
			break
		}
		RemoveNodeFromLocalConfStatus(nodeName, localConfStatus)
		overFlowNodeNum -= 1
		hasNewRemove = true
	}

	return hasNewRemove
}

// TryAddNodes foreach all conf, try add new nodes if needed.
func (info *ResourceFlavorInfo) TryAddNodes(addFunc func(confName string, conf *sev1alpha1.ResourceFlavorConf,
	localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool) bool {
	info.lock.Lock()
	defer info.lock.Unlock()

	//make test stable
	confNameSlice := make([]string, 0)
	for confName := range info.resourceFlavorCrd.Spec.Configs {
		confNameSlice = append(confNameSlice, confName)
	}
	sort.Strings(confNameSlice)

	hasNewAdd := false
	for _, confName := range confNameSlice {
		conf := info.resourceFlavorCrd.Spec.Configs[confName]

		//to make localConfStatus has same config dimension with resourceFlavorCrd.Spec.Configs
		if info.localConfStatus[confName] == nil {
			info.localConfStatus[confName] = &sev1alpha1.ResourceFlavorConfStatus{
				SelectedNodes: make(map[string]*sev1alpha1.SelectedNodeMeta),
			}
		}

		if addFunc(confName, conf, info.localConfStatus[confName]) {
			hasNewAdd = true
		}
	}

	return hasNewAdd
}

// TryUpdateNodeMeta foreach all conf, try update nodes meta if needed.
func (info *ResourceFlavorInfo) TryUpdateNodeMeta(updateMetaFunc func(confName string, conf *sev1alpha1.ResourceFlavorConf,
	localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool) bool {
	info.lock.Lock()
	defer info.lock.Unlock()

	hasNodeMetaChanged := false
	for confName := range info.resourceFlavorCrd.Spec.Configs {
		conf := info.resourceFlavorCrd.Spec.Configs[confName]

		//to make localConfStatus has same config dimension with resourceFlavorCrd.Spec.Configs
		if info.localConfStatus[confName] == nil {
			info.localConfStatus[confName] = &sev1alpha1.ResourceFlavorConfStatus{
				SelectedNodes: make(map[string]*sev1alpha1.SelectedNodeMeta),
			}
		}

		if updateMetaFunc(confName, conf, info.localConfStatus[confName]) {
			hasNodeMetaChanged = true
		}
	}

	return hasNodeMetaChanged
}

// AddNode add one node to localConfStatus
func AddNode(nodeName string, metaMap map[string]string, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) {
	if localConfStatus.SelectedNodes[nodeName] == nil {
		localConfStatus.SelectedNodeNum += 1
		localConfStatus.SelectedNodes[nodeName] = &sev1alpha1.SelectedNodeMeta{
			NodeMetaInfo: metaMap,
		}
	}
}

// GetNewResourceFlavorCrd return the latest resourceFlavorCrd after merge the localStatus.
func (info *ResourceFlavorInfo) GetNewResourceFlavorCrd() *sev1alpha1.ResourceFlavor {
	info.lock.Lock()
	defer info.lock.Unlock()

	newCrd := info.resourceFlavorCrd.DeepCopy()

	newCrd.Status.ConfigStatuses = make(map[string]*sev1alpha1.ResourceFlavorConfStatus)

	for confName, localStatus := range info.localConfStatus {
		newCrd.Status.ConfigStatuses[confName] = localStatus.DeepCopy()
	}

	return newCrd
}

// GetWantedForceAddNodes return force-added nodes after merge conf.ForceAddNodes and thirdPartyBoundNodes.
func GetWantedForceAddNodes(conf *sev1alpha1.ResourceFlavorConf, thirdPartyBoundNodes sets.String) sets.String {
	forceAddNodes := sets.NewString()
	forceAddNodes.Insert(conf.ForceAddNodes...)
	forceAddNodes = forceAddNodes.Union(thirdPartyBoundNodes)

	return forceAddNodes
}

// GetGeneralNodeTotalWantedNum no matter whether we can really add the force-added node successfully or not,
// it indeed holds a node position relative to general nodes.
func GetGeneralNodeTotalWantedNum(conf *sev1alpha1.ResourceFlavorConf, thirdPartyBoundNodes sets.String) int {
	wantedForceAddNodes := GetWantedForceAddNodes(conf, thirdPartyBoundNodes)

	generalNodeTotalWantedNum := conf.NodeNum - len(wantedForceAddNodes)
	return generalNodeTotalWantedNum
}

// GetGeneralSelectedNodes filter the selected nodes not in conf.ForceAddNodes and thirdPartyBoundNodes.
func GetGeneralSelectedNodes(conf *sev1alpha1.ResourceFlavorConf, localConfStatus *sev1alpha1.ResourceFlavorConfStatus,
	thirdPartyBoundNodes sets.String) sets.String {
	wantedForceAddNodes := GetWantedForceAddNodes(conf, thirdPartyBoundNodes)

	generalSelectedNodes := sets.NewString()
	for node := range localConfStatus.SelectedNodes {
		if !wantedForceAddNodes.Has(node) {
			generalSelectedNodes.Insert(node)
		}
	}

	return generalSelectedNodes
}

// GetForceSelectedNodes filter the selected nodes in conf.ForceAddNodes and thirdPartyBoundNodes.
func GetForceSelectedNodes(conf *sev1alpha1.ResourceFlavorConf, localConfStatus *sev1alpha1.ResourceFlavorConfStatus,
	thirdPartyBoundNodes sets.String) sets.String {
	wantedForceAddNodes := GetWantedForceAddNodes(conf, thirdPartyBoundNodes)

	forceSelectedNodes := sets.NewString()
	for node := range localConfStatus.SelectedNodes {
		if wantedForceAddNodes.Has(node) {
			forceSelectedNodes.Insert(node)
		}
	}

	return forceSelectedNodes
}
