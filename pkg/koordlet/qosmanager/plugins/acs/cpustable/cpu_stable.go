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
	"encoding/json"
	"flag"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metrics"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/qosmanager/framework"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

func init() {
	flag.DurationVar(&ReconcileInterval, "cpu-stable-reconcile-interval", ReconcileInterval, "Reconcile interval by CPUStable strategy. Minimum interval is 1 second. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h).")
	flag.StringVar(&ControllerName, "cpu-stable-controller", ControllerName, "The controller name for the CPUStable strategy.")
}

var ReconcileInterval = 5 * time.Second

const PluginName = "CPUStable"

var timeNow = time.Now // for testing

type cpuStable struct {
	statesInformer statesinformer.StatesInformer
	metricCache    metriccache.MetricCache
	executor       resourceexecutor.ResourceUpdateExecutor
	runInterval    time.Duration
	controller     PodStableController
}

func New(opt *framework.Options) framework.QOSStrategy {
	return &cpuStable{
		statesInformer: opt.StatesInformer,
		metricCache:    opt.MetricCache,
		executor:       resourceexecutor.NewResourceUpdateExecutor(),
		runInterval:    ReconcileInterval,
		controller:     newPodStableController(ControllerName, opt),
	}
}

func (c *cpuStable) Enabled() bool {
	return features.DefaultKoordletFeatureGate.Enabled(features.CPUStable) && c.runInterval > 0
}

func (c *cpuStable) Setup(ctx *framework.Context) {
}

func (c *cpuStable) Run(stopCh <-chan struct{}) {
	go wait.Until(c.runOnce, c.runInterval, stopCh)
}

func (c *cpuStable) runOnce() {
	// 1. get node state
	// 1.1 get node-level strategy
	// 1.2 get node util to generate warm-up operation
	// 2. list pods and for each pod:
	// 2.1 get pod-level cpu satisfaction rate and the cpu util
	// 2.2 calculate the operation (how to adjust the ht_ratio in the next step)
	// 2.3 apply the operation
	// 3. finish a turn
	node := c.statesInformer.GetNode()
	if node == nil {
		klog.Warningf("failed to get node, skip reconcile")
		return
	}
	nodeSLO := c.statesInformer.GetNodeSLO()
	strategy, err := getNodeCPUStableStrategy(nodeSLO)
	if err != nil {
		klog.Warningf("failed to get cpu stable strategy, err: %s", err)
		return
	}
	// validate node strategy
	if err = c.controller.ValidateStrategy(strategy); err != nil {
		klog.Warningf("failed to validate cpu stable strategy, controller %s, err: %s", ControllerName, err)
		return
	}

	// enable ht_ratio via sched_features
	isSchedFeatureEnabled, msg := system.EnableHTAwareQuotaIfSupported()
	if !isSchedFeatureEnabled {
		klog.V(4).Infof("failed to enable ht_ratio for the system, abort the strategy, msg: %s", msg)
		return
	}

	podMetas := c.statesInformer.GetAllPods()
	nodeWarmupStat, err := c.controller.GetWarmupState(node, podMetas, strategy)
	if err != nil {
		klog.Warningf("failed to get node warmup state, err: %s", err)
		return
	}

	metrics.ResetPodHTRatio()
	var updaters []resourceexecutor.ResourceUpdater
	var counter struct {
		Succeeded      int
		SkipNonRunning int
		SkipNonLS      int
		ErrStrategy    int
		ErrUpdater     int
		FailSignal     int
	}
	for _, podMeta := range podMetas {
		podKey := podMeta.Key()
		if !podMeta.IsRunningOrPending() {
			klog.V(6).Infof("skip pod %s is not running or pending", podKey)
			counter.SkipNonRunning++
			continue
		}
		pod := podMeta.Pod
		// currently we only handle LS pods
		if qos := extension.GetPodQoSClassRaw(pod); qos != extension.QoSLS && qos != extension.QoSNone {
			klog.V(6).Infof("skip pod %s qos is not LS or none, qos %s", podKey, qos)
			counter.SkipNonLS++
			continue
		}

		podStrategy, err := getPodCPUStableStrategy(pod, strategy)
		if err != nil {
			klog.V(4).Infof("failed to get cpu stable strategy for pod %s, aborted, err: %s", podKey, err)
			counter.ErrStrategy++
			continue
		}
		if podStrategy == nil { // no pod-level strategy, use the node-level strategy
			podStrategy = strategy
		} else if err = c.controller.ValidateStrategy(podStrategy); err != nil { // validate merged strategy
			klog.V(4).Infof("failed to validate cpu stable strategy for pod %s, aborted, err: %s", podKey, err)
			counter.ErrStrategy++
			continue
		}

		podSignal, err := c.controller.GetSignal(pod, podStrategy)
		if err != nil {
			klog.V(4).Infof("failed to calculate signal for pod %s, use signal %s, err: %s",
				podKey, podSignal.String(), err)
			counter.FailSignal++
		} else {
			klog.V(6).Infof("calculate signal for pod %s is %s", podKey, podSignal.String())
		}

		podUpdaters, err := c.controller.GetResourceUpdaters(podMeta, podSignal, podStrategy, nodeWarmupStat)
		if err != nil {
			klog.V(4).Infof("failed to generate updaters for pod %s, aborted, err: %s", podKey, err)
			counter.ErrUpdater++
			continue
		}

		updaters = append(updaters, podUpdaters...)
		counter.Succeeded++
		klog.V(5).Infof("generate resource updaters for pod %s, signal %s", podKey, podSignal.String())
	}

	c.executor.UpdateBatch(true, updaters...)
	klog.V(4).Infof("run once CPUStable finished, updaters num %v, pods num %v, counter %+v",
		len(updaters), len(podMetas), counter)
}

