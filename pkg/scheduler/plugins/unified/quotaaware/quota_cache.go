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

package quotaaware

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/api/v1/resource"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

type QuotaCache struct {
	quotas map[string]*QuotaObject
	lock   sync.Mutex
}

func newQuotaCache() *QuotaCache {
	cache := &QuotaCache{
		quotas: map[string]*QuotaObject{},
	}
	return cache
}

func (c *QuotaCache) getQuota(quotaName string) *QuotaObject {
	c.lock.Lock()
	defer c.lock.Unlock()

	q := c.quotas[quotaName]
	if q == nil {
		return nil
	}
	return q.clone()
}

func (c *QuotaCache) assumePod(pod *corev1.Pod, podRequests corev1.ResourceList) {
	quotaName := apiext.GetQuotaName(pod)

	c.lock.Lock()
	defer c.lock.Unlock()

	c.addPodUsedUnsafe(quotaName, pod, podRequests)
}

func (c *QuotaCache) forgetPod(pod *corev1.Pod, podRequests corev1.ResourceList) {
	quotaName := apiext.GetQuotaName(pod)

	c.lock.Lock()
	defer c.lock.Unlock()

	c.removePodUsedUnsafe(quotaName, pod, podRequests)
}

func (c *QuotaCache) updatePod(oldPod, newPod *corev1.Pod) {
	if newPod.Spec.NodeName == "" {
		return
	}

	var oldRequests corev1.ResourceList
	var oldQuotaName string
	if oldPod != nil {
		oldRequests, _ = resource.PodRequestsAndLimits(oldPod)
		oldQuotaName = apiext.GetQuotaName(oldPod)
	}
	newRequests, _ := resource.PodRequestsAndLimits(newPod)
	quotaName := apiext.GetQuotaName(newPod)

	c.lock.Lock()
	defer c.lock.Unlock()

	if oldPod != nil {
		c.removePodUsedUnsafe(oldQuotaName, oldPod, oldRequests)
	}
	c.addPodUsedUnsafe(quotaName, newPod, newRequests)
}

func (c *QuotaCache) deletePod(pod *corev1.Pod) {
	if pod.Spec.NodeName == "" {
		return
	}
	quotaName := apiext.GetQuotaName(pod)
	requests, _ := resource.PodRequestsAndLimits(pod)

	c.lock.Lock()
	defer c.lock.Unlock()
	c.removePodUsedUnsafe(quotaName, pod, requests)
}

func (c *QuotaCache) removePodUsedUnsafe(quotaName string, pod *corev1.Pod, requests corev1.ResourceList) {
	if quota := c.quotas[quotaName]; quota != nil {
		quota.removePod(pod, requests)
	}
}

func (c *QuotaCache) addPodUsedUnsafe(quotaName string, pod *corev1.Pod, requests corev1.ResourceList) {
	if quota := c.quotas[quotaName]; quota != nil {
		quota.addPod(pod, requests)
	}
}

func (c *QuotaCache) updateQuota(oldQuota, newQuota *schedv1alpha1.ElasticQuota) {
	if isParentQuota(newQuota) {
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	quota := c.quotas[newQuota.Name]
	if quota == nil {
		quota, err := newQuotaObject(newQuota)
		if err != nil {
			klog.ErrorS(err, "Failed to newQuotaObject", "quota", klog.KObj(newQuota))
			return
		}
		c.quotas[newQuota.Name] = quota
		return
	}

	quota.update(oldQuota, newQuota)
}

func (c *QuotaCache) deleteQuota(quota *schedv1alpha1.ElasticQuota) {
	if isParentQuota(quota) {
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	delete(c.quotas, quota.Name)
}
