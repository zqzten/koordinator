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

package volumebinding

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/services"
)

var _ services.APIServiceProvider = &VolumeBinding{}

type NodeStorageInfoResponse struct {
	GraphDiskPath  string                              `json:"graphDiskPath,omitempty"`
	Volumes        map[string]*LocalVolumeResourceItem `json:"volumes,omitempty"`
	LocalPVCAllocs map[string]*localPVCAlloc           `json:"localPVCAllocs,omitempty"`
	LocalPVAllocs  map[string]*localPVAlloc            `json:"localPVAllocs,omitempty"`
	AllocateSet    sets.String                         `json:"allocateSet,omitempty"`
}

type LocalVolumeResourceItem struct {
	*LocalVolume `json:",inline"`
	Used         int64
	Free         int64
}

func (pl *VolumeBinding) RegisterEndpoints(group *gin.RouterGroup) {
	group.GET("/volumes/:nodeName", func(c *gin.Context) {
		nodeName := c.Param("nodeName")
		nodeInfo := pl.nodeStorageCache.GetNodeStorageInfo(nodeName)
		if nodeInfo == nil {
			services.ResponseErrorMessage(c, http.StatusNotFound, "node not found")
			return
		}
		nodeInfo.lock.Lock()

		resp := &NodeStorageInfoResponse{
			GraphDiskPath:  nodeInfo.GraphDiskPath,
			Volumes:        make(map[string]*LocalVolumeResourceItem),
			LocalPVCAllocs: make(map[string]*localPVCAlloc),
			LocalPVAllocs:  make(map[string]*localPVAlloc),
			AllocateSet:    sets.NewString(nodeInfo.allocSet.List()...),
		}
		for k, v := range nodeInfo.LocalVolumesInfo {
			resp.Volumes[k] = &LocalVolumeResourceItem{
				LocalVolume: v,
				Used:        nodeInfo.Used.VolumeSize[k],
				Free:        nodeInfo.Free.VolumeSize[k],
			}
		}
		for k, v := range nodeInfo.localPVCAllocs {
			resp.LocalPVCAllocs[k] = v
		}
		for k, v := range nodeInfo.localPVAllocs {
			resp.LocalPVAllocs[k] = v
		}

		nodeInfo.lock.Unlock()
		c.JSON(http.StatusOK, resp)
	})
}
