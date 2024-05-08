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

package asiquotaadaptor

import (
	"context"
	"sync/atomic"
	"time"

	quotav1 "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/features"
)

func (pl *Plugin) startSyncTaskQuota() {
	if !k8sfeature.DefaultFeatureGate.Enabled(features.SyncTaskQuota) {
		return
	}
	go wait.Until(pl.syncTaskQuota, 100*time.Millisecond, wait.NeverStop)
}

func (pl *Plugin) syncTaskQuota() {
	nodeLister := pl.handle.SharedInformerFactory().Core().V1().Nodes().Lister()
	nodes, err := nodeLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "Failed to list nodes")
		return
	}

	var totalBatchCPU, totalBatchMemory int64
	pl.handle.Parallelizer().Until(context.TODO(), len(nodes), func(piece int) {
		node := nodes[piece]
		if unified.IsVirtualKubeletNode(node) {
			return
		}
		batchCPU := node.Status.Allocatable[apiext.BatchCPU]
		batchMemory := node.Status.Allocatable[apiext.BatchMemory]
		atomic.AddInt64(&totalBatchCPU, batchCPU.Value())
		atomic.AddInt64(&totalBatchMemory, batchMemory.Value())
	}, pl.Name())

	taskQuotaUsage := &quotav1.TaskQuotaUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name: quotav1.TaskQuotaUsageName,
		},
		Spec: quotav1.TaskQuotaUsageSpec{
			BatchTotalResource: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(totalBatchCPU, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(totalBatchMemory, resource.BinarySI),
			},
		},
	}

	oriTaskQuotaUsage, err := pl.unifiedClient.QuotasV1().TaskQuotaUsages().Get(context.TODO(), taskQuotaUsage.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = pl.unifiedClient.QuotasV1().TaskQuotaUsages().Create(context.TODO(), taskQuotaUsage, metav1.CreateOptions{})
			if err == nil {
				return
			}
		}

		klog.ErrorS(err, "Failed to get or create TaskQuotaUsage", "name", taskQuotaUsage.Name)
		return
	}

	oriTaskQuotaUsage.Spec.BatchTotalResource = taskQuotaUsage.Spec.BatchTotalResource
	_, err = pl.unifiedClient.QuotasV1().TaskQuotaUsages().Update(context.TODO(), oriTaskQuotaUsage, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to update TaskQuotaUsage", "name", taskQuotaUsage.Name)
	}
}
