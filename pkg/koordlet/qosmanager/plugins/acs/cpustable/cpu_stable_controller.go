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
	"flag"
	"fmt"
	"math"
	"strconv"
	"time"

	gocache "github.com/patrickmn/go-cache"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/audit"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metrics"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/qosmanager/framework"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/qosmanager/helpers"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	koordletutil "github.com/koordinator-sh/koordinator/pkg/koordlet/util"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

func init() {
	flag.Float64Var(&ProportionalGain, "cpu-stable-pid-proportional-gain", ProportionalGain, "The proportional gain (P) of the CPUStable PID controller.")
	flag.Float64Var(&IntegralGain, "cpu-stable-pid-integral-gain", IntegralGain, "The integral gain (I) of the CPUStable PID controller.")
	flag.Float64Var(&DerivativeGain, "cpu-stable-pid-derivative-gain", DerivativeGain, "The derivative gain (D) of the CPUStable PID controller.")
	flag.Float64Var(&SignalNormalized, "cpu-stable-pid-signal-normalized", SignalNormalized, "The normalized ratio for the control signal of the CPUStable PID controller.")
	flag.Float64Var(&ErrorIntegralLimit, "cpu-stable-pid-error-integral-limit", ErrorIntegralLimit, "The absolute limit of the error integral of the CPUStable PID controller.")
}

var (
	ControllerName = HTRatioAIMDController

	ProportionalGain   = 0.8
	IntegralGain       = 0.04
	DerivativeGain     = 0.1
	SignalNormalized   = -100.0
	ErrorIntegralLimit = 4.0
)

type PodControlSignal interface {
	String() string
}

// WarmupState is the warmup decision when the controller handles the pod of the warmup/init signal.
// A nil WarmupState can be regarded no warmup.
type WarmupState interface {
	GetResourceUpdaters(podMeta *statesinformer.PodMeta, strategy *unified.CPUStableStrategy) ([]resourceexecutor.ResourceUpdater, error)
}

// PodStableController is the controller that reconciles the pod cgroups with metrics to achieve the cpu stable state.
type PodStableController interface {
	// Name returns the name of the controller.
	Name() string
	// ValidateStrategy validates the strategy for the controller.
	ValidateStrategy(strategy *unified.CPUStableStrategy) error
	// GetWarmupState calculates the warmup state when the controller handles the pod of the warmup/init signal.
	GetWarmupState(node *corev1.Node, podMetas []*statesinformer.PodMeta, strategy *unified.CPUStableStrategy) (WarmupState, error)
	// GetSignal calculates the control signal of a pod according to pod metrics and strategy.
	GetSignal(pod *corev1.Pod, strategy *unified.CPUStableStrategy) (PodControlSignal, error)
	// GetResourceUpdaters generates resource updaters about how to change pod cgroup configs according to the signal.
	GetResourceUpdaters(podMeta *statesinformer.PodMeta, signal PodControlSignal, strategy *unified.CPUStableStrategy, warmupState WarmupState) ([]resourceexecutor.ResourceUpdater, error)
}

func newPodStableController(name string, opt *framework.Options) PodStableController {
	if name == HTRatioAIMDController {
		return &htRatioAIMDController{
			statesInformer: opt.StatesInformer,
			metricCache:    opt.MetricCache,
			reader:         resourceexecutor.NewCgroupReaderAnolis(),
			runInterval:    ReconcileInterval,
			startTime:      nil,
		}
	} else if name == HTRatioPIDController {
		return &htRatioPIDController{
			statesInformer:     opt.StatesInformer,
			metricCache:        opt.MetricCache,
			reader:             resourceexecutor.NewCgroupReaderAnolis(),
			runInterval:        ReconcileInterval,
			startTime:          nil,
			podControllerCache: gocache.New(5*time.Minute, 5*time.Minute),
		}
	}
	// default
	return &htRatioAIMDController{
		statesInformer: opt.StatesInformer,
		metricCache:    opt.MetricCache,
		reader:         resourceexecutor.NewCgroupReaderAnolis(),
		runInterval:    ReconcileInterval,
		startTime:      nil,
	}
}

type htRatioWarmupState struct {
	WarmupHTRatio int64
}

