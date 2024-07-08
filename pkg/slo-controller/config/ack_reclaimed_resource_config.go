package config

import (
	"encoding/json"
	"fmt"

	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	ReclaimedResExtKey = "reclaimedResource"
)

func init() {
	RegisterDefaultColocationExtension(ReclaimedResExtKey, defaultReclaimedResourceConfig)

}

type ReclaimedResourceConfig struct {
	NodeUpdate *bool `json:"nodeUpdate,omitempty"`
}

var (
	defaultReclaimedResourceConfig = &ReclaimedResourceConfig{
		NodeUpdate: pointer.BoolPtr(true),
	}
)

func ParseReclaimedResourceConfig(strategy *extension.ColocationStrategy) (*ReclaimedResourceConfig, error) {
	if strategy == nil || strategy.Extensions == nil {
		return nil, nil
	}
	cfgIf, exist := strategy.Extensions[ReclaimedResExtKey]
	if !exist {
		return defaultReclaimedResourceConfig, nil
	}

	cfgStr, err := json.Marshal(cfgIf)
	if err != nil {
		return nil, fmt.Errorf("ReclaimedResourceConfig interface json marshal failed")
	}

	cfg := &ReclaimedResourceConfig{}
	if err := json.Unmarshal(cfgStr, cfg); err != nil {
		return nil, fmt.Errorf("ReclaimedResourceConfig json unmarshal convert failed")
	}
	return cfg, nil
}
