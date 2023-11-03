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

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
)

type resourceManagerAdapter struct {
	nodenumaresource.ResourceManager
	updater *cpuSharePoolUpdater
}

func newResourceManagerAdapter(resourceManager nodenumaresource.ResourceManager, updater *cpuSharePoolUpdater) nodenumaresource.ResourceManager {
	return &resourceManagerAdapter{
		ResourceManager: resourceManager,
		updater:         updater,
	}
}

func (m *resourceManagerAdapter) Update(nodeName string, allocation *nodenumaresource.PodAllocation) {
	m.ResourceManager.Update(nodeName, allocation)
	m.updater.asyncUpdate(nodeName)
}

func (m *resourceManagerAdapter) Release(nodeName string, podUID types.UID) {
	m.ResourceManager.Release(nodeName, podUID)
	m.updater.asyncUpdate(nodeName)
}
