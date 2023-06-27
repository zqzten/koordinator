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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

const (
	Name     = "UnifiedCustomPodAffinity"
	stateKey = Name

	ErrReasonNotMatch = "node(s) didn't satisfy custom pods affinity"
)

var (
	_ framework.PreFilterPlugin     = &Plugin{}
	_ framework.PreFilterExtensions = &Plugin{}
	_ framework.FilterPlugin        = &Plugin{}
	_ framework.ReservePlugin       = &Plugin{}
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

func (p *Plugin) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	maxInstancePerHost, podSpreadInfo := extunified.GetCustomPodAffinity(pod)
	cycleState.Write(stateKey, &preFilterState{
		podSpreadInfo:      podSpreadInfo,
		maxInstancePerHost: maxInstancePerHost,
		preemptivePods:     map[string]sets.String{},
	})
	return nil, nil
}

type preFilterState struct {
	podSpreadInfo      *extunified.PodSpreadInfo
	maxInstancePerHost int
	preemptivePods     map[string]sets.String
}

func (s *preFilterState) Clone() framework.StateData {
	preemptivePods := map[string]sets.String{}
	for nodeName, podKeys := range preemptivePods {
		preemptivePods[nodeName] = sets.NewString(podKeys.List()...)
	}
	return &preFilterState{
		podSpreadInfo: &extunified.PodSpreadInfo{
			AppName:     s.podSpreadInfo.AppName,
			ServiceUnit: s.podSpreadInfo.ServiceUnit,
		},
		maxInstancePerHost: s.maxInstancePerHost,
		preemptivePods:     preemptivePods,
	}
}

func (p *Plugin) PreFilterExtensions() framework.PreFilterExtensions {
	return p
}

func (p *Plugin) AddPod(ctx context.Context, cycleState *framework.CycleState, podToSchedule *corev1.Pod, podInfoToAdd *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	state, status := getPreFilterState(cycleState)
	if status != nil {
		return status
	}
	if state.podSpreadInfo == nil {
		return nil
	}
	if state.preemptivePods[node.Name] == nil {
		return nil
	}
	state.preemptivePods[node.Name].Delete(getNamespacedName(podInfoToAdd.Pod.Namespace, podInfoToAdd.Pod.Name))
	if state.preemptivePods[node.Name].Len() == 0 {
		delete(state.preemptivePods, node.Name)
	}
	return nil
}

func (p *Plugin) RemovePod(ctx context.Context, cycleState *framework.CycleState, podToSchedule *corev1.Pod, podInfoToRemove *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	state, status := getPreFilterState(cycleState)
	if status != nil {
		return status
	}
	if state.podSpreadInfo == nil {
		return nil
	}
	_, podSpreadInfo := extunified.GetCustomPodAffinity(podInfoToRemove.Pod)
	if podSpreadInfo == nil {
		return nil
	}
	if (state.podSpreadInfo.ServiceUnit == "" && podSpreadInfo.AppName == state.podSpreadInfo.AppName) ||
		(state.podSpreadInfo.ServiceUnit == podSpreadInfo.ServiceUnit && state.podSpreadInfo.AppName == podSpreadInfo.AppName) {
		if state.preemptivePods[node.Name] == nil {
			state.preemptivePods[node.Name] = sets.NewString()
		}
		state.preemptivePods[node.Name].Insert(getNamespacedName(podInfoToRemove.Pod.Namespace, podInfoToRemove.Pod.Name))
	}
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
	preemptivePods := state.preemptivePods[node.Name]
	reservationRestoreState := getReservationRestoreState(cycleState)
	matchedReservePods := reservationRestoreState.getNodeMatchedReservedPods(node.Name)
	preemptivePods = preemptivePods.Union(matchedReservePods)
	if p.cache.GetAllocCount(node.Name, state.podSpreadInfo, preemptivePods)+1 > state.maxInstancePerHost {
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

func (p *Plugin) Reserve(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	p.cache.AddPod(nodeName, pod)
	return nil
}

func (p *Plugin) Unreserve(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) {
	p.cache.DeletePod(nodeName, pod)
}
