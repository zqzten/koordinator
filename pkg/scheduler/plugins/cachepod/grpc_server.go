package cachepod

import (
	"context"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/koordinator-sh/koordinator/apis/cachepod"
)

type CachePodServer struct {
	// internalHandler eventhandlers.SchedulerInternalHandler
}

func NewServer() *CachePodServer {
	return &CachePodServer{}
}

func (c *CachePodServer) AllocateCachedPods(ctx context.Context, req *cachepod.AllocateCachedPodsRequest) (*cachepod.AllocateCachedPodsResponse, error) {
	waitChan := make(chan *corev1.Pod, req.Replicas)
	AddWaitChan(req.RequestID, waitChan)
	// FIXME: how to add fakepod into scheduler's queue
	for i := 0; i < int(req.Replicas); i++ {
		// c.internalHandler.GetQueue().Add(NewFakePod(req.Template))
	}
	defer RemoveWaitChan(req.RequestID)
	timeout := time.After(req.Timeout.Duration)
	var resPod []*corev1.Pod
	exit := false
	for {
		select {
		case <-ctx.Done():
			exit = true
			klog.Infof("client canceled, request id: %s", req.RequestID)
		case <-timeout:
			exit = true
			klog.Infof("timeout of request %s", req.RequestID)
		case pod := <-waitChan:
			resPod = append(resPod, pod)
		}
		if len(resPod) == int(req.Replicas) || exit {
			break
		}
	}
	return &cachepod.AllocateCachedPodsResponse{RequestID: req.RequestID, Pods: resPod}, nil
}

func NewFakePod(template *corev1.PodTemplateSpec) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        uuid.New().String(),
			Namespace:   uuid.New().String(),
			Annotations: template.Annotations,
			Labels:      template.Labels,
		},
		Spec: *template.Spec.DeepCopy(),
	}
}
