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
	"crypto/tls"
	"log"
	"net"

	cachepodapis "gitlab.alibaba-inc.com/cache/api/pb/generated"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/hijack"
)

const (
	Name = "CachedPod"
)

type Plugin struct {
	isLeader *bool
	server   allocatorServerExt
}

var (
	_ framework.PostFilterPlugin = &Plugin{}
	_ framework.ReservePlugin    = &Plugin{}
	_ framework.BindPlugin       = &Plugin{}
)

func (pl *Plugin) Name() string {
	return Name
}

// New initializes and returns a new CachePod plugin.
func New(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	isLeader := false
	extendHandle := handle.(frameworkext.ExtendedHandle)
	addInQFn := func(pod *corev1.Pod) error {
		return extendHandle.Scheduler().GetSchedulingQueue().Add(pod)
	}
	deleteFromQFn := func(pod *corev1.Pod) error {
		return extendHandle.Scheduler().GetSchedulingQueue().Delete(pod)
	}
	allocatorServer := NewServer(&isLeader, addInQFn, deleteFromQFn)

	args := obj.(*schedulingconfig.CachedPodArgs)
	if err := startGRPCServer(args, allocatorServer); err != nil {
		klog.ErrorS(err, "Failed to start gRPC server")
		return nil, err
	}

	pl := &Plugin{
		isLeader: &isLeader,
		server:   allocatorServer,
	}
	extendHandle.RegisterErrorHandlerFilters(pl.ErrorHandler, nil)
	return pl, nil
}

func startGRPCServer(args *schedulingconfig.CachedPodArgs, allocatorServer cachepodapis.CacheSchedulerServer) error {
	var creds credentials.TransportCredentials
	if len(args.ServerCert) > 0 && len(args.ServerKey) > 0 {
		cert, err := tls.X509KeyPair(args.ServerCert, args.ServerKey)
		if err != nil {
			return err
		}
		creds = credentials.NewServerTLSFromCert(&cert)
	}

	grpcServer := grpc.NewServer(grpc.Creds(creds))
	cachepodapis.RegisterCacheSchedulerServer(grpcServer, allocatorServer)
	listener, err := net.Listen(args.Network, args.Address)
	if err != nil {
		return err
	}
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("cache pod grpc server exited: %v", err)
		}
	}()
	return nil
}

func (pl *Plugin) NewControllers() ([]frameworkext.Controller, error) {
	return []frameworkext.Controller{pl}, nil
}

func (pl *Plugin) Start() {
	*pl.isLeader = true
}

func (pl *Plugin) ErrorHandler(podInfo *framework.QueuedPodInfo, err error) bool {
	if !IsPodFromGRPC(podInfo.Pod) {
		return false
	}

	klog.V(4).InfoS("failed to find available cached pods, try to return the failed result", "pod", klog.KObj(podInfo.Pod), "err", err)
	pl.server.completeRequest(podInfo.Pod, nil, err)
	return true
}

func (pl *Plugin) PostFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, filteredNodeStatusMap framework.NodeToStatusMap) (*framework.PostFilterResult, *framework.Status) {
	if !IsPodFromGRPC(pod) {
		return nil, framework.NewStatus(framework.Unschedulable)
	}
	return nil, framework.NewStatus(framework.Error, "current pod comes from gRPC, stop scheduling")
}

func (pl *Plugin) Reserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	if !IsPodFromGRPC(pod) {
		return nil
	}

	nominatedReservation := frameworkext.GetNominatedReservation(cycleState, nodeName)
	if nominatedReservation == nil || !apiext.IsReservationOperatingMode(nominatedReservation.GetReservePod()) {
		return framework.NewStatus(framework.Error, "no satisfied cached pod")
	}
	hijack.SetTargetPod(cycleState, nominatedReservation.GetReservePod())
	return nil
}

func (pl *Plugin) Unreserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) {

}

func (pl *Plugin) Bind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	if !IsPodFromGRPC(pod) {
		return framework.NewStatus(framework.Skip)
	}

	nominatedReservation := frameworkext.GetNominatedReservation(cycleState, nodeName)
	if nominatedReservation == nil || !apiext.IsReservationOperatingMode(nominatedReservation.GetReservePod()) {
		return framework.NewStatus(framework.Error, "no satisfied cached pod")
	}

	pl.server.completeRequest(pod, nominatedReservation.GetReservePod(), nil)
	return framework.NewStatus(framework.Success)
}
