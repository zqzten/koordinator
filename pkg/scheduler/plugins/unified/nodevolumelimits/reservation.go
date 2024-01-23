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

package nodevolumelimits

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	volumeutil "k8s.io/kubernetes/pkg/volume/util"
	"k8s.io/utils/pointer"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/reservation"
)

const reservationRestoreStateKey = CSIName + "/reservationRestoreState"

type reservationRestoreStateData struct {
	skip        bool
	nodeToState frameworkext.NodeReservationRestoreStates
}

type nodeReservationRestoreStateData struct {
	unmatchedRInfos               []*frameworkext.ReservationInfo
	unmatchedReservedVolumeLimits map[string]int
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

func (pl *CSILimits) PreRestoreReservation(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) *framework.Status {
	cycleState.Write(reservationRestoreStateKey, &reservationRestoreStateData{skip: len(pod.Spec.Volumes) == 0})
	return nil
}

func (pl *CSILimits) RestoreReservation(ctx context.Context, cycleState *framework.CycleState, podToSchedule *corev1.Pod, matched []*frameworkext.ReservationInfo, unmatched []*frameworkext.ReservationInfo, nodeInfo *framework.NodeInfo) (interface{}, *framework.Status) {
	state := getReservationRestoreState(cycleState)
	if state.skip {
		return nil, nil
	}

	volumeReservationCounts := map[string]int{}
	var unmatchedRInfos []*frameworkext.ReservationInfo
	for _, rInfo := range unmatched {
		volumeReservationSpec, err := apiext.GetVolumeReservationSpec(rInfo.GetObject().GetAnnotations())
		if err != nil {
			return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
		}
		if volumeReservationSpec == nil || len(volumeReservationSpec.VolumeReservations) == 0 {
			continue
		}
		for _, v := range volumeReservationSpec.VolumeReservations {
			volumeReservationCounts[v.StorageClassName] += v.VolumeCount
		}
		unmatchedRInfos = append(unmatchedRInfos, rInfo)
	}
	s := &nodeReservationRestoreStateData{
		unmatchedRInfos:               unmatchedRInfos,
		unmatchedReservedVolumeLimits: volumeReservationCounts,
	}
	return s, nil
}

func (pl *CSILimits) FinalRestoreReservation(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeToStates frameworkext.NodeReservationRestoreStates) *framework.Status {
	state := getReservationRestoreState(cycleState)
	if state.skip {
		return nil
	}
	state.nodeToState = nodeToStates
	return nil
}

func (pl *CSILimits) getReservationInfoByPod(rrState *nodeReservationRestoreStateData, pod *corev1.Pod, nodeName string) (*frameworkext.ReservationInfo, *framework.Status) {
	var rInfo *frameworkext.ReservationInfo
	reservationCache := reservation.GetReservationCache()
	if reservationCache != nil {
		rInfo = reservationCache.GetReservationInfoByPod(pod, nodeName)
		if rInfo == nil {
			reservationNominator := pl.handle.GetReservationNominator()
			if reservationNominator != nil {
				rInfo = reservationNominator.GetNominatedReservation(pod, nodeName)
			}
		}
	}

	if rInfo == nil {
		return nil, nil
	}

	for _, v := range rrState.unmatchedRInfos {
		if v.UID() != rInfo.UID() {
			continue
		}
		return rInfo, nil
	}

	return nil, nil
}

func (pl *CSILimits) getReservationVolumeLimitKey(namespace string, csiNode *storagev1.CSINode, volumeReservations map[string]int) (map[string]int, error) {
	if len(volumeReservations) == 0 {
		return nil, nil
	}
	result := map[string]int{}
	for storageClassName, count := range volumeReservations {
		driverName, volumeHandle := pl.getCSIDriverInfoFromSC(csiNode, &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-reservation-%s", storageClassName, csiNode.UID),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: pointer.String(storageClassName),
			},
		})
		if driverName == "" || volumeHandle == "" {
			klog.V(5).InfoS("Could not find a CSI driver name or volume handle, not counting volume")
			continue
		}

		volumeLimitKey := volumeutil.GetCSIAttachLimitKey(driverName)
		result[volumeLimitKey] += count
	}

	return result, nil
}

func (pl *CSILimits) getReservationVolumeCounts(reservePod *corev1.Pod, nodeInfo *framework.NodeInfo) (map[string]int, *framework.Status) {
	volumeReservationSpec, err := apiext.GetVolumeReservationSpec(reservePod.Annotations)
	if err != nil {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	if volumeReservationSpec == nil {
		return nil, nil
	}

	counts := map[string]int{}
	for _, v := range volumeReservationSpec.VolumeReservations {
		counts[v.StorageClassName] += v.VolumeCount
	}

	csiNode, _ := pl.csiNodeLister.Get(nodeInfo.Node().Name)
	counts, err = pl.getReservationVolumeLimitKey(reservePod.Namespace, csiNode, counts)
	if err != nil {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	return counts, nil
}