func newHTRatioWarmupState(statesInformer statesinformer.StatesInformer, metricCache metriccache.MetricCache,
	podMetas []*statesinformer.PodMeta, strategy *unified.CPUStableStrategy) (WarmupState, error) {
	if strategy == nil || len(strategy.WarmupConfig.Entries) <= 0 { // no warmup
		klog.V(6).Info("skip warmup for cpu stable strategy, no warmup config")
		return nil, nil
	}

	// 1. get the cpu sharepool size
	// 2. calculate the cpu sharepool usage
	// 3. determine the node warm-up state with the cpu sharepool util
	sharePoolSize, beSharePoolSize, err := getCPUSharePoolSize(statesInformer)
	if err != nil {
		return nil, err
	}
	if sharePoolSize <= 0 {
		return nil, fmt.Errorf("get invalid sharepool size %d", sharePoolSize)
	}
	if beSharePoolSize <= 0 {
		klog.V(5).Infof("get be sharepool size is %d, cap to sharepool size %d", beSharePoolSize, sharePoolSize)
		beSharePoolSize = sharePoolSize
	}
	klog.V(6).Infof("calculated sharepool size is %d, be sharepool size is %d", sharePoolSize, beSharePoolSize)

	// FIXME: Due to the lack of per-core usage for the BE pod, we estimate the ratio of BE pods' usage on the LS
	//  sharepool (that is, BE sharepool excludes the LSR allocated) to BE pods' usage on the BE sharepool as the
	//  same ratio of the LS sharepool size to the BE sharepool size. It may be inaccurate when the BE usage varies
	//  much on different cores.
	beUsageOnSharepoolRatio := float64(sharePoolSize) / float64(beSharePoolSize)
	queryWindow := time.Duration(*strategy.PodMetricWindowSeconds) * time.Second
	sharepoolUsage, err := calculateCPUSharePoolUsage(statesInformer, metricCache, podMetas, queryWindow, beUsageOnSharepoolRatio)
	if err != nil {
		return nil, err
	}
	klog.V(6).Infof("calculated ls sharepool usage is %v", sharepoolUsage)

	wmState := &htRatioWarmupState{
		WarmupHTRatio: system.DefaultHTRatio,
	}
	sharepoolUtil := sharepoolUsage / float64(sharePoolSize)
	// assert the sharepool util thresholds are sorted ascending after strategy validation
	for _, entry := range strategy.WarmupConfig.Entries {
		if entry.SharePoolUtilPercent == nil || entry.HTRatio == nil {
			continue
		}
		if sharepoolUtil < float64(*entry.SharePoolUtilPercent)/100 {
			break
		}
		wmState.WarmupHTRatio = *entry.HTRatio
	}
	klog.V(4).Infof("calculated warmup state is %+v, ls sharepool [usage:%.3f, size:%d]",
		wmState, sharepoolUsage, sharePoolSize)

	return wmState, nil
}

func (h *htRatioWarmupState) GetResourceUpdaters(podMeta *statesinformer.PodMeta, strategy *unified.CPUStableStrategy) ([]resourceexecutor.ResourceUpdater, error) {
	if h == nil {
		return nil, nil
	}

	// pod is initializing, set the ht_ratio according to the warmup state
	nextHTRatio := h.WarmupHTRatio
	if nextHTRatio < *strategy.CPUStableScaleModel.HTRatioLowerBound {
		nextHTRatio = *strategy.CPUStableScaleModel.HTRatioLowerBound
	} else if nextHTRatio > *strategy.CPUStableScaleModel.HTRatioUpperBound {
		nextHTRatio = *strategy.CPUStableScaleModel.HTRatioUpperBound
	}
	klog.V(4).Infof("calculate ht_ratio for pod %s in warmup state, signal init, next %d", podMeta.Key(), nextHTRatio)

	// since the ht_ratio takes effect only on the leaf cgroups, we try updating both the pod-level and container-level
	eventHelper := audit.V(3).Pod(podMeta.Pod.Namespace, podMeta.Pod.Name).Reason("CPUStable").Message("update pod ht_ratio: %v", nextHTRatio)
	podUpdater, err := resourceexecutor.DefaultCgroupUpdaterFactory.New(system.CPUHTRatioName, podMeta.CgroupDir,
		strconv.FormatInt(nextHTRatio, 10), eventHelper)
	if err != nil {
		return nil, fmt.Errorf("get pod cgroup updater failed, err: %w", err)
	}
	updaters := []resourceexecutor.ResourceUpdater{podUpdater}
	containerCgroupDirs, err := koordletutil.GetCgroupPathsByTargetDepth(system.CPUHTRatioName, podMeta.CgroupDir, koordletutil.ContainerCgroupPathRelativeDepth-koordletutil.PodCgroupPathRelativeDepth)
	if err != nil {
		return nil, fmt.Errorf("get container cgroup dir failed, err: %w", err)
	}
	for _, containerDir := range containerCgroupDirs {
		containerUpdater, err := resourceexecutor.DefaultCgroupUpdaterFactory.New(system.CPUHTRatioName, containerDir,
			strconv.FormatInt(nextHTRatio, 10), eventHelper)
		if err != nil {
			return nil, fmt.Errorf("get container cgroup updater failed, parent dir %s, err: %w", containerDir, err)
		}
		updaters = append(updaters, containerUpdater)
	}

	return updaters, nil
}

// PodHTRatioPIDSignal means the deltas ht_ratio to scale for the pod and containers.
// Normal values are in range [-100, 100].
// Special numbers:
// - 0: remain.
// - -1000: reset.
// - 1000: init.
type PodHTRatioPIDSignal float64

func (p PodHTRatioPIDSignal) String() string {
	return strconv.FormatFloat(float64(p), 'f', 3, 64)
}

const (
	EpsilonHTRatio         = 1.0
	MaxPodHTRatioPIDSignal = 100.0
	MinPodHTRatioPIDSignal = -100.0

	PodHTRatioPIDSpecialSignalRemain PodHTRatioPIDSignal = 0.0     // reserved signal for keep ht_ratio unchanged
	PodHTRatioPIDSpecialSignalReset  PodHTRatioPIDSignal = -1000.0 // reserved signal for reset ht_ratio
	PodHTRatioPIDSpecialSignalInit   PodHTRatioPIDSignal = 1000.0  // reserved signal for ht_ratio init
	PodHTRatioPIDSpecialSignalRelax  PodHTRatioPIDSignal = -2000.0 // reserved signal for ht_ratio relax
)

