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

package cpusetallocator

import (
	"k8s.io/apimachinery/pkg/types"

	schedulingconfig "github.com/koordinator-sh/koordinator/apis/scheduling/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
)

type cpuManagerAdapter struct {
	nodenumaresource.CPUManager
	updater *cpuSharePoolUpdater
}

func newCPUManagerAdapter(cpuManager nodenumaresource.CPUManager, updater *cpuSharePoolUpdater) nodenumaresource.CPUManager {
	return &cpuManagerAdapter{
		CPUManager: cpuManager,
		updater:    updater,
	}
}

func (m *cpuManagerAdapter) UpdateAllocatedCPUSet(nodeName string, podUID types.UID, cpuset nodenumaresource.CPUSet, cpuExclusivePolicy schedulingconfig.CPUExclusivePolicy) {
	m.CPUManager.UpdateAllocatedCPUSet(nodeName, podUID, cpuset, cpuExclusivePolicy)
	m.updater.asyncUpdate(nodeName)
}

func (m *cpuManagerAdapter) Free(nodeName string, podUID types.UID) {
	m.CPUManager.Free(nodeName, podUID)
	m.updater.asyncUpdate(nodeName)
}
