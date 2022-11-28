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
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	utilclient "github.com/koordinator-sh/koordinator/pkg/util/client"
	"github.com/koordinator-sh/koordinator/pkg/webhook/elasticquota"
)

// TODO 需要对ElasticQuotaTree进行校验，相应地不单独校验ElasticQuotaTree创建的ElasticQuota
func init() {
	elasticquota.GetQuotaName = GetQuotaName
}

func GetQuotaName(clientImpl client.Client, pod *corev1.Pod) string {
	quotaList := &v1alpha1.ElasticQuotaList{}
	err := clientImpl.List(context.TODO(), quotaList, utilclient.DisableDeepCopy)
	if err != nil {
		runtime.HandleError(err)
		return extension.DefaultQuotaName
	}
	if len(quotaList.Items) == 0 {
		return extension.DefaultQuotaName
	}
	for _, eq := range quotaList.Items {
		namespaces := eq.Annotations[ack.AnnotationQuotaNamespaces]
		if strings.Contains(namespaces, pod.Namespace) {
			return eq.Name
		}
	}

	opts := &client.ListOptions{
		Namespace: pod.Namespace,
	}
	err = clientImpl.List(context.TODO(), quotaList, opts, utilclient.DisableDeepCopy)
	if err != nil {
		runtime.HandleError(err)
		return extension.DefaultQuotaName
	}
	if len(quotaList.Items) == 0 {
		return extension.DefaultQuotaName
	}
	return quotaList.Items[0].Name
}