func getNodeCPUStableStrategy(nodeSLO *slov1alpha1.NodeSLO) (*unified.CPUStableStrategy, error) {
	if nodeSLO == nil {
		return nil, fmt.Errorf("nodeSLO is nil")
	}

	cpuStableStrategyIf, exist := nodeSLO.Spec.Extensions.Object[unified.CPUStableExtKey]
	if !exist {
		return nil, fmt.Errorf("cpu stable strategy is nil")
	}
	cpuStableStrategyStr, err := json.Marshal(cpuStableStrategyIf)
	if err != nil {
		return nil, err
	}
	cpuStableStrategy := &unified.CPUStableStrategy{}
	if err = json.Unmarshal(cpuStableStrategyStr, cpuStableStrategy); err != nil {
		return nil, err
	}

	return cpuStableStrategy, nil
}

func getPodCPUStableStrategy(pod *corev1.Pod, nodeStrategy *unified.CPUStableStrategy) (*unified.CPUStableStrategy, error) {
	podStrategy, err := unified.GetPodCPUStableStrategy(pod.Annotations)
	if err != nil {
		return nil, err
	}
	if podStrategy == nil {
		return nil, nil
	}

	// merge with node-level strategy
	mergedStrategy := nodeStrategy.DeepCopy()
	mergedStrategyIf, err := util.MergeCfg(mergedStrategy, &podStrategy.CPUStableStrategy)
	if err != nil {
		return nil, fmt.Errorf("merge pod cpu stable strategy failed, err: %w", err)
	}
	mergedStrategy = mergedStrategyIf.(*unified.CPUStableStrategy)

	return mergedStrategy, nil
}

func isPodInitializing(pod *corev1.Pod, initializeWindowSeconds *int64) bool {
	if initializeWindowSeconds == nil {
		return false
	}
	// pod is not considered initializing if pod is not running; e.g. pod in image pulling
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	return getPodStartedDuration(pod) <= time.Duration(*initializeWindowSeconds)*time.Second
}

// getPodStartedDuration returns the duration since the all regular containers started.
func getPodStartedDuration(pod *corev1.Pod) time.Duration {
	startedPeriod := time.Duration(-1)
	now := timeNow()
	ignoredContainerMap := map[string]struct{}{}
	for i := range pod.Spec.Containers {
		c := pod.Spec.Containers[i]
		if unified.IsContainerIgnoreResource(&c) {
			ignoredContainerMap[c.Name] = struct{}{}
		}
	}
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if _, ok := ignoredContainerMap[containerStatus.Name]; ok {
			continue
		}
		// only count running container
		if containerStatus.State.Running != nil && (startedPeriod == -1 || now.Sub(containerStatus.State.Running.StartedAt.Time) < startedPeriod) {
			startedPeriod = now.Sub(containerStatus.State.Running.StartedAt.Time)
		}
	}
	if startedPeriod == -1 {
		return 0
	}
	return startedPeriod
}