const HTRatioPIDController = "htRatioPIDController"

// htRatioPIDController is a PodStableController which scales the pod's and its containers' cpu.ht_ratio base on
// the Proportional–Integral–Derivative (PID) algorithm.
type htRatioPIDController struct {
	statesInformer     statesinformer.StatesInformer
	metricCache        metriccache.MetricCache
	reader             resourceexecutor.CgroupReaderAnolis
	runInterval        time.Duration
	startTime          *time.Time
	podControllerCache *gocache.Cache
}

func (h *htRatioPIDController) Name() string {
	return HTRatioPIDController
}

func (h *htRatioPIDController) ValidateStrategy(strategy *unified.CPUStableStrategy) error {
	// common strategy
	if err := validateCommonStrategy(strategy); err != nil {
		return err
	}

	// PID config
	pidCfg := strategy.PIDConfig
	if v := pidCfg.ProportionalPermill; v != nil && *v < 0 {
		return fmt.Errorf("PIDConfig ProportionalPermill is illegal, %d", *v)
	}
	if v := pidCfg.IntegralPermill; v != nil && *v < 0 {
		return fmt.Errorf("PIDConfig IntegralPermill is illegal, %d", *v)
	}
	if v := pidCfg.DerivativePermill; v != nil && *v < 0 {
		return fmt.Errorf("PIDConfig DerivativePermill is illegal, %d", *v)
	}
	if v := pidCfg.SignalNormalizedPercent; v != nil && *v == 0 {
		return fmt.Errorf("PIDConfig SignalNormalizedPercent is illegal, %d", *v)
	}
	// NOTE: For the special signal relax where since the pod util is too low and its satisfaction should not
	// be affected by the ht ratio, we relax the ht_ratio to resume the ht_ratio state to default by steps.
	if v := strategy.CPUStableScaleModel.HTRatioZoomOutPercent; v == nil {
		return fmt.Errorf("HTRatioZoomOutPercent is nil")
	} else if *v <= 0 || *v > 100 {
		return fmt.Errorf("HTRatioZoomOutPercent is invalid, %d", *v)
	}

	return nil
}

func (h *htRatioPIDController) GetWarmupState(_ *corev1.Node, podMetas []*statesinformer.PodMeta, strategy *unified.CPUStableStrategy) (WarmupState, error) {
	return newHTRatioWarmupState(h.statesInformer, h.metricCache, podMetas, strategy)
}

