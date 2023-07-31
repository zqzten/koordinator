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

package deviceshare

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	reservationutil "github.com/koordinator-sh/koordinator/pkg/util/reservation"
)

const reservationRestoreStateKey = Name + "/reservationRestoreState"

type reservationRestoreStateData struct {
	skip        bool
	nodeToState frameworkext.NodeReservationRestoreStates
}

type nodeReservationRestoreStateData struct {
	matched   []reservationAlloc
	unmatched []reservationAlloc

	mergedMatchedAllocatable map[schedulingv1alpha1.DeviceType]deviceResources
	mergedMatchedAllocated   map[schedulingv1alpha1.DeviceType]deviceResources
	mergedUnmatchedUsed      map[schedulingv1alpha1.DeviceType]deviceResources
}

type reservationAlloc struct {
	rInfo       *frameworkext.ReservationInfo
	allocatable map[schedulingv1alpha1.DeviceType]deviceResources
	allocated   map[schedulingv1alpha1.DeviceType]deviceResources
	remained    map[schedulingv1alpha1.DeviceType]deviceResources
}

func getReservationRestoreState(cycleState *framework.CycleState) *reservationRestoreStateData {
	var state *reservationRestoreStateData
	value, err := cycleState.Read(reservationRestoreStateKey)
	if err == nil {
		state, _ = value.(*reservationRestoreStateData)
	}
	if state == nil {
		state = &reservationRestoreStateData{
			skip: true,
		}
	}
	return state
}

func (s *reservationRestoreStateData) Clone() framework.StateData {
	return s
}

func (s *reservationRestoreStateData) getNodeState(nodeName string) *nodeReservationRestoreStateData {
	val := s.nodeToState[nodeName]
	ns, ok := val.(*nodeReservationRestoreStateData)
	if !ok {
		ns = &nodeReservationRestoreStateData{}
	}
	return ns
}

func (rs *nodeReservationRestoreStateData) mergeReservationAllocations() {
	unmatched := rs.unmatched
	if len(unmatched) > 0 {
		mergedUnmatchedUsed := map[schedulingv1alpha1.DeviceType]deviceResources{}
		for _, alloc := range unmatched {
			used := subtractAllocated(copyDeviceResources(alloc.allocatable), alloc.remained)
			mergedUnmatchedUsed = appendAllocated(mergedUnmatchedUsed, used)
		}
		rs.mergedUnmatchedUsed = mergedUnmatchedUsed
	}

	matched := rs.matched
	if len(matched) > 0 {
		mergedMatchedAllocatable := map[schedulingv1alpha1.DeviceType]deviceResources{}
		mergedMatchedAllocated := map[schedulingv1alpha1.DeviceType]deviceResources{}
		for _, alloc := range matched {
			mergedMatchedAllocatable = appendAllocated(mergedMatchedAllocatable, alloc.allocatable)
			mergedMatchedAllocated = appendAllocated(mergedMatchedAllocated, alloc.allocated)
		}
		rs.mergedMatchedAllocatable = mergedMatchedAllocatable
		rs.mergedMatchedAllocated = mergedMatchedAllocated
	}

	return
}

func (p *Plugin) PreRestoreReservation(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) *framework.Status {
	skip, _, status := PreparePod(pod)
	if !status.IsSuccess() {
		return status
	}
	cycleState.Write(reservationRestoreStateKey, &reservationRestoreStateData{skip: skip})
	return nil
}

