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

package overquota

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
)

var (
	_ frameworkext.FilterPhaseHook = &Hook{}
)

type Hook struct {
}

func NewHook() *Hook {
	return &Hook{}
}

func (h *Hook) Name() string { return Name }

// FilterHook Scheduler在调用Filter插件和扩展点之前，会从内置的GenericScheduler的nodeInfoSnapShot中取出allNodes，然后将NodeInfo传入Filter过滤，
//	而不是从handle.SnapshotSharedLister拿出，所以这里需要进行FilterHook，取得超卖后视图
func (h *Hook) FilterHook(handle frameworkext.ExtendedHandle, cycleState *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) (*corev1.Pod, *framework.NodeInfo, bool) {
	if overQuotaNodeInfo, isNodeEnableOverQuota := HookNodeInfoWithOverQuota(nodeInfo); isNodeEnableOverQuota {
		return pod, overQuotaNodeInfo, true
	}
	return nil, nil, false
}
