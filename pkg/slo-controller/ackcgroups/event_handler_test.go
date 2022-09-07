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

package cgroups

import (
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

func init() {
	_ = resourcesv1alpha1.AddToScheme(scheme.Scheme)
}

func eventQueueEqual(q workqueue.RateLimitingInterface, want []interface{}) (bool, string) {
	if q.Len() != len(want) {
		return false, fmt.Sprintf("queue length not equal got(%v)!=want(%v)", q.Len(), len(want))
	}
	for i := range want {
		obj, _ := q.Get()
		if !reflect.DeepEqual(obj, want[i]) {
			return false, fmt.Sprintf("item %v not equal, detail:\n %+v\n%+v", i, obj, want[i])
		}
	}
	return true, ""
}

func cgroupsListEqual(a, b []*resourcesv1alpha1.Cgroups) (bool, string) {
	if len(a) != len(b) {
		return false, fmt.Sprintf("list length not equal %v!=%v", len(a), len(b))
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false, fmt.Sprintf("item %v not equal, detail:\n %+v\n%+v", i, a[i], b[i])
		}
	}
	return true, ""
}

func podRecycleListEqual(a, b []*podRecycleCandidate) (bool, string) {
	if len(a) != len(b) {
		return false, fmt.Sprintf("list length not equal %v!=%v", len(a), len(b))
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false, fmt.Sprintf("item %v not equal, detail:\n %+v\n%+v", i, a[i], b[i])
		}
	}
	return true, ""
}

func Test_cgroupsEventHandler_Create(t *testing.T) {
	testNs := "test-ns"
	testCgroupName := "test-cgroup-name"
	testCgroups := &resourcesv1alpha1.Cgroups{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testCgroupName,
			Namespace: testNs,
		},
		Spec: resourcesv1alpha1.CgroupsSpec{
			DeploymentInfo: resourcesv1alpha1.DeploymentInfoSpec{
				Name:      "test-dep",
				Namespace: "test-ns",
			},
		},
	}
	testEmptyCgroup := &resourcesv1alpha1.Cgroups{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-empty-cgroup-name",
			Namespace: "test-ns",
		},
	}
	type fields struct {
		objs     []runtime.Object
		recycler *CgroupsInfoRecycler
	}
	type want struct {
		objs []interface{}
	}
	type args struct {
		evt event.CreateEvent
		q   workqueue.RateLimitingInterface
	}
	tests := []struct {
		name   string
		fields fields
		want   want
		args   args
	}{
		{
			name: "cgroups crd create event",
			fields: fields{
				objs: []runtime.Object{
					testCgroups,
				},
			},
			args: args{
				evt: event.CreateEvent{
					Object: client.Object(testCgroups),
				},
				q: nil,
			},
			want: want{
				objs: []interface{}{
					reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testCgroupName}},
				},
			},
		},
		{
			name: "empty cgroups crd create event",
			fields: fields{
				objs: []runtime.Object{
					testEmptyCgroup,
				},
			},
			args: args{
				evt: event.CreateEvent{
					Object: client.Object(testEmptyCgroup),
				},
				q: nil,
			},
			want: want{
				objs: []interface{}{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.fields.objs...)
			fakeClient := clientBuilder.Build()
			c := &cgroupsEventHandler{
				Client:   fakeClient,
				recycler: tt.fields.recycler,
			}
			c.Create(tt.args.evt, q)
			if equal, msg := eventQueueEqual(q, tt.want.objs); !equal {
				t.Errorf("Cgroup event Create() not match, message %v", msg)
			}
		})
	}
}

