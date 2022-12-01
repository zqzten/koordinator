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

	jsonpatch "github.com/evanphx/json-patch"
	asiquota "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	schedulerv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

func (p *Plugin) OnQuotaAdd(obj interface{}) {
	asiQuota := toASIQuota(obj)
	if asiQuota == nil {
		return
	}

	eq, _ := p.quotaLister.ElasticQuotas(asiQuotaNamespace).Get(asiQuota.Name)
	if eq != nil {
		klog.Infof("elastic quota exist when asiQuota create, namespace:%v, name:%v", asiQuotaNamespace, asiQuota.Name)
		return
	}

	elasticQuota := &schedulerv1alpha1.ElasticQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:        asiQuota.Name,
			Namespace:   asiQuotaNamespace,
			Annotations: make(map[string]string),
			Labels:      make(map[string]string),
		},
		Spec: schedulerv1alpha1.ElasticQuotaSpec{
			Max: asiQuota.Spec.Hard,
			Min: getASIMinQuota(asiQuota),
		},
	}
	generateLabelsAndAnnotations(elasticQuota, asiQuota)
	err := util.RetryOnConflictOrTooManyRequests(func() error {
		eq, err := p.quotaClient.SchedulingV1alpha1().ElasticQuotas(elasticQuota.Namespace).
			Create(context.TODO(), elasticQuota, metav1.CreateOptions{})
		if err != nil {
			klog.V(4).ErrorS(err, "create elastic quota fail when asiQuota create, namespace:%v, name:%v",
				elasticQuota.Namespace, elasticQuota.Name)
			return err
		}
		klog.V(5).Infof("create elastic quota success when asiQuota create, namespace:%v, name:%v, quota:%v",
			elasticQuota.Namespace, elasticQuota.Name, eq)
		return nil
	})
	if err != nil {
		klog.Errorf("create elastic quota fail when asiQuota create, namespace:%v, name:%v, err:%v", elasticQuota.Namespace,
			elasticQuota.Name, err.Error())
	}
}

func (p *Plugin) OnQuotaUpdate(obj, newObj interface{}) {
	asiQuota := toASIQuota(newObj)
	if asiQuota == nil {
		return
	}

	oldElasticQuota, _ := p.quotaLister.ElasticQuotas(asiQuotaNamespace).Get(asiQuota.Name)
	if oldElasticQuota == nil {
		klog.Infof("elastic quota not exist when asiQuota update, namespace:%v, name:%v", asiQuotaNamespace, asiQuota.Name)
		return
	}

	newElasticQuota := oldElasticQuota.DeepCopy()
	newElasticQuota.Spec.Max = asiQuota.Spec.Hard
	newElasticQuota.Spec.Min = getASIMinQuota(asiQuota)
	generateLabelsAndAnnotations(newElasticQuota, asiQuota)

	oldData, _ := json.Marshal(oldElasticQuota)
	newData, _ := json.Marshal(newElasticQuota)
	patchBytes, _ := jsonpatch.CreateMergePatch(oldData, newData)

	err := util.RetryOnConflictOrTooManyRequests(func() error {
		eq, err := p.quotaClient.SchedulingV1alpha1().ElasticQuotas(newElasticQuota.Namespace).
			Patch(context.TODO(), newElasticQuota.Name, apimachinerytypes.MergePatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			klog.V(4).ErrorS(err, "update elastic quota fail when asiQuota update, namespace:%v, name:%v",
				newElasticQuota.Namespace, newElasticQuota.Name)
			return err
		}
		klog.V(5).Infof("update elastic quota success when asiQuota update, namespace:%v, name:%v, quota:%+v",
			newElasticQuota.Namespace, newElasticQuota.Name, eq)
		return nil
	})
	if err != nil {
		klog.Errorf("update elastic quota fail when asiQuota update, namespace:%v, name:%v, err:%v",
			newElasticQuota.Namespace, newElasticQuota.Name, err.Error())
	}
}

func generateLabelsAndAnnotations(elasticQuota *schedulerv1alpha1.ElasticQuota, asiQuota *asiquota.Quota) {
	parentName := asiQuota.Labels[asiquota.LabelQuotaParent]
	if parentName == extension.SystemQuotaName {
		parentName = extension.RootQuotaName
	}
	elasticQuota.Labels[extension.LabelQuotaIsParent] = asiQuota.Labels[asiquota.LabelQuotaIsParent]
	elasticQuota.Labels[extension.LabelQuotaParent] = parentName
	var sharedWeight corev1.ResourceList
	if asiQuota.Annotations[asiquota.AnnotationScaleRatio] != "" {
		if err := json.Unmarshal([]byte(asiQuota.Annotations[asiquota.AnnotationScaleRatio]), &sharedWeight); err != nil {
			return
		}
	}
	if !v1.IsZero(sharedWeight) {
		elasticQuota.Annotations[extension.AnnotationSharedWeight] = asiQuota.Annotations[asiquota.AnnotationScaleRatio]
	}
}

func (p *Plugin) OnQuotaDelete(obj interface{}) {
	asiQuota := toASIQuota(obj)
	if asiQuota == nil {
		return
	}

	err := util.RetryOnConflictOrTooManyRequests(func() error {
		err := p.quotaClient.SchedulingV1alpha1().ElasticQuotas(asiQuotaNamespace).
			Delete(context.TODO(), asiQuota.Name, metav1.DeleteOptions{})
		if err != nil {
			klog.V(4).ErrorS(err, "Delete quota fail, namespace:%v, name:%v", asiQuotaNamespace, asiQuota.Name)
			return err
		}
		return nil
	})
	if err != nil {
		klog.Errorf("Delete quota fail, namespace:%v, name:%v, err:%v", asiQuotaNamespace, asiQuota.Name, err.Error())
		return
	}
	klog.Infof("elastic quota deleted when asiQuota deleted, namespace:%v, name:%v", asiQuotaNamespace, asiQuota.Name)
}

func toASIQuota(obj interface{}) *asiquota.Quota {
	if obj == nil {
		return nil
	}
	var unstructedObj *unstructured.Unstructured
	switch t := obj.(type) {
	case *asiquota.Quota:
		return obj.(*asiquota.Quota)
	case *unstructured.Unstructured:
		unstructedObj = obj.(*unstructured.Unstructured)
	case cache.DeletedFinalStateUnknown:
		var ok bool
		unstructedObj, ok = t.Obj.(*unstructured.Unstructured)
		if !ok {
			klog.Errorf("Fail to convert quota object %T to *unstructured.Unstructured", obj)
			return nil
		}
	default:
		klog.Errorf("Unable to handle quota object in %T", obj)
		return nil
	}

	quota := &asiquota.Quota{}
	err := scheme.Scheme.Convert(unstructedObj, quota, nil)
	if err != nil {
		klog.Errorf("Fail to convert unstructed object %v to Quota: %v", obj, err)
		return nil
	}
	return quota
}

func getASIMinQuota(quota *asiquota.Quota) corev1.ResourceList {
	value, exist := quota.Annotations[asiquota.AnnotationMinQuota]
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
