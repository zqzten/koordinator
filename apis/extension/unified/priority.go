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
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

func init() {
	extension.DefaultPriorityClass = extension.PriorityProd
	extension.PriorityBatchValueMin = uniext.PriorityBatchValueMin
	extension.PriorityBatchValueMax = uniext.PriorityBatchValueMax
}

func GetPriorityClass(pod *corev1.Pod) extension.PriorityClass {
	if pod == nil {
		return extension.PriorityNone
	}
	priorityClass := uniext.GetPriorityClass(pod)
	switch priorityClass {
	case uniext.PriorityProd:
		return extension.PriorityProd
	case uniext.PriorityMid:
		return extension.PriorityMid
	case uniext.PriorityBatch:
		return extension.PriorityBatch
	case uniext.PriorityFree:
		return extension.PriorityFree
	}
	return extension.PriorityProd
}

func GetUnifiedPriorityClass(pod *corev1.Pod) uniext.PriorityClass {
	if pod == nil {
		return uniext.PriorityNone
	}
	priorityClass := uniext.GetPriorityClass(pod)
	switch priorityClass {
	case uniext.PriorityProd, uniext.PriorityMid, uniext.PriorityBatch, uniext.PriorityFree:
		return priorityClass
	default:
		return uniext.PriorityProd
	}
}