func (h *htRatioPIDController) GetSignal(pod *corev1.Pod, podStrategy *unified.CPUStableStrategy) (PodControlSignal, error) {
	podKey := util.GetPodKey(pod)
	if *podStrategy.Policy == unified.CPUStablePolicyIgnore {
		return PodHTRatioPIDSpecialSignalRemain, nil
	}
	if *podStrategy.Policy == unified.CPUStablePolicyNone {
		h.forgotController(string(pod.UID))
		return PodHTRatioPIDSpecialSignalReset, nil
	}
	// else assert policy == "auto"

	// if pod is unlimited, abort
	podMilliCPULimit := util.GetPodMilliCPULimit(pod)
	if podMilliCPULimit <= 0 {
		klog.V(5).Infof("aborted for pod %s whose cpu limit is %v so that cannot be affected by ht_ratio",
			podKey, podMilliCPULimit)
		return PodHTRatioPIDSpecialSignalRemain, nil
	}

	// if pod is initializing, return "init"
	if isPodInitializing(pod, podStrategy.PodInitializeWindowSeconds) {
		klog.V(5).Infof("pod %s is initializing, window %v s, need init ht_ratio",
			podKey, podStrategy.PodInitializeWindowSeconds)
		return PodHTRatioPIDSpecialSignalInit, nil
	}

	queryWindow := time.Duration(*podStrategy.PodMetricWindowSeconds) * time.Second
	end := timeNow()
	start := end.Add(-queryWindow)
	var minValidMetricCount int
	if *podStrategy.ValidMetricPointsPercent <= 0 {
		minValidMetricCount = 1
	} else if validMetricCount := int(float64(queryWindow) / float64(h.runInterval) * float64(*podStrategy.ValidMetricPointsPercent) / 100); validMetricCount > 1 {
		minValidMetricCount = validMetricCount
	} else {
		minValidMetricCount = 1
	}

	// NOTE: CPUSatisfactionWithThrottle := (onCPUDelta*0.65 + sibidleDelta*0.35) / (serveDelta + throttledDelta)
	queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string(pod.UID)))
	if err != nil {
		return PodHTRatioPIDSpecialSignalRemain, fmt.Errorf("build pod cpu satisfaction query failed, err: %w", err)
	}
	podCPUSatisfactionMetrics, err := helpers.CollectPodMetric(h.metricCache, queryMeta, start, end)
	if err != nil {
		return PodHTRatioPIDSpecialSignalRemain, fmt.Errorf("collect pod cpu satisfaction failed, err: %w", err)
	}

	// pod metrics not enough
	if metricCount := podCPUSatisfactionMetrics.Count(); metricCount < minValidMetricCount {
		// Note that a non-initializing pod may not have enough metrics in the degraded window, we should NOT
		// regard it as a degraded pod.
		if degradedWindowSeconds := podStrategy.PodDegradeWindowSeconds; degradedWindowSeconds != nil &&
			isPodShouldHaveMetricInWindow(pod, *degradedWindowSeconds, h.startTime) {
			startForDegrade := end.Add(-time.Duration(*degradedWindowSeconds) * time.Second)
			podCPUSatisfactionMetricsForDegrade, err := helpers.CollectPodMetric(h.metricCache, queryMeta, startForDegrade, end)
			if err != nil {
				return PodHTRatioPIDSpecialSignalRemain, fmt.Errorf("collect cpu satisfaction for degraded failed, err: %w", err)
			}
			if podCPUSatisfactionMetricsForDegrade.Count() <= 0 {
				h.forgotController(string(pod.UID))
				klog.V(4).Infof("pod %s has no valid metric in the degraded window seconds %v, need reset",
					podKey, *degradedWindowSeconds)
				return PodHTRatioPIDSpecialSignalReset, nil
			}
		}

		return PodHTRatioPIDSpecialSignalRemain, fmt.Errorf("metrics not enough, min %d, got %d", minValidMetricCount, metricCount)
	}

	podCPUSatisfactionAVG, err := podCPUSatisfactionMetrics.Value(metriccache.AggregationTypeAVG)
	if err != nil {
		return PodHTRatioPIDSpecialSignalRemain, fmt.Errorf("calculate AVG of pod cpu satisfaction failed, err: %w", err)
	}
	// init start time once podCPUSatisfactionMetrics is available
	if h.startTime == nil {
		h.startTime = &end
	}

	// check pod cpu util since the lower util pod may not be affected by the HTR
	queryMetaForUsage, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string(pod.UID)))
	if err != nil {
		return PodHTRatioPIDSpecialSignalRemain, fmt.Errorf("build pod cpu usage query failed, err: %w", err)
	}
	podCPUUsageMetrics, err := helpers.CollectPodMetric(h.metricCache, queryMetaForUsage, start, end)
	if err != nil {
		return PodHTRatioPIDSpecialSignalRemain, fmt.Errorf("collect pod cpu usage failed, err: %w", err)
	}
	// after the degradation check, a pod has no enough usage metrics is abnormal.
	if metricCount := podCPUUsageMetrics.Count(); metricCount < minValidMetricCount {
		return PodHTRatioPIDSpecialSignalRemain, fmt.Errorf("cpu usage metrics not enough, min %d, got %d", minValidMetricCount, metricCount)
	}
	podCPUUsageAVG, err := podCPUUsageMetrics.Value(metriccache.AggregationTypeAVG)
	if err != nil {
		return PodHTRatioPIDSpecialSignalRemain, fmt.Errorf("calculate AVG of pod cpu usage failed, err: %w", err)
	}
	podCPUUtil := podCPUUsageAVG / (float64(podMilliCPULimit) / 1000)
	if podCPUUtil < float64(*podStrategy.CPUStableScaleModel.CPUUtilMinPercent)/100 {
		// TBD: shall we keep remain for the low-util or the exactly latency-sensitive pods
		h.skipController(string(pod.UID), end)
		klog.V(5).Infof("ignore for pod %s whose cpu util is %v so that cannot be affected by ht_ratio",
			podKey, podCPUUtil)
		return PodHTRatioPIDSpecialSignalRelax, nil
	}

	// PID control does not rely on the epsilon to converge the scaling, so the epsilon can be set to zero. or an
	// exactly small value.
	// If set non-zero, it is filtering the little differences of the cpu satisfaction rates.
	targetCPUSatisfaction := float64(*podStrategy.CPUStableScaleModel.TargetCPUSatisfactionPercent) / 100

	// NOTE: When the sample intervals are not even, e.g. some updates are aborted (timestamp not updated), the
	// integral gain could be a bit large.
	signal := h.updateController(string(pod.UID), podCPUSatisfactionAVG, targetCPUSatisfaction, end, podStrategy.PIDConfig)

	klog.V(4).Infof("pod %s has cpu satisfaction %v, target %v, need PID control %s",
		podKey, podCPUSatisfactionAVG, targetCPUSatisfaction, signal.String())
	return signal, nil
}

