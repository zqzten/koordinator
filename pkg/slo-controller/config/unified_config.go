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

package config

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/configuration"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/util/sloconfig"
)

const (
	// DynamicProdResourceExtKey is the config key for the dynamic Prod resource overcommitment.
	// TODO: move the config outside the colocation-config
	DynamicProdResourceExtKey = "dynamicProdResource"
)

func init() {
	_ = sloconfig.RegisterDefaultColocationExtension(DynamicProdResourceExtKey, &unified.DynamicProdResourceConfig{})
}

var DefaultProdOvercommitPolicy = unified.ProdOvercommitPolicyNone

var DefaultDynamicProdResourceConfig = &unified.DynamicProdResourceConfig{
	ProdOvercommitPolicy:           &DefaultProdOvercommitPolicy,
	ProdCPUOvercommitMinPercent:    pointer.Int64(100),
	ProdMemoryOvercommitMinPercent: pointer.Int64(100),
}

func ParseDynamicProdResourceConfig(strategy *configuration.ColocationStrategy) (*unified.DynamicProdResourceConfig, error) {
	if strategy == nil {
		return nil, fmt.Errorf("strategy is nil")
	}
	if strategy.Extensions == nil {
		klog.V(5).Infof("abort to parse dynamic prod resource config, colocation extensions is nil, use the default")
		return DefaultDynamicProdResourceConfig, nil
	}
	cfgIf, exist := strategy.Extensions[DynamicProdResourceExtKey]
	if !exist {
		klog.V(5).Infof("abort to parse dynamic prod resource config, cfg data is nil, use the default")
		return DefaultDynamicProdResourceConfig, nil
	}
	cfgStr, err := json.Marshal(cfgIf)
	if err != nil {
		return nil, fmt.Errorf("marshal raw DynamicProdResourceConfig failed, data %v", cfgIf)
	}

	// merge with default
	cfg := DefaultDynamicProdResourceConfig.DeepCopy()
	if err := json.Unmarshal(cfgStr, cfg); err != nil {
		return nil, fmt.Errorf("unmarshal DynamicProdResourceConfig failed, err: %s", err)
	}
	return cfg, nil
}
