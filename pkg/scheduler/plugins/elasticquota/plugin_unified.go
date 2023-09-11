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

package elasticquota

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

func (g *Plugin) TryAdd(pod *corev1.Pod, quotaName string) {
	mgr := g.GetGroupQuotaManagerForQuota(quotaName)
	if mgr != nil {
		mgr.OnPodAdd(quotaName, pod)
	}
}

func (g *Plugin) Forget(pod *corev1.Pod, quotaName string) {
	mgr := g.GetGroupQuotaManagerForQuota(quotaName)
	if mgr != nil {
		mgr.OnPodDelete(quotaName, pod)
	}
}

func (g *Plugin) FitQuota(podRequests corev1.ResourceList, quotaName string) (bool, *framework.Status) {
	mgr := g.GetGroupQuotaManagerForQuota(quotaName)
	if mgr == nil {
		return false, framework.NewStatus(framework.Unschedulable, fmt.Sprintf("cannot find GroupQuotaManager by quota %s", quotaName))
	}
	mgr.RefreshRuntime(quotaName)
	quotaInfo := mgr.GetQuotaInfoByName(quotaName)
	if quotaInfo == nil {
		return false, framework.NewStatus(framework.Error, fmt.Sprintf("Could not find the specified ElasticQuota"))
	}
	state := &PostFilterState{
		quotaInfo: quotaInfo,
		used:      quotaInfo.GetUsed(),
		runtime:   quotaInfo.GetRuntime(),
	}

	used := quotav1.Add(podRequests, state.used)

	if isLessEqual, exceedDimensions := quotav1.LessThanOrEqual(used, state.runtime); !isLessEqual {
		return false, framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Insufficient quotas, "+
			"quotaName: %v, runtime: %v, used: %v, pod's request: %v, exceedDimensions: %v",
			quotaName, printResourceList(state.runtime), printResourceList(state.used), printResourceList(podRequests), exceedDimensions))
	}

	if *g.pluginArgs.EnableCheckParentQuota {
		status := g.checkQuotaRecursive(quotaName, []string{quotaName}, podRequests)
		if !status.IsSuccess() {
			return false, status
		}
	}

	return true, nil
}

func (g *Plugin) ReserveQuota(pod *corev1.Pod, quotaName string) {
	mgr := g.GetGroupQuotaManagerForQuota(quotaName)
	if mgr != nil {
		mgr.ReservePod(quotaName, pod)
	}
}

func (g *Plugin) UnreserveQuota(pod *corev1.Pod, quotaName string) {
	mgr := g.GetGroupQuotaManagerForQuota(quotaName)
	if mgr != nil {
		mgr.UnreservePod(quotaName, pod)
	}
}

func (g *Plugin) GetElasticQuotaInformer() cache.SharedIndexInformer {
	return g.quotaInformer
}
