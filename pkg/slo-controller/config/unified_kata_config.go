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

	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/configuration"
	"github.com/koordinator-sh/koordinator/pkg/util/sloconfig"
)

const (
	KataResExtKey = "kataResource"
)

type KataResourceConfig struct {
	Enable *bool `json:"enable,omitempty"`
}

var (
	defaultKataResourceConfig = &KataResourceConfig{
		Enable: pointer.BoolPtr(false),
	}
)

func init() {
	sloconfig.RegisterDefaultColocationExtension(KataResExtKey, defaultKataResourceConfig)
}

func ParseKataResourceConfig(strategy *configuration.ColocationStrategy) (*KataResourceConfig, error) {
	if strategy == nil || strategy.Extensions == nil {
		return nil, nil
	}
	cfgIf, exist := strategy.Extensions[KataResExtKey]
	if !exist {
		return defaultKataResourceConfig, nil
	}
	cfgStr, err := json.Marshal(cfgIf)
	if err != nil {
		return nil, fmt.Errorf("KataResourceConfig interface json marshal failed, err: %v", err)
	}
	cfg := &KataResourceConfig{}
	if err := json.Unmarshal(cfgStr, cfg); err != nil {
		return nil, fmt.Errorf("KataResourceConfig json unmarshal convert failed, err: %v", err)
	}
	return cfg, nil
}
