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
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

type podEventHandler struct {
	quotaCache   *QuotaCache
	podInfoCache *podInfoCache
}

func registerPodEventHandler(quotaCache *QuotaCache, podInfoCache *podInfoCache, factory informers.SharedInformerFactory) {
	handler := &podEventHandler{
		quotaCache:   quotaCache,
		podInfoCache: podInfoCache,
	}
	podInformer := factory.Core().V1().Pods().Informer()
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), factory, podInformer, handler)
}

func (h *podEventHandler) OnAdd(obj interface{}) {
	pod, _ := obj.(*corev1.Pod)
	if pod == nil {
		return
	}
	h.quotaCache.updatePod(nil, pod)
	h.podInfoCache.updatePod(nil, pod)
}

func (h *podEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldPod, _ := oldObj.(*corev1.Pod)
	if oldPod == nil {
		return
	}
	newPod, _ := newObj.(*corev1.Pod)
	if newPod == nil {
		return
	}

	if util.IsPodTerminated(newPod) {
		h.quotaCache.deletePod(newPod)
		return
	}

	h.quotaCache.updatePod(oldPod, newPod)
	h.podInfoCache.updatePod(oldPod, newPod)
}

func (h *podEventHandler) OnDelete(obj interface{}) {
	var pod *corev1.Pod
	switch t := obj.(type) {
	case *corev1.Pod:
		pod = t
	case cache.DeletedFinalStateUnknown:
		pod, _ = t.Obj.(*corev1.Pod)
	}
	if pod == nil {
		return
	}

	h.quotaCache.deletePod(pod)
	h.podInfoCache.deletePod(pod)
}
