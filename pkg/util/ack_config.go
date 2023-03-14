package util

import (
	"k8s.io/utils/pointer"

	ackapis "github.com/koordinator-sh/koordinator/apis/ackplugins"
)

func DefaultDsaAccelerateStrategy() *ackapis.DsaAccelerateStrategy {
	return &ackapis.DsaAccelerateStrategy{
		DsaAccelerateConfig: DefaultDsaAccelerateConfig(),
		Enable:              pointer.BoolPtr(true),
	}
}

func DefaultDsaAccelerateConfig() ackapis.DsaAccelerateConfig {
	return ackapis.DsaAccelerateConfig{
		PollingMode: pointer.BoolPtr(false),
	}
}
