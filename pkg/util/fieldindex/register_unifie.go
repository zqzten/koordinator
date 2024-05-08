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
