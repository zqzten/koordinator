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

package cpustable

import (
	"fmt"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/qosmanager/helpers"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
	"github.com/koordinator-sh/koordinator/pkg/util/cpuset"
)

type PIDController interface {
	// Update updates the controller states at the given timestamp. It returns the control signal.
	Update(actualState, refState float64, updateTime time.Time) float64
	// Skip skips updating the controller states at the given timestamp.
	Skip(updateTime time.Time)
}

type pidController struct {
	P                float64
	I                float64
	D                float64
	PrevErr          float64
	ErrIntegral      float64
	ErrIntegralLimit float64    // cap the abs(ErrIntegral)
	UpdateTime       *time.Time // nil if no previous update
	DefaultInterval  float64    // in seconds
}

func newPIDController(proportionalGain, integralGain, derivativeGain float64) PIDController {
	return &pidController{
		P:                proportionalGain,
		I:                integralGain,
		D:                derivativeGain,
		PrevErr:          0,
		ErrIntegral:      0,
		ErrIntegralLimit: ErrorIntegralLimit,
		UpdateTime:       nil,
		DefaultInterval:  ReconcileInterval.Seconds(),
	}
}

func (c *pidController) Update(actualState, refState float64, updateTime time.Time) float64 {
	// e.g. P = 0.8, I = 0.05, D = 0.2, ErrIntegral = -0.50, PrevErr = -0.25,
	//      updateTime = LastUpdateTime.Add(2*time.Second), actualState = 0.9, refState = 0.75
	//      then p = ctrlErr = 0.75 - 0.9 = -0.15,
	//           i = ErrIntegral = -0.50 + (-0.15) * 2 = -0.80,
	//           d = (-0.15 - (-0.25)) / 2 = 0.05,
	//      signal = -0.15 * 0.8 + -0.80 * 0.05 + 0.05 * 0.2 = -0.12 - 0.04 + 0.01 = -0.15
	var updateInterval float64
	ctrlErr := refState - actualState

	// Proportional
	signal := c.P * ctrlErr

	// Derivative
	if c.UpdateTime != nil {
		// minimum Derivative interval is DefaultInterval
		updateInterval = math.Max(c.DefaultInterval, updateTime.Sub(*c.UpdateTime).Seconds())
		signal += c.D * (ctrlErr - c.PrevErr) / updateInterval
	} else {
		// Set the derivative since the second sample to ignore the nonsense gradient at the first time.
		updateInterval = c.DefaultInterval
	}

	// Integral
	// Cap the err integral to mitigate a too big Integral when the actual state become hard to control.
	if nextErrIntegral := c.ErrIntegral + ctrlErr*updateInterval; math.Abs(nextErrIntegral) < c.ErrIntegralLimit {
		c.ErrIntegral = nextErrIntegral
	} else if nextErrIntegral >= 0 {
		c.ErrIntegral = c.ErrIntegralLimit
	} else {
		c.ErrIntegral = -c.ErrIntegralLimit
	}
	signal += c.I * c.ErrIntegral

	// update state
	c.PrevErr = ctrlErr
	c.UpdateTime = &updateTime
	return signal
}

func (c *pidController) Skip(updateTime time.Time) {
	if c.UpdateTime != nil {
		c.UpdateTime = &updateTime
	} // otherwise the controller is not initialized
}

