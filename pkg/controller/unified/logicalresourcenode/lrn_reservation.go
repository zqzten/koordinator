/*
Copyright 2023 The Koordinator Authors.

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

package logicalresourcenode

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	terwayapis "github.com/AliyunContainerService/terway-apis/network.alibabacloud.com/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/util"
	lrnutil "github.com/koordinator-sh/koordinator/pkg/util/logicalresourcenode"
)

type reservationReconciler struct {
	client.Client
}

func (r *reservationReconciler) reconcile(ctx context.Context, lrn *schedulingv1alpha1.LogicalResourceNode) (ctrl.Result, error) {
	activeReservations, terminatingReservations, err := r.getOwnedReservations(ctx, lrn)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get owned reservations: %v", err)
	}

	if allSatisfied, unsatisfiedNames := reservationExpectations.IsAllSatisfied(lrn.Name); !allSatisfied {
		klog.Warningf("Skip reconcile LogicalResourceNode %s for reservation expectations have %v unsatisfied.", lrn.Name, unsatisfiedNames)
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	var requeueDuration time.Duration

	// Cleanup LRN and all its Reservations and Pods
	if lrn.DeletionTimestamp != nil {
		requeueDuration, err = r.gcTerminatingLRN(ctx, lrn, append(terminatingReservations, activeReservations...))
		if err != nil {
			return ctrl.Result{}, err
		}
		klog.Infof("Successfully gc terminating LogicalResourceNode %s", lrn.Name)
		return ctrl.Result{RequeueAfter: requeueDuration}, nil
	}

	// Cleanup terminating Reservations and their Pods
	for _, rr := range terminatingReservations {
		d, err := r.gcTerminatingReservation(ctx, rr)
		if err != nil {
			klog.Errorf("Failed to gc terminating Reservation %s for LogicalResourceNode %s, ignoring.", rr.Name, lrn.Name)
		}
		if requeueDuration == 0 || d < requeueDuration {
			requeueDuration = d
		}
	}

	result := ctrl.Result{RequeueAfter: requeueDuration}

	// Get the current active Reservation
	var reservation *schedulingv1alpha1.Reservation
	currentGeneration := getGenerationFromLRN(lrn)
	// TODO: maybe there will be more than one Reservation after we support reservations migration.
	if len(activeReservations) > 1 {
		klog.Errorf("Found more than one active Reservations %v for LogicalResourceNode %s, aborting.", getObjectListNames(activeReservations), lrn.Name)
		return result, fmt.Errorf("more than one active Reservations")
	} else if len(activeReservations) == 1 {
		reservation = activeReservations[0]
		gen, err := getGenerationFromReservation(reservation)
		if err != nil {
			return result, err
		} else if gen > currentGeneration {
			klog.Warningf("Found generation of Reservation %s is bigger than %d in LRN, will update it.", reservation.Name, currentGeneration)
			currentGeneration = gen
		}
	}

	var qosGroup *terwayapis.ENIQosGroup
	if reservation != nil && hasQoSGroupAndEnabled(reservation) {
		qosGroup, err = r.getQoSGroupForReservation(ctx, reservation)
		if err != nil {
			return ctrl.Result{}, err
		}
		if allSatisfied, unsatisfiedNames := qosGroupExpectations.IsAllSatisfied(reservation.Name); !allSatisfied {
			klog.Warningf("Skip manage ENIQosGroup for reservation %s unsatisfied: %v.", reservation.Name, unsatisfiedNames)
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
	}

	klog.V(3).Infof("Reconciling LogicalResourceNode %s with current generation %v and reservation %v", lrn.Name, currentGeneration, nameOfReservation(reservation))

	// Initial steps for a new LRN
	if !slices.Contains(lrn.Finalizers, finalizerInternalGC) {
		defer func() {
			lrnExpectations.Expect(lrn)
		}()
		klog.V(3).Infof("Adding %s for LogicalResourceNode %s", finalizerInternalGC, lrn.Name)
		lrn.Finalizers = append(lrn.Finalizers, finalizerInternalGC)
		if err := r.Update(ctx, lrn); err != nil {
			klog.Errorf("Failed to add finalizer to LogicalResourceNode %s: %s", lrn.Name, err)
			return result, err
		}
		if lrn.Status.Phase == "" {
			lrn.Status.Phase = schedulingv1alpha1.LogicalResourceNodePending
			if err := r.Status().Update(ctx, lrn); err != nil {
				return result, err
			}
		}
		return result, nil
	}

	// The main reconcile is here.
	newStatus, err := r.reconcileWithReservation(ctx, lrn, reservation, currentGeneration, qosGroup)
	if err != nil {
		return result, err
	}

	if newStatus != nil && !apiequality.Semantic.DeepEqual(*newStatus, lrn.Status) {
		klog.Infof("Updating LogicalResourceNode %s status as %s", lrn.Name, util.DumpJSON(newStatus))
		lrn.Status = *newStatus
		if err = r.Status().Update(ctx, lrn); err != nil {
			return result, err
		}
		lrnExpectations.Expect(lrn)
	}
	return result, nil
}

func (r *reservationReconciler) reconcileWithReservation(ctx context.Context, lrn *schedulingv1alpha1.LogicalResourceNode,
	reservation *schedulingv1alpha1.Reservation, currentGeneration int64, qosGroup *terwayapis.ENIQosGroup) (status *schedulingv1alpha1.LogicalResourceNodeStatus, err error) {

	status = &schedulingv1alpha1.LogicalResourceNodeStatus{Phase: schedulingv1alpha1.LogicalResourceNodePending}

	// Create a new Reservation if not exists
	if reservation == nil {
		currentGeneration++
		reservation, err = generateNewReservation(lrn, currentGeneration)
		if err != nil {
			return
		}
		if err := r.Create(ctx, reservation); err != nil {
			klog.Warningf("Failed to create Reservation %s for LogicalResourceNode %s: %s", reservation.Name, lrn.Name, err)
			return nil, err
		}
		reservationExpectations.Expect(reservation, lrn.Name)
		klog.Infof("Successfully create Reservation %s for LogicalResourceNode %s", reservation.Name, lrn.Name)

		// Update LRN metadata
		if err := r.updateLRNMetadata(ctx, lrn, currentGeneration, reservation, nil); err != nil {
			return nil, fmt.Errorf("failed to patch LRN metadata: %v", err)
		}
		return
	}

	var node *corev1.Node
	switch reservation.Status.Phase {
	case "", schedulingv1alpha1.ReservationPending:
		for _, cond := range reservation.Status.Conditions {
			status.Conditions = append(status.Conditions, reservationConditionToNodeCondition(cond))
		}

	case schedulingv1alpha1.ReservationAvailable:
		if reservation.Status.NodeName == "" {
			return nil, fmt.Errorf("unexpected Reservation %s available but no nodeName", reservation.Name)
		}

		// Create or wait QoSGroup if needed
		if hasQoSGroupAndEnabled(reservation) {
			if qosGroup == nil {
				qosGroup, err = generateENIQoSGroup(reservation)
				if err != nil {
					return nil, fmt.Errorf("failed to generate ENIQoSGroup: %v", err)
				}

				if err = r.Create(ctx, qosGroup); err != nil {
					return nil, fmt.Errorf("failed to create ENIQoSGroup: %v", err)
				}
				qosGroupExpectations.Expect(qosGroup, reservation.Name)
				return nil, nil
			}
		}

		// Get Node of the Reservation
		node = &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: reservation.Status.NodeName}, node); err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get Node %s for Reservation %s: %v", reservation.Status.NodeName, reservation.Name, err)
			}

			klog.Errorf("Find LogicalResourceNode %s has Reservation %s with nodeName %s, but not found the Node.", lrn.Name, reservation.Name, reservation.Status.NodeName)
			status.Phase = schedulingv1alpha1.LogicalResourceNodeUnknown
			status.Message = fmt.Sprintf("not found Node %s", reservation.Status.NodeName)
			return status, nil
		}

		status.Phase = schedulingv1alpha1.LogicalResourceNodeAvailable
		status.NodeName = node.Name
		status.Allocatable = reservation.Status.Allocatable
		syncNodeStatus(status, node)

		if lrn.Spec.Unschedulable {
			status.Phase = schedulingv1alpha1.LogicalResourceNodeUnschedulable
		}

	default:
		return nil, fmt.Errorf("unexpected Reservation %s phase %s", reservation.Name, reservation.Status.Phase)
	}

	if err := r.updateReservation(ctx, lrn, reservation, qosGroup); err != nil {
		return nil, fmt.Errorf("failed to update reservation: %v", err)
	}

	// Update LRN metadata
	if err := r.updateLRNMetadata(ctx, lrn, currentGeneration, reservation, node); err != nil {
		return nil, fmt.Errorf("failed to update LRN metadata: %v", err)
	}

	return
}

func (r *reservationReconciler) getQoSGroupForReservation(ctx context.Context, reservation *schedulingv1alpha1.Reservation) (*terwayapis.ENIQosGroup, error) {
	qosGroup := &terwayapis.ENIQosGroup{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: metav1.NamespaceSystem, Name: reservation.Name}, qosGroup); err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get ENIQosGroup %s: %v", reservation.Name, err)
		}
		return nil, nil
	}
	qosGroupExpectations.Observe(qosGroup)

	ownerRef := metav1.GetControllerOf(qosGroup)
	if ownerRef == nil || ownerRef.UID != reservation.UID {
		return nil, fmt.Errorf("found ENIQosGroup owner %+v not matched reservation %s", ownerRef, reservation.Name)
	}

	return qosGroup, nil
}

// TODO: temporarily help users to update pod-label-selector on LRN. Should be removed later.
func (r *reservationReconciler) updateReservation(ctx context.Context, lrn *schedulingv1alpha1.LogicalResourceNode,
	reservation *schedulingv1alpha1.Reservation, qosGroup *terwayapis.ENIQosGroup) error {

	// TODO: temporarily help users to update pod-label-selector on LRN. Should be removed later.
	if len(reservation.Spec.Owners) != 1 || reservation.Spec.Owners[0].LabelSelector == nil || reservation.Spec.Owners[0].LabelSelector.MatchLabels == nil {
		return fmt.Errorf("invalid reservation %s with owner %v", reservation.Name, util.DumpJSON(reservation.Spec.Owners))
	}

	patchBody := newPatchObject()

	podLabelSelector, err := lrnutil.GetPodLabelSelector(lrn)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(podLabelSelector, reservation.Spec.Owners[0].LabelSelector.MatchLabels) {
		patchBody.Spec["owners"] = []schedulingv1alpha1.ReservationOwner{{LabelSelector: &metav1.LabelSelector{MatchLabels: podLabelSelector}}}
	}

	var qosGroupNotReady bool
	if qosGroup != nil {
		if qosGroup.Status.Phase == terwayapis.QosGroupPhaseReady {
			if reservation.Labels[schedulingv1alpha1.LabelVPCQoSGroupID] != qosGroup.Status.AutoCreatedID {
				patchBody.Metadata.Labels[schedulingv1alpha1.LabelVPCQoSGroupID] = qosGroup.Status.AutoCreatedID
			}
		} else {
			qosGroupNotReady = true
		}
	}

	expectedUnschedulable := lrn.Spec.Unschedulable || qosGroupNotReady
	if expectedUnschedulable != reservation.Spec.Unschedulable {
		if expectedUnschedulable {
			patchBody.Spec["unschedulable"] = true
		} else {
			patchBody.Spec["unschedulable"] = nil
		}
	}

	var forceSyncLabelRegex *regexp.Regexp
	if exp := lrn.Annotations[schedulingv1alpha1.AnnotationForceSyncLabelRegex]; exp != "" {
		forceSyncLabelRegex, err = regexp.Compile(exp)
		if err != nil {
			return fmt.Errorf("compile invalid force sync regex %s error: %s", exp, err)
		}
	}

	labelsSyncedFromNode := getLabelsSyncedFromNode(lrn.GetAnnotations())
	for k, v := range lrn.Labels {
		rVal := reservation.Labels[k]
		if labelsSyncedFromNode.Has(k) || skipSyncReservationLabels.Has(k) || rVal == v {
			continue
		}
		if rVal != "" && (forceSyncLabelRegex == nil || !forceSyncLabelRegex.MatchString(k)) {
			continue
		}
		patchBody.Metadata.Labels[k] = v
	}

	for k := range reservation.Labels {
		if _, exists := lrn.Labels[k]; !exists && forceSyncLabelRegex != nil && forceSyncLabelRegex.MatchString(k) {
			patchBody.Metadata.Labels[k] = nil
		}
	}

	if patchBody.isEmpty() {
		return nil
	}

	if err := r.Patch(ctx, reservation, client.RawPatch(types.MergePatchType, []byte(util.DumpJSON(patchBody)))); err != nil {
		return fmt.Errorf("failed to patch %s to reservation %s: %v", util.DumpJSON(patchBody), reservation.Name, err)
	}
	klog.Infof("Successfully patched %s to reservation %s", util.DumpJSON(patchBody), reservation.Name)
	reservationExpectations.Expect(reservation, lrn.Name)
	return nil
}

func (r *reservationReconciler) getOwnedReservations(ctx context.Context, lrn *schedulingv1alpha1.LogicalResourceNode) (
	activeReservations []*schedulingv1alpha1.Reservation, terminatingReservations []*schedulingv1alpha1.Reservation, err error) {
	reservationList := &schedulingv1alpha1.ReservationList{}
	if err := r.List(ctx, reservationList, client.MatchingFields{"ownerRefLRN": lrn.Name}); err != nil {
		return nil, nil, err
	}

	for i := range reservationList.Items {
		rr := &reservationList.Items[i]
		reservationExpectations.Observe(rr)

		if rr.DeletionTimestamp != nil {
			terminatingReservations = append(terminatingReservations, rr)
		} else {
			activeReservations = append(activeReservations, rr)
		}
	}
	sort.SliceStable(activeReservations, func(i, j int) bool {
		return activeReservations[i].Name < activeReservations[j].Name
	})
	return
}

func (r *reservationReconciler) gcTerminatingLRN(ctx context.Context, lrn *schedulingv1alpha1.LogicalResourceNode, reservations []*schedulingv1alpha1.Reservation) (time.Duration, error) {
	if len(reservations) == 0 {
		klog.Infof("No reservation found for terminating LogicalResourceNode %s", lrn.Name)
		return 0, r.removeGCFinalizerFromLRN(ctx, lrn.Name)
	}

	klog.Infof("To cleanup reservations %v for terminating LogicalResourceNode %s", getObjectListNames(reservations), lrn.Name)

	var requeueDuration time.Duration
	for _, reservation := range reservations {
		if reservation.DeletionTimestamp != nil {
			d, err := r.gcTerminatingReservation(ctx, reservation)
			if err != nil {
				return 0, err
			}
			if requeueDuration == 0 || d < requeueDuration {
				requeueDuration = d
			}
		} else {
			if err := r.Delete(ctx, reservation); err != nil {
				return 0, fmt.Errorf("failed to delete Reservation %s: %v", reservation.Name, err)
			}
		}
	}

	return requeueDuration, nil
}

func (r *reservationReconciler) gcTerminatingReservation(ctx context.Context, reservation *schedulingv1alpha1.Reservation) (time.Duration, error) {
	leftGrace := time.Duration(*reservationCleanupGracePeriodSeconds)*time.Second - time.Since(reservation.DeletionTimestamp.Time)
	if leftGrace > 0 {
		klog.Infof("Reservation %s is still in termination grace time.", reservation.Name)
		return leftGrace + time.Second, nil
	}

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.MatchingFields{"reservationAllocatedUID": string(reservation.UID)}); err != nil {
		return 0, fmt.Errorf("failed list Pod for terminating Reservation %s: %v", reservation.Name, err)
	}

	klog.V(3).Infof("Terminating Reservation %s is to delete owned pods: %v", reservation.Name, getObjectListNames(podList.Items))

	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		if err := r.Delete(ctx, pod); err != nil {
			return 0, fmt.Errorf("failed to delete Reservation %s pod %s/%s: %v", reservation.Name, pod.Namespace, pod.Name, err)
		}
	}

	return 0, r.removeGCFinalizerFromReservation(ctx, reservation.Name)
}

func (r *reservationReconciler) removeGCFinalizerFromLRN(ctx context.Context, lrnName string) error {
	return removeGCFinalizer(ctx, r.Client, types.NamespacedName{Name: lrnName}, func() client.Object { return &schedulingv1alpha1.LogicalResourceNode{} })
}

func (r *reservationReconciler) removeGCFinalizerFromReservation(ctx context.Context, reservationName string) error {
	return removeGCFinalizer(ctx, r.Client, types.NamespacedName{Name: reservationName}, func() client.Object { return &schedulingv1alpha1.Reservation{} })
}

func (r *reservationReconciler) updateLRNMetadata(ctx context.Context, lrn *schedulingv1alpha1.LogicalResourceNode, generation int64,
	reservation *schedulingv1alpha1.Reservation, node *corev1.Node) error {

	patchBody := generateLRNPatch(lrn, generation, reservation, node)
	if patchBody.isConsistent(lrn) {
		return nil
	}

	if err := r.Patch(ctx, lrn, client.RawPatch(types.MergePatchType, []byte(util.DumpJSON(patchBody)))); err != nil {
		return fmt.Errorf("failed to patch %s to LRN: %v", util.DumpJSON(patchBody), err)
	}
	lrnExpectations.Expect(lrn)
	klog.Infof("Successfully patched %s to LRN %s", util.DumpJSON(patchBody), lrn.Name)
	return nil
}

func generateLRNPatch(lrnMeta metav1.Object, generation int64, reservation *schedulingv1alpha1.Reservation, node *corev1.Node) *patchObject {

	patchBody := patchMetaData{
		Labels:      map[string]interface{}{},
		Annotations: map[string]interface{}{},
	}

	// first, remove all synced labels before
	alreadySyncedLabels := getLabelsSyncedFromNode(lrnMeta.GetAnnotations())
	for _, k := range alreadySyncedLabels.UnsortedList() {
		patchBody.Labels[k] = nil
	}

	// sync labels
	for k, v := range lrnMeta.GetLabels() {
		if alreadySyncedLabels.Has(k) {
			continue
		}
		patchBody.Labels[k] = v
	}

	patchBody.Labels[labelReservationGeneration] = fmt.Sprintf("%d", generation)

	if reservation == nil || reservation.Status.NodeName == "" {
		patchBody.Labels[schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode] = nil
	} else {
		patchBody.Labels[schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode] = reservation.Status.NodeName
	}

	devices := schedulingv1alpha1.LogicalResourceNodeDevices{}
	if reservation != nil {
		deviceAllocated, err := apiext.GetDeviceAllocations(reservation.Annotations)
		if err != nil {
			klog.Errorf("Failed to get device allocated from reservation %s: %v", reservation.Name, err)
		}

		for deviceType, allocs := range deviceAllocated {
			for _, alloc := range allocs {
				devices[deviceType] = append(devices[deviceType], schedulingv1alpha1.LogicalResourceNodeDeviceInfo{Minor: alloc.Minor})
			}
		}
		for _, dis := range devices {
			sort.Slice(dis, func(i, j int) bool {
				return dis[i].Minor < dis[j].Minor
			})
		}
	}

	if len(devices) == 0 {
		patchBody.Annotations[schedulingv1alpha1.AnnotationLogicalResourceNodeDevices] = nil
	} else {
		patchBody.Annotations[schedulingv1alpha1.AnnotationLogicalResourceNodeDevices] = util.DumpJSON(devices)
	}

	syncNodeLabels := getsSyncNodeLabels(node)
	syncNodeLabelKeys := make([]string, 0, len(syncNodeLabels))
	for k, v := range syncNodeLabels {
		patchBody.Labels[k] = v
		syncNodeLabelKeys = append(syncNodeLabelKeys, k)
	}
	sort.Strings(syncNodeLabelKeys)
	if len(syncNodeLabelKeys) == 0 {
		patchBody.Annotations[annotationSyncNodeLabels] = nil
	} else {
		patchBody.Annotations[annotationSyncNodeLabels] = strings.Join(syncNodeLabelKeys, ",")
	}

	return &patchObject{Metadata: patchBody}
}