func (h *htRatioPIDController) GetResourceUpdaters(podMeta *statesinformer.PodMeta, signal PodControlSignal, strategy *unified.CPUStableStrategy, warmupState WarmupState) ([]resourceexecutor.ResourceUpdater, error) {
	pidSignal, ok := signal.(PodHTRatioPIDSignal)
	if !ok {
		return nil, fmt.Errorf("illegal PID signal %v, type %T", signal, signal)
	}

	if math.Abs(float64(pidSignal)) < EpsilonHTRatio { // do nothing on cgroups
		return nil, nil
	}

	curHTRatio, err := h.reader.ReadCPUHTRatio(podMeta.CgroupDir)
	if err != nil {
		return nil, fmt.Errorf("read ht_ratio failed, err: %w", err)
	}
	metrics.RecordPodHTRatio(podMeta.Pod.Namespace, podMeta.Pod.Name, string(podMeta.Pod.UID), string(*strategy.Policy), float64(curHTRatio))

	var nextHTRatio int64
	switch pidSignal {
	case PodHTRatioPIDSpecialSignalRemain:
		return nil, nil
	case PodHTRatioPIDSpecialSignalReset:
		nextHTRatio = system.DefaultHTRatio
	case PodHTRatioPIDSpecialSignalInit:
		if warmupState == nil {
			klog.V(6).Infof("pod %s is initializing without warmup stat, cur %d, skip this round",
				podMeta.Key(), curHTRatio)
			return nil, nil
		}
		// pod is initializing, set the ht_ratio according to the warmup state
		return warmupState.GetResourceUpdaters(podMeta, strategy)
	case PodHTRatioPIDSpecialSignalRelax: // use a fix ratio to relax special cases, e.g. pod is low-util
		nextHTRatio = int64(float64(curHTRatio) * float64(*strategy.CPUStableScaleModel.HTRatioZoomOutPercent) / 100)
		klog.V(6).Infof("pod %s ht_ratio is relaxing, zoom-out ratio %v",
			podMeta.Key(), float64(*strategy.CPUStableScaleModel.HTRatioZoomOutPercent)/100)
	default: // common case: adjust ht_ratio with the control signal
		nextHTRatio = int64(float64(curHTRatio) + float64(pidSignal))
	}
	if nextHTRatio < *strategy.CPUStableScaleModel.HTRatioLowerBound {
		nextHTRatio = *strategy.CPUStableScaleModel.HTRatioLowerBound
	} else if nextHTRatio > *strategy.CPUStableScaleModel.HTRatioUpperBound {
		nextHTRatio = *strategy.CPUStableScaleModel.HTRatioUpperBound
	}
	if curHTRatio != nextHTRatio {
		klog.V(4).Infof("need to adjust ht_ratio for pod %s, signal %s, cur %d, next %d",
			podMeta.Key(), signal.String(), curHTRatio, nextHTRatio)
	} else {
		klog.V(6).Infof("calculate ht_ratio for pod %s, signal %s, cur %d, next %d",
			podMeta.Key(), signal.String(), curHTRatio, nextHTRatio)
	}

	// since the ht_ratio takes effect only on the leaf cgroups, we try updating both the pod-level and container-level
	eventHelper := audit.V(3).Pod(podMeta.Pod.Namespace, podMeta.Pod.Name).Reason("CPUStable").Message("update pod ht_ratio: %v", nextHTRatio)
	podUpdater, err := resourceexecutor.DefaultCgroupUpdaterFactory.New(system.CPUHTRatioName, podMeta.CgroupDir,
		strconv.FormatInt(nextHTRatio, 10), eventHelper)
	if err != nil {
		return nil, fmt.Errorf("get pod cgroup updater failed, err: %w", err)
	}
	updaters := []resourceexecutor.ResourceUpdater{podUpdater}
	containerCgroupDirs, err := koordletutil.GetCgroupPathsByTargetDepth(system.CPUHTRatioName, podMeta.CgroupDir, koordletutil.ContainerCgroupPathRelativeDepth-koordletutil.PodCgroupPathRelativeDepth)
	if err != nil {
		return nil, fmt.Errorf("get container cgroup dir failed, err: %w", err)
	}
	for _, containerDir := range containerCgroupDirs {
		containerUpdater, err := resourceexecutor.DefaultCgroupUpdaterFactory.New(system.CPUHTRatioName, containerDir,
			strconv.FormatInt(nextHTRatio, 10), eventHelper)
		if err != nil {
			return nil, fmt.Errorf("get container cgroup updater failed, parent dir %s, err: %w", containerDir, err)
		}
		updaters = append(updaters, containerUpdater)
	}

	return updaters, nil
}

func (h *htRatioPIDController) updateController(uid string, actualState, refState float64, updateTime time.Time, cfg unified.CPUStablePIDConfig) PodHTRatioPIDSignal {
	p, i, d := ProportionalGain, IntegralGain, DerivativeGain
	if cfg.ProportionalPermill != nil {
		p = float64(*cfg.ProportionalPermill) / 1000
	}
	if cfg.IntegralPermill != nil {
		i = float64(*cfg.IntegralPermill) / 1000
	}
	if cfg.DerivativePermill != nil {
		d = float64(*cfg.DerivativePermill) / 1000
	}
	normalized := SignalNormalized
	if cfg.SignalNormalizedPercent != nil {
		normalized = float64(*cfg.SignalNormalizedPercent) / 100
	}

	var podController PIDController
	if podControllerIf, ok := h.podControllerCache.Get(uid); ok {
		podController = podControllerIf.(PIDController)
	} else {
		podController = newPIDController(p, i, d)
		klog.V(5).Infof("add PID controller for pod uid %s", uid)
	}
	ctrlSignal := podController.Update(actualState, refState, updateTime)
	h.podControllerCache.SetDefault(uid, podController)

	// Convert cpu satisfaction signal to ht_ratio, need to multiply a normalized ratio.
	// e.g. ref satisfaction = 0.80, actual satisfaction = 0.90,
	//      => normalized signal = (0.80 - 0.90) * -100 = 12, which means increase ht_ratio by 12.
	normalizedSignal := math.Min(MaxPodHTRatioPIDSignal, math.Max(MinPodHTRatioPIDSignal, ctrlSignal*normalized))
	return PodHTRatioPIDSignal(normalizedSignal)
}

