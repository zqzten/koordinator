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
	"encoding/json"
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unsafe"

	terwayapis "github.com/AliyunContainerService/terway-apis/network.alibabacloud.com/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/util/slice"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/util"
	"github.com/koordinator-sh/koordinator/pkg/util/expectations"
	lrnutil "github.com/koordinator-sh/koordinator/pkg/util/logicalresourcenode"
)

const (
	// finalizerInternalGC is the finalizer on LRN and Reservation to do cleanup when deleted.
	finalizerInternalGC = "lrn.koordinator.sh/internal-gc"

	labelOwnedByLRN = "lrn.koordinator.sh/owned-by-lrn"

	annotationSyncNodeLabels = "lrn.koordinator.sh/sync-node-labels"

	reservationNameGen = "-gen-"
)

var (
	nodeExpectations        = expectations.NewResourceVersionExpectation()
	lrnExpectations         = expectations.NewResourceVersionExpectation()
	reservationExpectations = expectations.NewResourceVersionExpectation()
	qosGroupExpectations    = expectations.NewResourceVersionExpectation()

	workerNumFlag = flag.Int("lrn-controller-workers", 5, "The workers number of LRN controller.")
)

func getsSyncNodeLabels(cfg *lrnutil.Config, node *corev1.Node) (res map[string]string) {
	if node == nil {
		return
	}

	res = make(map[string]string, len(cfg.Common.SyncNodeLabelKeys))
	for _, key := range cfg.Common.SyncNodeLabelKeys {
		if val, ok := node.Labels[key]; ok {
			res[key] = val
		}
	}
	return
}

func syncNodeStatus(cfg *lrnutil.Config, lrnStatus *schedulingv1alpha1.LogicalResourceNodeStatus, node *corev1.Node) {
	lrnStatus.NodeStatus = &schedulingv1alpha1.LRNNodeStatus{
		Unschedulable: node.Spec.Unschedulable,
		PrintColumn:   "NotReady",
	}

	for _, cond := range node.Status.Conditions {
		if !slice.ContainsString(cfg.Common.SyncNodeConditionTypes, string(cond.Type), nil) {
			continue
		}
		// Currently no need to sync those translate/heartbeat timestamps
		lrnStatus.NodeStatus.Conditions = append(lrnStatus.NodeStatus.Conditions, corev1.NodeCondition{
			Type:    cond.Type,
			Status:  cond.Status,
			Reason:  cond.Reason,
			Message: cond.Message,
		})
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			lrnStatus.NodeStatus.PrintColumn = "Ready"
		}
	}
	if node.Spec.Unschedulable {
		lrnStatus.NodeStatus.PrintColumn = fmt.Sprintf("%s,%s", lrnStatus.NodeStatus.PrintColumn, "SchedulingDisabled")
	}
}

func generateNewReservation(cfg *lrnutil.Config, lrn *schedulingv1alpha1.LogicalResourceNode, generation int64) (*schedulingv1alpha1.Reservation, error) {
	owners, err := lrnutil.GetReservationOwners(lrn)
	if err != nil {
		return nil, err
	}

	ownerRef := metav1.NewControllerRef(lrn, schedulingv1alpha1.SchemeGroupVersion.WithKind("LogicalResourceNode"))
	reservation := &schedulingv1alpha1.Reservation{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s%s%d", lrn.Name, reservationNameGen, generation),
			Finalizers: []string{
				finalizerInternalGC,
			},
			Labels: map[string]string{
				labelOwnedByLRN: lrn.Name,
			},
			Annotations:     map[string]string{},
			OwnerReferences: []metav1.OwnerReference{*ownerRef},
		},
		Spec: schedulingv1alpha1.ReservationSpec{
			Template: &corev1.PodTemplateSpec{
				Spec: *lrnutil.RequirementsToPodSpec(&lrn.Spec.Requirements),
			},
			Owners:         owners,
			TTL:            &metav1.Duration{Duration: 0},
			AllocateOnce:   utilpointer.Bool(false),
			AllocatePolicy: schedulingv1alpha1.ReservationAllocatePolicyRestricted,
		},
	}

	syncedLabels := getLabelsSyncedFromNode(lrn.Annotations)
	for k, v := range lrn.Labels {
		if !syncedLabels.Has(k) && !slice.ContainsString(cfg.Common.SkipSyncReservationLabelKeys, k, nil) {
			reservation.Labels[k] = v
		}
	}

	for k, v := range lrn.Annotations {
		if slice.ContainsString(cfg.Common.SyncReservationAnnotationKeys, k, nil) {
			reservation.Annotations[k] = v
		}
	}

	if lrn.Spec.Unschedulable || hasQoSGroupAndEnabled(cfg, lrn) || isInitializing(lrn) {
		reservation.Spec.Unschedulable = true
	}

	return reservation, nil
}

func getGenerationFromLRN(lrn *schedulingv1alpha1.LogicalResourceNode) int64 {
	rg, ok := lrn.Labels[schedulingv1alpha1.LabelLogicalResourceNodeReservationGeneration]
	if !ok {
		return -1
	}
	generation, err := strconv.ParseInt(rg, 10, 64)
	if err != nil {
		klog.Warningf("Failed to get reservation generation %v from LogicalResourceNode %s", rg, lrn.Name)
		return -1
	}
	return generation
}

