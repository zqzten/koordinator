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

package unified

import (
	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	corev1 "k8s.io/api/core/v1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	LabelGPUCardModel           = "alibabacloud.com/gpu-card-model"
	LabelTenantDLC              = "tenant.dlc.alibaba-inc.com"
	LabelTenantDLCMachineGroup  = "tenant.dlc.alibaba-inc.com/machinegroup"
	LabelTenantDLCResourceGroup = "tenant.dlc.alibaba-inc.com/resourcegroup"
)

const (
	GPUCardRatio                          = apiext.DomainPrefix + "gpu-card-ratio"
	EnvActivelyAddedUnifiedGPUMemoryRatio = "ACTIVELY_ADDED_GPU_MEM_RATIO"
)

var (
	koordGPUResourcesToUnified = map[corev1.ResourceName]corev1.ResourceName{
		apiext.ResourceGPU:            unifiedresourceext.GPUResourceAlibaba,
		apiext.ResourceGPUCore:        unifiedresourceext.GPUResourceCore,
		apiext.ResourceGPUMemory:      unifiedresourceext.GPUResourceMem,
		apiext.ResourceGPUMemoryRatio: unifiedresourceext.GPUResourceMemRatio,
	}
	unifiedGPUResourcesToKoord = map[corev1.ResourceName]corev1.ResourceName{
		unifiedresourceext.GPUResourceAlibaba:  apiext.ResourceGPU,
		unifiedresourceext.GPUResourceCore:     apiext.ResourceGPUCore,
		unifiedresourceext.GPUResourceMem:      apiext.ResourceGPUMemory,
		unifiedresourceext.GPUResourceMemRatio: apiext.ResourceGPUMemoryRatio,
	}
)

func ConvertToUnifiedGPUResources(resourceList corev1.ResourceList) corev1.ResourceList {
	if len(resourceList) == 0 {
		return nil
	}
	resources := make(corev1.ResourceList)
	for resourceName, quantity := range resourceList {
		name, ok := koordGPUResourcesToUnified[resourceName]
		if !ok {
			continue
		}
		resources[name] = quantity
	}
	return resources
}

func ConvertToKoordGPUResources(resourceList corev1.ResourceList) corev1.ResourceList {
	if len(resourceList) == 0 {
		return nil
	}
	resources := make(corev1.ResourceList)
	for resourceName, quantity := range resourceList {
		name, ok := unifiedGPUResourcesToKoord[resourceName]
		if !ok {
			continue
		}
		resources[name] = quantity
	}
	return resources
}

func IsContainerActivelyAddedGPUMemRatio(c *corev1.Container) bool {
	for _, entry := range c.Env {
		if entry.Name == EnvActivelyAddedUnifiedGPUMemoryRatio && entry.Value == "true" {
			return true
		}
	}
	return false
}
