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

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

const (
	CgroupExtKey = "cgroups"
)

// +k8s:deepcopy-gen=false
type NodeCgroups []*resourcesv1alpha1.NodeCgroupsInfo

func GetNodeCgroups(nodeSLOSpec *NodeSLOSpec) (NodeCgroups, error) {
	if nodeSLOSpec.Extensions == nil {
		return nil, nil
	}
	nodeCgroupsIf, exist := nodeSLOSpec.Extensions.Object[CgroupExtKey]
	if !exist {
		return nil, nil
	}
	nodeCgroupsStr, err := json.Marshal(nodeCgroupsIf)
	if err != nil {
		return nil, err
	}
	nodeCgroup := NodeCgroups{}
	if err := json.Unmarshal(nodeCgroupsStr, &nodeCgroup); err != nil {
		return nil, err
	}
	return nodeCgroup, nil
}

func SetNodeCgroups(nodeSLOSpec *NodeSLOSpec, nodeCgroups NodeCgroups) {
	if nodeSLOSpec == nil {
		return
	}
	if nodeSLOSpec.Extensions == nil {
		nodeSLOSpec.Extensions = &ExtensionsMap{Object: map[string]interface{}{}}
	}
	nodeSLOSpec.Extensions.Object[CgroupExtKey] = nodeCgroups
}
