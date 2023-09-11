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
	"net/http"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/services"
)

var _ services.APIServiceProvider = &Plugin{}

type QuotaResponse struct {
	Name        string                      `json:"name"`
	QuotaObject *schedv1alpha1.ElasticQuota `json:"quotaObject"`
	Min         corev1.ResourceList         `json:"min"`
	Max         corev1.ResourceList         `json:"max"`
	Runtime     corev1.ResourceList         `json:"runtime"`
	Used        corev1.ResourceList         `json:"used"`
}

type PodInfoResponse struct {
	UID                 types.UID `json:"uid"`
	Namespace           string    `json:"namespace"`
	Name                string    `json:"name"`
	SelectedQuotaName   string    `json:"selectedQuotaName"`
	FrozenQuotas        []string  `json:"frozenQuotas"`
	PendingQuotas       []string  `json:"pendingQuotas"`
	ProcessedQuotas     []string  `json:"processedQuotas"`
	LastProcessedQuotas []string  `json:"lastProcessedQuotas"`
}

func (pl *Plugin) RegisterEndpoints(group *gin.RouterGroup) {
	group.GET("/quotas/:name", func(ctx *gin.Context) {
		quotaName := ctx.Param("name")
		quota := pl.quotaCache.getQuota(quotaName)
		if quota == nil {
			services.ResponseErrorMessage(ctx, http.StatusNotFound, "cannot find quota %s", quotaName)
			return
		}
		resp := &QuotaResponse{
			Name:        quota.name,
			QuotaObject: quota.quotaObj,
			Min:         quota.min,
			Max:         quota.max,
			Runtime:     quota.runtime,
			Used:        quota.used,
		}
		ctx.JSON(http.StatusOK, resp)
	})

	group.GET("/pods/:namespace/:name", func(ctx *gin.Context) {
		namespace := ctx.Param("namespace")
		name := ctx.Param("name")
		pod, err := pl.handle.SharedInformerFactory().Core().V1().Pods().Lister().Pods(namespace).Get(name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				services.ResponseErrorMessage(ctx, http.StatusNotFound, "cannot find pod %s/%s", namespace, name)
				return
			}
			services.ResponseErrorMessage(ctx, http.StatusInternalServerError, "cannot find pod %s/%s", namespace, name)
			return
		}
		pi := pl.podInfoCache.getAndClonePendingPodInfo(pod.UID)
		if pi == nil {
			services.ResponseErrorMessage(ctx, http.StatusNotFound, "cannot find pod pending info %s/%s", namespace, name)
			return
		}
		resp := &PodInfoResponse{
			UID:                 pi.uid,
			Namespace:           pi.namespace,
			Name:                pi.name,
			SelectedQuotaName:   pi.selectedQuotaName,
			FrozenQuotas:        pi.frozenQuotas.List(),
			PendingQuotas:       pi.pendingQuotas.List(),
			ProcessedQuotas:     pi.processedQuotas.List(),
			LastProcessedQuotas: pi.lastProcessedQuotas.List(),
		}
		ctx.JSON(http.StatusOK, resp)
	})
}
