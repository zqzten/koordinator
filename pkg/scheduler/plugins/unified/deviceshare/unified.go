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
	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/deviceshare"
)

const (
	unifiedGPU            = 1 << 50
	unifiedGPUCore        = 1 << 51
	unifiedGPUMemory      = 1 << 52
	unifiedGPUMemoryRatio = 1 << 53
	aliyunGPUCompute      = 1 << 63
)

var DeviceResourceFlags = map[corev1.ResourceName]uint{
	unifiedresourceext.GPUResourceAlibaba:  unifiedGPU,
	unifiedresourceext.GPUResourceCore:     unifiedGPUCore,
	unifiedresourceext.GPUResourceMem:      unifiedGPUMemory,
	unifiedresourceext.GPUResourceMemRatio: unifiedGPUMemoryRatio,
	ack.AliyunGPUCompute:                   aliyunGPUCompute,
}

var ValidResourceCombinations = map[uint]bool{
	aliyunGPUCompute:                       true,
	unifiedGPU:                             true,
	unifiedGPUMemoryRatio:                  true,
	unifiedGPUCore | unifiedGPUMemory:      true,
	unifiedGPUCore | unifiedGPUMemoryRatio: true,
}

var ResourceValidators = map[corev1.ResourceName]func(q resource.Quantity) bool{
	ack.AliyunGPUCompute:                   deviceshare.ValidatePercentageResource,
	unifiedresourceext.GPUResourceCore:     deviceshare.ValidatePercentageResource,
	unifiedresourceext.GPUResourceMemRatio: deviceshare.ValidatePercentageResource,
	unifiedresourceext.GPUResourceAlibaba:  deviceshare.ValidatePercentageResource,
}

var ResourceCombinationsMapper = map[uint]func(podRequest corev1.ResourceList) corev1.ResourceList{
	unifiedGPUCore | unifiedGPUMemory: func(podRequest corev1.ResourceList) corev1.ResourceList {
		return corev1.ResourceList{
			apiext.ResourceGPUCore:   podRequest[unifiedresourceext.GPUResourceCore],
			apiext.ResourceGPUMemory: podRequest[unifiedresourceext.GPUResourceMem],
		}
	},
	unifiedGPUCore | unifiedGPUMemoryRatio: func(podRequest corev1.ResourceList) corev1.ResourceList {
		return corev1.ResourceList{
			apiext.ResourceGPUCore:        podRequest[unifiedresourceext.GPUResourceCore],
			apiext.ResourceGPUMemoryRatio: podRequest[unifiedresourceext.GPUResourceMemRatio],
		}
	},
	unifiedGPUMemoryRatio: func(podRequest corev1.ResourceList) corev1.ResourceList {
		return corev1.ResourceList{
			apiext.ResourceGPUCore:        podRequest[unifiedresourceext.GPUResourceMemRatio],
			apiext.ResourceGPUMemoryRatio: podRequest[unifiedresourceext.GPUResourceMemRatio],
		}
	},
	unifiedGPU: func(podRequest corev1.ResourceList) corev1.ResourceList {
		return corev1.ResourceList{
			apiext.ResourceGPUCore:        podRequest[unifiedresourceext.GPUResourceAlibaba],
			apiext.ResourceGPUMemoryRatio: podRequest[unifiedresourceext.GPUResourceAlibaba],
		}
	},
	aliyunGPUCompute: func(podRequest corev1.ResourceList) corev1.ResourceList {
		return corev1.ResourceList{
			apiext.ResourceGPUCore:        podRequest[ack.AliyunGPUCompute],
			apiext.ResourceGPUMemoryRatio: podRequest[ack.AliyunGPUCompute],
		}
	},
}

func init() {
	resourceNames := deviceshare.DeviceResourceNames[schedulingv1alpha1.GPU]
	for name, flag := range DeviceResourceFlags {
		deviceshare.DeviceResourceFlags[name] = flag
		resourceNames = append(resourceNames, name)
	}
	deviceshare.DeviceResourceNames[schedulingv1alpha1.GPU] = resourceNames
	for k, v := range ValidResourceCombinations {
		deviceshare.ValidDeviceResourceCombinations[k] = v
	}
	for k, v := range ResourceValidators {
		deviceshare.DeviceResourceValidators[k] = v
	}
	for k, v := range ResourceCombinationsMapper {
		deviceshare.ResourceCombinationsMapper[k] = v
	}
}
