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

package asiquotaadaptor

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	schedulerconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
)

// isDaemonSetPod returns true if the pod is a IsDaemonSetPod.
func isDaemonSetPod(ownerRefList []metav1.OwnerReference) bool {
	for _, ownerRef := range ownerRefList {
		if ownerRef.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

func buildPriorityRangeConfigMap(args *schedulerconfig.ASIQuotaAdaptorArgs) map[int]*schedulerconfig.PriorityRangeConfig {
	items := make([]*schedulerconfig.PriorityRangeConfig, 0, len(args.PriorityRangeConfig))
	for _, item := range args.PriorityRangeConfig {
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].PriorityStart < items[j].PriorityStart
	})

	priorityRangeConfigByPriority := make(map[int]*schedulerconfig.PriorityRangeConfig, 10000)
	for _, item := range items {
		for i := item.PriorityStart; i <= item.PriorityEnd; i++ {
			priorityRangeConfigByPriority[i] = item
		}
	}
	return priorityRangeConfigByPriority
}

type PreemptionConfig struct {
	priorityRange map[int]*schedulerconfig.PriorityRangeConfig
}

func NewPreemptionConfig(args *schedulerconfig.ASIQuotaAdaptorArgs) *PreemptionConfig {
	priorityRange := buildPriorityRangeConfigMap(args)
	return &PreemptionConfig{
		priorityRange: priorityRange,
	}
}

func (c *PreemptionConfig) CanBePreempted(pod *corev1.Pod) bool {
	config := c.priorityRange[int(pointer.Int32Deref(pod.Spec.Priority, 0))]
	if config == nil {
		return false
	}
	return config.EnableBePreempted
}
