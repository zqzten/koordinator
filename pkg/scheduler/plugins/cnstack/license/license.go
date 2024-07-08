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

package license

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
)

const (
	preFilterStateKey = "PreFilterLicenseData"
)

type LicenseCheckFunc func(handle framework.Handle) bool
type ResponsibleForPodFunc func(handle framework.Handle, pod *corev1.Pod) bool

type licenseData struct {
	license bool // set true if license is valid
}

func (ld *licenseData) Clone() framework.StateData {
	return &licenseData{
		license: ld.license,
	}
}

type LicensePlugin struct {
	sync.RWMutex
	name        string
	condition   LicenseCheckFunc
	responsible ResponsibleForPodFunc
	factory     frameworkruntime.PluginFactory
	args        runtime.Object
	handle      framework.Handle
	plugin      framework.Plugin
	license     bool
}

var _ framework.PreFilterPlugin = &LicensePlugin{}
var _ framework.FilterPlugin = &LicensePlugin{}
var _ framework.PostFilterPlugin = &LicensePlugin{}
var _ framework.PreScorePlugin = &LicensePlugin{}
var _ framework.ScorePlugin = &LicensePlugin{}
var _ framework.ReservePlugin = &LicensePlugin{}
var _ framework.PermitPlugin = &LicensePlugin{}
var _ framework.PreBindPlugin = &LicensePlugin{}
var _ framework.BindPlugin = &LicensePlugin{}
var _ framework.PostBindPlugin = &LicensePlugin{}
var _ frameworkext.PreFilterTransformer = &LicensePlugin{}
var _ frameworkext.FilterTransformer = &LicensePlugin{}
var _ frameworkext.ScoreTransformer = &LicensePlugin{}

// LicensePlugin supports license check of Scheduler Plugin
func Register(name string, factory frameworkruntime.PluginFactory, condition LicenseCheckFunc, responsible ResponsibleForPodFunc) frameworkruntime.PluginFactory {
	return func(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
		lp := &LicensePlugin{
			name:        name,
			condition:   condition,
			responsible: responsible,
			factory:     factory,
			args:        obj,
			handle:      handle,
		}
		p, err := lp.factory(lp.args, lp.handle)
		if err != nil {
			klog.Fatalf("license check initializing plugin %q error: %v", lp.name, err)
		}
		klog.Infof("license check initializing plugin %q success", lp.name)
		lp.plugin = p

		go lp.start()
		return lp, nil
	}
}

func (lp *LicensePlugin) start() {
	err := wait.PollImmediateInfinite(3*time.Minute, func() (done bool, err error) {
		license := lp.condition(lp.handle)
		lp.Lock()
		lp.license = license
		lp.Unlock()
		return false, nil
	})
	if err != nil {
		klog.Fatalf("license check plugin initializing plugin %q error: %v", lp.name, err)
	}
}

func (lp *LicensePlugin) licenseValidWriteState(state *framework.CycleState) bool {
	lp.RLock()
	defer lp.RUnlock()
	state.Write(preFilterStateKey, &licenseData{
		license: lp.license,
	})
	return lp.license
}

func (lp *LicensePlugin) stateLicenseValid(state *framework.CycleState) bool {
	v, err := state.Read(preFilterStateKey)
	if err != nil {
		return false
	}
	cache, ok := v.(*licenseData)
	if !ok || cache == nil {
		return false
	}
	return cache.license
}

func (lp *LicensePlugin) Name() string {
	return lp.name
}

func (lp *LicensePlugin) PreFilter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod) *framework.Status {
	if lp.licenseValidWriteState(state) {
		if p, ok := lp.plugin.(framework.PreFilterPlugin); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run PreFilter", lp.name)
			return p.PreFilter(ctx, state, pod)
		}
		klog.V(6).Infof("license check plugin %q valid, PreFilter not implemented return nil", lp.name)
		return nil
	} else if lp.responsible(lp.handle, pod) {
		klog.V(6).Infof("license check plugin %q invalid, PreFilter return error", lp.name)
		return framework.NewStatus(framework.Error, fmt.Sprintf("license check plugin %q invalid", lp.name))
	}
	klog.V(6).Infof("license check plugin %q invalid, plugin not responsible for the pod PreFilter return nil", lp.name)
	return nil
}

func (lp *LicensePlugin) PreFilterExtensions() framework.PreFilterExtensions {
	if p, ok := lp.plugin.(framework.PreFilterPlugin); ok {
		klog.V(6).Infof("license check plugin %q, delegating plugin run PreFilterExtensions", lp.name)
		return p.PreFilterExtensions()
	}
	klog.V(6).Infof("license check plugin %q, PreFilterExtensions not implemented return nil", lp.name)
	return nil
}

