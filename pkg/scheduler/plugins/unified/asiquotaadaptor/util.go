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
	schedulerv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	schedclientset "sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned"
	schedlister "sigs.k8s.io/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
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

func createElasticQuota(elasticQuotaLister schedlister.ElasticQuotaLister, schedClient schedclientset.Interface, asiQuota *asiquotav1.Quota) error {
	eq, _ := elasticQuotaLister.ElasticQuotas(asiQuotaNamespace).Get(asiQuota.Name)
	if eq != nil {
		klog.Infof("elastic quota exist when asiQuota create, namespace:%v, name:%v", asiQuotaNamespace, asiQuota.Name)
		return nil
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
			Min: getMinQuota(asiQuota),
		},
	}
	generateLabelsAndAnnotations(elasticQuota, asiQuota)
	err := util.RetryOnConflictOrTooManyRequests(func() error {
		eq, err := schedClient.SchedulingV1alpha1().ElasticQuotas(elasticQuota.Namespace).
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
	return err
}

func generateLabelsAndAnnotations(elasticQuota *schedulerv1alpha1.ElasticQuota, asiQuota *asiquotav1.Quota) {
	parentName := asiQuota.Labels[asiquotav1.LabelQuotaParent]
	if parentName == apiext.SystemQuotaName {
		parentName = apiext.RootQuotaName
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
		elasticQuota.Annotations[apiext.AnnotationSharedWeight] = asiQuota.Annotations[asiquotav1.AnnotationScaleRatio]
	}
}

func deleteElasticQuota(schedClient schedclientset.Interface, asiQuota *asiquotav1.Quota) {
	err := util.RetryOnConflictOrTooManyRequests(func() error {
		err := schedClient.SchedulingV1alpha1().ElasticQuotas(asiQuotaNamespace).Delete(context.TODO(), asiQuota.Name, metav1.DeleteOptions{})
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

func updateElasticQuota(elasticQuotaLister schedlister.ElasticQuotaLister, schedClient schedclientset.Interface, asiQuota *asiquotav1.Quota) {
	oldElasticQuota, _ := elasticQuotaLister.ElasticQuotas(asiQuotaNamespace).Get(asiQuota.Name)
	if oldElasticQuota == nil {
		klog.Infof("elastic quota not exist when asiQuota update, namespace:%v, name:%v", asiQuotaNamespace, asiQuota.Name)
		return
	}

	newElasticQuota := oldElasticQuota.DeepCopy()
	newElasticQuota.Spec.Max = asiQuota.Spec.Hard
	newElasticQuota.Spec.Min = getMinQuota(asiQuota)
	generateLabelsAndAnnotations(newElasticQuota, asiQuota)

	oldData, _ := json.Marshal(oldElasticQuota)
	newData, _ := json.Marshal(newElasticQuota)
	patchBytes, _ := jsonpatch.CreateMergePatch(oldData, newData)

	err := util.RetryOnConflictOrTooManyRequests(func() error {
		eq, err := schedClient.SchedulingV1alpha1().ElasticQuotas(newElasticQuota.Namespace).
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

func createASIQuotaNamespace(client clientset.Interface, namespace string) error {
	if ns, err := client.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{}); err == nil {
		klog.Infof("get ASIQuota namespace success, namespace:%v", ns.Name)
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
				klog.V(4).ErrorS(err, "create ASIQuota namespace fail, namespace:%v", asiQuotaNamespace)
				return err
			}
			klog.Infof("create ASIQuota namespace success, namespace:%v", asiQuotaNamespace)
			return nil
		})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			klog.ErrorS(err, "Failed to create ASIQuota namespace", "namespace", asiQuotaNamespace)
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
