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

package overquota

import (
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func HookNodeInfosWithOverQuota(nodeInfos []*framework.NodeInfo) (overQuotaNodeInfos []*framework.NodeInfo, anyNodeEnableOverQuota bool) {
	anyNodeEnableOverQuota = false
	for _, nodeInfo := range nodeInfos {
		overQuotaNodeInfo, isNodeEnableOverQuota := HookNodeInfoWithOverQuota(nodeInfo)
		if isNodeEnableOverQuota {
			anyNodeEnableOverQuota = true
		}
		overQuotaNodeInfos = append(overQuotaNodeInfos, overQuotaNodeInfo)
	}
	return
}

func HookNodeInfoWithOverQuota(nodeInfo *framework.NodeInfo) (overQuotaNodeInfo *framework.NodeInfo, isNodeEnableOverQuota bool) {
	if node := nodeInfo.Node(); node == nil || !IsNodeEnableOverQuota(node) {
		return nodeInfo, false
	}
	isNodeEnableOverQuota = true
	overQuotaNodeInfo = nodeInfo.Clone()
	updateNodeInfoByOverQuota(overQuotaNodeInfo)
	return
}

func updateNodeInfoByOverQuota(nodeInfo *framework.NodeInfo) {
	node := nodeInfo.Node()
	if node == nil {
		return
	}
	cpuOverQuotaRatioSpec, memoryOverQuotaRatioSpec, diskOverQuotaRatioSpec := extunified.GetResourceOverQuotaSpec(node)
	milliCPU := node.Status.Allocatable.Cpu().MilliValue()
	nodeInfo.Allocatable.MilliCPU = milliCPU * cpuOverQuotaRatioSpec / 100
	memoryBytes := node.Status.Allocatable.Memory().Value()
	nodeInfo.Allocatable.Memory = memoryBytes * memoryOverQuotaRatioSpec / 100
	diskBytes := node.Status.Allocatable.StorageEphemeral().Value()
	nodeInfo.Allocatable.EphemeralStorage = diskBytes * diskOverQuotaRatioSpec / 100
}
