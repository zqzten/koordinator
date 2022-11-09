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

package custompodaffinity

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

const (
	Name     = "UnifiedCustomPodAffinity"
	stateKey = Name

	ErrReasonNotMatch = "node(s) didn't satisfy custom pods affinity"
)

var (
	_ framework.PreFilterPlugin = &Plugin{}
	_ framework.FilterPlugin    = &Plugin{}
)

type Plugin struct {
	handle framework.Handle
	cache  *Cache
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	plugin := &Plugin{
		handle: handle,
		cache:  newCache(),
	}
	registersPodEventHandler(handle, plugin.cache)
	return plugin, nil
}

func (p *Plugin) Name() string { return Name }

func (p *Plugin) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) *framework.Status {
	maxInstancePerHost, podSpreadInfo := extunified.GetCustomPodAffinity(pod)
	cycleState.Write(stateKey, &preFilterState{
		podSpreadInfo:      podSpreadInfo,
		maxInstancePerHost: maxInstancePerHost,
	})
	return nil
}

type preFilterState struct {
	podSpreadInfo      *extunified.PodSpreadInfo
	maxInstancePerHost int
}

func (s *preFilterState) Clone() framework.StateData {
	return &preFilterState{
		podSpreadInfo: &extunified.PodSpreadInfo{
			AppName:     s.podSpreadInfo.AppName,
			ServiceUnit: s.podSpreadInfo.ServiceUnit,
		},
		maxInstancePerHost: s.maxInstancePerHost,
	}
}

func (p *Plugin) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (p *Plugin) Filter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	state, status := getPreFilterState(cycleState)
	if status != nil {
		return status
	}
	if state.podSpreadInfo == nil {
		return nil
	}

	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if extunified.IsVirtualKubeletNode(node) {
		return nil
	}

	if p.cache.GetAllocCount(node.Name, state.podSpreadInfo)+1 > state.maxInstancePerHost {
		return framework.NewStatus(framework.Unschedulable, ErrReasonNotMatch)
	}
	return nil
}

func getPreFilterState(cycleState *framework.CycleState) (*preFilterState, *framework.Status) {
	value, err := cycleState.Read(stateKey)
	if err != nil {
		return nil, framework.AsStatus(err)
	}
	state := value.(*preFilterState)
	return state, nil
}
