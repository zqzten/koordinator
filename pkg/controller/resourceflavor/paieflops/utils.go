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
	"context"

	"gitlab.alibaba-inc.com/cos/scheduling-api/pkg/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/cache"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/flavor"
)

const (
	GpuModel        = "alibabacloud.com/gpu-card-model-detail"
	PointOfDelivery = "alibabacloud.com/point-of-delivery"
	ASWID           = "alibabacloud.com/asw-id"

	KubeSystem           = "kube-system"
	ElasticQuotaTreeName = "elasticquotatree"
	ResourceFlavorName   = extension.DomainPrefix + "resource-flavor-name"
	ResourceFlavorType   = extension.DomainPrefix + "resource-flavor-type"

	ResourceFlavorTypeAuto   = "Auto"
	ResourceFlavorTypeManual = "Manual"
)

func init() {
	flavor.GpuModel = GpuModel
	flavor.PointOfDelivery = PointOfDelivery
	flavor.ASWID = ASWID

	flavor.GetNodeGpuModel = GetNodeGpuModel
	flavor.GetNodePointOfDelivery = GetNodePointOfDelivery
	flavor.GetNodeASW = GetNodeASW
	flavor.GetAllThirdPartyBoundNodes = GetAllThirdPartyBoundNodes
	flavor.TryUpdateNode = TryUpdateNode
	flavor.FilterAndSortNodesByTopology = FilterAndSortNodesByTopology
}

// GetNodeGpuModel gpuModel like A100-SMX-80GB
func GetNodeGpuModel(node *corev1.Node) string {
	if node != nil && node.Labels != nil {
		return node.Labels[GpuModel]
	}

	return ""
}

// GetNodePointOfDelivery ASWID like VM-G6-P3
func GetNodePointOfDelivery(node *corev1.Node) string {
	if node != nil && node.Labels != nil {
		return node.Labels[PointOfDelivery]
	}

	return ""
}

// GetNodeASW ASW like ASW-VM-G6-P3-S15-2.NA130
func GetNodeASW(node *corev1.Node) string {
	if node != nil && node.Labels != nil {
		return node.Labels[ASWID]
	}

	return ""
}

// GetAllThirdPartyBoundNodes in pai-eflops case, used elasticQuotaTree for quota management, and they bind node to
// quota use patching label pair "quotaName:true" to node cr. so to recognize these manual bound nodes, we first get
// all quotaNames, and scan all node's labels to pick them out.
func GetAllThirdPartyBoundNodes(client client.Client, allNodes map[string]*corev1.Node) map[string]string {
	elasticQuotaTreeCr := GetElasticQuotaTreeCr(client)
	return getAllThirdPartyBoundNodes(elasticQuotaTreeCr, allNodes)
}

// getAllThirdPartyBoundNodes implement of GetAllThirdPartyBoundNodes
func getAllThirdPartyBoundNodes(elasticQuotaTreeCr *v1beta1.ElasticQuotaTree, allNodes map[string]*corev1.Node) map[string]string {
	//get all quotaNames
	allQuotaNames := sets.NewString()
	getAllQuotaNamesFromElasticQuotaTreeRecursive(&elasticQuotaTreeCr.Spec.Root, allQuotaNames)

	//get all manual bound nodes
	quotaBoundNodeByManuals := make(map[string]string)
	for nodeName, nodeCr := range allNodes {
		if bound, quotaName := quotaBoundNodeByManual(nodeCr.Labels, allQuotaNames); bound {
			quotaBoundNodeByManuals[nodeName] = quotaName
		}
	}

	return quotaBoundNodeByManuals
}

// GetElasticQuotaTreeCr return the elasticQuotaTreeCr
var GetElasticQuotaTreeCr = func(_client client.Client) *v1beta1.ElasticQuotaTree {

	elasticQuotaTreeCr := &v1beta1.ElasticQuotaTree{}
	err := _client.Get(context.TODO(), runtimeClient.ObjectKey{Namespace: KubeSystem, Name: ElasticQuotaTreeName}, elasticQuotaTreeCr)

	if err != nil {
		klog.Infof("Failed to get elasticQuotaTreeCr, ns:%v, name:%v", KubeSystem, ElasticQuotaTreeName)
		return nil
	}

	return elasticQuotaTreeCr
}

// getAllQuotaNamesFromElasticQuotaTreeRecursive get all quotaNames in elasticQuotaTree
func getAllQuotaNamesFromElasticQuotaTreeRecursive(root *v1beta1.ElasticQuotaSpec, allQuotaNames sets.String) {
	for _, children := range root.Children {
		allQuotaNames.Insert(children.Name)
		getAllQuotaNamesFromElasticQuotaTreeRecursive(&children, allQuotaNames)
	}
}

