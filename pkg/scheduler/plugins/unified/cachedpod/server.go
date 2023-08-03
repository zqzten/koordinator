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
	"sync"
	"time"

	cachepodapis "gitlab.alibaba-inc.com/cache/api/pb/generated"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	AnnotationAllocateCachedPodRequestID = apiext.SchedulingDomainPrefix + "/allocate-cachedpod-request-id"
	AnnotationPodHijackable              = apiext.SchedulingDomainPrefix + "/hijackable"
)

const (
	defaultTimeout = 1 * time.Second
	maxTimeout     = 30 * time.Second
)

type addPodInQueueFn func(pod *corev1.Pod) error
type deletePodFromQueueFn func(pod *corev1.Pod) error

type allocatorServerExt interface {
	cachepodapis.CacheSchedulerServer
	completeRequest(requestPod, cachedPod *corev1.Pod, err error)
}

type AllocatorServer struct {
	cachepodapis.UnimplementedCacheSchedulerServer
	isLeader           *bool
	addPodInQueue      addPodInQueueFn
	deletePodFromQueue deletePodFromQueueFn

	lock     sync.RWMutex
	waitChan map[string]chan scheduleResult
}

func NewServer(isLeader *bool, addInQFn addPodInQueueFn, deleteFromQFn deletePodFromQueueFn) *AllocatorServer {
	return &AllocatorServer{
		isLeader:           isLeader,
		addPodInQueue:      addInQFn,
		deletePodFromQueue: deleteFromQFn,
		waitChan:           map[string]chan scheduleResult{},
	}
}

func (srv *AllocatorServer) AllocateCachedPods(ctx context.Context, req *cachepodapis.AllocateCachedPodsRequest) (*cachepodapis.AllocateCachedPodsResponse, error) {
	if !*srv.isLeader {
		return nil, status.Error(codes.Unavailable, "the instance is not leader")
	}

	if err := validateAllocateCachedPodsRequest(req); err != nil {
		klog.ErrorS(err, "invalid request", "request", req.RequestId)
		return nil, err
	}

	if req.Replicas == 0 {
		req.Replicas = 1
	}
	klog.V(4).InfoS("Try add request pods into scheduling queue", "request", req.RequestId, "replicas", req.Replicas)

	waitChan := make(chan scheduleResult, req.Replicas)
	srv.addWaitChan(req.RequestId, waitChan)
	defer srv.removeWaitChan(req.RequestId)

	var err error
	pods := make([]*corev1.Pod, 0, int(req.Replicas))
	for i := 0; i < int(req.Replicas); i++ {
		fakePod := NewFakePod(req.RequestId, req.Template, i)
		if err = srv.addPodInQueue(fakePod); err != nil {
			break
		}
		pods = append(pods, fakePod)
	}
	if len(pods) == 0 {
		klog.ErrorS(err, "Failed to add request into scheduling queue", "request", req.RequestId)
		return nil, status.Errorf(codes.Internal, "failed to add request into scheduling queue, err: %v", err.Error())
	}
	defer func() {
		// maybe client already canceled the requests, we can abort all pending pods
		if srv.deletePodFromQueue != nil {
			for _, pod := range pods {
				_ = srv.deletePodFromQueue(pod)
			}
		}
	}()

	var timeout time.Duration
	if req.Timeout != nil {
		timeout = req.Timeout.Duration
	}
	if timeout == 0 || timeout > maxTimeout {
		timeout = defaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	response := &cachepodapis.AllocateCachedPodsResponse{RequestId: req.RequestId}
	canceled := false
	for i := 0; i < int(req.Replicas); i++ {
		select {
		case <-ctx.Done():
			canceled = true
			klog.V(4).InfoS("client canceled", "requestID", req.RequestId)

		case result := <-waitChan:
			if result.err == nil && result.cachedPod != nil {
				response.Pods = append(response.Pods, result.cachedPod)
				klog.V(4).InfoS("Successfully allocate cached pod on node",
					"request", req.RequestId, "requestPod", klog.KObj(result.requestPod),
					"cachedPod", klog.KObj(result.cachedPod), "node", result.cachedPod.Spec.NodeName)

			} else if result.err != nil {
				err = result.err
				klog.V(4).InfoS("Failed to scheduling", "request", req.RequestId, "requestPod", klog.KObj(result.requestPod), "err", result.err)
			} else {
				klog.Warningf("something impossible happened, requestPod: %v", "request", req.RequestId, klog.KObj(result.requestPod))
			}
		}
		if canceled {
			break
		}
	}

	if canceled && len(response.Pods) == 0 {
		klog.V(4).InfoS("No Cached Pod was scheduled within the timeout period", "request", req.RequestId)
		return nil, status.Errorf(codes.DeadlineExceeded, "No Cached Pod was scheduled within the timeout period")
	}
	if req.Replicas == 1 && err != nil {
		return nil, status.Errorf(codes.Internal, "Failed scheduling, %v", err)
	}

	return response, nil
}

func validateAllocateCachedPodsRequest(req *cachepodapis.AllocateCachedPodsRequest) error {
	var allErrs field.ErrorList

	if req.RequestId == "" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("requestID"), req.RequestId, "requestID must be set"))
	}
	if req.Template == nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("template"), req.Template, "template must be set"))
	}
	if val := req.Template.Annotations[apiext.AnnotationReservationAffinity]; val == "" {
		fieldPath := field.NewPath("template").Child("metadata").Child("annotations").Key(apiext.AnnotationReservationAffinity)
		allErrs = append(allErrs, field.Invalid(fieldPath, val, "reservation affinity must be set"))
	}
	if len(allErrs) == 0 {
		return nil
	}
	return status.Error(codes.InvalidArgument, allErrs.ToAggregate().Error())
}

