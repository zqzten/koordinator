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

package utils

import (
	"context"
	"reflect"
	"testing"

	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newNode(labels map[string]string, taint bool, condition bool) *v1.Node {
	n := &v1.Node{}
	n.Name = "test"
	n.ResourceVersion = "999"
	n.Labels = labels
	if taint {
		n.Spec.Taints = []v1.Taint{
			{
				Key:    GroupKey,
				Effect: v1.TaintEffectNoSchedule,
				Value:  "test",
			},
		}
	}
	if condition {
		n.Status.Conditions = []v1.NodeCondition{
			{
				Type:   v1.NodeReady,
				Status: v1.ConditionTrue,
			},
		}
	}
	return n
}

func TestPatch(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	type fields struct {
		objs []runtime.Object
	}
	type args struct {
		obj   client.Object
		funcs []PatchFunc
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
		wantObj client.Object
	}{
		{
			name: "success",
			fields: fields{
				objs: []runtime.Object{newNode(nil, false, false)},
			},
			args: args{
				obj: newNode(nil, false, false),
				funcs: []PatchFunc{
					func(o client.Object) client.Object {
						n := o.(*v1.Node)
						n.Labels = map[string]string{GroupKey: "test"}
						n.Spec.Taints = []v1.Taint{
							{
								Key:    GroupKey,
								Effect: v1.TaintEffectNoSchedule,
								Value:  "test",
							},
						}
						return n
					},
				},
			},
			want:    true,
			wantErr: false,
			wantObj: newNode(map[string]string{GroupKey: "test"}, true, false),
		}, {
			name: "ignore",
			fields: fields{
				objs: []runtime.Object{newNode(nil, false, false)},
			},
			args: args{
				obj: newNode(nil, false, false),
				funcs: []PatchFunc{
					func(o client.Object) client.Object {
						return o
					},
				},
			},
			want:    false,
			wantErr: false,
			wantObj: newNode(nil, false, false),
		}, {
			name: "error",
			fields: fields{
				objs: []runtime.Object{},
			},
			args: args{
				obj: newNode(nil, false, false),
				funcs: []PatchFunc{
					func(o client.Object) client.Object {
						n := o.(*v1.Node)
						n.Labels = map[string]string{GroupKey: "test"}
						n.Spec.Taints = []v1.Taint{
							{
								Key:    GroupKey,
								Effect: v1.TaintEffectNoSchedule,
								Value:  "test",
							},
						}
						return n
					},
				},
			},
			want:    false,
			wantErr: true,
			wantObj: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.objs...).Build()
			got, err := Patch(c, tt.args.obj, tt.args.funcs...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Patch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Patch() = %v, want %v", got, tt.want)
			}
			if tt.wantObj != nil {
				wantObj := tt.wantObj.DeepCopyObject().(client.Object)
				gotObj := tt.wantObj.DeepCopyObject().(client.Object)
				if err := c.Get(context.Background(), types.NamespacedName{Name: tt.wantObj.GetName()}, gotObj); err != nil {
					t.Errorf("Patch Obj() error = %v", err)
				}
				wantObj.GetObjectKind().SetGroupVersionKind(gotObj.GetObjectKind().GroupVersionKind())
				gotObj.SetResourceVersion(tt.wantObj.GetResourceVersion())
				if !reflect.DeepEqual(gotObj, wantObj) {
					t.Errorf("Patch() = %v, want %v", gotObj, wantObj)
				}
			}
		})
	}
}

func TestPatchStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	type fields struct {
		objs []runtime.Object
	}
	type args struct {
		obj   client.Object
		funcs []PatchFunc
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
		wantObj client.Object
	}{
		{
			name: "success",
			fields: fields{
				objs: []runtime.Object{newNode(nil, false, false)},
			},
			args: args{
				obj: newNode(nil, false, false),
				funcs: []PatchFunc{
					func(o client.Object) client.Object {
						n := o.(*v1.Node)
						n.Status.Conditions = []v1.NodeCondition{
							{
								Type:   v1.NodeReady,
								Status: v1.ConditionTrue,
							},
						}
						return n
					},
				},
			},
			want:    true,
			wantErr: false,
			wantObj: newNode(nil, false, true),
		}, {
			name: "ignore",
			fields: fields{
				objs: []runtime.Object{newNode(nil, false, false)},
			},
			args: args{
				obj: newNode(nil, false, false),
				funcs: []PatchFunc{
					func(o client.Object) client.Object {
						return o
					},
				},
			},
			want:    false,
			wantErr: false,
			wantObj: newNode(nil, false, false),
		}, {
			name: "error",
			fields: fields{
				objs: []runtime.Object{},
			},
			args: args{
				obj: newNode(nil, false, false),
				funcs: []PatchFunc{
					func(o client.Object) client.Object {
						n := o.(*v1.Node)
						n.Status.Conditions = []v1.NodeCondition{
							{
								Type:   v1.NodeReady,
								Status: v1.ConditionTrue,
							},
						}
						return n
					},
				},
			},
			want:    false,
			wantErr: true,
			wantObj: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.objs...).Build()
			got, err := PatchStatus(c, tt.args.obj, tt.args.funcs...)
			if (err != nil) != tt.wantErr {
				t.Errorf("PatchStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("PatchStatus() = %v, want %v", got, tt.want)
			}
			if tt.wantObj != nil {
				wantObj := tt.wantObj.DeepCopyObject().(client.Object)
				gotObj := tt.wantObj.DeepCopyObject().(client.Object)
				if err := c.Get(context.Background(), types.NamespacedName{Name: tt.wantObj.GetName()}, gotObj); err != nil {
					t.Errorf("PatchStatus Obj() error = %v", err)
				}
				wantObj.GetObjectKind().SetGroupVersionKind(gotObj.GetObjectKind().GroupVersionKind())
				gotObj.SetResourceVersion(tt.wantObj.GetResourceVersion())
				if !reflect.DeepEqual(gotObj, wantObj) {
					t.Errorf("PatchStatus() = %v, want %v", gotObj, wantObj)
				}
			}
		})
	}
}

func TestToggleDrainNodeState(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	type fields struct {
		objs []runtime.Object
	}
	type args struct {
		dn     *v1alpha1.DrainNode
		phase  v1alpha1.DrainNodePhase
		status []v1alpha1.PodMigration
		msg    string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
		wantObj *v1alpha1.DrainNode
	}{
		{
			name: "patch success",
			fields: fields{
				objs: []runtime.Object{
					&v1alpha1.DrainNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: "1",
						},
					},
				},
			},
			args: args{
				dn: &v1alpha1.DrainNode{
					ObjectMeta: metav1.ObjectMeta{
						Name: "1",
					},
				},
				phase: v1alpha1.DrainNodePhaseAborted,
				status: []v1alpha1.PodMigration{
					{Namespace: "1", PodName: "b"}, {Namespace: "2", PodName: "a"}, {Namespace: "1", PodName: "a"},
				},
				msg: "test",
			},
			wantErr: false,
			wantObj: &v1alpha1.DrainNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: "1",
				},
				Status: v1alpha1.DrainNodeStatus{
					Phase: v1alpha1.DrainNodePhaseAborted,
					PodMigrations: []v1alpha1.PodMigration{
						{Namespace: "1", PodName: "a"}, {Namespace: "1", PodName: "b"}, {Namespace: "2", PodName: "a"},
					},
					Conditions: nil,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.objs...).Build()
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "patch_test"})
			eventRecorder := record.NewEventRecorderAdapter(recorder)
			err := ToggleDrainNodeState(c, eventRecorder, tt.args.dn, &v1alpha1.DrainNodeStatus{
				Phase:               tt.args.phase,
				PodMigrations:       tt.args.status,
				PodMigrationSummary: nil,
				Conditions:          tt.args.dn.Status.Conditions,
			}, tt.args.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToggleDrainNodeState() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantObj != nil {
				wantObj := tt.wantObj.DeepCopyObject().(client.Object)
				gotObj := tt.wantObj.DeepCopyObject().(client.Object)
				if err := c.Get(context.Background(), types.NamespacedName{Name: tt.wantObj.GetName()}, gotObj); err != nil {
					t.Errorf("PatchStatus Obj() error = %v", err)
				}
				wantObj.GetObjectKind().SetGroupVersionKind(gotObj.GetObjectKind().GroupVersionKind())
				gotObj.SetResourceVersion(tt.wantObj.GetResourceVersion())
				if !reflect.DeepEqual(gotObj, wantObj) {
					t.Errorf("PatchStatus() = %v, want %v", gotObj, wantObj)
				}
			}
		})
	}
}
