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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
)

var (
	_ frameworkext.ReservationRestorePlugin = &Plugin{}
)

const reservationRestoreStateKey = Name + "/reservationRestoreState"

type reservationRestoreStateData struct {
	podSpreadInfo *extunified.PodSpreadInfo
	nodeToState   frameworkext.NodeReservationRestoreStates
}

func getReservationRestoreState(cycleState *framework.CycleState) *reservationRestoreStateData {
	var state *reservationRestoreStateData
	value, err := cycleState.Read(reservationRestoreStateKey)
	if err == nil {
		state, _ = value.(*reservationRestoreStateData)
	}
	if state == nil {
		state = &reservationRestoreStateData{}
	}
	return state
}

func (s *reservationRestoreStateData) Clone() framework.StateData {
	return s
}

func (s *reservationRestoreStateData) getNodeMatchedReservedPods(nodeName string) sets.String {
	val := s.nodeToState[nodeName]
	nodeMatchedReservedPods, ok := val.(sets.String)
	if !ok {
		nodeMatchedReservedPods = sets.NewString()
	}
	return nodeMatchedReservedPods
}

func (p *Plugin) PreRestoreReservation(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) *framework.Status {
	_, podSpreadInfo := extunified.GetCustomPodAffinity(pod)
	cycleState.Write(reservationRestoreStateKey, &reservationRestoreStateData{podSpreadInfo: podSpreadInfo})
	return nil
}

func (p *Plugin) RestoreReservation(ctx context.Context, cycleState *framework.CycleState, podToSchedule *corev1.Pod, matched []*frameworkext.ReservationInfo, unmatched []*frameworkext.ReservationInfo, nodeInfo *framework.NodeInfo) (interface{}, *framework.Status) {
	state := getReservationRestoreState(cycleState)
	if state.podSpreadInfo == nil {
		return nil, nil
	}
	node := nodeInfo.Node()
	if node == nil {
		return nil, framework.NewStatus(framework.Error, "node not found")
	}
	matchedReservePods := sets.NewString()
	for _, matchedReservationInfo := range matched {
		reservePod := matchedReservationInfo.GetReservePod()
		_, reservePodSpreadInfo := extunified.GetCustomPodAffinity(reservePod)
		if reservePodSpreadInfo == nil {
			continue
		}
		if (state.podSpreadInfo.ServiceUnit == "" && reservePodSpreadInfo.AppName == state.podSpreadInfo.AppName) ||
			(state.podSpreadInfo.ServiceUnit == reservePodSpreadInfo.ServiceUnit && state.podSpreadInfo.AppName == reservePodSpreadInfo.AppName) {
			matchedReservePods.Insert(getNamespacedName(reservePod.Namespace, reservePod.Name))
		}
	}
	return matchedReservePods, nil
}

func (p *Plugin) FinalRestoreReservation(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeToStates frameworkext.NodeReservationRestoreStates) *framework.Status {
	state := getReservationRestoreState(cycleState)
	if state.podSpreadInfo == nil {
		return nil
	}
	state.nodeToState = nodeToStates
	return nil
}