type scheduleResult struct {
	requestPod *corev1.Pod
	cachedPod  *corev1.Pod
	err        error
}

func (srv *AllocatorServer) addWaitChan(uuid string, c chan scheduleResult) {
	srv.lock.Lock()
	defer srv.lock.Unlock()
	if _, ok := srv.waitChan[uuid]; ok {
		klog.V(5).Infof("uuid already has a channel, uuid: %s", uuid)
	}
	srv.waitChan[uuid] = c
}

func (srv *AllocatorServer) removeWaitChan(uuid string) {
	srv.lock.Lock()
	defer srv.lock.Unlock()
	delete(srv.waitChan, uuid)
}

func (srv *AllocatorServer) getWaitChan(uuid string) chan scheduleResult {
	srv.lock.RLock()
	defer srv.lock.RUnlock()
	return srv.waitChan[uuid]
}

func (srv *AllocatorServer) completeRequest(requestPod, cachedPod *corev1.Pod, err error) {
	requestID := requestPod.Annotations[AnnotationAllocateCachedPodRequestID]
	if requestID == "" {
		return
	}
	klog.V(4).InfoS("complete one allocate cached pod request",
		"request", requestID, "requestPod", klog.KObj(requestPod), "cachedPod", klog.KObj(cachedPod), "err", err)
	ch := srv.getWaitChan(requestID)
	if ch != nil {
		select {
		case ch <- scheduleResult{
			requestPod: requestPod,
			cachedPod:  cachedPod,
			err:        err,
		}:
		default:
			klog.V(4).InfoS("cannot send the scheduleResult of cached pod request",
				"request", requestID, "requestPod", klog.KObj(requestPod), "cachedPod", klog.KObj(cachedPod))
		}
	}
}

func NewFakePod(requestID string, template *corev1.PodTemplateSpec, index int) *corev1.Pod {
	template = template.DeepCopy()
	uid := uuid.NewUUID()
	fakePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fakePodName(requestID, index),
			UID:         uid,
			Namespace:   template.Namespace,
			Annotations: template.Annotations,
			Labels:      template.Labels,
		},
		Spec: template.Spec,
	}
	if fakePod.Namespace == "" {
		fakePod.Namespace = corev1.NamespaceDefault
	}

	if fakePod.Spec.SchedulerName == "" {
		fakePod.Spec.SchedulerName = corev1.DefaultSchedulerName
	}

	if fakePod.Spec.Priority == nil {
		fakePod.Spec.Priority = pointer.Int32(0)
	}

	if fakePod.Annotations == nil {
		fakePod.Annotations = map[string]string{}
	}
	fakePod.Annotations[AnnotationAllocateCachedPodRequestID] = requestID
	fakePod.Annotations[AnnotationPodHijackable] = "true"
	return fakePod
}

func IsPodFromGRPC(pod *corev1.Pod) bool {
	return pod.Annotations[AnnotationAllocateCachedPodRequestID] != ""
}

func fakePodName(requestID string, index int) string {
	return fmt.Sprintf("find-cache-pod-%s-%d", requestID, index)
}
