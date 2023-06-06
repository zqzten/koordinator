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
	"testing"

	"github.com/stretchr/testify/assert"
	asiquotav1 "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
)

func TestASIQuotaCacheUpdateAndDeletePod(t *testing.T) {
	cache := newASIQuotaCache()
	cache.updateQuota(nil, testQuotaObj)

	requests := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("8Gi"),
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: "123456",
			Labels: map[string]string{
				asiquotav1.LabelQuotaName: testQuotaObj.Name,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: requests,
					},
				},
			},
		},
	}
	newPod := pod.DeepCopy()
	newPod.Spec.NodeName = "test-node"
	cache.updatePod(pod, newPod)
	quota := cache.getQuota(testQuotaObj.Name)
	assert.NotNil(t, quota)
	assert.True(t, assert.True(t, equality.Semantic.DeepEqual(corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("8Gi"),
	}, quota.used)))

	cache.deletePod(newPod)
	quota = cache.getQuota(testQuotaObj.Name)
	assert.True(t, !quota.pods.Has(string(pod.UID)))
	assert.True(t, quotav1.IsZero(quota.used))
}