func validateCommonStrategy(strategy *unified.CPUStableStrategy) error {
	// basic
	if strategy == nil {
		return fmt.Errorf("strategy is nil")
	}
	if policy := strategy.Policy; policy == nil {
		return fmt.Errorf("strategy policy is nil")
	} else if *policy != unified.CPUStablePolicyIgnore && *policy != unified.CPUStablePolicyNone &&
		*policy != unified.CPUStablePolicyAuto {
		return fmt.Errorf("strategy policy is unknown, %s", *policy)
	}
	if v := strategy.PodMetricWindowSeconds; v == nil {
		return fmt.Errorf("PodMetricWindowSeconds is nil")
	} else if *v <= 0 {
		return fmt.Errorf("PodMetricWindowSeconds is invalid, %d", *v)
	}
	if v := strategy.ValidMetricPointsPercent; v == nil {
		return fmt.Errorf("ValidMetricPointsPercent is nil")
	} else if *v < 0 || *v > 100 {
		return fmt.Errorf("ValidMetricPointsPercent is invalid, %d", *v)
	}

	// common scale config
	if v := strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent; v == nil {
		return fmt.Errorf("TargetCPUSatisfactionPercent is nil")
	} else if *v < 0 || *v > 100 {
		return fmt.Errorf("TargetCPUSatisfactionPercent is invalid, %d", *v)
	}
	if v := strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill; v == nil {
		return fmt.Errorf("CPUSatisfactionEpsilonPermill is nil")
	} else if *v < 0 || *v > 1000 {
		return fmt.Errorf("CPUSatisfactionEpsilonPermill is invalid, %d", *v)
	}
	if v := strategy.CPUStableScaleModel.HTRatioUpperBound; v == nil {
		return fmt.Errorf("HTRatioUpperBound is nil")
	} else if *v < system.DefaultHTRatio || *v > system.MaxCPUHTRatio {
		return fmt.Errorf("HTRatioUpperBound is illegal, %d", *v)
	}
	if v := strategy.CPUStableScaleModel.HTRatioLowerBound; v == nil {
		return fmt.Errorf("HTRatioLowerBound is nil")
	} else if *v < system.DefaultHTRatio || *v > system.MaxCPUHTRatio {
		return fmt.Errorf("HTRatioLowerBound is illegal, %d", *v)
	}
	if *strategy.CPUStableScaleModel.HTRatioUpperBound < *strategy.CPUStableScaleModel.HTRatioLowerBound {
		return fmt.Errorf("HTRatioUpperBound is less than HTRatioLowerBound")
	}
	if v := strategy.CPUStableScaleModel.CPUUtilMinPercent; v == nil {
		return fmt.Errorf("CPUUtilMinPercent is nil")
	} else if *v < 0 {
		return fmt.Errorf("CPUUtilMinPercent is illegal, %d", *v)
	}

	// warmup
	lastSharePoolUtilPercent := int64(-1)
	for i, e := range strategy.WarmupConfig.Entries {
		if e.SharePoolUtilPercent == nil {
			return fmt.Errorf("warmup config entry %d is invalid, SharePoolUtilPercent is nil", i)
		}
		if e.HTRatio == nil {
			return fmt.Errorf("warmup config entry %d is invalid, HTRatio is nil", i)
		}
		if *e.SharePoolUtilPercent <= lastSharePoolUtilPercent {
			return fmt.Errorf("warmup config entry %d is invalid, illegal SharePoolUtilPercent %d", i, *e.SharePoolUtilPercent)
		}
		if *e.HTRatio < system.DefaultHTRatio || *e.HTRatio > system.MaxCPUHTRatio {
			return fmt.Errorf("warmup config entry %d is invalid, illegal HTRatio %d", i, *e.HTRatio)
		}
		lastSharePoolUtilPercent = *e.SharePoolUtilPercent
	}

	return nil
}

