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

package estimator

import (
	uniext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	unifiedclient "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned"
	unifiedinformers "gitlab.alibaba-inc.com/cos/unified-resource-api/client/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/loadaware/recommendationfinder"
)

const (
	recommendEstimatorName = "recommendEstimator"
)

func init() {
	Estimators[recommendEstimatorName] = NewRecommendEstimator
}

type RecommendEstimator struct {
	baseEstimator   Estimator
	finder          *recommendationfinder.ControllerFinder
	resourceWeights map[corev1.ResourceName]int64
}

func NewRecommendEstimator(args *config.LoadAwareSchedulingArgs, handle framework.Handle) (Estimator, error) {
	defaultEstimator, err := NewDefaultEstimator(args, handle)
	if err != nil {
		return nil, err
	}

	var sloClientSet unifiedclient.Interface
	if clientSet, ok := handle.(unifiedclient.Interface); ok {
		sloClientSet = clientSet
	} else {
		kubeConfig := *handle.KubeConfig()
		kubeConfig.ContentType = runtime.ContentTypeJSON
		kubeConfig.AcceptContentTypes = runtime.ContentTypeJSON
		client, err := unifiedclient.NewForConfig(&kubeConfig)
		if err != nil {
			return nil, err
		}

		sloClientSet = client
	}

	unifiedSharedInformerFactory := unifiedinformers.NewSharedInformerFactory(sloClientSet, 0)
	recommendationInformer := unifiedSharedInformerFactory.Autoscaling().V1alpha1().Recommendations()
	err = recommendationInformer.Informer().AddIndexers(recommendationfinder.RecommendationIndexers)
	if err != nil {
		return nil, err
	}

	statefulSetInformer := handle.SharedInformerFactory().Apps().V1().StatefulSets()
	deploymentInformer := handle.SharedInformerFactory().Apps().V1().Deployments()
	replicaSetInformer := handle.SharedInformerFactory().Apps().V1().ReplicaSets()
	replicationControllerInformer := handle.SharedInformerFactory().Core().V1().ReplicationControllers()
	finder := recommendationfinder.NewControllerFinder(recommendationInformer, statefulSetInformer.Lister(), deploymentInformer.Lister(), replicaSetInformer.Lister(), replicationControllerInformer.Lister())

	unifiedSharedInformerFactory.Start(nil)
	unifiedSharedInformerFactory.WaitForCacheSync(nil)

	return &RecommendEstimator{
		baseEstimator:   defaultEstimator,
		finder:          finder,
		resourceWeights: args.ResourceWeights,
	}, nil
}

func (e *RecommendEstimator) Name() string {
	return recommendEstimatorName
}

func (e *RecommendEstimator) Estimate(pod *corev1.Pod) (map[corev1.ResourceName]int64, error) {
	// TODO(joseph): 需要统一处理 Batch 类型的资源，统一转换
	if extension.GetPriorityClass(pod) == extension.PriorityBatch {
		pod = pod.DeepCopy()
		replacePodBatchCPU(pod)
	}

	estimatedUsed, err := e.baseEstimator.Estimate(pod)
	if err != nil {
		return nil, err
	}

	recommendRequests, err := e.getRecommendRequests(pod)
	if err != nil {
		klog.Errorf("Failed to get recommendRequests for Pod %v, err: %v", klog.KObj(pod), err)
	}
	if recommendRequests != nil {
		estimatedUsed := make(map[corev1.ResourceName]int64)
		for resourceName := range e.resourceWeights {
			var quantity resource.Quantity
			quantity = recommendRequests[resourceName]
			if !quantity.IsZero() {
				switch resourceName {
				case corev1.ResourceCPU:
					estimatedUsed[resourceName] = quantity.MilliValue()
				default:
					estimatedUsed[resourceName] = quantity.Value()
				}
			}
		}
	}

	return estimatedUsed, nil
}

func (e *RecommendEstimator) getRecommendRequests(pod *corev1.Pod) (corev1.ResourceList, error) {
	ownerRef := metav1.GetControllerOf(pod)
	if ownerRef == nil {
		return nil, nil
	}
	matchedRecommendations, err := e.finder.GetRecommendationsForRef(ownerRef.APIVersion, ownerRef.Kind, ownerRef.Name, pod.Namespace)
	if err != nil {
		return nil, err
	}

	recommendRequests := corev1.ResourceList{}
	for _, v := range matchedRecommendations {
		for _, containerRecommendation := range v.Status.Recommendation.ContainerRecommendations {
			addResourceList(recommendRequests, containerRecommendation.Target)
		}
	}
	return recommendRequests, nil
}

// addResourceList adds the resources in newList to list.
func addResourceList(list, newList corev1.ResourceList) {
	for name, quantity := range newList {
		if value, ok := list[name]; !ok {
			list[name] = quantity.DeepCopy()
		} else {
			value.Add(quantity)
			list[name] = value
		}
	}
}

func replacePodBatchCPU(pod *corev1.Pod) {
	resourceNames := []corev1.ResourceName{
		uniext.AlibabaCloudReclaimedCPU, extension.BatchCPU,
		uniext.AlibabaCloudReclaimedMemory, extension.BatchMemory,
	}
	for i := range pod.Spec.InitContainers {
		replaceContainerResources(&pod.Spec.InitContainers[i], resourceNames...)
	}
	for i := range pod.Spec.Containers {
		replaceContainerResources(&pod.Spec.Containers[i], resourceNames...)
	}
	replaceResourceList(pod.Spec.Overhead, nil, resourceNames...)
}

func replaceContainerResources(container *corev1.Container, resourceNames ...corev1.ResourceName) {
	replaceResourceList(container.Resources.Requests, container.Resources.Limits, resourceNames...)
}

func replaceResourceList(requests, limits corev1.ResourceList, resourceNames ...corev1.ResourceName) {
	if len(resourceNames)%2 != 0 {
		panic("resourceNames must be pair")
	}
	for i := 0; i < len(resourceNames); i += 2 {
		originResourceName := resourceNames[i]
		newResourceName := resourceNames[i+1]
		if val, ok := requests[originResourceName]; ok {
			requests[newResourceName] = val
			delete(requests, originResourceName)
		}
		if val, ok := limits[originResourceName]; ok {
			limits[newResourceName] = val
			delete(limits, originResourceName)
		}
	}
}
