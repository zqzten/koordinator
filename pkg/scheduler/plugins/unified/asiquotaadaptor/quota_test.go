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

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

var testQuotaObj = &asiquotav1.Quota{
	ObjectMeta: metav1.ObjectMeta{
		Name: "test",
		Annotations: map[string]string{
			asiquotav1.AnnotationMinQuota:     `{"cpu":"320","memory":"1280Gi"}`,
			asiquotav1.AnnotationQuotaRuntime: `{"cpu":"1","memory":"4294967296"}`,
		},
		Labels: map[string]string{
			asiquotav1.LabelQuotaIsParent: "false",
			asiquotav1.LabelQuotaID:       "3970453551126677660",
		},
	},
	Spec: asiquotav1.QuotaSpec{
		Hard: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("320"),
			corev1.ResourceMemory: resource.MustParse("1280Gi"),
		},
	},
	Status: asiquotav1.QuotaStatus{
		Requested: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
		Runtime: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("4294967296"),
		},
		Used: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	},
}

func TestQuota(t *testing.T) {
	quota, err := newASIQuota(testQuotaObj)
	assert.NoError(t, err)
	assert.NotNil(t, quota)

	quotaObj := testQuotaObj.DeepCopy()
	quotaObj.Spec.Hard = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("300"),
		corev1.ResourceMemory: resource.MustParse("128Gi"),
	}
	quotaObj.Annotations[asiquotav1.AnnotationMinQuota] = `{"cpu":"220","memory":"60Gi"}`
	quotaObj.Annotations[asiquotav1.AnnotationQuotaRuntime] = `{"cpu":"4","memory":"8Gi"}`
	quotaObj.Status.Runtime = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("8Gi"),
	}
	quota.update(testQuotaObj, quotaObj)
	assert.True(t, equality.Semantic.DeepEqual(corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("220"),
		corev1.ResourceMemory: resource.MustParse("60Gi"),
	}, quota.min))
	assert.True(t, equality.Semantic.DeepEqual(corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("300"),
		corev1.ResourceMemory: resource.MustParse("128Gi"),
	}, quota.max))
	assert.True(t, equality.Semantic.DeepEqual(corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("8Gi"),
	}, quota.runtime))

	assert.True(t, equality.Semantic.DeepEqual(corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("220"),
		corev1.ResourceMemory: resource.MustParse("60Gi"),
	}, quota.getAvailable()))

	requests := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("8Gi"),
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: "123456",
		},
	}
	quota.addPod(pod, requests)
	assert.True(t, quota.pods.Has(string(pod.UID)))
	assert.True(t, assert.True(t, equality.Semantic.DeepEqual(corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("8Gi"),
	}, quota.used)))
	quota.removePod(pod, requests)
	assert.True(t, !quota.pods.Has(string(pod.UID)))
	assert.True(t, quotav1.IsZero(quota.used))

	requests = corev1.ResourceList{
		apiext.BatchCPU:    resource.MustParse("4000"),
		apiext.BatchMemory: resource.MustParse("8Gi"),
	}
	pod.Labels = map[string]string{
		apiext.LabelPodQoS: string(apiext.QoSBE),
	}
	quota.addPod(pod, requests)
	assert.True(t, quota.pods.Has(string(pod.UID)))
	assert.True(t, assert.True(t, equality.Semantic.DeepEqual(corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("8Gi"),
	}, quota.used)))
	quota.removePod(pod, requests)
	assert.True(t, !quota.pods.Has(string(pod.UID)))
	assert.True(t, quotav1.IsZero(quota.used))
}
