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

package cachepod

import (
	"context"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	cachepodapis "github.com/koordinator-sh/koordinator/apis/cachepod"
	"github.com/koordinator-sh/koordinator/apis/extension"
)

var (
	AnnotationCachePodUUID = extension.SchedulingDomainPrefix + "/cachepod-uuid"
	Name                   = "CachePod"
)

type Plugin struct {
}

var (
	_ framework.BindPlugin = &Plugin{}
)

func (p *Plugin) Name() string {
	return Name
}

// New initializes and returns a new CachePod plugin.
func New(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	grpcServer := grpc.NewServer()
	cachePodServer := NewServer()
	cachepodapis.RegisterCachedPodAllocatorServer(grpcServer, cachePodServer)
	// FIXME: add a flag for grpc server port
	listener, err := net.Listen("tcp", ":8089")
	if err != nil {
		os.Exit(1)
	}
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("cache pod grpc server exited: %v", err)
		}
	}()
	return &Plugin{}, nil
}

func (p *Plugin) Bind(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	uuid := pod.Annotations[AnnotationCachePodUUID]
	if uuid == "" {
		// not cache pod
		return framework.NewStatus(framework.Skip, "")
	}
	waitChan := GetWaitChan(uuid)
	if waitChan == nil {
		// request canceled
		return framework.NewStatus(framework.Skip, "lack of wait channel of cache pod")
	}
	// FIXME: 这里需要返回CachePod，而不是fakePod
	waitChan <- pod
	return framework.NewStatus(framework.Success, "")
}
