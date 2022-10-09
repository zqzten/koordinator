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

package cpushare

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/runtimehooks/protocol"
	"github.com/koordinator-sh/koordinator/pkg/util/system"
)

func TestSetContainerShares(t *testing.T) {
	requests := &extunified.CustomCgroup{
		ContainerCgroups: []*extunified.ContainerCgroup{
			{Name: "container0", CPUShares: pointer.Int64Ptr(0)},
			{Name: "container1", CPUShares: pointer.Int64Ptr(1024)},
			{Name: "container2", CPUShares: pointer.Int64Ptr(2048)},
			{Name: "container3", CPUShares: pointer.Int64Ptr(3072)},
		},
	}
	requestsJsonBytes, err := json.Marshal(requests)
	assert.NoError(t, err)

	tests := []struct {
		name         string
		proto        protocol.HooksProtocol
		wantErr      bool
		wantCPUShare *int64
	}{
		{
			name: "bad pod container resource request format",
			proto: &protocol.ContainerContext{
				Request: protocol.ContainerRequest{
					CgroupParent: "kubepods/test-pod/test-container/",
					PodAnnotations: map[string]string{
						extunified.AnnotationCustomCgroup: "bad-format",
					},
					ContainerEnvs: map[string]string{extunified.EnvSigmaIgnoreResource: "true"},
					ContainerMeta: protocol.ContainerMeta{Name: "container1"},
				},
			},
			wantErr: true,
		},
		{
			name: "normal flow",
			proto: &protocol.ContainerContext{
				Request: protocol.ContainerRequest{
					CgroupParent: "kubepods/test-pod/test-container/",
					PodAnnotations: map[string]string{
						extunified.AnnotationCustomCgroup: string(requestsJsonBytes),
					},
					ContainerEnvs: map[string]string{extunified.EnvSigmaIgnoreResource: "true"},
					ContainerMeta: protocol.ContainerMeta{Name: "container1"},
				},
			},
			wantErr:      false,
			wantCPUShare: pointer.Int64Ptr(1024),
		},
		{
			name: "container doesn't exist in annotations",
			proto: &protocol.ContainerContext{
				Request: protocol.ContainerRequest{
					CgroupParent: "kubepods/test-pod/test-container/",
					PodAnnotations: map[string]string{
						extunified.AnnotationCustomCgroup: string(requestsJsonBytes),
					},
					ContainerMeta: protocol.ContainerMeta{Name: "container_not_exist"},
				},
			},
			wantErr: false,
		},
		{
			name: "ignored container doesn't exist in annotations",
			proto: &protocol.ContainerContext{
				Request: protocol.ContainerRequest{
					CgroupParent: "kubepods/test-pod/test-container/",
					PodAnnotations: map[string]string{
						extunified.AnnotationCustomCgroup: string(requestsJsonBytes),
					},
					ContainerEnvs: map[string]string{extunified.EnvSigmaIgnoreResource: "true"},
					ContainerMeta: protocol.ContainerMeta{Name: "container_not_exist"},
				},
			},
			wantErr: false,
		},
		{
			name: "container resource request is zero",
			proto: &protocol.ContainerContext{
				Request: protocol.ContainerRequest{
					CgroupParent: "kubepods/test-pod/test-container/",
					PodAnnotations: map[string]string{
						extunified.AnnotationCustomCgroup: string(requestsJsonBytes),
					},
					ContainerEnvs: map[string]string{extunified.EnvSigmaIgnoreResource: "true"},
					ContainerMeta: protocol.ContainerMeta{Name: "container0"},
				},
			},
			wantErr:      false,
			wantCPUShare: pointer.Int64Ptr(0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testHelper := system.NewFileTestUtil(t)

			containerCtx := tt.proto.(*protocol.ContainerContext)
			testHelper.WriteCgroupFileContents(containerCtx.Request.CgroupParent, system.CPUShares, "")

			if err = SetContainerShares(tt.proto); (err != nil) != tt.wantErr {
				t.Errorf("SetContainerShares() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr == true {
				return
			}

			if tt.wantCPUShare == nil {
				assert.Nil(t, containerCtx.Response.Resources.CPUShares, "CPUShares value should be nil")
			} else {
				assert.Equal(t, *tt.wantCPUShare, *containerCtx.Response.Resources.CPUShares, "container CPUShares should be equal")
				containerCtx.ReconcilerDone()
				gotCPUShare := testHelper.ReadCgroupFileContents(containerCtx.Request.CgroupParent, system.CPUShares)
				assert.Equal(t, strconv.FormatInt(*tt.wantCPUShare, 10), gotCPUShare, "container CPUShares should be equal")
			}
		})
	}
}
