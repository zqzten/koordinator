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
	asiquotav1 "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

type ASIQuota struct {
	name     string
	quotaObj *asiquotav1.Quota
	min      corev1.ResourceList
	max      corev1.ResourceList
	runtime  corev1.ResourceList

	pods           sets.String
	used           corev1.ResourceList
	nonPreemptible corev1.ResourceList
}

func newASIQuota(quotaObj *asiquotav1.Quota) (*ASIQuota, error) {
	runtimeQuota, err := getQuotaRuntime(quotaObj)
	if err != nil {
		return nil, err
	}
	q := &ASIQuota{
		name:     quotaObj.Name,
		quotaObj: quotaObj,
		pods:     sets.NewString(),
		min:      getMinQuota(quotaObj),
		max:      quotaObj.Spec.Hard.DeepCopy(),
		runtime:  runtimeQuota,
	}
	return q, nil
}

func (q *ASIQuota) Clone() *ASIQuota {
	return &ASIQuota{
		name:           q.name,
		quotaObj:       q.quotaObj,
		pods:           sets.NewString(q.pods.UnsortedList()...),
		used:           q.used.DeepCopy(),
		nonPreemptible: q.nonPreemptible.DeepCopy(),
		min:            q.min.DeepCopy(),
		max:            q.max.DeepCopy(),
		runtime:        q.runtime.DeepCopy(),
	}
}

func (q *ASIQuota) getAvailable() corev1.ResourceList {
	available := quotav1.Max(q.runtime, q.min)
	return available
}

func (q *ASIQuota) update(oldQuotaObj, newQuotaObj *asiquotav1.Quota) {
	var oldMin, oldMax corev1.ResourceList
	if oldQuotaObj != nil {
		oldMin = getMinQuota(oldQuotaObj)
		oldMax = oldQuotaObj.Spec.Hard.DeepCopy()
	}
	min := getMinQuota(newQuotaObj)
	max := newQuotaObj.Spec.Hard.DeepCopy()
	if !quotav1.Equals(oldMin, min) {
		q.min = min
	}
	if !quotav1.Equals(oldMax, max) {
		q.max = max
	}
	runtimeQuota, err := getQuotaRuntime(newQuotaObj)
	if err != nil {
		klog.ErrorS(err, "Failed to getQuotaRuntime", "quota", klog.KObj(newQuotaObj))
		return
	}
	q.runtime = runtimeQuota
}

func (q *ASIQuota) addPod(pod *corev1.Pod, requests corev1.ResourceList, nonPreemptible bool) {
	key, err := framework.GetPodKey(pod)
	if err != nil {
		klog.ErrorS(err, "Failed to GetPodKey", "pod", klog.KObj(pod))
		return
	}

	if q.pods.Has(key) {
		return
	}
	if apiext.GetPodQoSClassRaw(pod) == apiext.QoSBE {
		requests = convertToBatchRequests(requests)
	}
	q.used = quotav1.Add(q.used, requests)
	if nonPreemptible {
		q.nonPreemptible = quotav1.Add(q.nonPreemptible, requests)
	}
	q.pods.Insert(key)
}

func (q *ASIQuota) removePod(pod *corev1.Pod, requests corev1.ResourceList, nonPreemptible bool) {
	key, err := framework.GetPodKey(pod)
	if err != nil {
		klog.ErrorS(err, "Failed to GetPodKey", "pod", klog.KObj(pod))
		return
	}

	if !q.pods.Has(key) {
		return
	}

	if apiext.GetPodQoSClassRaw(pod) == apiext.QoSBE {
		requests = convertToBatchRequests(requests)
	}
	q.used = quotav1.SubtractWithNonNegativeResult(q.used, requests)
	if nonPreemptible {
		q.nonPreemptible = quotav1.SubtractWithNonNegativeResult(q.nonPreemptible, requests)
	}
	q.pods.Delete(key)
}