func getGenerationFromReservation(reservation *schedulingv1alpha1.Reservation) (int64, error) {
	words := strings.Split(reservation.Name, reservationNameGen)
	if len(words) <= 1 {
		return 0, fmt.Errorf("not found generation in Reservation name %s", reservation.Name)
	}
	generation, err := strconv.ParseInt(words[len(words)-1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error parse generation in Reservation name %s: %v", reservation.Name, err)
	}
	return generation, nil
}

func getObjectListNames(objList interface{}) []string {
	defer func() {
		if r := recover(); r != nil {
			klog.Warningf("Recover panic from getObjectListNames %v : %v", util.DumpJSON(objList), r)
		}
	}()
	rv := reflect.ValueOf(objList)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	names := make([]string, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		if obj, ok := rv.Index(i).Interface().(metav1.Object); ok {
			names = append(names, obj.GetName())
		} else {
			metadataField := rv.Index(i).FieldByName("ObjectMeta")
			if !metadataField.IsValid() || metadataField.IsZero() {
				continue
			}
			metadataFieldPtr := (*metav1.ObjectMeta)(unsafe.Pointer(metadataField.UnsafeAddr()))
			names = append(names, metadataFieldPtr.Name)
		}
	}
	return names
}

func nameOfReservation(reservation *schedulingv1alpha1.Reservation) string {
	if reservation == nil {
		return "<nil>"
	}
	return reservation.Name
}

func removeGCFinalizer(ctx context.Context, c client.Client, req types.NamespacedName, newObjFunc func() client.Object) error {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		newObj := newObjFunc()
		if err := c.Get(ctx, req, newObj); err != nil {
			return err
		}

		newFinalizers := slice.RemoveString(newObj.GetFinalizers(), finalizerInternalGC, nil)
		if len(newFinalizers) == len(newObj.GetFinalizers()) {
			return nil
		}
		newObj.SetFinalizers(newFinalizers)
		return c.Update(ctx, newObj)
	})
	return fmt.Errorf("failed to remove gc finalizer: %s", err)
}

func reservationConditionToNodeCondition(rCond schedulingv1alpha1.ReservationCondition) corev1.NodeCondition {
	return corev1.NodeCondition{
		Type:               corev1.NodeConditionType(rCond.Type),
		Status:             corev1.ConditionStatus(rCond.Status),
		LastHeartbeatTime:  rCond.LastProbeTime,
		LastTransitionTime: rCond.LastTransitionTime,
		Message:            rCond.Message,
		Reason:             rCond.Reason,
	}
}

type patchObject struct {
	Metadata patchMetaData          `json:"metadata"`
	Spec     map[string]interface{} `json:"spec,omitempty"`
}

type patchMetaData struct {
	Labels      map[string]interface{} `json:"labels,omitempty"`
	Annotations map[string]interface{} `json:"annotations,omitempty"`
}

func newPatchObject() *patchObject {
	return &patchObject{
		Metadata: patchMetaData{Labels: map[string]interface{}{}, Annotations: map[string]interface{}{}},
		Spec:     map[string]interface{}{},
	}
}

func (po *patchObject) isConsistent(obj metav1.Object) bool {
	return compareMap(obj.GetLabels(), po.Metadata.Labels) && compareMap(obj.GetAnnotations(), po.Metadata.Annotations)
}

func (po *patchObject) isEmpty() bool {
	return len(po.Spec) == 0 && len(po.Metadata.Labels) == 0 && len(po.Metadata.Annotations) == 0
}

func compareMap(original map[string]string, patch map[string]interface{}) bool {
	for k, v := range patch {
		if v == nil {
			if _, exists := original[k]; exists {
				return false
			}
		} else {
			if original[k] != v {
				return false
			}
		}
	}
	return true
}

func getLabelsSyncedFromNode(annotations map[string]string) sets.String {
	val, ok := annotations[annotationSyncNodeLabels]
	if !ok || len(val) == 0 {
		return nil
	}
	return sets.NewString(strings.Split(val, ",")...)
}

func hasQoSGroupAndEnabled(cfg *lrnutil.Config, obj metav1.Object) bool {
	return cfg.Common.EnableQoSGroup && obj.GetAnnotations()[schedulingv1alpha1.AnnotationVPCQoSThreshold] != ""
}

func generateENIQoSGroup(reservation *schedulingv1alpha1.Reservation) (*terwayapis.ENIQosGroup, error) {
	str, ok := reservation.Annotations[schedulingv1alpha1.AnnotationVPCQoSThreshold]
	if !ok {
		return nil, fmt.Errorf("no annotation %s in reservation %s", schedulingv1alpha1.AnnotationVPCQoSThreshold, reservation.Name)
	}

	if reservation.Status.NodeName == "" {
		return nil, fmt.Errorf("no nodeName in reservation %s status", reservation.Name)
	}

	threshold := schedulingv1alpha1.LRNVPCQoSThreshold{}
	if err := json.Unmarshal([]byte(str), &threshold); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s: %v", str, err)
	}

	ownerRef := metav1.NewControllerRef(reservation, schedulingv1alpha1.SchemeGroupVersion.WithKind("Reservation"))
	qosGroup := terwayapis.ENIQosGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      reservation.Name,
			Labels: map[string]string{
				labelOwnedByLRN: reservation.Labels[labelOwnedByLRN],
			},
			OwnerReferences: []metav1.OwnerReference{*ownerRef},
		},
		Spec: terwayapis.ENIQosGroupSpec{
			NodeName: reservation.Status.NodeName,
			Bandwidth: terwayapis.ENIQosGroupBandwidth{
				Tx: resource.MustParse(threshold.Tx),
				Rx: resource.MustParse(threshold.Rx),
			},
			PPS: terwayapis.ENIQosGroupPPS{
				Tx: resource.MustParse(threshold.TxPps),
				Rx: resource.MustParse(threshold.RxPps),
			},
		},
	}
	return &qosGroup, nil
}

func isInitializing(lrn *schedulingv1alpha1.LogicalResourceNode) bool {
	return lrn.Labels[schedulingv1alpha1.LabelLogicalResourceNodeInitializing] == "true"
}
