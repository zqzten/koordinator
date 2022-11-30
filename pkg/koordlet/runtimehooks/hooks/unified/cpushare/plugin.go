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

package cpushare

import (
	"fmt"

	"k8s.io/klog/v2"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/runtimehooks/protocol"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/runtimehooks/reconciler"
	sysutil "github.com/koordinator-sh/koordinator/pkg/util/system"
)

const (
	Name        = "UnifiedCPUShare"
	description = "set container level CPUShares for ignored container resource request"
)

type plugin struct {
}

func (p *plugin) Register() {
	klog.V(5).Infof("register hook %v", Name)
	reconciler.RegisterCgroupReconciler(reconciler.ContainerLevel, sysutil.CPUShares, description, SetContainerShares, reconciler.NoneFilter())
}

var singleton *plugin

func Object() *plugin {
	if singleton == nil {
		singleton = &plugin{}
	}
	return singleton
}

func SetContainerShares(proto protocol.HooksProtocol) error {
	containerCtx := proto.(*protocol.ContainerContext)
	if containerCtx == nil {
		return fmt.Errorf("container protocol is nil for plugin %v", Name)
	}
	containerReq := containerCtx.Request
	containerCgroup, err := extunified.GetCustomCgroupByContainerName(containerReq.PodAnnotations, containerReq.ContainerMeta.Name)
	if err != nil {
		return err
	}
	if containerCgroup == nil || containerCgroup.CPUShares == nil {
		klog.V(5).Infof("container %v/%v/%v doesn't have containerCgroup or containerCgroup.CPUShares is nil ", containerReq.PodMeta.Namespace,
			containerReq.PodMeta.Name, containerReq.ContainerMeta.Name)
		return nil
	}
	containerCtx.Response.Resources.CPUShares = containerCgroup.CPUShares
	return nil
}
