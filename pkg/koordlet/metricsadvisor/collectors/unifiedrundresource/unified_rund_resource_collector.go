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

package unifiedrundresource

import (
	"flag"
	"fmt"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"go.uber.org/atomic"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metricsadvisor/framework"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	koordletutil "github.com/koordinator-sh/koordinator/pkg/koordlet/util"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

func init() {
	flag.DurationVar(&CollectRundInterval, "collect-rund-interval", CollectRundInterval, "Collect rund pod resource usage interval. Minimum interval is 1 second. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h).")
}

const (
	CollectorName = "RundResourceCollector"
)

var CollectRundInterval = 10 * time.Second // use a different interval considering the overhead of rund ExtendedStats

type rundResourceCollector struct {
	collectInterval      time.Duration
	started              *atomic.Bool
	appendableDB         metriccache.Appendable
	statesInformer       statesinformer.StatesInformer
	cgroupReader         resourceexecutor.CgroupReader
	podFilter            framework.PodFilter
	lastPodCPUStat       *gocache.Cache
	lastContainerCPUStat *gocache.Cache

	deviceCollectors map[string]framework.DeviceCollector
}

func New(opt *framework.Options) framework.Collector {
	collectInterval := CollectRundInterval
	podFilter := framework.DefaultPodFilter
	if filter, ok := opt.PodFilters[CollectorName]; ok {
		podFilter = filter
	}
	return &rundResourceCollector{
		collectInterval:      collectInterval,
		started:              atomic.NewBool(false),
		appendableDB:         opt.MetricCache,
		statesInformer:       opt.StatesInformer,
		cgroupReader:         opt.CgroupReader,
		podFilter:            podFilter,
		lastPodCPUStat:       gocache.New(collectInterval*framework.ContextExpiredRatio, framework.CleanupInterval),
		lastContainerCPUStat: gocache.New(collectInterval*framework.ContextExpiredRatio, framework.CleanupInterval),
	}
}

func (p *rundResourceCollector) Enabled() bool {
	return true
}

func (p *rundResourceCollector) Setup(c *framework.Context) {
	p.deviceCollectors = c.DeviceCollectors
}

func (p *rundResourceCollector) Run(stopCh <-chan struct{}) {
	devicesSynced := func() bool {
		return framework.DeviceCollectorsStarted(p.deviceCollectors)
	}
	if !cache.WaitForCacheSync(stopCh, p.statesInformer.HasSynced, devicesSynced) {
		// Koordlet exit because of statesInformer sync failed.
		klog.Fatalf("timed out waiting for states informer caches to sync")
	}
	go wait.Until(p.collectRundPodsResUsed, p.collectInterval, stopCh)
}

func (p *rundResourceCollector) Started() bool {
	return p.started.Load()
}

func (p *rundResourceCollector) FilterPod(meta *statesinformer.PodMeta) (bool, string) {
	return p.podFilter.FilterPod(meta)
}

func (p *rundResourceCollector) collectRundPodsResUsed() {
	klog.V(6).Info("start collectRundPodResUsed")
	podMetas := p.statesInformer.GetAllPods()
	for _, meta := range podMetas {
		if filtered, msg := p.FilterPod(meta); filtered {
			klog.V(5).Infof("skip collect pod %s/%s since it is filtered, msg: %s",
				meta.Pod.Namespace, meta.Pod.Name, msg)
			continue
		}

		p.collectRundPod(meta)
	}

	// update collect time
	p.started.Store(true)
	klog.Infof("collectRundPodsResUsed finished, pod num %d", len(podMetas))
}

func (p *rundResourceCollector) collectRundPod(meta *statesinformer.PodMeta) {
	pod := meta.Pod
	uid := string(pod.UID) // types.UID
	collectTime := time.Now()
	podCgroupDir := meta.CgroupDir
	podKey := util.GetPodKey(pod)

	// TODO: retrieve the cached sandbox ID from ContainersInformer
	// https://github.com/koordinator-sh/koordinator/issues/1249
	sandboxIDRaw, err := koordletutil.GetPodSandboxContainerID(pod)
	if err != nil {
		// higher verbosity for probably non-running pods
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			klog.V(6).Infof("failed to collect non-running rund pod usage for %s, get sandbox ID err: %s",
				podKey, err)
		} else {
			klog.V(4).Infof("failed to collect rund pod usage for %s, get sandbox ID err: %s", podKey, err)
		}
		return
	}
	if len(sandboxIDRaw) <= 0 {
		klog.V(6).Infof("failed to collect non-running rund pod usage for %s, no container status", podKey)
		return
	}
	_, sandboxID, err := util.ParseContainerId(sandboxIDRaw)
	if err != nil {
		klog.V(4).Infof("failed to parse rund pod sandbox id for %s, err: %s", podKey, err)
		return
	}

	// rund pod use a special memcg path
	// https://aliyuque.antfin.com/sigmahost/bdrtdk/mq5buk#baiVb
	rundMemoryCgroupDir := system.GetRundMemoryCgroupParentDir(sandboxID)

	currentCPUUsage, err0 := p.cgroupReader.ReadCPUAcctUsage(podCgroupDir)
	memStat, err1 := p.cgroupReader.ReadMemoryStat(rundMemoryCgroupDir)
	if err0 != nil || err1 != nil {
		klog.V(4).Infof("failed to collect rund pod usage for %s, CPU err: %s, Memory err: %s",
			podKey, err0, err1)
		return
	}

	lastCPUStatValue, ok := p.lastPodCPUStat.Get(uid)
	p.lastPodCPUStat.Set(uid, framework.CPUStat{
		CPUUsage:  currentCPUUsage,
		Timestamp: collectTime,
	}, gocache.DefaultExpiration)
	klog.V(6).Infof("last rund pod cpu stat size in pod resource collector cache %v", p.lastPodCPUStat.ItemCount())
	if !ok {
		klog.V(4).Infof("ignore the first cpu stat collection for rund pod %s", podKey)
		return
	}
	lastCPUStat := lastCPUStatValue.(framework.CPUStat)
	// do subtraction and division first to avoid overflow
	cpuUsageValue := float64(currentCPUUsage-lastCPUStat.CPUUsage) / float64(collectTime.Sub(lastCPUStat.Timestamp))

	memUsageValue := memStat.Usage()

	metrics := make([]metriccache.MetricSample, 0)

	cpuUsageMetric, err := metriccache.PodCPUUsageMetric.GenerateSample(
		metriccache.MetricPropertiesFunc.Pod(uid), collectTime, cpuUsageValue)
	if err != nil {
		klog.V(4).Infof("failed to generate pod cpu metrics for rund pod %s, err: %v", podKey, err)
		return
	}
	memUsageMetric, err := metriccache.PodMemUsageMetric.GenerateSample(
		metriccache.MetricPropertiesFunc.Pod(uid), collectTime, float64(memUsageValue))
	if err != nil {
		klog.V(4).Infof("failed to generate pod mem metrics for rund pod %s, err %v", podKey, err)
		return
	}

	metrics = append(metrics, cpuUsageMetric, memUsageMetric)
	for deviceName, deviceCollector := range p.deviceCollectors {
		if deviceMetrics, err := deviceCollector.GetPodMetric(uid, meta.CgroupDir, pod.Status.ContainerStatuses); err != nil {
			klog.V(4).Infof("get rund pod %s device usage failed for %v, err: %v", podKey, deviceName, err)
		} else if len(metrics) > 0 {
			metrics = append(metrics, deviceMetrics...)
		}
	}

	klog.V(6).Infof("collect rund pod %s, uid %s finished, metric %+v", podKey, meta.Pod.UID, metrics)

	containerMetrics := p.collectRundContainersResUsed(meta, sandboxID)
	metrics = append(metrics, containerMetrics...)

	appender := p.appendableDB.Appender()
	if err = appender.Append(metrics); err != nil {
		klog.Warningf("Append rund pod metrics error: %v", err)
		return
	}

	if err = appender.Commit(); err != nil {
		klog.Warningf("Commit rund pod metrics failed, error: %v", err)
		return
	}
}

