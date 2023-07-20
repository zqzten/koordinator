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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	quotav1 "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"
	unifiedfake "gitlab.alibaba-inc.com/unischeduler/api/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

func TestSyncTaskQuota(t *testing.T) {
	var nodes []runtime.Object
	for i := 0; i < 10; i++ {
		nodes = append(nodes, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("node-%d", i),
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					apiext.BatchCPU:    resource.MustParse("32000"),
					apiext.BatchMemory: resource.MustParse("16Gi"),
				},
			},
		})
	}
	kubeClient := kubefake.NewSimpleClientset(nodes...)
	informerFactory := informers.NewSharedInformerFactory(kubeClient, 0)
	_ = informerFactory.Core().V1().Nodes().Informer()
	informerFactory.Start(nil)
	informerFactory.WaitForCacheSync(nil)

	registeredPlugins := []schedulertesting.RegisterPluginFunc{
		schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
		schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
	}
	fh, err := schedulertesting.NewFramework(
		registeredPlugins,
		"koord-scheduler",
		frameworkruntime.WithInformerFactory(informerFactory),
	)
	assert.NoError(t, err)

	pl := &Plugin{
		handle:        fh,
		unifiedClient: unifiedfake.NewSimpleClientset(),
	}
	pl.syncTaskQuota()

	quota, err := pl.unifiedClient.QuotasV1().TaskQuotaUsages().Get(context.TODO(), quotav1.TaskQuotaUsageName, metav1.GetOptions{})
	assert.NoError(t, err)

	expectedTotal := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("320000m"),
		corev1.ResourceMemory: resource.MustParse("160Gi"),
	}
	assert.Equal(t, expectedTotal, quota.Spec.BatchTotalResource)
}
