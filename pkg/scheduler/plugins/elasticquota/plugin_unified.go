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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/cache"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/thirdparty/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/util/transformer"
)

func (g *Plugin) GetElasticQuotaInformer() cache.SharedIndexInformer {
	return g.quotaInformer
}

func init() {
	resourceDecorators = append(resourceDecorators, decorateResourceForACSQuota)
}

// convert batch resource to normal resource
func decorateResourceForACSQuota(quota *v1alpha1.ElasticQuota, res corev1.ResourceList) {
	// TODO: import acs vendor.
	switch quota.Labels[transformer.LabelInstanceType] {
	case transformer.BestEffortInstanceType:
		quantity, ok := res[extension.BatchCPU]
		if ok {
			res[corev1.ResourceCPU] = *resource.NewMilliQuantity(quantity.Value(), resource.DecimalSI)
			delete(res, extension.BatchCPU)
		}
		quantity, ok = res[extension.BatchMemory]
		if ok {
			res[corev1.ResourceMemory] = quantity
			delete(res, extension.BatchMemory)
		}
	}
}
