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
	"encoding/json"

	"gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
)

const (
	AnnotationNodeCPUSharePool = "node.beta1.sigma.ali/cpu-sharepool"
	AnnotationLocalInfo        = "node.beta1.sigma.ali/local-info"
)

// CPUSharePool describes the CPU IDs that the cpu share pool uses.
// The entire struct is json marshalled and saved in pod annotation.
// It is referenced in pod's annotation by key AnnotationNodeCPUSharePool.
type CPUSharePool struct {
	CPUIDs []int `json:"cpuIDs,omitempty"`
}

func LocalInfoFromNode(node *corev1.Node) (*extension.LocalInfo, error) {
	localInfo := &extension.LocalInfo{}
	localData, ok := node.Annotations[extension.AnnotationLocalInfo]
	if !ok {
		localData, ok = node.Annotations[AnnotationLocalInfo]
	}
	if !ok {
		return localInfo, nil
	}

	if err := json.Unmarshal([]byte(localData), localInfo); err != nil {
		return nil, err
	}
	return localInfo, nil
}
