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

package extension

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

const (
	// LabelReservationOrder controls the preference logic for Reservation.
	// Reservation with lower order is preferred to be selected before Reservation with higher order.
	// But if it is 0, Reservation will be selected according to the capacity score.
	LabelReservationOrder = SchedulingDomainPrefix + "/reservation-order"

	// AnnotationReservationAllocated represents the reservation allocated by the pod.
	AnnotationReservationAllocated = SchedulingDomainPrefix + "/reservation-allocated"

	// AnnotationReservationAffinity represents the constraints of Pod selection Reservation
	AnnotationReservationAffinity = SchedulingDomainPrefix + "/reservation-affinity"

	// AnnotationReservationRestrictedOptions represent the Reservation Restricted options
	AnnotationReservationRestrictedOptions = SchedulingDomainPrefix + "/reservation-restricted-options"

	// AnnotationReservationTemplate includes the specification for a potential Reservation CRD
	AnnotationReservationTemplate = SchedulingDomainPrefix + "/reservation-template"

	// AnnotationReservationRequirement triggers an external provisioner to create an actual Reservation CRD instance based on the provided template.
	AnnotationReservationRequirement = SchedulingDomainPrefix + "/reservation-requirement"

	// LabelReservationID represents reservation's custom id.
	// When the label exists, the id will be used as UID first, otherwise the UID in reservation's ObjectMeta will be used.
	LabelReservationID = SchedulingDomainPrefix + "/reservation-id"
)

type ReservationAllocated struct {
	Name string    `json:"name,omitempty"`
	UID  types.UID `json:"uid,omitempty"`
}

// ReservationAffinity represents the constraints of Pod selection Reservation
type ReservationAffinity struct {
	// If the affinity requirements specified by this field are not met at
	// scheduling time, the pod will not be scheduled onto the node.
	// If the affinity requirements specified by this field cease to be met
	// at some point during pod execution (e.g. due to an update), the system
	// may or may not try to eventually evict the pod from its node.
	RequiredDuringSchedulingIgnoredDuringExecution *ReservationAffinitySelector `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`
	// ReservationSelector is a selector which must be true for the pod to fit on a reservation.
	// Selector which must match a reservation's labels for the pod to be scheduled on that node.
	ReservationSelector map[string]string `json:"reservationSelector,omitempty"`
}

// ReservationAffinitySelector represents the union of the results of one or more label queries
// over a set of reservations; that is, it represents the OR of the selectors represented
// by the reservation selector terms.
type ReservationAffinitySelector struct {
	// Required. A list of reservation selector terms. The terms are ORed.
	// Reuse corev1.NodeSelectorTerm to avoid defining too many repeated definitions.
	ReservationSelectorTerms []corev1.NodeSelectorTerm `json:"reservationSelectorTerms,omitempty"`
}

type ReservationRestrictedOptions struct {
	// Resources means that when the Pod intersects with these resources,
	// it can only allocate the reserved amount at most.
	// If the Reservation's AllocatePolicy is Restricted, and no resources configured,
	// by default the resources equal all reserved resources by the Reservation.
	Resources []corev1.ResourceName `json:"resources,omitempty"`
}

// ReservationTemplate includes the specification for a potential Reservation CRD
type ReservationTemplate struct {
	// Labels defines the reservation must be declared
	Labels map[string]string `json:"labels,omitempty"`
	// Owners represents who can use the reservation
	Owners []schedulingv1alpha1.ReservationOwner `json:"owners"`
	// RecommendedSize represents the desired maximum specification for a Reservation, but it is not mandatory.
	RecommendedSize corev1.ResourceList `json:"recommendedSize,omitempty"`
}

// ReservationRequirement triggers an external provisioner to create an actual Reservation CRD instance based on the provided template.
type ReservationRequirement struct {
	Labels map[string]string `json:"labels"`
	// Resources represents the minimum reserved resource specification for a Reservation
	Resources corev1.ResourceList `json:"resources"`
}

func GetReservationAllocated(pod *corev1.Pod) (*ReservationAllocated, error) {
	if pod.Annotations == nil {
		return nil, nil
	}
	data, ok := pod.Annotations[AnnotationReservationAllocated]
	if !ok {
		return nil, nil
	}
	reservationAllocated := &ReservationAllocated{}
	err := json.Unmarshal([]byte(data), reservationAllocated)
	if err != nil {
		return nil, err
	}
	return reservationAllocated, nil
}

func SetReservationAllocated(pod *corev1.Pod, reservationName string, reservationUID types.UID) {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	reservationAllocated := &ReservationAllocated{
		Name: reservationName,
		UID:  reservationUID,
	}
	data, _ := json.Marshal(reservationAllocated) // assert no error
	pod.Annotations[AnnotationReservationAllocated] = string(data)
}

func IsReservationAllocateOnce(r *schedulingv1alpha1.Reservation) bool {
	return pointer.BoolDeref(r.Spec.AllocateOnce, true)
}

func GetReservationAffinity(annotations map[string]string) (*ReservationAffinity, error) {
	var affinity ReservationAffinity
	if s := annotations[AnnotationReservationAffinity]; s != "" {
		if err := json.Unmarshal([]byte(s), &affinity); err != nil {
			return nil, err
		}
	}
	return &affinity, nil
}

func SetReservationAffinity(obj metav1.Object, affinity *ReservationAffinity) error {
	data, err := json.Marshal(affinity)
	if err != nil {
		return err
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[AnnotationReservationAffinity] = string(data)
	obj.SetAnnotations(annotations)
	return nil
}

func GetReservationRestrictedOptions(annotations map[string]string) (*ReservationRestrictedOptions, error) {
	var options ReservationRestrictedOptions
	if s, ok := annotations[AnnotationReservationRestrictedOptions]; ok && s != "" {
		if err := json.Unmarshal([]byte(s), &options); err != nil {
			return nil, err
		}
	}
	return &options, nil
}

func SetReservationRestrictedOptions(obj metav1.Object, options *ReservationRestrictedOptions) error {
	data, err := json.Marshal(options)
	if err != nil {
		return err
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[AnnotationReservationRestrictedOptions] = string(data)
	obj.SetAnnotations(annotations)
	return nil
}

func GetReservationTemplate(annotations map[string]string) (*ReservationTemplate, error) {
	if s := annotations[AnnotationReservationTemplate]; s != "" {
		var template ReservationTemplate
		if err := json.Unmarshal([]byte(s), &template); err != nil {
			return nil, err
		}
		return &template, nil
	}
	return nil, nil
}

func SetReservationTemplate(obj metav1.Object, template *ReservationTemplate) error {
	data, err := json.Marshal(template)
	if err != nil {
		return err
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[AnnotationReservationTemplate] = string(data)
	obj.SetAnnotations(annotations)
	return nil
}

func GetReservationRequirement(annotations map[string]string) (*ReservationRequirement, error) {
	var requirement ReservationRequirement
	if s := annotations[AnnotationReservationRequirement]; s != "" {
		if err := json.Unmarshal([]byte(s), &requirement); err != nil {
			return nil, err
		}
	}
	return &requirement, nil
}

func SetReservationRequirement(obj metav1.Object, requirement *ReservationRequirement) error {
	data, err := json.Marshal(requirement)
	if err != nil {
		return err
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[AnnotationReservationRequirement] = string(data)
	obj.SetAnnotations(annotations)
	return nil
}

func GetReservationID(obj metav1.Object) types.UID {
	if obj == nil {
		return ""
	}
	reservationID := obj.GetLabels()[LabelReservationID]
	if reservationID != "" {
		return types.UID(reservationID)
	}
	return obj.GetUID()
}
