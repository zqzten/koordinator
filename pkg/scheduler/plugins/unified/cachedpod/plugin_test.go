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

package cachedpod

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	cachepodapis "gitlab.alibaba-inc.com/cache/api/pb/generated"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/hijack"
)

type fakeServer struct {
	cachepodapis.UnimplementedCacheSchedulerServer
	completeRequestPod *corev1.Pod
	cachedPod          *corev1.Pod
	err                error
}

func (s *fakeServer) AllocateCachedPods(ctx context.Context, req *cachepodapis.AllocateCachedPodsRequest) (*cachepodapis.AllocateCachedPodsResponse, error) {
	return nil, nil
}

func (s *fakeServer) completeRequest(requestPod, cachedPod *corev1.Pod, err error) {
	s.completeRequestPod = requestPod
	s.cachedPod = cachedPod
	s.err = err
}

func TestErrorHandler(t *testing.T) {
	fs := &fakeServer{}
	pl := &Plugin{
		server: fs,
	}
	requestPod := NewFakePod("123456", &corev1.PodTemplateSpec{}, 0)
	queuedPodInfo := &framework.QueuedPodInfo{
		PodInfo: framework.NewPodInfo(requestPod),
	}
	err := fmt.Errorf("fake error")
	assert.True(t, pl.ErrorHandler(queuedPodInfo, err))

	assert.Equal(t, requestPod, fs.completeRequestPod)
	assert.Nil(t, fs.cachedPod)
	assert.Equal(t, err, fs.err)

	assert.False(t, pl.ErrorHandler(&framework.QueuedPodInfo{PodInfo: framework.NewPodInfo(&corev1.Pod{})}, err))
}

func TestPostFilter(t *testing.T) {
	pl := &Plugin{}
	_, status := pl.PostFilter(context.TODO(), framework.NewCycleState(), &corev1.Pod{}, nil)
	assert.Equal(t, framework.NewStatus(framework.Unschedulable), status)

	requestPod := NewFakePod("123456", &corev1.PodTemplateSpec{}, 0)
	_, status = pl.PostFilter(context.TODO(), framework.NewCycleState(), requestPod, nil)
	assert.True(t, status.Code() == framework.Error)
}

func TestReserve(t *testing.T) {
	pl := &Plugin{}
	status := pl.Reserve(context.TODO(), framework.NewCycleState(), &corev1.Pod{}, "")
	assert.True(t, status.IsSuccess())

	// no nominated reservation
	cycleState := framework.NewCycleState()
	requestPod := NewFakePod("123456", &corev1.PodTemplateSpec{}, 0)
	status = pl.Reserve(context.TODO(), cycleState, requestPod, "")
	assert.False(t, status.IsSuccess())

	// nominated reservation is reservation
	frameworkext.SetNominatedReservation(cycleState, map[string]*frameworkext.ReservationInfo{
		"test-node": frameworkext.NewReservationInfo(&schedulingv1alpha1.Reservation{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-r",
			},
			Spec: schedulingv1alpha1.ReservationSpec{
				Template: &corev1.PodTemplateSpec{},
			},
		}),
	})
	status = pl.Reserve(context.TODO(), cycleState, requestPod, "test-node")
	assert.False(t, status.IsSuccess())

	targetPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Labels: map[string]string{
				apiext.LabelPodOperatingMode: string(apiext.ReservationPodOperatingMode),
			},
		},
	}
	frameworkext.SetNominatedReservation(cycleState, map[string]*frameworkext.ReservationInfo{
		"test-node": frameworkext.NewReservationInfoFromPod(targetPod),
	})
	status = pl.Reserve(context.TODO(), cycleState, requestPod, "test-node")
	targetPodInState := hijack.GetTargetPod(cycleState)
	assert.Equal(t, targetPod, targetPodInState)
	assert.True(t, status.IsSuccess())
}

func TestBind(t *testing.T) {
	fs := &fakeServer{}
	pl := &Plugin{
		server: fs,
	}
	status := pl.Bind(context.TODO(), framework.NewCycleState(), &corev1.Pod{}, "nodeName")
	assert.True(t, status.Code() == framework.Skip)

	// no nominated reservation
	cycleState := framework.NewCycleState()
	requestPod := NewFakePod("123456", &corev1.PodTemplateSpec{}, 0)
	assert.Equal(t, corev1.NamespaceDefault, requestPod.Namespace)
	assert.Equal(t, corev1.DefaultSchedulerName, requestPod.Spec.SchedulerName)
	assert.NotNil(t, requestPod.Spec.Priority)
	status = pl.Bind(context.TODO(), cycleState, requestPod, "")
	assert.False(t, status.IsSuccess())

	// nominated reservation is reservation
	frameworkext.SetNominatedReservation(cycleState, map[string]*frameworkext.ReservationInfo{
		"test-node": frameworkext.NewReservationInfo(&schedulingv1alpha1.Reservation{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-r",
			},
			Spec: schedulingv1alpha1.ReservationSpec{
				Template: &corev1.PodTemplateSpec{},
			},
		}),
	})
	status = pl.Bind(context.TODO(), cycleState, requestPod, "test-node")
	assert.False(t, status.IsSuccess())

	cachedPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Labels: map[string]string{
				apiext.LabelPodOperatingMode: string(apiext.ReservationPodOperatingMode),
			},
		},
	}
	frameworkext.SetNominatedReservation(cycleState, map[string]*frameworkext.ReservationInfo{
		"test-node": frameworkext.NewReservationInfoFromPod(cachedPod),
	})
	status = pl.Bind(context.TODO(), cycleState, requestPod, "test-node")
	assert.True(t, status.IsSuccess())

	assert.Equal(t, requestPod, fs.completeRequestPod)
	assert.Equal(t, cachedPod, fs.cachedPod)
	assert.Nil(t, fs.err)
}
