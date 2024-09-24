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

package main

import (
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/besteffortscheduling"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/devicesharing/gpushare"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/gputopology"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/cpusetallocator"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/firstfit"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/gpuoversell"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/hybridnet"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/intelligentscheduler"
	newlicense "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/intelligentscheduler/license"
	newextender "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/intelligentscheduler/license/extender"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/lazyload"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/maxinstance"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/openlocal"
)

func init() {
	koordinatorPlugins[openlocal.Name] = lazyload.Register(openlocal.Name, openlocal.New, openlocal.OpenLocalCondition)
	koordinatorPlugins[hybridnet.Name] = lazyload.Register(hybridnet.Name, hybridnet.New, hybridnet.HybridnetCondition)
	koordinatorPlugins[maxinstance.Name] = maxinstance.New
	koordinatorPlugins[cpusetallocator.Name] = cpusetallocator.New
	koordinatorPlugins[besteffortscheduling.BatchResourceFitName] = besteffortscheduling.NewFit
	koordinatorPlugins[besteffortscheduling.BELeastAllocatedName] = besteffortscheduling.NewBELeastAllocated
	koordinatorPlugins[firstfit.Name] = firstfit.NewInterceptorPlugin
	koordinatorPlugins[gpushare.GPUShareName] = newlicense.Register(
		gpushare.GPUShareName,
		gpushare.New,
		intelligentscheduler.GPUShareCrdCondition,
		newextender.GPUShareLicenseCheckFunc,
		newextender.GPUShareResponsibleForPodFunc,
	) // 智算平台该调度器可删
	koordinatorPlugins[gputopology.GPUTopologyName] = gputopology.New // 智算平台该调度器可删
	koordinatorPlugins[gpuoversell.GPUOversellName] = gpuoversell.New // 智算平台该调度器可删
	koordinatorPlugins[intelligentscheduler.IntelligentSchedulerName] = lazyload.Register(
		intelligentscheduler.IntelligentSchedulerName,
		intelligentscheduler.New,
		intelligentscheduler.IntelligentSchedulerCrdCondition,
	)
}