func (lp *LicensePlugin) Filter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(framework.FilterPlugin); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run Filter", lp.name)
			return p.Filter(ctx, state, pod, nodeInfo)
		}
		klog.V(6).Infof("license check plugin %q valid, FilterPlugin not implemented return nil", lp.name)
		return nil
	} else if lp.responsible(lp.handle, pod) {
		klog.V(6).Infof("license check plugin %q invalid, Filter return error", lp.name)
		return framework.NewStatus(framework.Error, fmt.Sprintf("license check plugin %q invalid", lp.name))
	}
	klog.V(6).Infof("license check plugin %q invalid, plugin not responsible for the pod PreFilter return nil", lp.name)
	return nil
}

func (lp *LicensePlugin) PostFilter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, filteredNodeStatusMap framework.NodeToStatusMap) (*framework.PostFilterResult, *framework.Status) {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(framework.PostFilterPlugin); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run PostFilter", lp.name)
			return p.PostFilter(ctx, state, pod, filteredNodeStatusMap)
		}
		klog.V(6).Infof("license check plugin %q valid, PostFilter not implemented return skip", lp.name)
		return nil, framework.NewStatus(framework.Skip)
	} else if lp.responsible(lp.handle, pod) {
		klog.V(6).Infof("license check plugin %q invalid, PostFilter return error", lp.name)
		return nil, framework.NewStatus(framework.Error, fmt.Sprintf("license check plugin %q invalid", lp.name))
	}
	klog.V(6).Infof("license check plugin %q invalid, plugin not responsible for the pod PostFilter return skip", lp.name)
	return nil, framework.NewStatus(framework.Skip)
}

func (lp *LicensePlugin) PreScore(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodes []*corev1.Node) *framework.Status {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(framework.PreScorePlugin); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run PreScore", lp.name)
			return p.PreScore(ctx, state, pod, nodes)
		}
		klog.V(6).Infof("license check plugin %q valid, PreScore not implemented return nil", lp.name)
		return nil
	} else if lp.responsible(lp.handle, pod) {
		klog.V(6).Infof("license check plugin %q invalid, PreScore return error", lp.name)
		return framework.NewStatus(framework.Error, fmt.Sprintf("license check plugin %q invalid", lp.name))
	}
	klog.V(6).Infof("license check plugin %q invalid, plugin not responsible for the pod PreScore return nil", lp.name)
	return nil
}

func (lp *LicensePlugin) Score(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(framework.ScorePlugin); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run Score", lp.name)
			return p.Score(ctx, state, pod, nodeName)
		}
		klog.V(6).Infof("license check plugin %q valid, Score not implemented return nil", lp.name)
		return 0, nil
	} else if lp.responsible(lp.handle, pod) {
		klog.V(6).Infof("license check plugin %q invalid, Score return error", lp.name)
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("license check plugin %q invalid", lp.name))
	}
	klog.V(6).Infof("license check plugin %q invalid, plugin not responsible for the pod Score return nil", lp.name)
	return 0, nil
}

func (lp *LicensePlugin) ScoreExtensions() framework.ScoreExtensions {
	if p, ok := lp.plugin.(framework.ScorePlugin); ok {
		klog.V(6).Infof("license check plugin %q, delegating plugin run ScoreExtensions", lp.name)
		return p.ScoreExtensions()
	}
	klog.V(6).Infof("license check plugin %q, ScoreExtensions not implemented return nil", lp.name)
	return nil
}

func (lp *LicensePlugin) Reserve(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(framework.ReservePlugin); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run Reserve", lp.name)
			return p.Reserve(ctx, state, pod, nodeName)
		}
		klog.V(6).Infof("license check plugin %q valid, Reserve not implemented return nil", lp.name)
		return nil
	} else if lp.responsible(lp.handle, pod) {
		klog.V(6).Infof("license check plugin %q invalid, Reserve return error", lp.name)
		return framework.NewStatus(framework.Error, fmt.Sprintf("license check plugin %q invalid", lp.name))
	}
	klog.V(6).Infof("license check plugin %q invalid, plugin not responsible for the pod Reserve return nil", lp.name)
	return nil
}

func (lp *LicensePlugin) Unreserve(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(framework.ReservePlugin); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run Unreserve", lp.name)
			p.Unreserve(ctx, state, pod, nodeName)
			return
		}
		klog.V(6).Infof("license check plugin %q valid, Unreserve not implemented return", lp.name)
		return
	}
	klog.V(6).Infof("license check plugin %q invalid, Unreserve return", lp.name)
}

