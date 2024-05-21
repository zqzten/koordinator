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

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DomainPrefix = "alibabacloud.com"

	ACSType = "virtual-cluster-node"

	AnnotationTenancyOwnerReferences = "tenancy.x-k8s.io/ownerReferences"
)

func IsACSVirtualNode(labels map[string]string) bool {
	return labels[uniext.LabelNodeType] == ACSType
}

func GetTenancyOwnerReferences(annotations map[string]string) ([]metav1.OwnerReference, error) {
	val, ok := annotations[AnnotationTenancyOwnerReferences]
	if !ok {
		return nil, nil
	}
	var ownerReferences []metav1.OwnerReference
	err := json.Unmarshal([]byte(val), &ownerReferences)
	if err != nil {
		return nil, err
	}
	return ownerReferences, nil
}
