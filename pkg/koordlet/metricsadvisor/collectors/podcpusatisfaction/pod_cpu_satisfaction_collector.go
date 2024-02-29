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

package podcpusatisfaction

import (
	"flag"
	"math"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"go.uber.org/atomic"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	koordletmetrics "github.com/koordinator-sh/koordinator/pkg/koordlet/metrics"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metricsadvisor/framework"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

func init() {
	flag.Float64Var(&HTPerfDegCoeff, "cpu-satisfaction-ht-perf-deg-coeff", HTPerfDegCoeff, "Hyper-Threading performance degradation coefficient. Default value is set to 0.65, and the recommended value is between 0.5 and 0.8.")
	flag.DurationVar(&CollectInterval, "collect-pod-cpu-satisfaction-interval", CollectInterval, "Collect pod cpu satisfaction interval. Minimum interval is 1 second. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h).")
}

var (
	// Check kernel configuration  periodically
	KernelConfigurationCheckIntervel = 30 * time.Second
	// Hyper-Threading Performance Degradation Coefficient (HTPerfDegCoeff)
	// This variable represents the factor by which the CPU thread performance is expected to degrade caused by Hyper-Threading.
	// It's a value between 0 and 1, where 1 means no performance loss and 0 means complete performance loss.
	// Default value is set to 0.65, and the recommended value is between 0.5 and 0.8.
	HTPerfDegCoeff = 0.65
	// To stop collecting, set CollectInterval <= 0.
	CollectInterval = 1 * time.Second
)

const (
	CollectorName = "PodCPUSatisfactionCollector"
)

var (
	timeNow = time.Now
)

type podCPUSatisfactionCollector struct {
	collectInterval     time.Duration
	lastCheckTime       time.Time
	checkIntervel       time.Duration
	sysEnabled          bool
	started             *atomic.Bool
	appendableDB        metriccache.Appendable
	statesInformer      statesinformer.StatesInformer
	cgroupReader        resourceexecutor.CgroupReaderAnolis
	podFilter           framework.PodFilter
	lastPodCPUSchedStat *gocache.Cache
	htPerfDegCoeff      float64
}

func New(opt *framework.Options) framework.Collector {
	collectInterval := CollectInterval
	podFilter := framework.DefaultPodFilter
	if filter, ok := opt.PodFilters[CollectorName]; ok {
		podFilter = filter
	}
	return &podCPUSatisfactionCollector{
		collectInterval:     collectInterval,
		lastCheckTime:       time.Time{},
		checkIntervel:       KernelConfigurationCheckIntervel,
		sysEnabled:          false,
		started:             atomic.NewBool(false),
		appendableDB:        opt.MetricCache,
		statesInformer:      opt.StatesInformer,
		cgroupReader:        resourceexecutor.NewCgroupReaderAnolis(),
		podFilter:           podFilter,
		lastPodCPUSchedStat: gocache.New(collectInterval*framework.ContextExpiredRatio, framework.CleanupInterval),
		htPerfDegCoeff:      HTPerfDegCoeff,
	}
}

var _ framework.PodCollector = &podCPUSatisfactionCollector{}

func (p *podCPUSatisfactionCollector) Enabled() bool {
	return features.DefaultMutableKoordletFeatureGate.Enabled(features.CPUSatisfactionCollector)
}

func (p *podCPUSatisfactionCollector) Setup(c *framework.Context) {}

func (p *podCPUSatisfactionCollector) Run(stopCh <-chan struct{}) {
	if !cache.WaitForCacheSync(stopCh, p.statesInformer.HasSynced) {
		// Koordlet exit because of statesInformer sync failed.
		klog.Fatalf("timed out waiting for states informer caches to sync")
	}
	if p.htPerfDegCoeff < 0 || p.htPerfDegCoeff > 1 {
		klog.Warningf("the parameter cpu-satisfaction-ht-perf-deg-coeff value %f is illegal (acceptable range [0,1]), force it to 0.65", p.htPerfDegCoeff)
		p.htPerfDegCoeff = 0.65
	}

	//skip collect pod cpu satisfaction when system not support
	defer p.started.Store(true)

	if system.GetCurrentCgroupVersion() != system.CgroupVersionV2 {
		klog.V(0).Infof("the current cgroup version is not cgroup v2, %s is only implemented for cgroup v2", CollectorName)
		return
	}

	if p.collectInterval <= 0 {
		klog.V(0).Infof("stop collecting pod cpu satisfaction since the collect interval %d is less than or equal to 0", p.collectInterval)
		return
	}

	sysSupported, msg := system.IsSchedAcpuSupported()
	if !sysSupported {
		klog.V(0).Infof("collect pod cpu satisfaction failed since acpu statistics feature is not supported, msg: %s", msg)
		return
	}

	cpuSchedCfsStatisticsV2AnolisCheck, _ := system.CPUSchedCfsStatisticsV2Anolis.IsSupported("")
	if !cpuSchedCfsStatisticsV2AnolisCheck {
		klog.V(0).Info("cpu.sched_cfs_statistics file not found or unreadable in kubepods directory")
		return
	}

	go wait.Until(p.collectPodCPUSatisfactionUsed, p.collectInterval, stopCh)
}

func (p *podCPUSatisfactionCollector) Started() bool {
	return p.started.Load()
}

func (p *podCPUSatisfactionCollector) FilterPod(meta *statesinformer.PodMeta) (bool, string) {
	return p.podFilter.FilterPod(meta)
}

type CPUSchedStat struct {
	Serve         int64
	OnCpu         int64
	SibidleUSec   int64
	ThrottledUSec int64
}

func (p *podCPUSatisfactionCollector) collectPodCPUSatisfactionUsed() {
	klog.V(6).Info("start collectPodCPUSatisfactionUsed")

	// check if acpu statistics feature is enabled
	if p.lastCheckTime.IsZero() || time.Since(p.lastCheckTime) >= p.checkIntervel {
		var msg string
		p.lastCheckTime = time.Now()
		p.sysEnabled, msg = system.IsSchedAcpuEnabled()
		if !p.sysEnabled {
			klog.V(4).Infof("collectPodCPUSatisfactionUsed failed, acpu statistics feature is not enabled, msg: %s", msg)
		}
	}

	if !p.sysEnabled {
		time.Sleep(p.checkIntervel)
		return
	}

	podMetas := p.statesInformer.GetAllPods()
	count := 0
	metrics := make([]metriccache.MetricSample, 0)
	koordletmetrics.ResetPodCPUCfsSatisfaction()
	koordletmetrics.ResetPodCPUSatisfaction()
	koordletmetrics.ResetPodCPUSatisfactionWithThrottle()
	for _, meta := range podMetas {
		pod := meta.Pod
		uid := string(pod.UID) // types.UID
		podKey := util.GetPodKey(pod)
		if filtered, msg := p.FilterPod(meta); filtered {
			klog.V(5).Infof("skip collect pod %s, reason: %s", podKey, msg)
			continue
		}

		collectTime := timeNow()
		podCgroupDir := meta.CgroupDir

		currentCPUStat, err0 := p.cgroupReader.ReadCPUStat(podCgroupDir)
		currentCPUSchedCfsStatistics, err1 := p.cgroupReader.ReadCPUSchedCfsStatistics(podCgroupDir)
		if err0 != nil || err1 != nil {
			// higher verbosity for probably non-running pods
			if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
				klog.V(6).Infof("failed to collect non-running pod scheduling metrics for %s, CPU stat err: %s, sched_cfs_statistics err: %s",
					podKey, err0, err1)
			} else {
				klog.V(4).Infof("failed to collect pod scheduling metrics for %s, CPU stat err: %s, sched_cfs_statistics err: %s", podKey, err0, err1)
			}
			continue
		}

		lastCPUSchedStatValue, ok := p.lastPodCPUSchedStat.Get(uid)
		p.lastPodCPUSchedStat.Set(uid, CPUSchedStat{
			Serve:         currentCPUSchedCfsStatistics.Serve,
			OnCpu:         currentCPUSchedCfsStatistics.OnCpu,
			SibidleUSec:   currentCPUStat.SibidleUSec,
			ThrottledUSec: currentCPUStat.ThrottledUSec,
		}, gocache.DefaultExpiration)
		klog.V(6).Infof("last pod cpu scheduling stat size in pod resource collector cache %v", p.lastPodCPUSchedStat.ItemCount())
		if !ok {
			klog.V(4).Infof("ignore the first cpu scheduling stat collection for pod %s", podKey)
			continue
		}
		lastCPUSchedStat := lastCPUSchedStatValue.(CPUSchedStat)
		// do subtraction and division first to avoid overflow
		serveDelta := float64(currentCPUSchedCfsStatistics.Serve - lastCPUSchedStat.Serve)
		onCpuDelta := float64(currentCPUSchedCfsStatistics.OnCpu - lastCPUSchedStat.OnCpu)
		sibidleUsecDelta := float64(currentCPUStat.SibidleUSec-lastCPUSchedStat.SibidleUSec) * 1000
		throttledUSecDelta := float64(currentCPUStat.ThrottledUSec-lastCPUSchedStat.ThrottledUSec) * 1000
		if serveDelta < 0 || onCpuDelta < 0 || sibidleUsecDelta < 0 || throttledUSecDelta < 0 || serveDelta+throttledUSecDelta == 0 {
			continue
		}

		// CPUCfsSatisfactionValue is the ratio of time spent on CPU (onCpuDelta)
		// to the total time the thread is queued for execution (serveDelta), representing the raw CPU availability.
		CPUCfsSatisfactionValue := onCpuDelta / serveDelta

		// CPUSatisfactionValue combines the weighted on-CPU time and weighted sibling idle time divided by the total serve time,
		// indicating the effective CPU resource satisfaction considering HT effects.
		//
		// Due to a known kernel bug, the calculated CPUSatisfactionValue may exceed 1.
		// To ensure it never goes beyond the maximum expected value of 1,
		// we use the math.Min function to cap it at 1. Once the kernel bug is fixed,
		// this can be simplified to just the calculation without using math.Min.
		// TODO: Remove math.Min after kernel bug is resolved.
		CPUSatisfactionValue := math.Min((onCpuDelta*p.htPerfDegCoeff+sibidleUsecDelta*(1-p.htPerfDegCoeff))/serveDelta, 1)

		// TODO: Remove math.Min after kernel bug is resolved.
		CPUSatisfactionWithThrottleValue := math.Min((onCpuDelta*p.htPerfDegCoeff+sibidleUsecDelta*(1-p.htPerfDegCoeff))/(serveDelta+throttledUSecDelta), 1)

		CPUCfsSatisfactionMetric, err := metriccache.PodCPUCfsSatisfactionMetric.GenerateSample(
			metriccache.MetricPropertiesFunc.Pod(uid), collectTime, CPUCfsSatisfactionValue)
		if err != nil {
			klog.V(4).Infof("failed to generate pod cpu cfs satisfaction metric for pod %s, err %v", podKey, err)
			continue
		}
		koordletmetrics.RecordPodCPUCfsSatisfaction(pod, CPUCfsSatisfactionValue)

		CPUSatisfactionMetric, err := metriccache.PodCPUSatisfactionMetric.GenerateSample(
			metriccache.MetricPropertiesFunc.Pod(uid), collectTime, CPUSatisfactionValue)
		if err != nil {
			klog.V(4).Infof("failed to generate pod cpu satisfaction metric for pod %s, err %v", podKey, err)
			continue
		}
		koordletmetrics.RecordPodCPUSatisfaction(pod, CPUSatisfactionValue)

		CPUSatisfactionWithThrottleMetric, err := metriccache.PodCPUSatisfactionWithThrottleMetric.GenerateSample(
			metriccache.MetricPropertiesFunc.Pod(uid), collectTime, CPUSatisfactionWithThrottleValue)
		if err != nil {
			klog.V(4).Infof("failed to generate pod cpu satisfaction metric for pod %s, err %v", podKey, err)
			continue
		}
		koordletmetrics.RecordPodCPUSatisfactionWithThrottle(pod, CPUSatisfactionWithThrottleValue)

		metrics = append(metrics, CPUCfsSatisfactionMetric)
		metrics = append(metrics, CPUSatisfactionMetric)
		metrics = append(metrics, CPUSatisfactionWithThrottleMetric)

		klog.V(6).Infof("collect pod %s, uid %s finished, metric %+v", podKey, pod.UID, metrics)

		count++
	}

	appender := p.appendableDB.Appender()
	if err := appender.Append(metrics); err != nil {
		klog.Warningf("Append pod metrics error: %v", err)
		return
	}

	if err := appender.Commit(); err != nil {
		klog.Warningf("Commit pod metrics failed, error: %v", err)
		return
	}

	klog.V(4).Infof("collectPodResUsed finished, pod num %d, collected %d", len(podMetas), count)
}
