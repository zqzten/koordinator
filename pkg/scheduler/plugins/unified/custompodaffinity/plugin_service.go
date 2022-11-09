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
}
