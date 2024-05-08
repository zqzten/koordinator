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
	"context"

	asiquotav1 "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"
	uniexternalversions "gitlab.alibaba-inc.com/unischeduler/api/client/informers/externalversions"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
)

type asiQuotaEventHandler struct {
	cache *ASIQuotaCache
}

func registerASIQuotaEventHandler(cache *ASIQuotaCache, asiQuotaInformerFactory uniexternalversions.SharedInformerFactory) {
	handler := &asiQuotaEventHandler{
		cache: cache,
	}
	asiQuotaInformer := asiQuotaInformerFactory.Quotas().V1().Quotas().Informer()
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), asiQuotaInformerFactory, asiQuotaInformer, handler)
}

func (h *asiQuotaEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	asiQuota := toASIQuota(obj)
	if asiQuota == nil {
		return
	}

	h.cache.updateQuota(nil, asiQuota)
}

func (h *asiQuotaEventHandler) OnUpdate(obj, newObj interface{}) {
	oldASIQuota := toASIQuota(obj)
	if oldASIQuota == nil {
		return
	}
	asiQuota := toASIQuota(newObj)
	if asiQuota == nil {
		return
	}

	h.cache.updateQuota(oldASIQuota, asiQuota)
}

func (h *asiQuotaEventHandler) OnDelete(obj interface{}) {
	asiQuota := toASIQuota(obj)
	if asiQuota == nil {
		return
	}

	h.cache.deleteQuota(asiQuota)
}

func toASIQuota(obj interface{}) *asiquotav1.Quota {
	if obj == nil {
		return nil
	}
	var asiQuota *asiquotav1.Quota
	switch t := obj.(type) {
	case *asiquotav1.Quota:
		asiQuota = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		asiQuota, ok = t.Obj.(*asiquotav1.Quota)
		if !ok {
			klog.Errorf("Fail to convert quota object %T to *unstructured.Unstructured", obj)
			return nil
		}
	default:
		klog.Errorf("Unable to handle quota object in %T", obj)
		return nil
	}

	return asiQuota
}