func (h *htRatioPIDController) forgotController(uid string) {
	if _, ok := h.podControllerCache.Get(uid); ok {
		h.podControllerCache.Delete(uid)
		klog.V(5).Infof("forgot PID controller for pod uid %s", uid)
	}
}

func (h *htRatioPIDController) skipController(uid string, updateTime time.Time) {
	if podControllerIf, ok := h.podControllerCache.Get(uid); ok {
		podController := podControllerIf.(PIDController)
		podController.Skip(updateTime)
		h.podControllerCache.SetDefault(uid, podController)
		klog.V(6).Infof("skip PID controller for pod uid %s", uid)
	}
}

// PodHTRatioAIMDSignal represents the pod signal of the htRatioAIMDController.
// It remembers the bias direction from the current cpu satisfaction to the target without the distance.
type PodHTRatioAIMDSignal int

const (
	PodHTRatioAIMDSignalReset    PodHTRatioAIMDSignal = iota // reset the pod cgroup configs
	PodHTRatioAIMDSignalRemain                               // do nothing on the pod cgroup config
	PodHTRatioAIMDSignalThrottle                             // throttle the pod cgroup config
	PodHTRatioAIMDSignalRelax                                // relax the pod cgroup config
	PodHTRatioAIMDSignalInit                                 // pod is just initializing and missing metrics
)

func (o PodHTRatioAIMDSignal) String() string {
	switch o {
	case PodHTRatioAIMDSignalReset:
		return "reset"
	case PodHTRatioAIMDSignalRemain:
		return "remain"
	case PodHTRatioAIMDSignalThrottle:
		return "throttle"
	case PodHTRatioAIMDSignalRelax:
		return "relax"
	case PodHTRatioAIMDSignalInit:
		return "init"
	default:
		return "unknown"
	}
}

const HTRatioAIMDController = "htRatioAIMDController"

// htRatioAIMDController is a PodStableController which scales the pod's and its containers' cpu.ht_ratio base on
// the Additive-Increase/Multiplicative-Decrease (AIMD) algorithm.
type htRatioAIMDController struct {
	statesInformer statesinformer.StatesInformer
	metricCache    metriccache.MetricCache
	reader         resourceexecutor.CgroupReaderAnolis
	runInterval    time.Duration
	startTime      *time.Time
}

func (h *htRatioAIMDController) Name() string {
	return HTRatioAIMDController
}

func (h *htRatioAIMDController) ValidateStrategy(strategy *unified.CPUStableStrategy) error {
	// common strategy
	if err := validateCommonStrategy(strategy); err != nil {
		return err
	}

	// AIMD
	if v := strategy.CPUStableScaleModel.HTRatioIncreasePercent; v == nil {
		return fmt.Errorf("HTRatioIncreasePercent is nil")
	} else if *v < 0 || *v > 100 {
		return fmt.Errorf("HTRatioIncreasePercent is invalid, %d", *v)
	}
	if v := strategy.CPUStableScaleModel.HTRatioZoomOutPercent; v == nil {
		return fmt.Errorf("HTRatioZoomOutPercent is nil")
	} else if *v <= 0 || *v > 100 {
		return fmt.Errorf("HTRatioZoomOutPercent is invalid, %d", *v)
	}

	return nil
}

func (h *htRatioAIMDController) GetWarmupState(_ *corev1.Node, podMetas []*statesinformer.PodMeta, strategy *unified.CPUStableStrategy) (WarmupState, error) {
	return newHTRatioWarmupState(h.statesInformer, h.metricCache, podMetas, strategy)
}

