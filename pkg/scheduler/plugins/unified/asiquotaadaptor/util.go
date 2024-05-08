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
	"encoding/json"
	"strconv"

	jsonpatch "github.com/evanphx/json-patch"
	asiquotav1 "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	resourceapi "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/apiserver/pkg/quota/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulerv1alpha1 "github.com/koordinator-sh/koordinator/apis/thirdparty/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	schedclientset "github.com/koordinator-sh/koordinator/apis/thirdparty/scheduler-plugins/pkg/generated/clientset/versioned"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

func isParentQuota(quota *asiquotav1.Quota) bool {
	if val, exist := quota.Labels[asiquotav1.LabelQuotaIsParent]; exist {
		isParent, err := strconv.ParseBool(val)
		if err == nil {
			return isParent
		}
	}

	return false
}

func getQuotaRuntime(quota *asiquotav1.Quota) (corev1.ResourceList, error) {
	if quota.Status.Runtime != nil {
		return quota.Status.Runtime, nil
	}
	runtimeAnnotation, ok := quota.Annotations[asiquotav1.AnnotationQuotaRuntime]
	if ok {
		resourceList := make(corev1.ResourceList)
		err := json.Unmarshal([]byte(runtimeAnnotation), &resourceList)
		if err != nil {
			return nil, err
		}
		return resourceList, nil
	}
	return nil, nil
}

func getMinQuota(quota *asiquotav1.Quota) corev1.ResourceList {
	value, exist := quota.Annotations[asiquotav1.AnnotationMinQuota]
	if !exist {
		return nil
	}

	resList := corev1.ResourceList{}
	err := json.Unmarshal([]byte(value), &resList)
	if err != nil {
		return nil
	}
	return resList
}

func createElasticQuota(schedClient schedclientset.Interface, asiQuota *asiquotav1.Quota) error {
	elasticQuota := &schedulerv1alpha1.ElasticQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:        asiQuota.Name,
			Namespace:   asiQuotaNamespace,
			Annotations: make(map[string]string),
			Labels:      make(map[string]string),
		},
		Spec: schedulerv1alpha1.ElasticQuotaSpec{
			Max: asiQuota.Spec.Hard,
			Min: getMinQuota(asiQuota),
		},
	}
	generateLabelsAndAnnotations(elasticQuota, asiQuota)
	err := util.RetryOnConflictOrTooManyRequests(func() error {
		_, err := schedClient.SchedulingV1alpha1().ElasticQuotas(elasticQuota.Namespace).Create(context.TODO(), elasticQuota, metav1.CreateOptions{})
		if err != nil {
			klog.V(4).ErrorS(err, "Failed to create ElasticQuota by ASIQuota", "quota", klog.KObj(elasticQuota))
			return err
		}
		klog.V(4).InfoS("Successfully create ElasticQuota by ASIQuota", "quota", klog.KObj(elasticQuota))
		return nil
	})
	return err
}

func generateLabelsAndAnnotations(elasticQuota *schedulerv1alpha1.ElasticQuota, asiQuota *asiquotav1.Quota) {
	parentName := asiQuota.Labels[asiquotav1.LabelQuotaParent]
	if parentName == "system" {
		parentName = apiext.RootQuotaName
	}
	if elasticQuota.Labels == nil {
		elasticQuota.Labels = map[string]string{}
	}
	elasticQuota.Labels[apiext.LabelQuotaIsParent] = asiQuota.Labels[asiquotav1.LabelQuotaIsParent]
	elasticQuota.Labels[apiext.LabelQuotaParent] = parentName
	var sharedWeight corev1.ResourceList
	if asiQuota.Annotations[asiquotav1.AnnotationScaleRatio] != "" {
		if err := json.Unmarshal([]byte(asiQuota.Annotations[asiquotav1.AnnotationScaleRatio]), &sharedWeight); err != nil {
			return
		}
	}
	if !v1.IsZero(sharedWeight) {
		if elasticQuota.Annotations == nil {
			elasticQuota.Annotations = map[string]string{}
		}
		elasticQuota.Annotations[apiext.AnnotationSharedWeight] = asiQuota.Annotations[asiquotav1.AnnotationScaleRatio]
	}
}

func updateElasticQuota(oldElasticQuota *schedulerv1alpha1.ElasticQuota, schedClient schedclientset.Interface, asiQuota *asiquotav1.Quota) error {
	newElasticQuota := oldElasticQuota.DeepCopy()
	newElasticQuota.Spec.Max = asiQuota.Spec.Hard
	newElasticQuota.Spec.Min = getMinQuota(asiQuota)
	generateLabelsAndAnnotations(newElasticQuota, asiQuota)

	oldData, _ := json.Marshal(oldElasticQuota)
	newData, _ := json.Marshal(newElasticQuota)
	patchBytes, _ := jsonpatch.CreateMergePatch(oldData, newData)

	if string(patchBytes) == "{}" {
		return nil
	}

	err := util.RetryOnConflictOrTooManyRequests(func() error {
		_, err := schedClient.SchedulingV1alpha1().ElasticQuotas(newElasticQuota.Namespace).
			Patch(context.TODO(), newElasticQuota.Name, apimachinerytypes.MergePatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			klog.V(4).ErrorS(err, "Failed to update ElasticQuota by ASIQuota", "quota", klog.KObj(newElasticQuota))
			return err
		}
		klog.V(4).InfoS("Successfully update ElasticQuota by ASIQuota", "quota", klog.KObj(newElasticQuota))
		return nil
	})
	return err
}

func createASIQuotaNamespace(client clientset.Interface, namespace string) error {
	if ns, err := client.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{}); err == nil {
		klog.V(4).InfoS("ASIQuota namespace already exists, skip create it.", "namespace", ns.Name)
		return nil
	}
	err := util.RetryOnConflictOrTooManyRequests(
		func() error {
			namespaceObj := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: asiQuotaNamespace,
				},
			}
			_, err := client.CoreV1().Namespaces().Create(context.TODO(), namespaceObj, metav1.CreateOptions{})
			if err != nil {
				klog.V(4).ErrorS(err, "Failed to create ASIQuota namespace", "namespace", asiQuotaNamespace)
				return err
			}
			klog.V(4).InfoS("Successfully create ASIQuota namespace", "namespace", asiQuotaNamespace)
			return nil
		})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func marshalResourceList(res corev1.ResourceList) string {
	data, _ := json.Marshal(res)
	return string(data)
}

func convertToBatchRequests(podRequests corev1.ResourceList) corev1.ResourceList {
	r := podRequests.DeepCopy()
	for resourceName, quantity := range r {
		switch resourceName {
		case apiext.BatchCPU:
			cpu := quantity.Value()
			r[corev1.ResourceCPU] = *resourceapi.NewMilliQuantity(cpu, resourceapi.DecimalSI)
			delete(r, resourceName)
		case apiext.BatchMemory:
			r[corev1.ResourceMemory] = quantity.DeepCopy()
			delete(r, resourceName)
		}
	}
	return r
}
