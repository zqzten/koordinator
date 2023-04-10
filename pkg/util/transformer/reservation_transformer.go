package transformer

import (
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

var reservationTransformers = []func(reservation *schedulingv1alpha1.Reservation){
	TransformSigmaIgnoreResourceContainersForReservation,
	TransformNonProdPodResourceSpecForReservation,
}

func InstallReservationTransformer(reservationInformer cache.SharedIndexInformer) {
	transformerSetter, ok := reservationInformer.(cache.TransformerSetter)
	if !ok {
		klog.Fatalf("cache.TransformerSetter is not implemented")
	}
	transformerSetter.SetTransform(func(obj interface{}) (interface{}, error) {
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
	})
}

func TransformSigmaIgnoreResourceContainersForReservation(reservation *schedulingv1alpha1.Reservation) {
	transformSigmaIgnoreResourceContainers(&reservation.Spec.Template.Spec)
}

func TransformNonProdPodResourceSpecForReservation(reservation *schedulingv1alpha1.Reservation) {
	transformNonProdPodResourceSpec(&reservation.Spec.Template.Spec)
}
