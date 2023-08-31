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

package mutating

import (
	"context"
	"encoding/json"
	"fmt"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/util"
	utilclient "github.com/koordinator-sh/koordinator/pkg/util/client"
	lrnutil "github.com/koordinator-sh/koordinator/pkg/util/logicalresourcenode"

	terwaytypes "github.com/AliyunContainerService/terway-apis/types"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (h *PodMutatingHandler) mutatingLRNPodCreate(ctx context.Context, req admission.Request, pod *corev1.Pod) error {
	if req.Operation != admissionv1.Create {
		return nil
	}

	lrnList := schedulingv1alpha1.LogicalResourceNodeList{}
	if err := h.Client.List(ctx, &lrnList, utilclient.DisableDeepCopy); err != nil {
		return err
	}

	var lrn *schedulingv1alpha1.LogicalResourceNode
	for i := range lrnList.Items {
		lrn1 := &lrnList.Items[i]
		if lrn1.DeletionTimestamp != nil {
			continue
		}

		podLabelSelector1, err := lrnutil.GetPodLabelSelector(lrn1)
		if err != nil {
			klog.Warningf("Failed to get pod label selector from LogicalResourceNode %s when admit Pod %s/%s: %v", lrn1.Name, req.Namespace, pod.Name, err)
			continue
		}

		podSelector := labels.SelectorFromSet(podLabelSelector1)
		if !podSelector.Matches(labels.Set(pod.Labels)) {
			continue
		}

		lrn = lrn1
		break
	}

	if lrn == nil {
		return nil
	}

	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	pod.Labels[apiext.LabelPodMutatingUpdate] = "true"

	reservationAffinity := apiext.ReservationAffinity{}

	if len(pod.Spec.NodeSelector) > 0 {
		reservationAffinity.ReservationSelector = pod.Spec.NodeSelector
		pod.Spec.NodeSelector = nil
	}

	if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil && pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		reservationAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &apiext.ReservationAffinitySelector{
			ReservationSelectorTerms: pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
		}
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = nil
	}

	if reservationAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil || len(reservationAffinity.ReservationSelector) > 0 {
		if pod.Annotations == nil {
			pod.Annotations = map[string]string{}
		}
		pod.Annotations[apiext.AnnotationReservationAffinity] = util.DumpJSON(reservationAffinity)
	}

	// append LRN tolerations into pod
	for _, taint := range lrn.Spec.Requirements.Tolerations {
		pod.Spec.Tolerations = append(pod.Spec.Tolerations, taint)
	}

	return nil
}

func (h *PodMutatingHandler) mutatingLRNPodUpdate(ctx context.Context, req admission.Request, pod *corev1.Pod) error {
	if req.Operation != admissionv1.Update || pod.Annotations[apiext.AnnotationReservationAllocated] == "" {
		return nil
	}

	oldPod := &corev1.Pod{}
	if err := h.Decoder.DecodeRaw(req.OldObject, oldPod); err != nil {
		return fmt.Errorf("failed to decode old pod object from request: %v", err)
	}

	if oldPod.Annotations[apiext.AnnotationReservationAllocated] != "" {
		return nil
	}

	ra, err := apiext.GetReservationAllocated(pod)
	if ra == nil || err != nil {
		return fmt.Errorf("failed to get reservation allocated %s from pod: %v", pod.Annotations[apiext.AnnotationReservationAllocated], err)
	}

	reservation := &schedulingv1alpha1.Reservation{}
	if err := h.Client.Get(ctx, types.NamespacedName{Name: ra.Name}, reservation); err != nil {
		return fmt.Errorf("failed to get reservation %s: %v", util.DumpJSON(ra), err)
	}

	lrnName := lrnutil.GetReservationOwnerLRN(reservation)
	if lrnName == "" {
		klog.Warningf("Found no LRN owner in Reservation %s for Pod %s/%s", reservation.Name, pod.Namespace, pod.Name)
		return nil
	}

	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	pod.Labels[schedulingv1alpha1.LabelLogicalResourceNodePodAssign] = lrnName

	if groupID := reservation.Labels[schedulingv1alpha1.LabelVPCQoSGroupID]; groupID != "" {
		if err = injectQoSGroupID(pod, groupID); err != nil {
			return err
		}
	}

	delete(pod.Labels, apiext.LabelPodMutatingUpdate)
	return nil
}

func injectQoSGroupID(pod *corev1.Pod, groupID string) error {
	if groupID == "" {
		return nil
	}

	str, ok := pod.Annotations[terwaytypes.PodNetworksAnnotation]
	if !ok {
		return nil
	}

	jsonObj := make(map[string]interface{})
	if err := json.Unmarshal([]byte(str), &jsonObj); err != nil {
		return fmt.Errorf("failed to unmarshal %s: %v", str, err)
	}
	if podNetworks, ok := jsonObj["podNetworks"]; ok {
		if podNetworksSlice, ok := podNetworks.([]interface{}); ok {
			for i := range podNetworksSlice {
				if network, ok := podNetworksSlice[i].(map[string]interface{}); ok {
					network["eniQosGroupID"] = groupID
				}
			}
		}
	}

	pod.Annotations[terwaytypes.PodNetworksAnnotation] = util.DumpJSON(jsonObj)
	return nil
}
