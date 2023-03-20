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

package cache

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/reservation"
)

func generateReservation(rName, nName string, phase sev1alpha1.ReservationPhase, allocated bool) *sev1alpha1.Reservation {
	r := &sev1alpha1.Reservation{}
	r.Name = rName
	r.Status.NodeName = nName
	r.Status.Phase = phase
	if allocated {
		r.Status.CurrentOwners = []corev1.ObjectReference{{}}
	}
	return r
}

func Test_drainNodeCache_OnReservationAdd(t *testing.T) {
	type fields struct {
		info *NodeInfo
	}
	type args struct {
		obj interface{}
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *NodeInfo
	}{
		{
			name: "scheduled reservation, add to cache",
			fields: fields{
				info: &NodeInfo{
					Name:        "node1",
					Drainable:   true,
					Pods:        map[types.UID]*PodInfo{},
					Reservation: map[string]struct{}{},
				},
			},
			args: args{
				obj: generateReservation("test-r", "node1", sev1alpha1.ReservationAvailable, false),
			},
			want: &NodeInfo{
				Name:      "node1",
				Drainable: false,
				Pods:      map[types.UID]*PodInfo{},
				Reservation: map[string]struct{}{
					"test-r": {},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &drainNodeCache{
				nodes:                  map[string]*NodeInfo{},
				reservationInterpreter: reservation.NewInterpreter(nil, nil),
			}
			if tt.fields.info != nil {
				c.nodes[tt.fields.info.Name] = tt.fields.info
			}
			c.OnReservationAdd(tt.args.obj)
			if tt.want != nil {
				got := c.nodes[tt.want.Name]
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("OnReservationAdd() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func Test_drainNodeCache_OnReservationDelete(t *testing.T) {
	type fields struct {
		info *NodeInfo
	}
	type args struct {
		obj interface{}
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *NodeInfo
	}{
		{
			name: "scheduled reservation, delete from cache",
			fields: fields{
				info: &NodeInfo{
					Name:      "node1",
					Drainable: false,
					Pods:      map[types.UID]*PodInfo{},
					Reservation: map[string]struct{}{
						"test-r": {},
					},
				},
			},
			args: args{
				obj: generateReservation("test-r", "node1", sev1alpha1.ReservationAvailable, false),
			},
			want: &NodeInfo{
				Name:        "node1",
				Drainable:   true,
				Pods:        map[types.UID]*PodInfo{},
				Reservation: map[string]struct{}{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &drainNodeCache{
				nodes:                  map[string]*NodeInfo{},
				reservationInterpreter: reservation.NewInterpreter(nil, nil),
			}
			if tt.fields.info != nil {
				c.nodes[tt.fields.info.Name] = tt.fields.info
			}
			c.OnReservationDelete(tt.args.obj)
			if tt.want != nil {
				got := c.nodes[tt.want.Name]
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("OnReservationDelete() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func Test_drainNodeCache_OnReservationUpdate(t *testing.T) {
	type fields struct {
		info *NodeInfo
	}
	type args struct {
		oldObj interface{}
		newObj interface{}
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *NodeInfo
	}{
		{
			name: "reservation update scheduled",
			fields: fields{
				info: &NodeInfo{
					Name:        "node1",
					Drainable:   true,
					Pods:        map[types.UID]*PodInfo{},
					Reservation: map[string]struct{}{},
				},
			},
			args: args{
				oldObj: generateReservation("test-r", "", sev1alpha1.ReservationPending, false),
				newObj: generateReservation("test-r", "node1", sev1alpha1.ReservationAvailable, false),
			},
			want: &NodeInfo{
				Name:      "node1",
				Drainable: false,
				Pods:      map[types.UID]*PodInfo{},
				Reservation: map[string]struct{}{
					"test-r": {},
				},
			},
		}, {
			name: "reservation update allocated",
			fields: fields{
				info: &NodeInfo{
					Name:      "node1",
					Drainable: false,
					Pods:      map[types.UID]*PodInfo{},
					Reservation: map[string]struct{}{
						"test-r": {},
					},
				},
			},
			args: args{
				oldObj: generateReservation("test-r", "node1", sev1alpha1.ReservationAvailable, false),
				newObj: generateReservation("test-r", "node1", sev1alpha1.ReservationSucceeded, true),
			},
			want: &NodeInfo{
				Name:        "node1",
				Drainable:   true,
				Pods:        map[types.UID]*PodInfo{},
				Reservation: map[string]struct{}{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &drainNodeCache{
				nodes:                  map[string]*NodeInfo{},
				reservationInterpreter: reservation.NewInterpreter(nil, nil),
			}
			if tt.fields.info != nil {
				c.nodes[tt.fields.info.Name] = tt.fields.info
			}
			c.OnReservationUpdate(tt.args.oldObj, tt.args.newObj)
			if tt.want != nil {
				got := c.nodes[tt.want.Name]
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("OnReservationAdd() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