func (p *rundResourceCollector) collectRundContainersResUsed(meta *statesinformer.PodMeta, sandboxID string) []metriccache.MetricSample {
	klog.V(6).Infof("start collectRundContainersResUsed")
	pod := meta.Pod

	containerStatsMap, err := system.GetRundContainerStats(sandboxID)
	if err != nil {
		klog.V(4).Infof("failed to collect rund container usage for %s/%s, get PodStats err: %s",
			pod.Namespace, pod.Name, err)
		return nil
	}
	if len(containerStatsMap) <= 0 {
		klog.V(5).Infof("skip to collect rund container usage for %s/%s, got no valid ContainerStats",
			pod.Namespace, pod.Name)
		return nil
	}

	var containerMetrics []metriccache.MetricSample
	for i := range pod.Status.ContainerStatuses {
		containerStat := &pod.Status.ContainerStatuses[i]
		containerKey := fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, containerStat.Name)
		collectTime := time.Now()
		if len(containerStat.ContainerID) <= 0 {
			klog.V(5).Infof("rund container %s id is empty, maybe not ready, skip this round",
				containerKey)
			continue
		}
		_, containerID, err := util.ParseContainerId(containerStat.ContainerID)
		if err != nil {
			klog.V(4).Infof("failed to parse rund container %s id, err: %s", containerKey, err)
			continue
		}

		containerStats, ok := containerStatsMap[containerID]
		if !ok {
			klog.V(4).Infof("failed to find rund container %s, err: missing ContainerStats", containerKey)
			continue
		}
		if containerStats == nil || containerStats.BaseStats == nil {
			klog.V(4).Infof("failed to get usage for rund container %s, err: invalid ContainerStats %v",
				containerKey, containerStats)
			continue
		}

		currentCPUTick, err := system.GetRundContainerCPUUsageTick(containerStats.BaseStats)
		if err != nil {
			klog.V(4).Infof("failed to get cpu usage for rund container %s, err: %s", containerKey, err)
			continue
		}

		lastCPUStatValue, ok := p.lastContainerCPUStat.Get(containerStat.ContainerID)
		p.lastContainerCPUStat.Set(containerStat.ContainerID, framework.CPUStat{
			CPUTick:   currentCPUTick,
			Timestamp: collectTime,
		}, gocache.DefaultExpiration)
		klog.V(6).Infof("last container cpu stat size in pod resource collector cache %v", p.lastPodCPUStat.ItemCount())
		if !ok {
			klog.V(5).Infof("ignore the first cpu stat collection for rund container %s", containerKey)
			continue
		}
		lastCPUStat := lastCPUStatValue.(framework.CPUStat)
		// 1 jiffy could be 10ms
		// do subtraction and division first to avoid overflow
		cpuUsageValue := float64(currentCPUTick-lastCPUStat.CPUTick) / system.GetPeriodTicks(lastCPUStat.Timestamp, collectTime)

		memoryStat, err := system.GetRundContainerMemoryStat(containerStats.BaseStats)
		if err != nil {
			klog.V(4).Infof("failed to get memory usage for rund container %s, err: %s", containerKey, err)
			continue
		}
		memUsageValue := memoryStat.UsageForRund()

		cpuUsageMetric, cpuErr := metriccache.ContainerCPUUsageMetric.GenerateSample(
			metriccache.MetricPropertiesFunc.Container(containerStat.ContainerID), collectTime, cpuUsageValue)
		memUsageMetric, memErr := metriccache.ContainerMemUsageMetric.GenerateSample(
			metriccache.MetricPropertiesFunc.Container(containerStat.ContainerID), collectTime, float64(memUsageValue))
		if cpuErr != nil || memErr != nil {
			klog.Warningf("generate rund container %s metrics failed, cpu: %v, mem: %v", containerKey, cpuErr, memErr)
			continue
		}

		containerMetrics = append(containerMetrics, cpuUsageMetric, memUsageMetric)

		for deviceName, deviceCollector := range p.deviceCollectors {
			if metrics, err := deviceCollector.GetContainerMetric(containerStat.ContainerID, meta.CgroupDir, containerStat); err != nil {
				klog.Warningf("get rund container %s device usage failed for %v, error: %v", containerKey, deviceName, err)
			} else {
				containerMetrics = append(containerMetrics, metrics...)
			}
		}

		klog.V(6).Infof("collect rund container %s, id %s finished, metric %+v", containerKey, pod.UID, containerMetrics)
	}
	klog.V(5).Infof("collectRundContainersResUsed for rund pod %s/%s finished, container num %d",
		pod.Namespace, pod.Name, len(pod.Status.ContainerStatuses))

	return containerMetrics
}
