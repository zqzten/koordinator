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

package tainttoleration

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	v1helper "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/tainttoleration"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

const (
	Name = "UnifiedTaintToleration"
)

// Plugin is a plugin that checks if a pod tolerates a node's taints.
type Plugin struct {
	handle framework.Handle
	*tainttoleration.TaintToleration
}

var _ framework.FilterPlugin = &Plugin{}
var _ framework.PreScorePlugin = &Plugin{}
var _ framework.ScorePlugin = &Plugin{}
var _ framework.EnqueueExtensions = &Plugin{}

// New initializes a new plugin and returns it.
func New(args runtime.Object, h framework.Handle) (framework.Plugin, error) {
	internalPlugin, err := tainttoleration.New(args, h)
	if err != nil {
		return nil, err
	}
	return &Plugin{
		handle:          h,
		TaintToleration: internalPlugin.(*tainttoleration.TaintToleration),
	}, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (pl *Plugin) Name() string {
	return Name
}

// Filter invoked at the filter extension point.
func (pl *Plugin) Filter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	if nodeInfo == nil || nodeInfo.Node() == nil {
		return framework.AsStatus(fmt.Errorf("invalid nodeInfo"))
	}
	if extunified.AffinityECI(pod) && extunified.IsVirtualKubeletNode(nodeInfo.Node()) && !isPodTolerateVkRequiredTaint(pod) {
		podTolerateVK := pod.DeepCopy()
		podTolerateVK.Spec.Tolerations = append(podTolerateVK.Spec.Tolerations, corev1.Toleration{
			Key:      extunified.VKTaintKey,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		})
		return pl.TaintToleration.Filter(ctx, state, podTolerateVK, nodeInfo)
	}
	return pl.TaintToleration.Filter(ctx, state, pod, nodeInfo)
}

func isPodTolerateVkRequiredTaint(pod *corev1.Pod) bool {
	return v1helper.TolerationsTolerateTaint(pod.Spec.Tolerations, &corev1.Taint{
		Key:    extunified.VKTaintKey,
		Effect: corev1.TaintEffectNoSchedule,
	})
}

// PreScore builds and writes cycle state used by Score and NormalizeScore.
func (pl *Plugin) PreScore(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodes []*corev1.Node) *framework.Status {
	if len(nodes) == 0 {
		return nil
	}
	if extunified.AffinityECI(pod) && !isPodTolerateVkPreferTaint(pod) {
		podTolerateVK := pod.DeepCopy()
		podTolerateVK.Spec.Tolerations = append(podTolerateVK.Spec.Tolerations, corev1.Toleration{
			Key:      extunified.VKTaintKey,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectPreferNoSchedule,
		})
		return pl.TaintToleration.PreScore(ctx, state, podTolerateVK, nodes)
	}
	return pl.TaintToleration.PreScore(ctx, state, pod, nodes)
}

func isPodTolerateVkPreferTaint(pod *corev1.Pod) bool {
	return v1helper.TolerationsTolerateTaint(pod.Spec.Tolerations, &corev1.Taint{
		Key:    extunified.VKTaintKey,
		Effect: corev1.TaintEffectPreferNoSchedule,
	})
}
