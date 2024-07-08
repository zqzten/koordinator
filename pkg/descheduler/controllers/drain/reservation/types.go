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

package reservation

import (
	"context"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultCreator = "koord-descheduler"
	LabelCreatedBy = "app.kubernetes.io/created-by"
)

var NewInterpreter = newInterpreter

type Interpreter interface {
	GetReservationType() client.Object
	ToReservation(interface{}) Object
	CreateReservation(ctx context.Context, drainNode *sev1alpha1.DrainNode, pod *corev1.Pod) (Object, error)
	GetReservation(ctx context.Context, drainNode string, podUID types.UID) (Object, error)
	ListReservation(ctx context.Context, drainNode string) ([]Object, error)
	DeleteReservations(ctx context.Context, drainNode string) error
	DeleteReservation(ctx context.Context, reservationName string) error
}

type Object interface {
	metav1.Object
	runtime.Object
	String() string
	OriginObject() client.Object
	GetReservationCondition(conditionType sev1alpha1.ReservationConditionType) *sev1alpha1.ReservationCondition
	GetUnschedulableCondition() *sev1alpha1.ReservationCondition
	GetScheduledNodeName() string
	IsPending() bool
	IsScheduled() bool
	IsAllocated() bool
	GetPhase() string
}
