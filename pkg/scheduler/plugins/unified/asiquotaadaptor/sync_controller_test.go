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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	unifiedclientsetfake "gitlab.alibaba-inc.com/unischeduler/api/client/clientset/versioned/fake"
	unifiedinformer "gitlab.alibaba-inc.com/unischeduler/api/client/informers/externalversions"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schedclientsetfake "sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned/fake"
	schedinformer "sigs.k8s.io/scheduler-plugins/pkg/generated/informers/externalversions"
)

func TestQuotaSyncControllerReconcile(t *testing.T) {
	schedClient := schedclientsetfake.NewSimpleClientset()
	schedInformerFactory := schedinformer.NewSharedInformerFactory(schedClient, 0)
	elasticQuotaLister := schedInformerFactory.Scheduling().V1alpha1().ElasticQuotas().Lister()
	schedInformerFactory.Start(nil)
	schedInformerFactory.WaitForCacheSync(nil)

	unifiedClient := unifiedclientsetfake.NewSimpleClientset()
	unifiedInformerFactory := unifiedinformer.NewSharedInformerFactory(unifiedClient, 0)
	controller := NewQuotaSyncController(schedClient, elasticQuotaLister, unifiedInformerFactory)
	controller.setupEventHandler()
	unifiedInformerFactory.Start(nil)
	unifiedInformerFactory.WaitForCacheSync(nil)

	asiQuota := testQuotaObj.DeepCopy()
	unifiedQuotaClient := unifiedClient.QuotasV1().Quotas()
	_, err := unifiedQuotaClient.Create(context.TODO(), asiQuota, metav1.CreateOptions{})
	assert.NoError(t, err)
	assert.True(t, controller.processNextWorkItem())

	time.Sleep(1 * time.Second)
	elasticQuota, err := elasticQuotaLister.ElasticQuotas(asiQuotaNamespace).Get(asiQuota.Name)
	assert.NoError(t, err)
	assert.NotNil(t, elasticQuota)

	asiQuota.Spec.Hard["fakeResource"] = resource.MustParse("1000")
	_, err = unifiedQuotaClient.Update(context.TODO(), asiQuota, metav1.UpdateOptions{})
	assert.NoError(t, err)
	assert.True(t, controller.processNextWorkItem())

	time.Sleep(1 * time.Second)
	elasticQuota, err = schedClient.SchedulingV1alpha1().ElasticQuotas(asiQuotaNamespace).Get(context.TODO(), asiQuota.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, elasticQuota)
	quantity := elasticQuota.Spec.Max["fakeResource"]
	assert.Equal(t, int64(1000), quantity.Value())

	err = unifiedQuotaClient.Delete(context.TODO(), asiQuota.Name, metav1.DeleteOptions{})
	assert.NoError(t, err)
	assert.True(t, controller.processNextWorkItem())
	time.Sleep(1 * time.Second)
	_, err = elasticQuotaLister.ElasticQuotas(asiQuotaNamespace).Get(asiQuota.Name)
	assert.True(t, apierrors.IsNotFound(err))
}
