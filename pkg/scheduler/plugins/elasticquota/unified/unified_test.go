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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"
	"sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	pgfake "sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned/fake"
	"sigs.k8s.io/scheduler-plugins/pkg/generated/informers/externalversions"

	"github.com/koordinator-sh/koordinator/apis/extension/ack"
)

func TestGetQuotaNameByList(t *testing.T) {
	pgClientSet := pgfake.NewSimpleClientset()
	ctx := context.TODO()
	scheSharedInformerFactory := externalversions.NewSharedInformerFactory(pgClientSet, 0)
	quotaLister := scheSharedInformerFactory.Scheduling().V1alpha1().ElasticQuotas().Lister()
	scheSharedInformerFactory.Start(ctx.Done())
	scheSharedInformerFactory.WaitForCacheSync(ctx.Done())
	eq := &v1alpha1.ElasticQuota{
		TypeMeta: metav1.TypeMeta{Kind: "ElasticQuota", APIVersion: "scheduling.sigs.k8s.io/v1alpha1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test",
			Namespace:   "testNamespace",
			Annotations: make(map[string]string),
		},
	}
	eq.Annotations[ack.AnnotationQuotaNamespaces] = "12"
	eQ, err := pgClientSet.SchedulingV1alpha1().ElasticQuotas(eq.Namespace).Create(context.TODO(), eq, metav1.CreateOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, eQ)
	time.Sleep(time.Second)
	pod := schedulertesting.MakePod().Obj()
	name := GetQuotaName(quotaLister, pod)
	assert.Equal(t, "test", name)
}