func (p *Plugin) RestoreReservation(ctx context.Context, cycleState *framework.CycleState, podToSchedule *corev1.Pod, matched []*frameworkext.ReservationInfo, unmatched []*frameworkext.ReservationInfo, nodeInfo *framework.NodeInfo) (interface{}, *framework.Status) {
	state := getReservationRestoreState(cycleState)
	if state.skip {
		return nil, nil
	}

	nodeName := nodeInfo.Node().Name
	nd := p.nodeDeviceCache.getNodeDevice(nodeName, false)
	if nd == nil {
		return nil, nil
	}

	nd.lock.RLock()
	defer nd.lock.RUnlock()

	filterFn := func(reservations []*frameworkext.ReservationInfo) []reservationAlloc {
		if len(reservations) == 0 {
			return nil
		}
		result := make([]reservationAlloc, 0, len(reservations))
		for _, rInfo := range reservations {
			reservePod := rInfo.GetReservePod()
			allocatable := nd.getUsed(reservePod.Namespace, reservePod.Name)
			if len(allocatable) == 0 {
				continue
			}
			minorHints := newDeviceMinorMap(allocatable)
			var allocated map[schedulingv1alpha1.DeviceType]deviceResources
			for _, podRequirement := range rInfo.AssignedPods {
				podAllocated := nd.getUsed(podRequirement.Namespace, podRequirement.Name)
				if len(podAllocated) > 0 {
					allocated = appendAllocatedByHints(minorHints, allocated, podAllocated)
				}
			}
			result = append(result, reservationAlloc{
				rInfo:       rInfo,
				allocatable: allocatable,
				allocated:   allocated,
				remained:    subtractAllocated(copyDeviceResources(allocatable), allocated),
			})
		}
		return result
	}
	filteredMatched := filterFn(matched)
	filteredUnmatched := filterFn(unmatched)
	s := &nodeReservationRestoreStateData{
		matched:   filteredMatched,
		unmatched: filteredUnmatched,
	}
	s.mergeReservationAllocations()
	return s, nil
}

func (p *Plugin) FinalRestoreReservation(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeToStates frameworkext.NodeReservationRestoreStates) *framework.Status {
	state := getReservationRestoreState(cycleState)
	if state.skip {
		return nil
	}
	state.nodeToState = nodeToStates
	return nil
}

func (p *Plugin) tryAllocateFromReservation(
	state *preFilterState,
	restoreState *nodeReservationRestoreStateData,
	matchedReservations []reservationAlloc,
	nodeDeviceInfo *nodeDevice,
	nodeName string,
	pod *corev1.Pod,
	basicPreemptible map[schedulingv1alpha1.DeviceType]deviceResources,
	requiredFromReservation bool,
) (apiext.DeviceAllocations, *framework.Status) {
	if len(matchedReservations) == 0 {
		return nil, nil
	}

	var (
		totalAligned           int
		totalRestricted        int
		insufficientAligned    int
		insufficientRestricted int

		preemptibleForDefault map[schedulingv1alpha1.DeviceType]deviceResources
		preemptibleForAligned map[schedulingv1alpha1.DeviceType]deviceResources
	)

	for _, alloc := range matchedReservations {
		rInfo := alloc.rInfo
		preemptibleInRR := state.preemptibleInRRs[nodeName][rInfo.UID()]
		preferred := newDeviceMinorMap(alloc.allocatable)

		allocatePolicy := rInfo.GetAllocatePolicy()
		if allocatePolicy == schedulingv1alpha1.ReservationAllocatePolicyDefault {
			//
			// The Default Policy is a relax policy that allows Pods to be allocated from multiple Reservations
			// and the remaining resources of the node.
			// The formula for calculating the remaining amount per device instance in this scenario is as follows:
			// basicPreemptible = sum(allocated(unmatched reservations)) + preemptible(node)
			// free = total - (used - basicPreemptible - sum(allocatable(matched reservations)) - preemptible(currentReservation))
			//
			if preemptibleForDefault == nil {
				preemptibleForDefault = appendAllocated(nil, basicPreemptible, restoreState.mergedMatchedAllocatable)
			}
			preemptible := appendAllocated(nil, preemptibleForDefault, preemptibleInRR)
			var required map[schedulingv1alpha1.DeviceType]sets.Int
			if requiredFromReservation {
				required = preferred
			}
			result, err := p.allocator.Allocate(nodeName, pod, state.podRequests, nodeDeviceInfo, required, preferred, nil, preemptible)
			if len(result) > 0 && err == nil {
				return result, nil
			}
			continue
		}

		if allocatePolicy == schedulingv1alpha1.ReservationAllocatePolicyAligned {
			totalAligned++
		} else if allocatePolicy == schedulingv1alpha1.ReservationAllocatePolicyRestricted {
			totalRestricted++
		}

		//
		// The Aligned and Restricted Policies only allow Pods to be allocated from current Reservation
		// and the remaining resources of the node.
		// And the Restricted policy requires that if the device resources requested by the Pod overlap with
		// the device resources reserved by the current Reservation, such devices can only be allocated from the current Reservation.
		// The formula for calculating the remaining amount per device instance in this scenario is as follows:
		// basicPreemptible = sum(allocated(unmatched reservations)) + preemptible(node)
		// free = total - (used - basicPreemptible - sum(allocated(matched reservations)) - sum(remained(currentReservation)) - preemptible(currentReservation))
		//
		if preemptibleForAligned == nil {
			preemptibleForAligned = appendAllocated(nil, basicPreemptible, restoreState.mergedMatchedAllocated)
		}
		preemptible := appendAllocated(nil, preemptibleForAligned, alloc.remained, preemptibleInRR)
		if allocatePolicy == schedulingv1alpha1.ReservationAllocatePolicyAligned {
			result, err := p.allocator.Allocate(nodeName, pod, state.podRequests, nodeDeviceInfo, preferred, preferred, nil, preemptible)
			if len(result) > 0 && err == nil {
				return result, nil
			}
			insufficientAligned++
		} else if allocatePolicy == schedulingv1alpha1.ReservationAllocatePolicyRestricted {
			result, err := p.allocator.Allocate(nodeName, pod, state.podRequests, nodeDeviceInfo, preferred, preferred, nil, preemptible)
			nodeFits := len(result) > 0 && err == nil
			if nodeFits {
				//
				// It is necessary to check separately whether the remaining resources of the device instance
				// reserved by the Restricted Reservation meet the requirements of the Pod, to ensure that
				// the intersecting resources do not exceed the reserved range of the Restricted Reservation.
				//
				requiredDeviceResources := calcRequiredDeviceResources(&alloc, preemptibleInRR)
				result, err := p.allocator.Allocate(nodeName, pod, state.podRequests, nodeDeviceInfo, preferred, preferred, requiredDeviceResources, preemptible)
				if len(result) > 0 && err == nil {
					return result, nil
				}
			}
			insufficientRestricted++
		}
	}
	if (totalAligned > 0 && totalAligned == insufficientAligned) ||
		(totalRestricted > 0 && totalRestricted == insufficientRestricted) {
		return nil, framework.NewStatus(framework.Unschedulable, "node(s) reservations insufficient devices")
	}
	return nil, nil
}

