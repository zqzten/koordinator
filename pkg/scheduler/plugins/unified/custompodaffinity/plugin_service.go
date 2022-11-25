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

package custompodaffinity

import (
	"net/http"

	"github.com/gin-gonic/gin"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/services"
)

var _ services.APIServiceProvider = &Plugin{}

type serviceUnitStatsResponse struct {
	PodSpreadInfo *extunified.PodSpreadInfo `json:"podSpreadInfo,omitempty"`
	AllocCount    int                       `json:"allocCount,omitempty"`
}

func (p *Plugin) RegisterEndpoints(group *gin.RouterGroup) {
	group.GET("/serviceUnitStats/:nodeName/:appName", func(c *gin.Context) {
		nodeName := c.Param("nodeName")
		podSpreadInfo := &extunified.PodSpreadInfo{
			AppName: c.Param("appName"),
		}
		allocCount := p.cache.GetAllocCount(nodeName, podSpreadInfo)
		serviceStatsResponse := &serviceUnitStatsResponse{
			PodSpreadInfo: podSpreadInfo,
			AllocCount:    allocCount,
		}
		c.JSON(http.StatusOK, serviceStatsResponse)
	})
	group.GET("/serviceUnitStats/:nodeName/:appName/:serviceUnitName", func(c *gin.Context) {
		nodeName := c.Param("nodeName")
		podSpreadInfo := &extunified.PodSpreadInfo{
			AppName:     c.Param("appName"),
			ServiceUnit: c.Param("serviceUnitName"),
		}
		allocCount := p.cache.GetAllocCount(nodeName, podSpreadInfo)
		serviceStatsResponse := &serviceUnitStatsResponse{
			PodSpreadInfo: podSpreadInfo,
			AllocCount:    allocCount,
		}
		c.JSON(http.StatusOK, serviceStatsResponse)
	})
	group.GET("/allocSet/:nodeName", func(c *gin.Context) {
		nodeName := c.Param("nodeName")
		allocSet := p.cache.GetAllocSet(nodeName)
		c.JSON(http.StatusOK, allocSet)
	})
}
