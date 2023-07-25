//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by conversion-gen. DO NOT EDIT.

package v1beta2

import (
	unsafe "unsafe"

	extension "github.com/koordinator-sh/koordinator/apis/extension"
	config "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
	configv1beta2 "k8s.io/kube-scheduler/config/v1beta2"
	apisconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
)

func init() {
	localSchemeBuilder.Register(RegisterConversions)
}

// RegisterConversions adds conversion functions to the given scheme.
// Public to allow building arbitrary schemes.
func RegisterConversions(s *runtime.Scheme) error {
	if err := s.AddGeneratedConversionFunc((*CachedPodArgs)(nil), (*config.CachedPodArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_CachedPodArgs_To_config_CachedPodArgs(a.(*CachedPodArgs), b.(*config.CachedPodArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.CachedPodArgs)(nil), (*CachedPodArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_CachedPodArgs_To_v1beta2_CachedPodArgs(a.(*config.CachedPodArgs), b.(*CachedPodArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*CoschedulingArgs)(nil), (*config.CoschedulingArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_CoschedulingArgs_To_config_CoschedulingArgs(a.(*CoschedulingArgs), b.(*config.CoschedulingArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.CoschedulingArgs)(nil), (*CoschedulingArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_CoschedulingArgs_To_v1beta2_CoschedulingArgs(a.(*config.CoschedulingArgs), b.(*CoschedulingArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*DeviceShareArgs)(nil), (*config.DeviceShareArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_DeviceShareArgs_To_config_DeviceShareArgs(a.(*DeviceShareArgs), b.(*config.DeviceShareArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.DeviceShareArgs)(nil), (*DeviceShareArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_DeviceShareArgs_To_v1beta2_DeviceShareArgs(a.(*config.DeviceShareArgs), b.(*DeviceShareArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*ElasticQuotaArgs)(nil), (*config.ElasticQuotaArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_ElasticQuotaArgs_To_config_ElasticQuotaArgs(a.(*ElasticQuotaArgs), b.(*config.ElasticQuotaArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.ElasticQuotaArgs)(nil), (*ElasticQuotaArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_ElasticQuotaArgs_To_v1beta2_ElasticQuotaArgs(a.(*config.ElasticQuotaArgs), b.(*ElasticQuotaArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*LimitAwareArgs)(nil), (*config.LimitAwareArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_LimitAwareArgs_To_config_LimitAwareArgs(a.(*LimitAwareArgs), b.(*config.LimitAwareArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.LimitAwareArgs)(nil), (*LimitAwareArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_LimitAwareArgs_To_v1beta2_LimitAwareArgs(a.(*config.LimitAwareArgs), b.(*LimitAwareArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*LoadAwareSchedulingAggregatedArgs)(nil), (*config.LoadAwareSchedulingAggregatedArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_LoadAwareSchedulingAggregatedArgs_To_config_LoadAwareSchedulingAggregatedArgs(a.(*LoadAwareSchedulingAggregatedArgs), b.(*config.LoadAwareSchedulingAggregatedArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.LoadAwareSchedulingAggregatedArgs)(nil), (*LoadAwareSchedulingAggregatedArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_LoadAwareSchedulingAggregatedArgs_To_v1beta2_LoadAwareSchedulingAggregatedArgs(a.(*config.LoadAwareSchedulingAggregatedArgs), b.(*LoadAwareSchedulingAggregatedArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.LoadAwareSchedulingArgs)(nil), (*LoadAwareSchedulingArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_LoadAwareSchedulingArgs_To_v1beta2_LoadAwareSchedulingArgs(a.(*config.LoadAwareSchedulingArgs), b.(*LoadAwareSchedulingArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*NodeNUMAResourceArgs)(nil), (*config.NodeNUMAResourceArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_NodeNUMAResourceArgs_To_config_NodeNUMAResourceArgs(a.(*NodeNUMAResourceArgs), b.(*config.NodeNUMAResourceArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.NodeNUMAResourceArgs)(nil), (*NodeNUMAResourceArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_NodeNUMAResourceArgs_To_v1beta2_NodeNUMAResourceArgs(a.(*config.NodeNUMAResourceArgs), b.(*NodeNUMAResourceArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*ReservationArgs)(nil), (*config.ReservationArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_ReservationArgs_To_config_ReservationArgs(a.(*ReservationArgs), b.(*config.ReservationArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.ReservationArgs)(nil), (*ReservationArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_ReservationArgs_To_v1beta2_ReservationArgs(a.(*config.ReservationArgs), b.(*ReservationArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*ScoringStrategy)(nil), (*config.ScoringStrategy)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_ScoringStrategy_To_config_ScoringStrategy(a.(*ScoringStrategy), b.(*config.ScoringStrategy), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.ScoringStrategy)(nil), (*ScoringStrategy)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_ScoringStrategy_To_v1beta2_ScoringStrategy(a.(*config.ScoringStrategy), b.(*ScoringStrategy), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*LoadAwareSchedulingArgs)(nil), (*config.LoadAwareSchedulingArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_LoadAwareSchedulingArgs_To_config_LoadAwareSchedulingArgs(a.(*LoadAwareSchedulingArgs), b.(*config.LoadAwareSchedulingArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*UnifiedPodConstraintArgs)(nil), (*config.UnifiedPodConstraintArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_UnifiedPodConstraintArgs_To_config_UnifiedPodConstraintArgs(a.(*UnifiedPodConstraintArgs), b.(*config.UnifiedPodConstraintArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.UnifiedPodConstraintArgs)(nil), (*UnifiedPodConstraintArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_UnifiedPodConstraintArgs_To_v1beta2_UnifiedPodConstraintArgs(a.(*config.UnifiedPodConstraintArgs), b.(*UnifiedPodConstraintArgs), scope)
	}); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1beta2_CachedPodArgs_To_config_CachedPodArgs(in *CachedPodArgs, out *config.CachedPodArgs, s conversion.Scope) error {
	out.Network = in.Network
	out.Address = in.Address
	out.ServerCert = *(*[]byte)(unsafe.Pointer(&in.ServerCert))
	out.ServerKey = *(*[]byte)(unsafe.Pointer(&in.ServerKey))
	return nil
}

// Convert_v1beta2_CachedPodArgs_To_config_CachedPodArgs is an autogenerated conversion function.
func Convert_v1beta2_CachedPodArgs_To_config_CachedPodArgs(in *CachedPodArgs, out *config.CachedPodArgs, s conversion.Scope) error {
	return autoConvert_v1beta2_CachedPodArgs_To_config_CachedPodArgs(in, out, s)
}

func autoConvert_config_CachedPodArgs_To_v1beta2_CachedPodArgs(in *config.CachedPodArgs, out *CachedPodArgs, s conversion.Scope) error {
	out.Network = in.Network
	out.Address = in.Address
	out.ServerCert = *(*[]byte)(unsafe.Pointer(&in.ServerCert))
	out.ServerKey = *(*[]byte)(unsafe.Pointer(&in.ServerKey))
	return nil
}

// Convert_config_CachedPodArgs_To_v1beta2_CachedPodArgs is an autogenerated conversion function.
func Convert_config_CachedPodArgs_To_v1beta2_CachedPodArgs(in *config.CachedPodArgs, out *CachedPodArgs, s conversion.Scope) error {
	return autoConvert_config_CachedPodArgs_To_v1beta2_CachedPodArgs(in, out, s)
}

func autoConvert_v1beta2_CoschedulingArgs_To_config_CoschedulingArgs(in *CoschedulingArgs, out *config.CoschedulingArgs, s conversion.Scope) error {
	if err := v1.Convert_Pointer_v1_Duration_To_v1_Duration(&in.DefaultTimeout, &out.DefaultTimeout, s); err != nil {
		return err
	}
	if err := v1.Convert_Pointer_int64_To_int64(&in.ControllerWorkers, &out.ControllerWorkers, s); err != nil {
		return err
	}
	if err := v1.Convert_Pointer_bool_To_bool(&in.SkipCheckScheduleCycle, &out.SkipCheckScheduleCycle, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1beta2_CoschedulingArgs_To_config_CoschedulingArgs is an autogenerated conversion function.
func Convert_v1beta2_CoschedulingArgs_To_config_CoschedulingArgs(in *CoschedulingArgs, out *config.CoschedulingArgs, s conversion.Scope) error {
	return autoConvert_v1beta2_CoschedulingArgs_To_config_CoschedulingArgs(in, out, s)
}

func autoConvert_config_CoschedulingArgs_To_v1beta2_CoschedulingArgs(in *config.CoschedulingArgs, out *CoschedulingArgs, s conversion.Scope) error {
	if err := v1.Convert_v1_Duration_To_Pointer_v1_Duration(&in.DefaultTimeout, &out.DefaultTimeout, s); err != nil {
		return err
	}
	if err := v1.Convert_int64_To_Pointer_int64(&in.ControllerWorkers, &out.ControllerWorkers, s); err != nil {
		return err
	}
	if err := v1.Convert_bool_To_Pointer_bool(&in.SkipCheckScheduleCycle, &out.SkipCheckScheduleCycle, s); err != nil {
		return err
	}
	return nil
}

// Convert_config_CoschedulingArgs_To_v1beta2_CoschedulingArgs is an autogenerated conversion function.
func Convert_config_CoschedulingArgs_To_v1beta2_CoschedulingArgs(in *config.CoschedulingArgs, out *CoschedulingArgs, s conversion.Scope) error {
	return autoConvert_config_CoschedulingArgs_To_v1beta2_CoschedulingArgs(in, out, s)
}

func autoConvert_v1beta2_DeviceShareArgs_To_config_DeviceShareArgs(in *DeviceShareArgs, out *config.DeviceShareArgs, s conversion.Scope) error {
	out.Allocator = in.Allocator
	out.ScoringStrategy = (*config.ScoringStrategy)(unsafe.Pointer(in.ScoringStrategy))
	return nil
}

// Convert_v1beta2_DeviceShareArgs_To_config_DeviceShareArgs is an autogenerated conversion function.
func Convert_v1beta2_DeviceShareArgs_To_config_DeviceShareArgs(in *DeviceShareArgs, out *config.DeviceShareArgs, s conversion.Scope) error {
	return autoConvert_v1beta2_DeviceShareArgs_To_config_DeviceShareArgs(in, out, s)
}

func autoConvert_config_DeviceShareArgs_To_v1beta2_DeviceShareArgs(in *config.DeviceShareArgs, out *DeviceShareArgs, s conversion.Scope) error {
	out.Allocator = in.Allocator
	out.ScoringStrategy = (*ScoringStrategy)(unsafe.Pointer(in.ScoringStrategy))
	return nil
}

// Convert_config_DeviceShareArgs_To_v1beta2_DeviceShareArgs is an autogenerated conversion function.
func Convert_config_DeviceShareArgs_To_v1beta2_DeviceShareArgs(in *config.DeviceShareArgs, out *DeviceShareArgs, s conversion.Scope) error {
	return autoConvert_config_DeviceShareArgs_To_v1beta2_DeviceShareArgs(in, out, s)
}

func autoConvert_v1beta2_ElasticQuotaArgs_To_config_ElasticQuotaArgs(in *ElasticQuotaArgs, out *config.ElasticQuotaArgs, s conversion.Scope) error {
	if err := v1.Convert_Pointer_v1_Duration_To_v1_Duration(&in.DelayEvictTime, &out.DelayEvictTime, s); err != nil {
		return err
	}
	if err := v1.Convert_Pointer_v1_Duration_To_v1_Duration(&in.RevokePodInterval, &out.RevokePodInterval, s); err != nil {
		return err
	}
	out.DefaultQuotaGroupMax = *(*corev1.ResourceList)(unsafe.Pointer(&in.DefaultQuotaGroupMax))
	out.SystemQuotaGroupMax = *(*corev1.ResourceList)(unsafe.Pointer(&in.SystemQuotaGroupMax))
	out.QuotaGroupNamespace = in.QuotaGroupNamespace
	if err := v1.Convert_Pointer_bool_To_bool(&in.MonitorAllQuotas, &out.MonitorAllQuotas, s); err != nil {
		return err
	}
	if err := v1.Convert_Pointer_bool_To_bool(&in.EnableCheckParentQuota, &out.EnableCheckParentQuota, s); err != nil {
		return err
	}
	if err := v1.Convert_Pointer_bool_To_bool(&in.EnableRuntimeQuota, &out.EnableRuntimeQuota, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1beta2_ElasticQuotaArgs_To_config_ElasticQuotaArgs is an autogenerated conversion function.
func Convert_v1beta2_ElasticQuotaArgs_To_config_ElasticQuotaArgs(in *ElasticQuotaArgs, out *config.ElasticQuotaArgs, s conversion.Scope) error {
	return autoConvert_v1beta2_ElasticQuotaArgs_To_config_ElasticQuotaArgs(in, out, s)
}

func autoConvert_config_ElasticQuotaArgs_To_v1beta2_ElasticQuotaArgs(in *config.ElasticQuotaArgs, out *ElasticQuotaArgs, s conversion.Scope) error {
	if err := v1.Convert_v1_Duration_To_Pointer_v1_Duration(&in.DelayEvictTime, &out.DelayEvictTime, s); err != nil {
		return err
	}
	if err := v1.Convert_v1_Duration_To_Pointer_v1_Duration(&in.RevokePodInterval, &out.RevokePodInterval, s); err != nil {
		return err
	}
	out.DefaultQuotaGroupMax = *(*corev1.ResourceList)(unsafe.Pointer(&in.DefaultQuotaGroupMax))
	out.SystemQuotaGroupMax = *(*corev1.ResourceList)(unsafe.Pointer(&in.SystemQuotaGroupMax))
	out.QuotaGroupNamespace = in.QuotaGroupNamespace
	if err := v1.Convert_bool_To_Pointer_bool(&in.MonitorAllQuotas, &out.MonitorAllQuotas, s); err != nil {
		return err
	}
	if err := v1.Convert_bool_To_Pointer_bool(&in.EnableCheckParentQuota, &out.EnableCheckParentQuota, s); err != nil {
		return err
	}
	if err := v1.Convert_bool_To_Pointer_bool(&in.EnableRuntimeQuota, &out.EnableRuntimeQuota, s); err != nil {
		return err
	}
	return nil
}

// Convert_config_ElasticQuotaArgs_To_v1beta2_ElasticQuotaArgs is an autogenerated conversion function.
func Convert_config_ElasticQuotaArgs_To_v1beta2_ElasticQuotaArgs(in *config.ElasticQuotaArgs, out *ElasticQuotaArgs, s conversion.Scope) error {
	return autoConvert_config_ElasticQuotaArgs_To_v1beta2_ElasticQuotaArgs(in, out, s)
}

func autoConvert_v1beta2_LimitAwareArgs_To_config_LimitAwareArgs(in *LimitAwareArgs, out *config.LimitAwareArgs, s conversion.Scope) error {
	out.ScoringResourceWeights = *(*map[corev1.ResourceName]int64)(unsafe.Pointer(&in.ScoringResourceWeights))
	out.DefaultLimitToAllocatable = *(*extension.LimitToAllocatable)(unsafe.Pointer(&in.DefaultLimitToAllocatable))
	return nil
}

// Convert_v1beta2_LimitAwareArgs_To_config_LimitAwareArgs is an autogenerated conversion function.
func Convert_v1beta2_LimitAwareArgs_To_config_LimitAwareArgs(in *LimitAwareArgs, out *config.LimitAwareArgs, s conversion.Scope) error {
	return autoConvert_v1beta2_LimitAwareArgs_To_config_LimitAwareArgs(in, out, s)
}

func autoConvert_config_LimitAwareArgs_To_v1beta2_LimitAwareArgs(in *config.LimitAwareArgs, out *LimitAwareArgs, s conversion.Scope) error {
	out.ScoringResourceWeights = *(*map[corev1.ResourceName]int64)(unsafe.Pointer(&in.ScoringResourceWeights))
	out.DefaultLimitToAllocatable = *(*extension.LimitToAllocatable)(unsafe.Pointer(&in.DefaultLimitToAllocatable))
	return nil
}

// Convert_config_LimitAwareArgs_To_v1beta2_LimitAwareArgs is an autogenerated conversion function.
func Convert_config_LimitAwareArgs_To_v1beta2_LimitAwareArgs(in *config.LimitAwareArgs, out *LimitAwareArgs, s conversion.Scope) error {
	return autoConvert_config_LimitAwareArgs_To_v1beta2_LimitAwareArgs(in, out, s)
}

func autoConvert_v1beta2_LoadAwareSchedulingAggregatedArgs_To_config_LoadAwareSchedulingAggregatedArgs(in *LoadAwareSchedulingAggregatedArgs, out *config.LoadAwareSchedulingAggregatedArgs, s conversion.Scope) error {
	out.UsageThresholds = *(*map[corev1.ResourceName]int64)(unsafe.Pointer(&in.UsageThresholds))
	out.UsageAggregationType = extension.AggregationType(in.UsageAggregationType)
	if err := v1.Convert_Pointer_v1_Duration_To_v1_Duration(&in.UsageAggregatedDuration, &out.UsageAggregatedDuration, s); err != nil {
		return err
	}
	out.ScoreAggregationType = extension.AggregationType(in.ScoreAggregationType)
	if err := v1.Convert_Pointer_v1_Duration_To_v1_Duration(&in.ScoreAggregatedDuration, &out.ScoreAggregatedDuration, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1beta2_LoadAwareSchedulingAggregatedArgs_To_config_LoadAwareSchedulingAggregatedArgs is an autogenerated conversion function.
func Convert_v1beta2_LoadAwareSchedulingAggregatedArgs_To_config_LoadAwareSchedulingAggregatedArgs(in *LoadAwareSchedulingAggregatedArgs, out *config.LoadAwareSchedulingAggregatedArgs, s conversion.Scope) error {
	return autoConvert_v1beta2_LoadAwareSchedulingAggregatedArgs_To_config_LoadAwareSchedulingAggregatedArgs(in, out, s)
}

func autoConvert_config_LoadAwareSchedulingAggregatedArgs_To_v1beta2_LoadAwareSchedulingAggregatedArgs(in *config.LoadAwareSchedulingAggregatedArgs, out *LoadAwareSchedulingAggregatedArgs, s conversion.Scope) error {
	out.UsageThresholds = *(*map[corev1.ResourceName]int64)(unsafe.Pointer(&in.UsageThresholds))
	out.UsageAggregationType = extension.AggregationType(in.UsageAggregationType)
	if err := v1.Convert_v1_Duration_To_Pointer_v1_Duration(&in.UsageAggregatedDuration, &out.UsageAggregatedDuration, s); err != nil {
		return err
	}
	out.ScoreAggregationType = extension.AggregationType(in.ScoreAggregationType)
	if err := v1.Convert_v1_Duration_To_Pointer_v1_Duration(&in.ScoreAggregatedDuration, &out.ScoreAggregatedDuration, s); err != nil {
		return err
	}
	return nil
}

// Convert_config_LoadAwareSchedulingAggregatedArgs_To_v1beta2_LoadAwareSchedulingAggregatedArgs is an autogenerated conversion function.
func Convert_config_LoadAwareSchedulingAggregatedArgs_To_v1beta2_LoadAwareSchedulingAggregatedArgs(in *config.LoadAwareSchedulingAggregatedArgs, out *LoadAwareSchedulingAggregatedArgs, s conversion.Scope) error {
	return autoConvert_config_LoadAwareSchedulingAggregatedArgs_To_v1beta2_LoadAwareSchedulingAggregatedArgs(in, out, s)
}

func autoConvert_v1beta2_LoadAwareSchedulingArgs_To_config_LoadAwareSchedulingArgs(in *LoadAwareSchedulingArgs, out *config.LoadAwareSchedulingArgs, s conversion.Scope) error {
	out.FilterExpiredNodeMetrics = (*bool)(unsafe.Pointer(in.FilterExpiredNodeMetrics))
	out.NodeMetricExpirationSeconds = (*int64)(unsafe.Pointer(in.NodeMetricExpirationSeconds))
	out.ResourceWeights = *(*map[corev1.ResourceName]int64)(unsafe.Pointer(&in.ResourceWeights))
	out.UsageThresholds = *(*map[corev1.ResourceName]int64)(unsafe.Pointer(&in.UsageThresholds))
	out.ProdUsageThresholds = *(*map[corev1.ResourceName]int64)(unsafe.Pointer(&in.ProdUsageThresholds))
	if err := v1.Convert_Pointer_bool_To_bool(&in.ScoreAccordingProdUsage, &out.ScoreAccordingProdUsage, s); err != nil {
		return err
	}
	out.Estimator = in.Estimator
	out.EstimatedScalingFactors = *(*map[corev1.ResourceName]int64)(unsafe.Pointer(&in.EstimatedScalingFactors))
	if in.Aggregated != nil {
		in, out := &in.Aggregated, &out.Aggregated
		*out = new(config.LoadAwareSchedulingAggregatedArgs)
		if err := Convert_v1beta2_LoadAwareSchedulingAggregatedArgs_To_config_LoadAwareSchedulingAggregatedArgs(*in, *out, s); err != nil {
			return err
		}
	} else {
		out.Aggregated = nil
	}
	return nil
}

func autoConvert_config_LoadAwareSchedulingArgs_To_v1beta2_LoadAwareSchedulingArgs(in *config.LoadAwareSchedulingArgs, out *LoadAwareSchedulingArgs, s conversion.Scope) error {
	out.FilterExpiredNodeMetrics = (*bool)(unsafe.Pointer(in.FilterExpiredNodeMetrics))
	out.NodeMetricExpirationSeconds = (*int64)(unsafe.Pointer(in.NodeMetricExpirationSeconds))
	out.ResourceWeights = *(*map[corev1.ResourceName]int64)(unsafe.Pointer(&in.ResourceWeights))
	out.UsageThresholds = *(*map[corev1.ResourceName]int64)(unsafe.Pointer(&in.UsageThresholds))
	out.ProdUsageThresholds = *(*map[corev1.ResourceName]int64)(unsafe.Pointer(&in.ProdUsageThresholds))
	if err := v1.Convert_bool_To_Pointer_bool(&in.ScoreAccordingProdUsage, &out.ScoreAccordingProdUsage, s); err != nil {
		return err
	}
	out.Estimator = in.Estimator
	out.EstimatedScalingFactors = *(*map[corev1.ResourceName]int64)(unsafe.Pointer(&in.EstimatedScalingFactors))
	if in.Aggregated != nil {
		in, out := &in.Aggregated, &out.Aggregated
		*out = new(LoadAwareSchedulingAggregatedArgs)
		if err := Convert_config_LoadAwareSchedulingAggregatedArgs_To_v1beta2_LoadAwareSchedulingAggregatedArgs(*in, *out, s); err != nil {
			return err
		}
	} else {
		out.Aggregated = nil
	}
	return nil
}

// Convert_config_LoadAwareSchedulingArgs_To_v1beta2_LoadAwareSchedulingArgs is an autogenerated conversion function.
func Convert_config_LoadAwareSchedulingArgs_To_v1beta2_LoadAwareSchedulingArgs(in *config.LoadAwareSchedulingArgs, out *LoadAwareSchedulingArgs, s conversion.Scope) error {
	return autoConvert_config_LoadAwareSchedulingArgs_To_v1beta2_LoadAwareSchedulingArgs(in, out, s)
}

func autoConvert_v1beta2_NodeNUMAResourceArgs_To_config_NodeNUMAResourceArgs(in *NodeNUMAResourceArgs, out *config.NodeNUMAResourceArgs, s conversion.Scope) error {
	if err := v1.Convert_Pointer_string_To_string(&in.DefaultCPUBindPolicy, &out.DefaultCPUBindPolicy, s); err != nil {
		return err
	}
	out.ScoringStrategy = (*config.ScoringStrategy)(unsafe.Pointer(in.ScoringStrategy))
	out.NUMAScoringStrategy = (*config.ScoringStrategy)(unsafe.Pointer(in.NUMAScoringStrategy))
	return nil
}

// Convert_v1beta2_NodeNUMAResourceArgs_To_config_NodeNUMAResourceArgs is an autogenerated conversion function.
func Convert_v1beta2_NodeNUMAResourceArgs_To_config_NodeNUMAResourceArgs(in *NodeNUMAResourceArgs, out *config.NodeNUMAResourceArgs, s conversion.Scope) error {
	return autoConvert_v1beta2_NodeNUMAResourceArgs_To_config_NodeNUMAResourceArgs(in, out, s)
}

func autoConvert_config_NodeNUMAResourceArgs_To_v1beta2_NodeNUMAResourceArgs(in *config.NodeNUMAResourceArgs, out *NodeNUMAResourceArgs, s conversion.Scope) error {
	if err := v1.Convert_string_To_Pointer_string(&in.DefaultCPUBindPolicy, &out.DefaultCPUBindPolicy, s); err != nil {
		return err
	}
	out.ScoringStrategy = (*ScoringStrategy)(unsafe.Pointer(in.ScoringStrategy))
	out.NUMAScoringStrategy = (*ScoringStrategy)(unsafe.Pointer(in.NUMAScoringStrategy))
	return nil
}

// Convert_config_NodeNUMAResourceArgs_To_v1beta2_NodeNUMAResourceArgs is an autogenerated conversion function.
func Convert_config_NodeNUMAResourceArgs_To_v1beta2_NodeNUMAResourceArgs(in *config.NodeNUMAResourceArgs, out *NodeNUMAResourceArgs, s conversion.Scope) error {
	return autoConvert_config_NodeNUMAResourceArgs_To_v1beta2_NodeNUMAResourceArgs(in, out, s)
}

func autoConvert_v1beta2_ReservationArgs_To_config_ReservationArgs(in *ReservationArgs, out *config.ReservationArgs, s conversion.Scope) error {
	if err := v1.Convert_Pointer_bool_To_bool(&in.EnablePreemption, &out.EnablePreemption, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1beta2_ReservationArgs_To_config_ReservationArgs is an autogenerated conversion function.
func Convert_v1beta2_ReservationArgs_To_config_ReservationArgs(in *ReservationArgs, out *config.ReservationArgs, s conversion.Scope) error {
	return autoConvert_v1beta2_ReservationArgs_To_config_ReservationArgs(in, out, s)
}

func autoConvert_config_ReservationArgs_To_v1beta2_ReservationArgs(in *config.ReservationArgs, out *ReservationArgs, s conversion.Scope) error {
	if err := v1.Convert_bool_To_Pointer_bool(&in.EnablePreemption, &out.EnablePreemption, s); err != nil {
		return err
	}
	return nil
}

// Convert_config_ReservationArgs_To_v1beta2_ReservationArgs is an autogenerated conversion function.
func Convert_config_ReservationArgs_To_v1beta2_ReservationArgs(in *config.ReservationArgs, out *ReservationArgs, s conversion.Scope) error {
	return autoConvert_config_ReservationArgs_To_v1beta2_ReservationArgs(in, out, s)
}

func autoConvert_v1beta2_ScoringStrategy_To_config_ScoringStrategy(in *ScoringStrategy, out *config.ScoringStrategy, s conversion.Scope) error {
	out.Type = config.ScoringStrategyType(in.Type)
	out.Resources = *(*[]apisconfig.ResourceSpec)(unsafe.Pointer(&in.Resources))
	return nil
}

// Convert_v1beta2_ScoringStrategy_To_config_ScoringStrategy is an autogenerated conversion function.
func Convert_v1beta2_ScoringStrategy_To_config_ScoringStrategy(in *ScoringStrategy, out *config.ScoringStrategy, s conversion.Scope) error {
	return autoConvert_v1beta2_ScoringStrategy_To_config_ScoringStrategy(in, out, s)
}

func autoConvert_config_ScoringStrategy_To_v1beta2_ScoringStrategy(in *config.ScoringStrategy, out *ScoringStrategy, s conversion.Scope) error {
	out.Type = ScoringStrategyType(in.Type)
	out.Resources = *(*[]configv1beta2.ResourceSpec)(unsafe.Pointer(&in.Resources))
	return nil
}

// Convert_config_ScoringStrategy_To_v1beta2_ScoringStrategy is an autogenerated conversion function.
func Convert_config_ScoringStrategy_To_v1beta2_ScoringStrategy(in *config.ScoringStrategy, out *ScoringStrategy, s conversion.Scope) error {
	return autoConvert_config_ScoringStrategy_To_v1beta2_ScoringStrategy(in, out, s)
}

func autoConvert_v1beta2_UnifiedPodConstraintArgs_To_config_UnifiedPodConstraintArgs(in *UnifiedPodConstraintArgs, out *config.UnifiedPodConstraintArgs, s conversion.Scope) error {
	out.EnableDefaultPodConstraint = (*bool)(unsafe.Pointer(in.EnableDefaultPodConstraint))
	return nil
}

// Convert_v1beta2_UnifiedPodConstraintArgs_To_config_UnifiedPodConstraintArgs is an autogenerated conversion function.
func Convert_v1beta2_UnifiedPodConstraintArgs_To_config_UnifiedPodConstraintArgs(in *UnifiedPodConstraintArgs, out *config.UnifiedPodConstraintArgs, s conversion.Scope) error {
	return autoConvert_v1beta2_UnifiedPodConstraintArgs_To_config_UnifiedPodConstraintArgs(in, out, s)
}

func autoConvert_config_UnifiedPodConstraintArgs_To_v1beta2_UnifiedPodConstraintArgs(in *config.UnifiedPodConstraintArgs, out *UnifiedPodConstraintArgs, s conversion.Scope) error {
	out.EnableDefaultPodConstraint = (*bool)(unsafe.Pointer(in.EnableDefaultPodConstraint))
	return nil
}

// Convert_config_UnifiedPodConstraintArgs_To_v1beta2_UnifiedPodConstraintArgs is an autogenerated conversion function.
func Convert_config_UnifiedPodConstraintArgs_To_v1beta2_UnifiedPodConstraintArgs(in *config.UnifiedPodConstraintArgs, out *UnifiedPodConstraintArgs, s conversion.Scope) error {
	return autoConvert_config_UnifiedPodConstraintArgs_To_v1beta2_UnifiedPodConstraintArgs(in, out, s)
}
