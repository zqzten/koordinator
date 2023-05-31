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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

func TestAllocateCachedPods(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	queueReq := make(chan *corev1.Pod, 100)
	server := NewServer(pointer.Bool(true), func(pod *corev1.Pod) error {
		queueReq <- pod
		return nil
	}, nil)

	requestID := "123456"
	results := map[string]scheduleResult{
		fakePodName(requestID, 0): {
			cachedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cached-pod-1",
					Namespace: "default",
				},
			},
		},
		fakePodName(requestID, 1): {
			err: fmt.Errorf("fake error"),
		},
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pod := <-queueReq:
				r := results[pod.Name]
				r.requestPod = pod
				server.completeRequest(pod, r.cachedPod, r.err)
			}
		}
	}()

	resp, err := server.AllocateCachedPods(context.TODO(), &cachepodapis.AllocateCachedPodsRequest{
		RequestId: requestID,
		Template: &corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					apiext.AnnotationReservationAffinity: "{}",
				},
			},
		},
		Replicas: 2,
		Timeout:  nil,
	})
	assert.NoError(t, err)
	assert.Len(t, resp.Pods, 1)
	assert.Equal(t, results[fakePodName(requestID, 0)].cachedPod, resp.Pods[0])
}

func TestAllocateCachedPodsWithClientCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	cancel()

	var queuedPods []*corev1.Pod
	var canceledPods []*corev1.Pod
	server := NewServer(pointer.Bool(true), func(pod *corev1.Pod) error {
		queuedPods = append(queuedPods, pod)
		return nil
	}, func(pod *corev1.Pod) error {
		canceledPods = append(canceledPods, pod)
		return nil
	})

	requestID := "123456"
	resp, err := server.AllocateCachedPods(ctx, &cachepodapis.AllocateCachedPodsRequest{
		RequestId: requestID,
		Template: &corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					apiext.AnnotationReservationAffinity: "{}",
				},
			},
		},
		Replicas: 2,
		Timeout:  nil,
	})
	assert.Error(t, err)
	statusErr, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.DeadlineExceeded, statusErr.Code())
	assert.Nil(t, resp)
	assert.Equal(t, len(queuedPods), len(canceledPods))
}