func (lp *LicensePlugin) Permit(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (*framework.Status, time.Duration) {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(framework.PermitPlugin); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run Permit", lp.name)
			return p.Permit(ctx, state, pod, nodeName)
		}
		klog.V(6).Infof("license check plugin %q valid, Permit not implemented return nil", lp.name)
		return nil, 0
	} else if lp.responsible(lp.handle, pod) {
		klog.V(6).Infof("license check plugin %q invalid, Permit return error", lp.name)
		return framework.NewStatus(framework.Error, fmt.Sprintf("license check plugin %q invalid", lp.name)), 0
	}
	klog.V(6).Infof("license check plugin %q invalid, plugin not responsible for the pod Permit return nil", lp.name)
	return nil, 0
}

func (lp *LicensePlugin) PreBind(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(framework.PreBindPlugin); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run PreBind", lp.name)
			return p.PreBind(ctx, state, pod, nodeName)
		}
		klog.V(6).Infof("license check plugin %q valid, PreBind not implemented return nil", lp.name)
		return nil
	} else if lp.responsible(lp.handle, pod) {
		klog.V(6).Infof("license check plugin %q invalid, PreBind return error", lp.name)
		return framework.NewStatus(framework.Error, fmt.Sprintf("license check plugin %q invalid", lp.name))
	}
	klog.V(6).Infof("license check plugin %q invalid, plugin not responsible for the pod PreBind return nil", lp.name)
	return nil
}

func (lp *LicensePlugin) Bind(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(framework.BindPlugin); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run Bind", lp.name)
			return p.Bind(ctx, state, pod, nodeName)
		}
		klog.V(6).Infof("license check plugin %q valid, Bind not implemented return nil", lp.name)
		return nil
	} else if lp.responsible(lp.handle, pod) {
		klog.V(6).Infof("license check plugin %q invalid, Bind return error", lp.name)
		return framework.NewStatus(framework.Error, fmt.Sprintf("license check plugin %q invalid", lp.name))
	}
	klog.V(6).Infof("license check plugin %q invalid, plugin not responsible for the pod Bind return nil", lp.name)
	return nil
}

func (lp *LicensePlugin) PostBind(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(framework.PostBindPlugin); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run PostBind", lp.name)
			p.PostBind(ctx, state, pod, nodeName)
			return
		}
		klog.V(6).Infof("license check plugin %q valid, PostBind not implemented return", lp.name)
		return
	}
	klog.V(6).Infof("license check plugin %q invalid, PostBind return", lp.name)
}

func (lp *LicensePlugin) BeforePreFilter(handle frameworkext.ExtendedHandle, state *framework.CycleState, pod *corev1.Pod) (*corev1.Pod, bool) {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(frameworkext.PreFilterTransformer); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run PreFilterHook", lp.name)
			return p.BeforePreFilter(handle, state, pod)
		}
		klog.V(6).Infof("license check plugin %q valid, PreFilterHook not implemented return false", lp.name)
		return pod, false
	}
	klog.V(6).Infof("license check plugin %q invalid, PreFilterHook return false", lp.name)
	return pod, false
}

func (lp *LicensePlugin) BeforeFilter(handle frameworkext.ExtendedHandle, state *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) (*corev1.Pod, *framework.NodeInfo, bool) {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(frameworkext.FilterTransformer); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run FilterHook", lp.name)
			return p.BeforeFilter(handle, state, pod, nodeInfo)
		}
		klog.V(6).Infof("license check plugin %q valid, FilterHook not implemented return false", lp.name)
		return pod, nodeInfo, false
	}
	klog.V(6).Infof("license check plugin %q invalid, FilterHook return false", lp.name)
	return pod, nodeInfo, false
}

func (lp *LicensePlugin) BeforeScore(handle frameworkext.ExtendedHandle, state *framework.CycleState, pod *corev1.Pod, nodes []*corev1.Node) (*corev1.Pod, []*corev1.Node, bool) {
	if lp.stateLicenseValid(state) {
		if p, ok := lp.plugin.(frameworkext.ScoreTransformer); ok {
			klog.V(6).Infof("license check plugin %q valid, delegating plugin run ScoreHook", lp.name)
			return p.BeforeScore(handle, state, pod, nodes)
		}
		klog.V(6).Infof("license check plugin %q valid, ScoreHook not implemented return false", lp.name)
		return pod, nodes, false
	}
	klog.V(6).Infof("license check plugin %q invalid, ScoreHook return false", lp.name)
	return pod, nodes, false
}
