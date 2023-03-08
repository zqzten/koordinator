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
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/sharedlisterext"
)

func init() {
	sharedlisterext.RegisterNodeInfoTransformer(hookNodeInfoByOverQuota)
}

func hookNodeInfoByOverQuota(nodeInfo *framework.NodeInfo) {
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