func (h *htRatioAIMDController) GetSignal(pod *corev1.Pod, podStrategy *unified.CPUStableStrategy) (PodControlSignal, error) {
	podKey := util.GetPodKey(pod)
	if *podStrategy.Policy == unified.CPUStablePolicyIgnore {
		return PodHTRatioAIMDSignalRemain, nil
	}
	if *podStrategy.Policy == unified.CPUStablePolicyNone {
		return PodHTRatioAIMDSignalReset, nil
	}
	// else assert policy == "auto"

	// if pod is unlimited, abort
	podMilliCPULimit := util.GetPodMilliCPULimit(pod)
	if podMilliCPULimit <= 0 {
		klog.V(5).Infof("aborted for pod %s whose cpu limit is %v so that cannot be affected by ht_ratio",
			podKey, podMilliCPULimit)
		return PodHTRatioAIMDSignalRemain, nil
	}

	// if pod is initializing, return "init"
	if isPodInitializing(pod, podStrategy.PodInitializeWindowSeconds) {
		klog.V(5).Infof("pod %s is initializing, window %v s, need init ht_ratio",
			podKey, podStrategy.PodInitializeWindowSeconds)
		return PodHTRatioAIMDSignalInit, nil
	}

	queryWindow := time.Duration(*podStrategy.PodMetricWindowSeconds) * time.Second
	end := timeNow()
	start := end.Add(-queryWindow)
	var minValidMetricCount int
	if *podStrategy.ValidMetricPointsPercent <= 0 {
		minValidMetricCount = 1
	} else if validMetricCount := int(float64(queryWindow) / float64(h.runInterval) * float64(*podStrategy.ValidMetricPointsPercent) / 100); validMetricCount > 1 {
		minValidMetricCount = validMetricCount
	} else {
		minValidMetricCount = 1
	}

	// NOTE: CPUSatisfactionWithThrottle := (onCPUDelta*0.65 + sibidleDelta*0.35) / (serveDelta + throttledDelta)
	queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string(pod.UID)))
	if err != nil {
		return PodHTRatioAIMDSignalRemain, fmt.Errorf("build pod cpu satisfaction query failed, err: %w", err)
	}
	podCPUSatisfactionMetrics, err := helpers.CollectPodMetric(h.metricCache, queryMeta, start, end)
	if err != nil {
		return PodHTRatioAIMDSignalRemain, fmt.Errorf("collect pod cpu satisfaction failed, err: %w", err)
	}

	// pod metrics not enough
	if metricCount := podCPUSatisfactionMetrics.Count(); metricCount < minValidMetricCount {
		// Note that a non-initializing pod may not have enough metrics in the degraded window, we should NOT
		// regard it as a degraded pod.
		if degradedWindowSeconds := podStrategy.PodDegradeWindowSeconds; degradedWindowSeconds != nil &&
			isPodShouldHaveMetricInWindow(pod, *degradedWindowSeconds, h.startTime) {
			startForDegrade := end.Add(-time.Duration(*degradedWindowSeconds) * time.Second)
			podCPUSatisfactionMetricsForDegrade, err := helpers.CollectPodMetric(h.metricCache, queryMeta, startForDegrade, end)
			if err != nil {
				return PodHTRatioAIMDSignalRemain, fmt.Errorf("collect cpu satisfaction for degraded failed, err: %w", err)
			}
			if podCPUSatisfactionMetricsForDegrade.Count() <= 0 {
				klog.V(4).Infof("pod %s has no valid metric in the degraded window seconds %v, need reset",
					podKey, *degradedWindowSeconds)
				return PodHTRatioAIMDSignalReset, nil
			}
		}

		return PodHTRatioAIMDSignalRemain, fmt.Errorf("metrics not enough, min %d, got %d", minValidMetricCount, metricCount)
	}

	podCPUSatisfactionAVG, err := podCPUSatisfactionMetrics.Value(metriccache.AggregationTypeAVG)
	if err != nil {
		return PodHTRatioAIMDSignalRemain, fmt.Errorf("calculate AVG of pod cpu satisfaction failed, err: %w", err)
	}
	// init start time once podCPUSatisfactionMetrics is available
	if h.startTime == nil {
		h.startTime = &end
	}

	// check pod cpu util since the lower util pod may not be affected by the HTR
	queryMetaForUsage, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string(pod.UID)))
	if err != nil {
		return PodHTRatioAIMDSignalRemain, fmt.Errorf("build pod cpu usage query failed, err: %w", err)
	}
	podCPUUsageMetrics, err := helpers.CollectPodMetric(h.metricCache, queryMetaForUsage, start, end)
	if err != nil {
		return PodHTRatioAIMDSignalRemain, fmt.Errorf("collect pod cpu usage failed, err: %w", err)
	}
	// after the degradation check, a pod has no enough usage metrics is abnormal.
	if metricCount := podCPUUsageMetrics.Count(); metricCount < minValidMetricCount {
		return PodHTRatioAIMDSignalRemain, fmt.Errorf("cpu usage metrics not enough, min %d, got %d", minValidMetricCount, metricCount)
	}
	podCPUUsageAVG, err := podCPUUsageMetrics.Value(metriccache.AggregationTypeAVG)
	if err != nil {
		return PodHTRatioAIMDSignalRemain, fmt.Errorf("calculate AVG of pod cpu usage failed, err: %w", err)
	}
	podCPUUtil := podCPUUsageAVG / (float64(podMilliCPULimit) / 1000)
	if podCPUUtil < float64(*podStrategy.CPUStableScaleModel.CPUUtilMinPercent)/100 {
		klog.V(5).Infof("just relax for pod %s whose cpu util is %v so that cannot be affected by ht_ratio",
			podKey, podCPUUtil)
		return PodHTRatioAIMDSignalRelax, nil
	}

	targetCPUSatisfaction := float64(*podStrategy.CPUStableScaleModel.TargetCPUSatisfactionPercent) / 100
	targetEpsilon := float64(*podStrategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill) / 1000
	if podCPUSatisfactionAVG > targetCPUSatisfaction+targetEpsilon {
		klog.V(4).Infof("pod %s has cpu satisfaction %v, larger than target %v(+-%v), need throttle",
			podKey, podCPUSatisfactionAVG, targetCPUSatisfaction, targetEpsilon)
		return PodHTRatioAIMDSignalThrottle, nil
	} else if podCPUSatisfactionAVG < targetCPUSatisfaction-targetEpsilon {
		klog.V(4).Infof("pod %s has cpu satisfaction %v, less than target %v(+-%v), need relax",
			podKey, podCPUSatisfactionAVG, targetCPUSatisfaction, targetEpsilon)
		return PodHTRatioAIMDSignalRelax, nil
	}

	klog.V(6).Infof("pod %s has cpu satisfaction %v, approach to target %v(+-%v), keep remain",
		podKey, podCPUSatisfactionAVG, targetCPUSatisfaction, targetEpsilon)
	return PodHTRatioAIMDSignalRemain, nil
}