func getCPUSharePoolSize(statesInformer statesinformer.StatesInformer) (int, int, error) {
	nodeTopology := statesInformer.GetNodeTopo()
	if nodeTopology == nil {
		return -1, -1, fmt.Errorf("nodeTopo is nil")
	}
	cpuSharePools, err := extension.GetNodeCPUSharePools(nodeTopology.Annotations)
	if err != nil {
		return -1, -1, fmt.Errorf("parse cpu sharepool failed, err: %w", err)
	}
	beCPUSharePools, err := extension.GetNodeBECPUSharePools(nodeTopology.Annotations)
	if err != nil {
		return -1, -1, fmt.Errorf("parse be cpu sharepool failed, err: %w", err)
	}

	sharepoolSize, beSharepoolSize := 0, 0
	for _, cpuSharePool := range cpuSharePools {
		cpusetCPUs, err := cpuset.Parse(cpuSharePool.CPUSet)
		if err != nil {
			return -1, -1, fmt.Errorf("parse cpu sharepool failed, cpuset %s, err: %w", cpuSharePool.CPUSet, err)
		}
		sharepoolSize += cpusetCPUs.Size()
	}
	for _, beCPUSharePool := range beCPUSharePools {
		cpusetCPUs, err := cpuset.Parse(beCPUSharePool.CPUSet)
		if err != nil {
			return -1, -1, fmt.Errorf("parse be cpu sharepool failed, cpuset %s, err: %w", beCPUSharePool.CPUSet, err)
		}
		beSharepoolSize += cpusetCPUs.Size()
	}
	klog.V(6).Infof("get cpu sharepool %+v, size %v, BE cpu sharepool %+v, size %v",
		cpuSharePools, sharepoolSize, beCPUSharePools, beSharepoolSize)

	return sharepoolSize, beSharepoolSize, nil
}

func calculateCPUSharePoolUsage(statesInformer statesinformer.StatesInformer, metricCache metriccache.MetricCache,
	podMetas []*statesinformer.PodMeta, queryWindow time.Duration, beScaleRatio float64) (float64, error) {
	end := timeNow()
	start := end.Add(-queryWindow)
	queryParam := &metriccache.QueryParam{
		Aggregate: metriccache.AggregationTypeAVG,
		Start:     &start,
		End:       &end,
	}
	queryMeta, err := metriccache.NodeCPUUsageMetric.BuildQueryMeta(nil)
	if err != nil {
		return -1, fmt.Errorf("build node metric queryMeta failed, err: %w", err)
	}
	queryResult, err := helpers.CollectNodeMetrics(metricCache, *queryParam.Start, *queryParam.End, queryMeta)
	if err != nil {
		return -1, fmt.Errorf("collect node metric failed, duration %v, err: %w", queryWindow.String(), err)
	}
	if queryResult.Count() == 0 {
		return -1, fmt.Errorf("no data in node metric, duration %v", queryWindow.String())
	}
	nodeCPUCoresUsage, err := queryResult.Value(queryParam.Aggregate)
	if err != nil {
		return -1, fmt.Errorf("get node cpu used failed, duration %v, err: %w", queryWindow.String(), err)
	}

	podUsageMap := helpers.CollectAllPodMetrics(statesInformer, metricCache, *queryParam, metriccache.PodCPUUsageMetric)
	sharePoolCPUCoresUsage := nodeCPUCoresUsage
	for _, podMeta := range podMetas {
		podUsage, exist := podUsageMap[string(podMeta.Pod.UID)]
		if !exist {
			continue
		}
		if podQOS := extension.GetPodQoSClassRaw(podMeta.Pod); podQOS == extension.QoSLSE || podQOS == extension.QoSLSR {
			sharePoolCPUCoresUsage -= podUsage
		} else if podQOS == extension.QoSBE {
			sharePoolCPUCoresUsage -= podUsage * beScaleRatio
		}
	}

	klog.V(6).Infof("calculate cpu sharepool usage %v, node usage %v, BE scale ratio %v",
		sharePoolCPUCoresUsage, nodeCPUCoresUsage, beScaleRatio)

	return sharePoolCPUCoresUsage, nil
}

func isPodShouldHaveMetricInWindow(pod *corev1.Pod, degradedWindowSeconds int64, startTime *time.Time) bool {
	if startTime == nil { // has no valid run before
		return false
	}
	// degradation should consider the agent itself just starts up, at this time the metrics are not enough for pods
	end := timeNow()
	if end.Sub(*startTime) < time.Duration(degradedWindowSeconds)*time.Second { // no enough metric to judge
		return false
	}
	// pod has not been starting for all the degraded window
	return time.Duration(degradedWindowSeconds)*time.Second < getPodStartedDuration(pod)
}