func calcRequiredDeviceResources(alloc *reservationAlloc, preemptibleInRR map[schedulingv1alpha1.DeviceType]deviceResources) map[schedulingv1alpha1.DeviceType]deviceResources {
	var required map[schedulingv1alpha1.DeviceType]deviceResources
	minorHints := newDeviceMinorMap(alloc.allocatable)
	required = appendAllocatedByHints(minorHints, required, alloc.remained, preemptibleInRR)
	if len(required) == 0 {
		// required is empty to indicate that there are no resources left.
		// A valid object must be constructed with no remaining capacity.
		if required == nil {
			required = map[schedulingv1alpha1.DeviceType]deviceResources{}
		}
		for deviceType, minors := range minorHints {
			resources := deviceResources{}
			for minor := range minors.UnsortedList() {
				resources[minor] = corev1.ResourceList{}
			}
			required[deviceType] = resources
		}
	}
	return required
}

func (p *Plugin) allocateWithNominatedReservation(
	cycleState *framework.CycleState,
	state *preFilterState,
	restoreState *nodeReservationRestoreStateData,
	nodeDeviceInfo *nodeDevice,
	nodeName string,
	pod *corev1.Pod,
	basicPreemptible map[schedulingv1alpha1.DeviceType]deviceResources,
) (apiext.DeviceAllocations, *framework.Status) {
	if reservationutil.IsReservePod(pod) {
		return nil, nil
	}

	reservation := frameworkext.GetNominatedReservation(cycleState, nodeName)
	if reservation == nil {
		return nil, nil
	}

	allocIndex := -1
	for i, v := range restoreState.matched {
		if v.rInfo.UID() == reservation.UID() {
			allocIndex = i
			break
		}
	}
	if allocIndex == -1 {
		return nil, framework.AsStatus(fmt.Errorf("missing nominated reservation %v", klog.KObj(reservation)))
	}

	result, status := p.tryAllocateFromReservation(
		state,
		restoreState,
		restoreState.matched[allocIndex:allocIndex+1],
		nodeDeviceInfo,
		nodeName,
		pod,
		basicPreemptible,
		false,
	)
	return result, status
}
