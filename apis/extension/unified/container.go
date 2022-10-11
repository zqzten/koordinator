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

package unified

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	AnnotationContainerResourceRequest = extension.DomainPrefix + "containerResourceRequest"
	EnvSigmaIgnoreResource             = "SIGMA_IGNORE_RESOURCE"
	EnvECIResourceIgnore               = "__ECI_RESOURCE_IGNORE__"
	ENVECIResourceIgnoreTrue           = "TRUE"
)

type ContainerResourceRequest struct {
	Name            string              `json:"name,omitempty"`
	ResourceRequest corev1.ResourceList `json:"resourceRequest,omitempty"`
}

func GetResourceRequestByContainerName(annotations map[string]string, containerName string) (*ContainerResourceRequest, error) {
	requests, err := GetResourceRequests(annotations)
	if err != nil {
		return nil, err
	}
	if requests == nil {
		return nil, nil
	}
	return requests[containerName], nil
}

func GetResourceRequests(annotations map[string]string) (map[string]*ContainerResourceRequest, error) {
	if annotations == nil {
		return nil, nil
	}
	rawRequests, ok := annotations[AnnotationContainerResourceRequest]
	if !ok || len(rawRequests) == 0 {
		return nil, nil
	}
	var requests []*ContainerResourceRequest
	if err := json.Unmarshal([]byte(rawRequests), &requests); err != nil {
		return nil, err
	}
	result := map[string]*ContainerResourceRequest{}
	for _, request := range requests {
		result[request.Name] = request
	}
	return result, nil
}

func IsContainerIgnoreResource(c *corev1.Container) bool {
	for _, entry := range c.Env {
		if entry.Name == EnvSigmaIgnoreResource && entry.Value == "true" {
			return true
		}
	}
	return false
}
