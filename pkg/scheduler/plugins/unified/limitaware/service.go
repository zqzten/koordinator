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

package limitaware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/services"
)

var _ services.APIServiceProvider = &Plugin{}

type nodeLimitInfoResponse struct {
	AllocSet         sets.String         `json:"allocSet,omitempty"`
	Allocated        *framework.Resource `json:"allocated,omitempty"`
	NonZeroAllocated *framework.Resource `json:"nonZeroAllocated,omitempty"`
}

func (p *Plugin) RegisterEndpoints(group *gin.RouterGroup) {
	group.GET("/nodeLimitInfo/:nodeName", func(c *gin.Context) {
		nodeName := c.Param("nodeName")
		nodeLimitSummary := p.nodeLimitsCache.GetNodeLimitInfoSummary(nodeName)
		c.JSON(http.StatusOK, nodeLimitSummary)
	})
}

func (c *Cache) GetNodeLimitInfoSummary(nodeName string) *nodeLimitInfoResponse {
	c.lock.RLock()
	defer c.lock.RUnlock()

	info, ok := c.nodeLimitInfos[nodeName]
	if !ok {
		return &nodeLimitInfoResponse{}
	}
	return &nodeLimitInfoResponse{
		AllocSet:         sets.NewString(info.allocSet.List()...),
		Allocated:        info.GetAllocated(),
		NonZeroAllocated: info.GetNonZeroAllocated(),
	}
}
