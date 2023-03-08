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

package deviceshare

import (
	"sync/atomic"

	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/sharedlisterext"
)

var hook = &nodeInfoHook{}

type nodeInfoHook struct {
	value atomic.Value
}

func init() {
	sharedlisterext.RegisterNodeInfoTransformer(hook.hookNodeInfo)
}

func (h *nodeInfoHook) hookNodeInfo(nodeInfo *framework.NodeInfo) {
	deviceCache, ok := h.value.Load().(*nodeDeviceCache)
	if !ok {
		return
	}

	node := nodeInfo.Node()

	nodeDevice := deviceCache.getNodeDevice(node.Name)
	if nodeDevice == nil {
		return
	}
	gpuResources := getUnifiedGPUResourcesFromDeviceCache(nodeDevice)
	if len(gpuResources) > 0 {
		if nodeInfo.Allocatable.ScalarResources == nil {
			nodeInfo.Allocatable.ScalarResources = make(map[corev1.ResourceName]int64)
		}
		for resourceName, quantity := range gpuResources {
			if _, ok := nodeInfo.Allocatable.ScalarResources[resourceName]; ok {
				continue
			}
			nodeInfo.Allocatable.ScalarResources[resourceName] = quantity.Value()
		}
	}
}

func getUnifiedGPUResourcesFromDeviceCache(nodeDevice *nodeDevice) corev1.ResourceList {
	gpuResources := make(corev1.ResourceList)
	nodeDevice.lock.RLock()
	defer nodeDevice.lock.RUnlock()
	totalDeviceResources := nodeDevice.deviceTotal[schedulingv1alpha1.GPU]
	if len(totalDeviceResources) > 0 {
		totalGPUResources := make(corev1.ResourceList)
		for _, v := range totalDeviceResources {
			totalGPUResources = quotav1.Add(totalGPUResources, v)
		}
		gpuResources = extunified.ConvertToUnifiedGPUResources(totalGPUResources)
	}

	if memoryRatio, ok := gpuResources[unifiedresourceext.GPUResourceMemRatio]; ok {
		gpuCount := memoryRatio.Value() / 100
		gpuResources[apiext.ResourceNvidiaGPU] = *resource.NewQuantity(gpuCount, resource.DecimalSI)
	}
	return gpuResources
}
