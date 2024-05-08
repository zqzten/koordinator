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
	"sync"

	asiquotav1 "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/api/v1/resource"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

type ASIQuotaCache struct {
	quotas           map[string]*ASIQuota
	lock             sync.Mutex
	preemptionConfig *PreemptionConfig
}

func withPreemptionConfig(preemptionConfig *PreemptionConfig) func(*ASIQuotaCache) {
	return func(cache *ASIQuotaCache) {
		cache.preemptionConfig = preemptionConfig
	}
}

func newASIQuotaCache(options ...func(cache *ASIQuotaCache)) *ASIQuotaCache {
	cache := &ASIQuotaCache{
		quotas: map[string]*ASIQuota{},
	}
	for _, opt := range options {
		opt(cache)
	}
	return cache
}

func (c *ASIQuotaCache) getQuota(quotaName string) *ASIQuota {
	c.lock.Lock()
	defer c.lock.Unlock()

	q := c.quotas[quotaName]
	if q == nil {
		return nil
	}
	return q.Clone()
}

func (c *ASIQuotaCache) assumePod(pod *corev1.Pod, podRequests corev1.ResourceList) {
	quotaName := pod.Labels[asiquotav1.LabelQuotaName]

	c.lock.Lock()
	defer c.lock.Unlock()

	c.addPodUsedUnsafe(quotaName, pod, podRequests)
}

func (c *ASIQuotaCache) forgetPod(pod *corev1.Pod, podRequests corev1.ResourceList) {
	quotaName := pod.Labels[asiquotav1.LabelQuotaName]

	c.lock.Lock()
	defer c.lock.Unlock()

	c.removePodUsedUnsafe(quotaName, pod, podRequests)
}

func (c *ASIQuotaCache) updatePod(oldPod, newPod *corev1.Pod) {
	if newPod.Spec.NodeName == "" {
		return
	}

	var oldRequests corev1.ResourceList
	var oldQuotaName string
	if oldPod != nil {
		oldRequests = resource.PodRequests(oldPod, resource.PodResourcesOptions{})
		if apiext.GetPodQoSClassRaw(oldPod) == apiext.QoSBE {
			oldRequests = convertToBatchRequests(oldRequests)
		}
		oldQuotaName = oldPod.Labels[asiquotav1.LabelQuotaName]
	}
	newRequests := resource.PodRequests(newPod, resource.PodResourcesOptions{})
	if apiext.GetPodQoSClassRaw(newPod) == apiext.QoSBE {
		newRequests = convertToBatchRequests(newRequests)
	}
	quotaName := newPod.Labels[asiquotav1.LabelQuotaName]

	c.lock.Lock()
	defer c.lock.Unlock()

	if oldPod != nil {
		c.removePodUsedUnsafe(oldQuotaName, oldPod, oldRequests)
	}
	c.addPodUsedUnsafe(quotaName, newPod, newRequests)
}

func (c *ASIQuotaCache) deletePod(pod *corev1.Pod) {
	if pod.Spec.NodeName == "" {
		return
	}
	quotaName := pod.Labels[asiquotav1.LabelQuotaName]
	requests := resource.PodRequests(pod, resource.PodResourcesOptions{})
	if apiext.GetPodQoSClassRaw(pod) == apiext.QoSBE {
		requests = convertToBatchRequests(requests)
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	c.removePodUsedUnsafe(quotaName, pod, requests)
}

func (c *ASIQuotaCache) removePodUsedUnsafe(quotaName string, pod *corev1.Pod, requests corev1.ResourceList) {
	canBePreempted := c.preemptionConfig != nil && c.preemptionConfig.CanBePreempted(pod)
	if quota := c.quotas[quotaName]; quota != nil {
		quota.removePod(pod, requests, !canBePreempted)
	}
}

func (c *ASIQuotaCache) addPodUsedUnsafe(quotaName string, pod *corev1.Pod, requests corev1.ResourceList) {
	canBePreempted := c.preemptionConfig != nil && c.preemptionConfig.CanBePreempted(pod)
	if quota := c.quotas[quotaName]; quota != nil {
		quota.addPod(pod, requests, !canBePreempted)
	}
}

func (c *ASIQuotaCache) updateQuota(oldQuota, newQuota *asiquotav1.Quota) {
	if isParentQuota(newQuota) {
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	quota := c.quotas[newQuota.Name]
	if quota == nil {
		quota, err := newASIQuota(newQuota)
		if err != nil {
			klog.ErrorS(err, "Failed to newASIQuota", "quota", klog.KObj(newQuota))
			return
		}
		c.quotas[newQuota.Name] = quota
		return
	}

	quota.update(oldQuota, newQuota)
}

func (c *ASIQuotaCache) deleteQuota(quota *asiquotav1.Quota) {
	if isParentQuota(quota) {
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	delete(c.quotas, quota.Name)
}
