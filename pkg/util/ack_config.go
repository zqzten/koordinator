package util

import (
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	ackapis "github.com/koordinator-sh/koordinator/apis/ackplugins"
	apiext "github.com/koordinator-sh/koordinator/apis/extension"
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

func DefaultMemoryLocalityStrategy() *ackapis.MemoryLocalityStrategy {
	return &ackapis.MemoryLocalityStrategy{
		LSRClass: &ackapis.MemoryLocalityQOS{
			MemoryLocality: DefaultMemoryLocality(apiext.QoSLSR),
		},
		LSClass: &ackapis.MemoryLocalityQOS{
			MemoryLocality: DefaultMemoryLocality(apiext.QoSLS),
		},
		BEClass: &ackapis.MemoryLocalityQOS{
			MemoryLocality: DefaultMemoryLocality(apiext.QoSBE),
		},
	}
}

func DefaultMemoryLocality(qos apiext.QoSClass) *ackapis.MemoryLocality {
	var memoryLocality *ackapis.MemoryLocality
	switch qos {
	case apiext.QoSLSR:
		memoryLocality = &ackapis.MemoryLocality{
			Policy:               ackapis.MemoryLocalityPolicyNone.Pointer(),
			MemoryLocalityConfig: *DefaultMemoryLocalityConfig(apiext.QoSLSR),
		}
	case apiext.QoSLS:
		memoryLocality = &ackapis.MemoryLocality{
			Policy:               ackapis.MemoryLocalityPolicyNone.Pointer(),
			MemoryLocalityConfig: *DefaultMemoryLocalityConfig(apiext.QoSLS),
		}
	case apiext.QoSBE:
		memoryLocality = &ackapis.MemoryLocality{
			Policy:               ackapis.MemoryLocalityPolicyNone.Pointer(),
			MemoryLocalityConfig: *DefaultMemoryLocalityConfig(apiext.QoSBE),
		}
	default:
		klog.Infof("memory locality has no auto config for qos %s", qos)

	}
	return memoryLocality
}

func DefaultMemoryLocalityConfig(qos apiext.QoSClass) *ackapis.MemoryLocalityConfig {
	var memoryLocality *ackapis.MemoryLocalityConfig
	switch qos {
	case apiext.QoSLSR:
		memoryLocality = &ackapis.MemoryLocalityConfig{
			TargetLocalityRatio:    pointer.Int64Ptr(100),
			MigrateIntervalMinutes: pointer.Int64Ptr(0),
		}
	case apiext.QoSLS:
		memoryLocality = &ackapis.MemoryLocalityConfig{
			TargetLocalityRatio:    pointer.Int64Ptr(100),
			MigrateIntervalMinutes: pointer.Int64Ptr(0),
		}
	case apiext.QoSBE:
		memoryLocality = &ackapis.MemoryLocalityConfig{
			TargetLocalityRatio:    pointer.Int64Ptr(100),
			MigrateIntervalMinutes: pointer.Int64Ptr(0),
		}
	default:
		klog.Infof("memory locality config has no auto config for qos %s", qos)
	}
	return memoryLocality
}

// DefaultPodMemoryLocality returns the default memory locality for auto policy of single pod
func DefaultPodMemoryLocalityConfig() *ackapis.MemoryLocalityConfig {
	memoryLocality := &ackapis.MemoryLocalityConfig{
		// targetLocalityRatio=100 means do memory locality anyway
		TargetLocalityRatio:    pointer.Int64Ptr(100),
		MigrateIntervalMinutes: pointer.Int64Ptr(0),
	}
	return memoryLocality
}

// NoneMemoryLocality returns the default configuration for memory locality strategy
//  as if enabled or not only depends on policy.
func NoneMemoryLocalityConfig() *ackapis.MemoryLocalityConfig {
	return &ackapis.MemoryLocalityConfig{
		TargetLocalityRatio:    pointer.Int64Ptr(100),
		MigrateIntervalMinutes: pointer.Int64(0),
	}
}
