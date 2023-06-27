package fieldindex

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

func init() {
	indexDescriptors = append(indexDescriptors,
		fieldIndexDescriptor{
			description: "index reservation by status.nodeName",
			obj:         &schedulingv1alpha1.Reservation{},
			field:       "status.nodeName",
			indexerFunc: func(obj client.Object) []string {
				reservation, ok := obj.(*schedulingv1alpha1.Reservation)
				if !ok {
					return []string{}
				}
				if len(reservation.Status.NodeName) == 0 {
					return []string{}
				}
				return []string{reservation.Status.NodeName}
			},
		},
	)
}
