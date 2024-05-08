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

package transformer

import (
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func init() {
	transformers[schedulingv1alpha1.SchemeGroupVersion.WithResource("reservations")] = TransformReservation
}

var reservationTransformers = []func(reservation *schedulingv1alpha1.Reservation){
	TransformSigmaIgnoreResourceContainersForReservation,
	TransformNonProdPodResourceSpecForReservation,
	TransformENIResourceForReservation,
}

func InstallReservationTransformer(informer cache.SharedIndexInformer) {
	if err := informer.SetTransform(TransformReservation); err != nil {
		klog.Fatalf("Failed to SetTransform with reservation, err: %v", err)
	}
}

func TransformReservation(obj interface{}) (interface{}, error) {
	var reservation *schedulingv1alpha1.Reservation
	switch t := obj.(type) {
	case *schedulingv1alpha1.Reservation:
		reservation = t
	case cache.DeletedFinalStateUnknown:
		reservation, _ = t.Obj.(*schedulingv1alpha1.Reservation)
	}
	if reservation == nil {
		return obj, nil
	}

	reservation = reservation.DeepCopy()
	for _, fn := range reservationTransformers {
		fn(reservation)
	}

	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		unknown.Obj = reservation
		return unknown, nil
	}
	return reservation, nil
}

func TransformSigmaIgnoreResourceContainersForReservation(reservation *schedulingv1alpha1.Reservation) {
	transformSigmaIgnoreResourceContainers(&reservation.Spec.Template.Spec)
}

func TransformNonProdPodResourceSpecForReservation(reservation *schedulingv1alpha1.Reservation) {
	transformNonProdPodResourceSpec(&reservation.Spec.Template.Spec)
}

func TransformENIResourceForReservation(reservation *schedulingv1alpha1.Reservation) {
	transformENIResource(&reservation.Spec.Template.Spec)
}
