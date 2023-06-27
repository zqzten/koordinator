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

package ack

import (
	"gitlab.alibaba-inc.com/sigma/sigma-k8s-api/pkg/api"
	v1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	LabelQuotaName            = api.AlibabaCloudPrefix + "/quota-name"
	AnnotationQuotaNamespaces = extension.QuotaKoordinatorPrefix + "/namespaces"
)

func init() {
	extension.GetQuotaName = GetQuotaName
}

func GetQuotaName(pod *v1.Pod) string {
	quotaName := pod.Labels[extension.LabelQuotaName]
	if quotaName == "" {
		quotaName = pod.Labels[LabelQuotaName]
	}
	return quotaName
}
