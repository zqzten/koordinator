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

package lazyload

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
)

type ConditionFunc func(handle framework.Handle) bool

type LazyLoadPlugin struct {
	name      string
	condition ConditionFunc
	factory   frameworkruntime.PluginFactory
	args      runtime.Object
	handle    framework.Handle
	plugin    framework.Plugin
}

var _ framework.PreFilterPlugin = &LazyLoadPlugin{}
var _ framework.FilterPlugin = &LazyLoadPlugin{}
var _ framework.PostFilterPlugin = &LazyLoadPlugin{}
var _ framework.PreScorePlugin = &LazyLoadPlugin{}
var _ framework.ScorePlugin = &LazyLoadPlugin{}
var _ framework.ReservePlugin = &LazyLoadPlugin{}
var _ framework.PermitPlugin = &LazyLoadPlugin{}
var _ framework.PreBindPlugin = &LazyLoadPlugin{}
var _ framework.BindPlugin = &LazyLoadPlugin{}
var _ framework.PostBindPlugin = &LazyLoadPlugin{}
var _ frameworkext.PreFilterPhaseHook = &LazyLoadPlugin{}
var _ frameworkext.FilterPhaseHook = &LazyLoadPlugin{}
var _ frameworkext.ScorePhaseHook = &LazyLoadPlugin{}

// LazyLoadPlugin supports lazy loading of Scheduler Plugin
func Register(name string, factory frameworkruntime.PluginFactory, condition ConditionFunc) frameworkruntime.PluginFactory {
	return func(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
		llp := &LazyLoadPlugin{
			name:      name,
			condition: condition,
			factory:   factory,
			args:      obj,
			handle:    handle,
		}
		go llp.lazyLoad()
		return llp, nil
	}
}

func (llp *LazyLoadPlugin) lazyLoad() {
	err := wait.PollUntil(5*time.Second, func() (done bool, err error) {
		klog.V(6).Infof("lazy load plugin %q run ConditionFunc", llp.name)
		return llp.condition(llp.handle), nil
	}, context.TODO().Done())

	if err != nil {
		klog.Fatalf("lazy load initializing plugin %q error: %v", llp.name, err)
	}

	p, err := llp.factory(llp.args, llp.handle)
	if err != nil {
		klog.Fatalf("lazy load initializing plugin %q error: %v", llp.name, err)
	}
	klog.Infof("lazy load initializing plugin %q success", llp.name)
	llp.plugin = p
}

func (llp *LazyLoadPlugin) Name() string {
	return llp.name
}

func (llp *LazyLoadPlugin) PreFilter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod) *framework.Status {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.PreFilterPlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run PreFilter", llp.name)
			return p.PreFilter(ctx, state, pod)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, PreFilter return nil", llp.name)
	return nil
}

func (llp *LazyLoadPlugin) PreFilterExtensions() framework.PreFilterExtensions {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.PreFilterPlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run PreFilterExtensions", llp.name)
			return p.PreFilterExtensions()
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, PreFilterExtensions return nil", llp.name)
	return nil
}

func (llp *LazyLoadPlugin) Filter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.FilterPlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run Filter", llp.name)
			return p.Filter(ctx, state, pod, nodeInfo)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, Filter return nil", llp.name)
	return nil
}

func (llp *LazyLoadPlugin) PostFilter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, filteredNodeStatusMap framework.NodeToStatusMap) (*framework.PostFilterResult, *framework.Status) {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.PostFilterPlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run PostFilter", llp.name)
			return p.PostFilter(ctx, state, pod, filteredNodeStatusMap)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, PostFilter return skip", llp.name)
	return nil, framework.NewStatus(framework.Skip)
}

func (llp *LazyLoadPlugin) PreScore(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodes []*corev1.Node) *framework.Status {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.PreScorePlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run PreScore", llp.name)
			return p.PreScore(ctx, state, pod, nodes)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, PreScore return nil", llp.name)
	return nil
}

func (llp *LazyLoadPlugin) Score(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.ScorePlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run Score", llp.name)
			return p.Score(ctx, state, pod, nodeName)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, Score return nil", llp.name)
	return 0, nil
}

func (llp *LazyLoadPlugin) ScoreExtensions() framework.ScoreExtensions {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.ScorePlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run ScoreExtensions", llp.name)
			return p.ScoreExtensions()
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, ScoreExtensions return nil", llp.name)
	return nil
}

func (llp *LazyLoadPlugin) Reserve(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.ReservePlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run Reserve", llp.name)
			return p.Reserve(ctx, state, pod, nodeName)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, Reserve return nil", llp.name)
	return nil
}

func (llp *LazyLoadPlugin) Unreserve(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.ReservePlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run Unreserve", llp.name)
			p.Unreserve(ctx, state, pod, nodeName)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, Unreserve return", llp.name)
}

func (llp *LazyLoadPlugin) Permit(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (*framework.Status, time.Duration) {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.PermitPlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run Permit", llp.name)
			return p.Permit(ctx, state, pod, nodeName)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, Permit return nil", llp.name)
	return nil, 0
}

func (llp *LazyLoadPlugin) PreBind(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.PreBindPlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run PreBind", llp.name)
			return p.PreBind(ctx, state, pod, nodeName)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, PreBind return nil", llp.name)
	return nil
}

func (llp *LazyLoadPlugin) Bind(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.BindPlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run Bind", llp.name)
			return p.Bind(ctx, state, pod, nodeName)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, Bind return nil", llp.name)
	return nil
}

func (llp *LazyLoadPlugin) PostBind(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(framework.PostBindPlugin); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run PostBind", llp.name)
			p.PostBind(ctx, state, pod, nodeName)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, PostBind return", llp.name)
}

func (llp *LazyLoadPlugin) PreFilterHook(handle frameworkext.ExtendedHandle, state *framework.CycleState, pod *corev1.Pod) (*corev1.Pod, bool) {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(frameworkext.PreFilterPhaseHook); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run PreFilterHook", llp.name)
			return p.PreFilterHook(handle, state, pod)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, PreFilterHook return false", llp.name)
	return pod, false
}

func (llp *LazyLoadPlugin) FilterHook(handle frameworkext.ExtendedHandle, cycleState *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) (*corev1.Pod, *framework.NodeInfo, bool) {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(frameworkext.FilterPhaseHook); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run FilterHook", llp.name)
			return p.FilterHook(handle, cycleState, pod, nodeInfo)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, FilterHook return false", llp.name)
	return pod, nodeInfo, false
}

func (llp *LazyLoadPlugin) ScoreHook(handle frameworkext.ExtendedHandle, cycleState *framework.CycleState, pod *corev1.Pod, nodes []*corev1.Node) (*corev1.Pod, []*corev1.Node, bool) {
	if llp.plugin != nil {
		if p, ok := llp.plugin.(frameworkext.ScorePhaseHook); ok {
			klog.V(6).Infof("lazy load plugin %q initialized, delegating plugin run ScoreHook", llp.name)
			return p.ScoreHook(handle, cycleState, pod, nodes)
		}
	}
	klog.V(6).Infof("lazy load plugin %q uninitialized, ScoreHook return false", llp.name)
	return pod, nodes, false
}
