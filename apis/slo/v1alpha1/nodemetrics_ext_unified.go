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

package v1alpha1

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
)

const (
	VirtualResourceKey = "virtual-resource"
)

// GetVirtualResource will retrieve virtual resource from given PodMetricInfo
func GetVirtualResource(info *PodMetricInfo) (corev1.ResourceList, error) {
	if info.Extensions == nil {
		return nil, nil
	}
	virtualResourceIf, exist := info.Extensions.Object[VirtualResourceKey]
	if !exist {
		return nil, nil
	}
	virtualResourceStr, err := json.Marshal(virtualResourceIf)
	if err != nil {
		return nil, err
	}
	virtualResource := corev1.ResourceList{}
	if err := json.Unmarshal(virtualResourceStr, &virtualResource); err != nil {
		return nil, err
	}
	return virtualResource, nil
}

// SetVirtualResource will update PodMetricInfo using values reported by batch-agent under kata scenario.
func SetVirtualResource(info *PodMetricInfo, resource corev1.ResourceList) {
	if info == nil {
		return
	}
	if info.Extensions == nil {
		info.Extensions = &ExtensionsMap{Object: map[string]interface{}{}}
	}
	info.Extensions.Object[VirtualResourceKey] = resource
}
