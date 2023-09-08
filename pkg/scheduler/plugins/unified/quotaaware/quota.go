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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
)

type QuotaObject struct {
	name     string
	quotaObj *schedv1alpha1.ElasticQuota
	min      corev1.ResourceList
	max      corev1.ResourceList
	runtime  corev1.ResourceList

	pods sets.String
	used corev1.ResourceList
}

func newQuotaObject(quotaObj *schedv1alpha1.ElasticQuota) (*QuotaObject, error) {
	runtimeQuota, err := getQuotaRuntime(quotaObj)
	if err != nil {
		return nil, err
	}
	q := &QuotaObject{
		name:     quotaObj.Name,
		quotaObj: quotaObj,
		pods:     sets.NewString(),
		min:      quotaObj.Spec.Min.DeepCopy(),
		max:      quotaObj.Spec.Max.DeepCopy(),
		runtime:  runtimeQuota,
	}
	return q, nil
}

func (q *QuotaObject) Clone() *QuotaObject {
	return &QuotaObject{
		name:     q.name,
		quotaObj: q.quotaObj,
		pods:     sets.NewString(q.pods.UnsortedList()...),
		used:     q.used.DeepCopy(),
		min:      q.min.DeepCopy(),
		max:      q.max.DeepCopy(),
		runtime:  q.runtime.DeepCopy(),
	}
}

func (q *QuotaObject) getAvailable() corev1.ResourceList {
	available := quotav1.Max(q.runtime, q.min)
	return available
}

func (q *QuotaObject) update(oldQuotaObj, newQuotaObj *schedv1alpha1.ElasticQuota) {
	var oldMin, oldMax corev1.ResourceList
	if oldQuotaObj != nil {
		oldMin = oldQuotaObj.Spec.Min.DeepCopy()
		oldMax = oldQuotaObj.Spec.Max.DeepCopy()
	}
	min := newQuotaObj.Spec.Min.DeepCopy()
	max := newQuotaObj.Spec.Max.DeepCopy()
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

func (q *QuotaObject) addPod(pod *corev1.Pod, requests corev1.ResourceList) {
	key, err := framework.GetPodKey(pod)
	if err != nil {
		klog.ErrorS(err, "Failed to GetPodKey", "pod", klog.KObj(pod))
		return
	}

	if q.pods.Has(key) {
		return
	}
	q.used = quotav1.Add(q.used, requests)
	q.pods.Insert(key)
}

func (q *QuotaObject) removePod(pod *corev1.Pod, requests corev1.ResourceList) {
	key, err := framework.GetPodKey(pod)
	if err != nil {
		klog.ErrorS(err, "Failed to GetPodKey", "pod", klog.KObj(pod))
		return
	}

	if !q.pods.Has(key) {
		return
	}

	q.used = quotav1.SubtractWithNonNegativeResult(q.used, requests)
	q.pods.Delete(key)
}
