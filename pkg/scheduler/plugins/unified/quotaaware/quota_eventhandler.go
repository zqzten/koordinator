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

	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"

	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
)

type quotaEventHandler struct {
	cache *QuotaCache
}

func registerElasticQuotaEventHandler(cache *QuotaCache, elasticQuotaInformer cache.SharedIndexInformer) {
	handler := &quotaEventHandler{
		cache: cache,
	}
	frameworkexthelper.ForceSyncFromInformer(context.Background().Done(), nil, elasticQuotaInformer, handler)
}

func (h *quotaEventHandler) OnAdd(obj interface{}) {
	quota := toElasticQuota(obj)
	if quota == nil {
		return
	}

	h.cache.updateQuota(nil, quota)
}

func (h *quotaEventHandler) OnUpdate(obj, newObj interface{}) {
	oldQuota := toElasticQuota(obj)
	if oldQuota == nil {
		return
	}
	quota := toElasticQuota(newObj)
	if quota == nil {
		return
	}

	h.cache.updateQuota(oldQuota, quota)
}

func (h *quotaEventHandler) OnDelete(obj interface{}) {
	quota := toElasticQuota(obj)
	if quota == nil {
		return
	}

	h.cache.deleteQuota(quota)
}

func toElasticQuota(obj interface{}) *schedv1alpha1.ElasticQuota {
	if obj == nil {
		return nil
	}
	var elasticQuota *schedv1alpha1.ElasticQuota
	switch t := obj.(type) {
	case *schedv1alpha1.ElasticQuota:
		elasticQuota = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		elasticQuota, ok = t.Obj.(*schedv1alpha1.ElasticQuota)
		if !ok {
			klog.Errorf("Fail to convert quota object %T", obj)
			return nil
		}
	default:
		klog.Errorf("Unable to handle quota object in %T", obj)
		return nil
	}

	return elasticQuota
}