// quotaBoundNodeByManual get all manual bound nodes, to compatible with the concurrent scheduling method: 'node patch
// "quotaName:true" to labels, then pod's affinity also patch "quotaName:true"', after resourceFlavor select nodes automatically,
// resourceFlavor should also patch "quotaName:true" to node label. In order to diff auto or manual way, if in auto-case,
// we patch another label: "ResourceFlavorType:Auto".
// in the future, we will apply a new plugin in scheduler to filter bound node by quota, then we don't need to patch node's labels
// and pod's affinity.
func quotaBoundNodeByManual(labels map[string]string, allQuotaNames sets.String) (bool, string) {
	if labels[ResourceFlavorType] == ResourceFlavorTypeAuto {
		return false, ""
	}
	for quotaName := range allQuotaNames {
		if labels[quotaName] == "true" {
			return true, quotaName
		}
	}

	return false, ""
}

// TryUpdateNode to compatible with the concurrent scheduling method: 'node patch "quotaName:true" to labels,
// then pod's affinity also patch "quotaName:true"', after resourceFlavor select nodes automatically,
// resourceFlavor should also patch "quotaName:true" to node label. In order to diff auto or manual way, if in auto-case,
// we patch another label: "ResourceFlavorPatch:true".
// in the future, we will apply a new plugin in scheduler to filter bound node by quota, then we don't need to patch node's labels
// and pod's affinity.
func TryUpdateNode(client client.Client, flavorName string,
	flavorInfo *cache.ResourceFlavorInfo, nodeCache *cache.NodeCache) map[string]*corev1.Node {
	allSelectedNodes := flavorInfo.GetSelectedNodes()
	thirdPartyBoundNodes := nodeCache.GetThirdPartyBoundNodes(flavorName)

	newNodeCrMap := make(map[string]*corev1.Node)
	for nodeName, nodeBoundInfo := range allSelectedNodes {
		nodeCr := nodeCache.GetNodeCr(nodeName)
		if nodeCr != nil {
			isManual := thirdPartyBoundNodes.Has(nodeName)
			_, newNodeCr := doUpdateNodeCr(client, nodeCr, nodeBoundInfo.GetQuotaName(), isManual, true)
			newNodeCrMap[nodeName] = newNodeCr
		}
	}

	//means remove nodes
	allNodesCr := nodeCache.GetAllNodeCr()
	for nodeName, nodeCr := range allNodesCr {
		if allSelectedNodes[nodeName] == nil && nodeCr.Labels[ResourceFlavorName] == flavorName {
			isManual := thirdPartyBoundNodes.Has(nodeName)
			_, newNodeCr := doUpdateNodeCr(client, nodeCr, flavorName, isManual, false)
			newNodeCrMap[nodeName] = newNodeCr
		}
	}

	return newNodeCrMap
}

// doUpdateNodeCr to compatible with the concurrent scheduling method: 'node patch "quotaName:true" to labels,
// then pod's affinity also patch "quotaName:true"', after resourceFlavor select nodes automatically,
// resourceFlavor should also patch "quotaName:true" to node label. In order to diff auto or manual way, if in auto-case,
// we patch another label: "ResourceFlavorPatch:true".
// in the future, we will apply a new plugin in scheduler to filter bound node by quota, then we don't need to patch node's labels
// and pod's affinity.
func doUpdateNodeCr(_client client.Client, oriNode *corev1.Node, quotaName string, isManual bool, add bool) (bool, *corev1.Node) {
	// generate patch bytes for the update
	flavorType := ResourceFlavorTypeAuto
	if isManual {
		flavorType = ResourceFlavorTypeManual
	}

	var newNode *corev1.Node
	if add {
		if oriNode.Labels[quotaName] == "true" && oriNode.Labels[ResourceFlavorName] == quotaName &&
			oriNode.Labels[ResourceFlavorType] == flavorType {
			return false, oriNode
		}

		newNode = oriNode.DeepCopy()
		newNode.Labels[quotaName] = "true"
		newNode.Labels[ResourceFlavorName] = quotaName
		newNode.Labels[ResourceFlavorType] = flavorType
	} else {
		if isManual {
			if oriNode.Labels[ResourceFlavorName] == "" && oriNode.Labels[ResourceFlavorType] == "" {
				return false, oriNode
			}

			newNode = oriNode.DeepCopy()
			delete(newNode.Labels, ResourceFlavorName)
			delete(newNode.Labels, ResourceFlavorType)
		} else {
			if oriNode.Labels[quotaName] == "" && oriNode.Labels[ResourceFlavorName] == "" &&
				oriNode.Labels[ResourceFlavorType] == "" {
				return false, oriNode
			}

			newNode = oriNode.DeepCopy()
			delete(newNode.Labels, quotaName)
			delete(newNode.Labels, ResourceFlavorName)
			delete(newNode.Labels, ResourceFlavorType)
		}
	}

	// patch with pod client
	if _client != nil {
		err := _client.Patch(context.TODO(), newNode, client.MergeFrom(oriNode))
		if err != nil {
			klog.Errorf("Failed to patch node:%s, err: %v", newNode.Name, err)
		}
	}

	return true, newNode
}
