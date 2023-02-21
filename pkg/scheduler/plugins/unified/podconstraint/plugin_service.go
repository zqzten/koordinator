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

package podconstraint

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/services"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/podconstraint/cache"
)

var _ services.APIServiceProvider = &Plugin{}

type constraintStatesResponse struct {
	DefaultPodConstraint       bool                              `json:"defaultPodConstraint,omitempty"`
	SpreadTypeRequired         bool                              `json:"spreadTypeRequired,omitempty"`
	RequiredSpreadConstraints  []*cache.TopologySpreadConstraint `json:"requiredSpreadConstraints,omitempty"`
	PreferredSpreadConstraints []*cache.TopologySpreadConstraint `json:"preferredSpreadConstraints,omitempty"`
	// TpPairToMatchNum is keyed with topologyPair, and valued with the number of matching pods.
	TpPairToMatchNum     map[cache.TopologyPair]int              `json:"tpPairToMatchNum,omitempty"`
	TpKeyToTotalMatchNum map[string]int                          `json:"tpKeyToTotalMatchNum,omitempty"`
	TpKeyToCriticalPaths map[string]*cache.TopologyCriticalPaths `json:"tpKeyToCriticalPaths,omitempty"`
}

func newConstraintStateResponse(constraintState *cache.TopologySpreadConstraintState) *constraintStatesResponse {
	constraintState.RLock()
	defer constraintState.RUnlock()
	constraintStateResponse := &constraintStatesResponse{
		DefaultPodConstraint:       constraintState.DefaultPodConstraint,
		SpreadTypeRequired:         constraintState.SpreadTypeRequired,
		RequiredSpreadConstraints:  constraintState.CopyRequiredSpreadConstraints(),
		PreferredSpreadConstraints: constraintState.CopyPreferredSpreadConstraints(),
		TpPairToMatchNum:           constraintState.CopyTpPairToMatchNum(),
		TpKeyToTotalMatchNum:       constraintState.CopyTpKeyToTotalMatchNum(),
		TpKeyToCriticalPaths:       constraintState.CopyTpKeyToCriticalPath(),
	}
	return constraintStateResponse
}

func (p *Plugin) RegisterEndpoints(group *gin.RouterGroup) {
	group.GET("/constraintStates/:constraintNamespace/:constraintName", func(c *gin.Context) {
		constraintNamespace := c.Param("constraintNamespace")
		constraintName := c.Param("constraintName")
		constraintState := p.podConstraintCache.GetState(cache.GetNamespacedName(constraintNamespace, constraintName))
		if constraintState == nil {
			services.ResponseErrorMessage(c, http.StatusNotFound, "cannot find constraintState %s", cache.GetNamespacedName(constraintNamespace, constraintName))
			return
		}
		constraintStateResponse := newConstraintStateResponse(constraintState)
		c.JSON(http.StatusOK, constraintStateResponse)
	})
	group.GET("/allocSet", func(c *gin.Context) {
		allocSet := sets.NewString(p.podConstraintCache.AllocSet.List()...)
		c.JSON(http.StatusOK, allocSet)
	})
}
