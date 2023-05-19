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
	corev1 "k8s.io/api/core/v1"
	v1helper "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/klog"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/cache"
)

// TryRemoveInvalidNodes remove nodes for resourceFlavor
func (flavor *ResourceFlavor) TryRemoveInvalidNodes(flavorName string, flavorInfo *cache.ResourceFlavorInfo) bool {
	hasNewRemoved := false

	//if resourceFlavor enable flag=false, then we remove all nodes for all conf.
	if !flavorInfo.Enable() {
		if flavorInfo.RemoveAllLocalConfStatus(func(confName string, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
			localConfStatus.SelectedNodeNum = 0
			localConfStatus.SelectedNodes = make(map[string]*sev1alpha1.SelectedNodeMeta)
			return true
		}) {
			hasNewRemoved = true
		}
		return hasNewRemoved
	}

	//when resourceFlavor add some new conf and delete some conf, we should release the nodes from the deleted-conf.
	if flavorInfo.RemoveAllInvalidLocalConfStatus(func(confName string, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
		localConfStatus.SelectedNodeNum = 0
		localConfStatus.SelectedNodes = make(map[string]*sev1alpha1.SelectedNodeMeta)
		return true
	}) {
		hasNewRemoved = true
	}

	//foreach all conf, when some selected nodes no longer satisfy the conf-requirement, we should remove them.
	if flavorInfo.TryRemoveNodes(func(confName string, conf *sev1alpha1.ResourceFlavorConf, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
		return flavor.removeNodes(flavorName, confName, conf, localConfStatus)
	}) {
		hasNewRemoved = true
	}

	return hasNewRemoved
}

// removeNodes do remove node logic for each conf.
func (flavor *ResourceFlavor) removeNodes(flavorName string, confName string,
	conf *sev1alpha1.ResourceFlavorConf, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {

	thirdPartyBoundNodes := flavor.nodeCache.GetThirdPartyBoundNodes(flavorName)
	generalSelectedNodes := cache.GetGeneralSelectedNodes(conf, localConfStatus, thirdPartyBoundNodes)

	//first remove not satisfy conf-requirement nodes.
	hasNewRemove := false
	for nodeName := range generalSelectedNodes {
		if flavor.IsNodeShouldRemove(flavorName, confName, nodeName, conf) {
			cache.RemoveNodeFromLocalConfStatus(nodeName, localConfStatus)
			hasNewRemove = true
		}
	}

	//then try to remove nodes due to configNodeNum reason.
	if cache.RemoveUnnecessaryNodes(conf, localConfStatus, thirdPartyBoundNodes) {
		hasNewRemove = true
	}

	return hasNewRemove
}

// IsNodeShouldRemove judge one node whether satisfy conf-requirement.
func (flavor *ResourceFlavor) IsNodeShouldRemove(resourceFlavorName string, confName string,
	nodeName string, conf *sev1alpha1.ResourceFlavorConf) bool {

	//if node not exist
	nodeCr := flavor.nodeCache.GetNodeCr(nodeName)
	if nodeCr == nil {
		klog.Errorf("find node should remove due to node has been deleted, "+
			"resourceFlavorName:%v, confName:%v, nodeName:%v", resourceFlavorName, confName, nodeName)
		return true
	}

	//if node affinity not match
	if conf.NodeAffinity != nil {
		affinity := GetNodeAffinity(conf.NodeAffinity)
		if match, err := MatchesNodeSelectorAndAffinityTerms(affinity, nodeCr); !match {
			errMsg := ""
			if err != nil {
				errMsg = err.Error()
			}
			klog.V(3).Infof("find node should remove due to node affinity check failed, "+
				"resourceFlavorName:%v, confName:%v, nodeName:%v, err:%v", resourceFlavorName, confName, nodeName, errMsg)
			return true
		}
	}

	//if node toleration not match
	_, isUnTolerated := v1helper.FindMatchingUntoleratedTaint(nodeCr.Spec.Taints, conf.Toleration, func(t *corev1.Taint) bool {
		return t.Effect == corev1.TaintEffectNoSchedule || t.Effect == corev1.TaintEffectNoExecute
	})
	if isUnTolerated {
		klog.V(3).Infof("find node should remove due to toleration check failed, "+
			"resourceFlavorName:%v, confName:%v, nodeName:%v", resourceFlavorName, confName, nodeName)
		return true
	}

	return false
}