func (h *htRatioAIMDController) GetResourceUpdaters(podMeta *statesinformer.PodMeta, rawSignal PodControlSignal, strategy *unified.CPUStableStrategy, warmupState WarmupState) ([]resourceexecutor.ResourceUpdater, error) {
	signal, ok := rawSignal.(PodHTRatioAIMDSignal)
	if !ok {
		return nil, fmt.Errorf("illegal AIMD signal %v, type %T", signal, signal)
	}

	// 1. fetch current cgroup status
	// 2. calculate the next cgroup configs
	// 3. make cgroup updater
	if signal == PodHTRatioAIMDSignalRemain { // do nothing on cgroups
		return nil, nil
	}

	curHTRatio, err := h.reader.ReadCPUHTRatio(podMeta.CgroupDir)
	if err != nil {
		return nil, fmt.Errorf("read ht_ratio failed, err: %w", err)
	}
	metrics.RecordPodHTRatio(podMeta.Pod.Namespace, podMeta.Pod.Name, string(podMeta.Pod.UID), string(*strategy.Policy), float64(curHTRatio))

	var nextHTRatio int64
	switch signal {
	case PodHTRatioAIMDSignalReset:
		nextHTRatio = system.DefaultHTRatio
	case PodHTRatioAIMDSignalThrottle:
		nextHTRatio = curHTRatio + int64(system.DefaultHTRatio*float64(*strategy.CPUStableScaleModel.HTRatioIncreasePercent)/100)
	case PodHTRatioAIMDSignalRelax:
		nextHTRatio = int64(float64(curHTRatio) * float64(*strategy.CPUStableScaleModel.HTRatioZoomOutPercent) / 100)
	case PodHTRatioAIMDSignalInit:
		if warmupState == nil {
			klog.V(6).Infof("pod %s is initializing without warmup stat, cur %d, skip this round",
				podMeta.Key(), curHTRatio)
			return nil, nil
		}
		// pod is initializing, set the ht_ratio according to the warmup state
		return warmupState.GetResourceUpdaters(podMeta, strategy)
	default:
		return nil, fmt.Errorf("unexpected signal %s", signal.String())
	}
	if nextHTRatio < *strategy.CPUStableScaleModel.HTRatioLowerBound {
		nextHTRatio = *strategy.CPUStableScaleModel.HTRatioLowerBound
	} else if nextHTRatio > *strategy.CPUStableScaleModel.HTRatioUpperBound {
		nextHTRatio = *strategy.CPUStableScaleModel.HTRatioUpperBound
	}
	if curHTRatio != nextHTRatio {
		klog.V(4).Infof("need to adjust ht_ratio for pod %s, signal %s, cur %d, next %d",
			podMeta.Key(), signal.String(), curHTRatio, nextHTRatio)
	} else {
		klog.V(6).Infof("calculate ht_ratio for pod %s, signal %s, cur %d, next %d",
			podMeta.Key(), signal.String(), curHTRatio, nextHTRatio)
	}

	// since the ht_ratio takes effect only on the leaf cgroups, we try updating both the pod-level and container-level
	eventHelper := audit.V(3).Pod(podMeta.Pod.Namespace, podMeta.Pod.Name).Reason("CPUStable").Message("update pod ht_ratio: %v", nextHTRatio)
	podUpdater, err := resourceexecutor.DefaultCgroupUpdaterFactory.New(system.CPUHTRatioName, podMeta.CgroupDir,
		strconv.FormatInt(nextHTRatio, 10), eventHelper)
	if err != nil {
		return nil, fmt.Errorf("get pod cgroup updater failed, err: %w", err)
	}
	updaters := []resourceexecutor.ResourceUpdater{podUpdater}
	containerCgroupDirs, err := koordletutil.GetCgroupPathsByTargetDepth(system.CPUHTRatioName, podMeta.CgroupDir, koordletutil.ContainerCgroupPathRelativeDepth-koordletutil.PodCgroupPathRelativeDepth)
	if err != nil {
		return nil, fmt.Errorf("get container cgroup dir failed, err: %w", err)
	}
	for _, containerDir := range containerCgroupDirs {
		containerUpdater, err := resourceexecutor.DefaultCgroupUpdaterFactory.New(system.CPUHTRatioName, containerDir,
			strconv.FormatInt(nextHTRatio, 10), eventHelper)
		if err != nil {
			return nil, fmt.Errorf("get container cgroup updater failed, parent dir %s, err: %w", containerDir, err)
		}
		updaters = append(updaters, containerUpdater)
	}

	return updaters, nil
}
