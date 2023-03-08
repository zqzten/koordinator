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

package gpumodel

import (
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	koordinatorinformers "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions"
)

var GPUResourceMem = extension.ResourceGPUMemory

var GlobalGPUModelCache *GPUModelCache

type GPUModelCache struct {
	mut                        sync.Mutex
	koordSharedInformerFactory koordinatorinformers.SharedInformerFactory
	ModelToMemCapacity         map[string]int64
	NodeToGpuModel             map[string]string
}

func GetNodeGPUModel(nodeLabels map[string]string) string {
	return nodeLabels[extension.LabelGPUModel]
}

func (gm *GPUModelCache) UpdateByNode(node *v1.Node) {
	deviceObj, err := gm.koordSharedInformerFactory.Scheduling().V1alpha1().Devices().Lister().Get(node.Name)

	if err != nil {
		klog.Errorf("node has gpu resource but can not find Device, err: %v", err)
		return
	}

	gm.UpdateByNodeInternal(node, deviceObj)
}

func (gm *GPUModelCache) UpdateByNodeInternal(node *v1.Node, device *v1alpha1.Device) {
	gpuModel := GetNodeGPUModel(node.Labels)
	if gpuModel == "" {
		return
	}

	gm.mut.Lock()
	defer gm.mut.Unlock()

	gm.NodeToGpuModel[node.Name] = gpuModel

	// model to memory capacity has existed.
	if c, ok := gm.ModelToMemCapacity[gpuModel]; ok && c > 0 {
		return
	}

	var gpuDevice *v1alpha1.DeviceInfo
	for i := range device.Spec.Devices {
		if device.Spec.Devices[i].Type == v1alpha1.GPU {
			gpuDevice = &device.Spec.Devices[i]
		}
	}
	if gpuDevice == nil {
		return
	}

	if quant, ok := gpuDevice.Resources[GPUResourceMem]; ok {
		gm.ModelToMemCapacity[gpuModel] = quant.Value()
		return
	}
}

func (gm *GPUModelCache) GetNodeGPUModel(nodeName string) string {
	gm.mut.Lock()
	defer gm.mut.Unlock()

	return gm.NodeToGpuModel[nodeName]
}

func (gm *GPUModelCache) GetMaxCapacity() int64 {
	gm.mut.Lock()
	defer gm.mut.Unlock()

	result := int64(0)
	for _, capacity := range gm.ModelToMemCapacity {
		if capacity > result {
			result = capacity
		}
	}
	return result
}

func (gm *GPUModelCache) GetModelMemoryCapacity(gpuModel string) int64 {
	gm.mut.Lock()
	defer gm.mut.Unlock()

	return gm.ModelToMemCapacity[gpuModel]
}

// GetPodGPUModel if the pod is assigned, get model from GPUModelCache
func GetPodGPUModel(pod *v1.Pod) string {
	if pod == nil {
		return ""
	}
	if pod.Spec.NodeName != "" {
		if model := GlobalGPUModelCache.GetNodeGPUModel(pod.Spec.NodeName); model != "" {
			return model
		}
	}

	if pod.Spec.NodeSelector != nil {
		if model, ok := pod.Spec.NodeSelector[extension.LabelGPUModel]; ok {
			return model
		}
	}

	if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil &&
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil &&
		len(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) > 0 {
		for i := range pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			term := &pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[i]
			for ei := range term.MatchExpressions {
				e := term.MatchExpressions[ei]
				gpuModel := ""
				if e.Key == extension.LabelGPUModel && e.Operator == v1.NodeSelectorOpIn && len(e.Values) == 1 {
					gpuModel = e.Values[0]
				}
				if gpuModel != "" {
					return gpuModel
				}
			}
		}
	}
	return ""
}
