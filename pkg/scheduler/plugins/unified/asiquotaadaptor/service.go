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
	"net/http"

	"github.com/gin-gonic/gin"
	asiquotav1 "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/services"
)

var _ services.APIServiceProvider = &Plugin{}

type QuotaSummary struct {
	Name           string              `json:"name,omitempty"`
	Pods           []string            `json:"pods,omitempty"`
	Used           corev1.ResourceList `json:"used,omitempty"`
	NonPreemptible corev1.ResourceList `json:"nonPreemptible,omitempty"`
	Min            corev1.ResourceList `json:"min,omitempty"`
	Max            corev1.ResourceList `json:"max,omitempty"`
	Runtime        corev1.ResourceList `json:"runtime,omitempty"`
	QuotaObj       *asiquotav1.Quota   `json:"quotaObj,omitempty"`
}

func (pl *Plugin) RegisterEndpoints(group *gin.RouterGroup) {
	group.GET("/quotas/:quotaName", func(c *gin.Context) {
		quotaName := c.Param("quotaName")
		quota := pl.cache.getQuota(quotaName)
		if quota == nil {
			services.ResponseErrorMessage(c, http.StatusNotFound, "missing target quota %s", quotaName)
			return
		}

		qs := &QuotaSummary{
			Name:           quota.name,
			Pods:           quota.pods.List(),
			Used:           quota.used,
			NonPreemptible: quota.nonPreemptible,
			Min:            quota.min,
			Max:            quota.max,
			Runtime:        quota.runtime,
			QuotaObj:       quota.quotaObj,
		}
		c.JSON(http.StatusOK, qs)
	})
}
