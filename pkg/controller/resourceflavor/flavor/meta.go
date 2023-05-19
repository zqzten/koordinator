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

	"k8s.io/klog"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/cache"
)

// TryUpdateNodeMeta update node meta for quotaNodeResourceFlavor
func (flavor *ResourceFlavor) TryUpdateNodeMeta(resourceFlavorName string, resourceFlavorInfo *cache.ResourceFlavorInfo) bool {
	if !resourceFlavorInfo.Enable() {
		return false
	}

	hasNodeMetaChanged := resourceFlavorInfo.TryUpdateNodeMeta(func(confName string, conf *sev1alpha1.ResourceFlavorConf,
		localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
		return flavor.updateNodeMeta(resourceFlavorName, confName, conf, localConfStatus)
	})

	return hasNodeMetaChanged
}

func (flavor *ResourceFlavor) updateNodeMeta(resourceFlavorName string, confName string,
	conf *sev1alpha1.ResourceFlavorConf, localConfStatus *sev1alpha1.ResourceFlavorConfStatus) bool {
	thirdPartyBoundNodes := flavor.nodeCache.GetThirdPartyBoundNodes(resourceFlavorName)
	forceNodesWanted := cache.GetWantedForceAddNodes(conf, thirdPartyBoundNodes)

	hasNodeMetaChanged := false
	for nodeName, nodeMeta := range localConfStatus.SelectedNodes {
		oldForceAddStatus := nodeMeta.NodeMetaInfo[ForceWanted] == "true"
		newForceAddStatus := forceNodesWanted.Has(nodeName)

		oldForceAddType := nodeMeta.NodeMetaInfo[ForceWantedType]
		newForceAddType := ""
		if newForceAddStatus {
			if thirdPartyBoundNodes.Has(nodeName) {
				newForceAddType = ForceAddTypeManual
			} else {
				newForceAddType = ForceAddTypeFlavor
			}
		}

		if oldForceAddStatus != newForceAddStatus || oldForceAddType != newForceAddType {
			hasNodeMetaChanged = true
			nodeMeta.NodeMetaInfo[ForceWanted] = fmt.Sprintf("%t", newForceAddStatus)
			nodeMeta.NodeMetaInfo[ForceWantedType] = newForceAddType

			klog.V(3).Infof("Find node force add status changed, resourceFlavorName:%v, confName:%v,"+
				"nodeName:%v, oldForceAddStatus:%v, newForceAddStatus:%v, oldForceAddType:%v, newForceAddType:%v",
				resourceFlavorName, confName, nodeName, oldForceAddStatus, newForceAddStatus, oldForceAddType, newForceAddType)
		}
	}

	return hasNodeMetaChanged
}