func Test_cgroupsEventHandler_Update(t *testing.T) {
	testNs := "test-ns"
	testCgroupName := "test-cgroup-name"
	testPodName := "test-pod-name"
	testDepName := "test-dep-name"
	testJobName := "test-job-name"
	testOldCgroups := &resourcesv1alpha1.Cgroups{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testCgroupName,
			Namespace: testNs,
		},
		Spec: resourcesv1alpha1.CgroupsSpec{
			PodInfo: resourcesv1alpha1.PodInfoSpec{
				Name:      testPodName,
				Namespace: testNs,
			},
			DeploymentInfo: resourcesv1alpha1.DeploymentInfoSpec{
				Name:      testDepName,
				Namespace: testNs,
			},
		},
	}
	testNewCgroups := &resourcesv1alpha1.Cgroups{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testCgroupName,
			Namespace: testNs,
		},
		Spec: resourcesv1alpha1.CgroupsSpec{
			DeploymentInfo: resourcesv1alpha1.DeploymentInfoSpec{
				Name:      testDepName,
				Namespace: testNs,
			},
			JobInfo: resourcesv1alpha1.JobInfoSpec{
				Name:      testJobName,
				Namespace: testNs,
			},
		},
	}
	type fields struct {
		objs []runtime.Object
	}
	type want struct {
		objs           []interface{}
		recycleCgroups []*resourcesv1alpha1.Cgroups
	}
	type args struct {
		evt event.UpdateEvent
	}
	tests := []struct {
		name   string
		fields fields
		want   want
		args   args
	}{
		{
			name: "cgroup crd update event",
			fields: fields{
				objs: []runtime.Object{
					testNewCgroups,
				},
			},
			args: args{
				evt: event.UpdateEvent{
					ObjectOld: client.Object(testOldCgroups),
					ObjectNew: client.Object(testNewCgroups),
				},
			},
			want: want{
				objs: []interface{}{
					reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testCgroupName}},
				},
				recycleCgroups: []*resourcesv1alpha1.Cgroups{
					cgroupsSub(testOldCgroups, testNewCgroups),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.fields.objs...)
			fakeClient := clientBuilder.Build()
			recycler := NewCgroupsInfoRecycler(fakeClient)
			c := &cgroupsEventHandler{
				Client:   fakeClient,
				recycler: recycler,
			}
			c.Update(tt.args.evt, q)
			gotRecycleCgroups := recycler.popAllCgroups()
			if equal, msg := eventQueueEqual(q, tt.want.objs); !equal {
				t.Errorf("Cgroup event Update() not match, message %v", msg)
			}
			if equal, msg := cgroupsListEqual(gotRecycleCgroups, tt.want.recycleCgroups); !equal {
				t.Errorf("recyle cgroups after Cgroup event Update() not match, message %v", msg)
			}
		})
	}
}

func Test_cgroupsEventHandler_Delete(t *testing.T) {
	testNs := "test-ns"
	testCgroupName := "test-cgroup-name"
	testCgroups := &resourcesv1alpha1.Cgroups{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testCgroupName,
			Namespace: testNs,
		},
		Spec: resourcesv1alpha1.CgroupsSpec{
			DeploymentInfo: resourcesv1alpha1.DeploymentInfoSpec{
				Name:      "test-dep",
				Namespace: "test-ns",
			},
		},
	}
	type fields struct {
		objs     []runtime.Object
		recycler *CgroupsInfoRecycler
	}
	type want struct {
		objs []interface{}
	}
	type args struct {
		evt event.DeleteEvent
		q   workqueue.RateLimitingInterface
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "cgroups crd delete event",
			fields: fields{
				objs: []runtime.Object{},
			},
			args: args{
				evt: event.DeleteEvent{
					Object: client.Object(testCgroups),
				},
				q: nil,
			},
			want: want{
				objs: []interface{}{
					reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testCgroupName}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.fields.objs...)
			fakeClient := clientBuilder.Build()
			c := &cgroupsEventHandler{
				Client:   fakeClient,
				recycler: tt.fields.recycler,
			}
			c.Delete(tt.args.evt, q)
			if equal, msg := eventQueueEqual(q, tt.want.objs); !equal {
				t.Errorf("Cgroup event Create() not match, message %v", msg)
			}
		})
	}
}

func Test_podEventHandler_Create(t *testing.T) {
	testNs := "test-ns"
	testPodName := "test-pod-name"
	testNodeName := "test-node-name"
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNs,
		},
		Spec: corev1.PodSpec{
			NodeName: testNodeName,
		},
	}
	testPodNoNode := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNs,
		},
	}
	type fields struct {
		objs     []runtime.Object
		recycler *CgroupsInfoRecycler
	}
	type want struct {
		objs []interface{}
	}
	type args struct {
		evt event.CreateEvent
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "pod create event",
			fields: fields{
				objs: []runtime.Object{
					testPod,
				},
			},
			args: args{
				evt: event.CreateEvent{
					Object: client.Object(testPod),
				},
			},
			want: want{
				objs: []interface{}{
					reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testPodName}},
				},
			},
		},
		{
			name: "pod no node create event",
			fields: fields{
				objs: []runtime.Object{
					testPodNoNode,
				},
			},
			args: args{
				evt: event.CreateEvent{
					Object: client.Object(testPodNoNode),
				},
			},
			want: want{
				objs: []interface{}{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.fields.objs...)
			fakeClient := clientBuilder.Build()
			p := &podEventHandler{
				Client:   fakeClient,
				recycler: tt.fields.recycler,
			}
			p.Create(tt.args.evt, q)
			if equal, msg := eventQueueEqual(q, tt.want.objs); !equal {
				t.Errorf("Cgroup event Create() not match, message %v", msg)
			}
		})
	}
}

