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

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

// DynamicProdResourceConfig defines the configuration for dynamic Prod overcommitment.
// +k8s:deepcopy-gen=true
type DynamicProdResourceConfig struct {
	ProdOvercommitPolicy *ProdOvercommitPolicy `json:"prodOvercommitPolicy,omitempty"`
	// ProdCPUOvercommitDefaultPercent is the default cpu ratio which the prod resource will reset to it when the
	// overcommit policy is set to `static` or the node metrics is expired (if not set, do nothing).
	ProdCPUOvercommitDefaultPercent *int64 `json:"prodCPUOvercommitDefaultPercent,omitempty"`
	// ProdCPUOvercommitMaxPercent indicates the maximum ratio of prod overcommit CPU resource dividing node allocatable.
	// If the calculated prod allocatable is larger than the maximum allocatable, the manager updates with the maximal.
	ProdCPUOvercommitMaxPercent *int64 `json:"prodCPUOvercommitMaxPercent,omitempty"`
	// ProdCPUOvercommitMinPercent indicates the minimum ratio of prod overcommit CPU resource dividing node allocatable.
	// If the calculated prod allocatable is less than the minimum allocatable, the manager updates with the minimal.
	ProdCPUOvercommitMinPercent *int64 `json:"prodCPUOvercommitMinPercent,omitempty"`
	// ProdMemoryOvercommitDefaultPercent is the default memory ratio which the prod resource will reset to it when the
	// overcommit policy is set to `static` or the node metrics is expired (if not set, do nothing).
	ProdMemoryOvercommitDefaultPercent *int64 `json:"prodMemoryOvercommitDefaultPercent,omitempty"`
	// ProdMemoryOvercommitMaxPercent indicates the maximum ratio of prod overcommit memory resource dividing node allocatable.
	// If the calculated prod allocatable is larger than the maximum allocatable, the manager updates with the maximal.
	ProdMemoryOvercommitMaxPercent *int64 `json:"prodMemoryOvercommitMaxPercent,omitempty"`
	// ProdMemoryOvercommitMinPercent indicates the minimum ratio of prod overcommit CPU resource dividing node allocatable.
	// If the calculated prod allocatable is less than the minimum allocatable, the manager updates with the minimal.
	ProdMemoryOvercommitMinPercent *int64 `json:"prodMemoryOvercommitMinPercent,omitempty"`
}

// ProdOvercommitPolicy determines how the Prod overcommitment take effect on resource allocatable.
type ProdOvercommitPolicy string

const (
	// ProdOvercommitPolicyNone indicates that the prod overcommitment is fully-disabled. The manager neither update
	// the prod allocatable on node nor calculate or record the overcommit result.
	ProdOvercommitPolicyNone ProdOvercommitPolicy = "none"
	// ProdOvercommitPolicyDryRun indicates that the manager does not update the prod allocatable with the calculated
	// result, but record dry-run result in label and metrics to show the estimated values.
	ProdOvercommitPolicyDryRun ProdOvercommitPolicy = "dryRun"
	// ProdOvercommitPolicyStatic indicates that the manager updates the prod allocatable with a fixed ratio, instead
	// of the calculated result from node metrics.
	ProdOvercommitPolicyStatic ProdOvercommitPolicy = "static"
	// ProdOvercommitPolicyAuto indicates that the manager update the prod allocatable with the calculated result
	// automatically.
	ProdOvercommitPolicyAuto ProdOvercommitPolicy = "auto"
)

const (
	// AnnotationDynamicProdConfig is the node annotation key of dynamic prod resource overcommitment config.
	AnnotationDynamicProdConfig = apiext.NodeDomainPrefix + "/dynamic-prod-config"
)

// NodeDynamicProdConfig is the node-level configuration for dynamic prod resource overcommitment.
// +k8s:deepcopy-gen=true
type NodeDynamicProdConfig struct {
	DynamicProdResourceConfig `json:",inline"`
}

func GetNodeDynamicProdConfig(annotations map[string]string) (*NodeDynamicProdConfig, error) {
	data := annotations[AnnotationDynamicProdConfig]
	if data == "" {
		return nil, nil
	}
	cfg := &NodeDynamicProdConfig{}
	if err := json.Unmarshal([]byte(data), cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