func Test_podEventHandler_Update(t *testing.T) {
	testNs := "test-ns"
	testPodName := "test-pod-name"
	testOldNodeName := "test-old-node-name"
	testNewNodeName := "test-new-node-name"
	testOldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNs,
		},
		Spec: corev1.PodSpec{
			NodeName: testOldNodeName,
		},
	}
	testNewPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNs,
		},
		Spec: corev1.PodSpec{
			NodeName: testNewNodeName,
		},
	}
	type fields struct {
		objs     []runtime.Object
		recycler *CgroupsInfoRecycler
	}
	type want struct {
		objs           []interface{}
		recycleCgroups []*podRecycleCandidate
	}
	type args struct {
		evt event.UpdateEvent
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "pod update event",
			fields: fields{
				objs: []runtime.Object{
					testNewPod,
				},
			},
			args: args{
				evt: event.UpdateEvent{
					ObjectOld: client.Object(testOldPod),
					ObjectNew: client.Object(testNewPod),
				},
			},
			want: want{
				objs: []interface{}{
					reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testPodName}},
				},
				recycleCgroups: []*podRecycleCandidate{
					{
						namespace: testNs,
						name:      testPodName,
						nodeName:  testOldNodeName,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.fields.objs...)
			fakeClient := clientBuilder.Build()
			recycler := NewCgroupsInfoRecycler(fakeClient)
			p := &podEventHandler{
				Client:   fakeClient,
				recycler: recycler,
			}
			p.Update(tt.args.evt, q)
			gotRecyclePods := recycler.popAllPods()
			if equal, msg := eventQueueEqual(q, tt.want.objs); !equal {
				t.Errorf("Cgroup event Create() not match, message %v", msg)
			}
			if equal, msg := podRecycleListEqual(gotRecyclePods, tt.want.recycleCgroups); !equal {
				t.Errorf("recyle cgroups after Cgroup event Update() not match, message %v", msg)
			}
		})
	}
}

func Test_podEventHandler_Delete(t *testing.T) {
	testNs := "test-ns"
	testPodName := "test-pod-name"
	testNodeName := "test-node-name"
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNs,
		},
		Spec: corev1.PodSpec{
			NodeName: testNodeName,
		},
	}
	testPodNoNode := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNs,
		},
	}
	type fields struct {
		objs []runtime.Object
	}
	type want struct {
		objs           []interface{}
		recycleCgroups []*podRecycleCandidate
	}
	type args struct {
		evt event.DeleteEvent
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "pod delete event",
			fields: fields{
				objs: []runtime.Object{},
			},
			args: args{
				evt: event.DeleteEvent{
					Object: client.Object(testPod),
				},
			},
			want: want{
				objs: []interface{}{},
				recycleCgroups: []*podRecycleCandidate{
					{
						namespace: testNs,
						name:      testPodName,
						nodeName:  testNodeName,
					},
				},
			},
		},
		{
			name: "pod no node delete event",
			fields: fields{
				objs: []runtime.Object{},
			},
			args: args{
				evt: event.DeleteEvent{
					Object: client.Object(testPodNoNode),
				},
			},
			want: want{
				objs:           []interface{}{},
				recycleCgroups: []*podRecycleCandidate{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.fields.objs...)
			fakeClient := clientBuilder.Build()
			recycler := NewCgroupsInfoRecycler(fakeClient)
			p := &podEventHandler{
				Client:   fakeClient,
				recycler: recycler,
			}
			p.Delete(tt.args.evt, q)
			if equal, msg := eventQueueEqual(q, tt.want.objs); !equal {
				t.Errorf("Cgroup event Create() not match, message %v", msg)
			}
		})
	}
}
