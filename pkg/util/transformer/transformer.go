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

package transformer

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	syncerconsts "sigs.k8s.io/cluster-api-provider-nested/virtualcluster/pkg/syncer/constants"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

var podTransformers = []func(pod *corev1.Pod){
	TransformSigmaIgnoreResourceContainers,
	TransformTenantPod,
	TransformNonProdPodResourceSpec,
}

func InstallPodTransformer(podInformer cache.SharedIndexInformer) {
	transformerSetter, ok := podInformer.(cache.TransformerSetter)
	if !ok {
		klog.Fatalf("cache.TransformerSetter is not implemented")
	}
	transformerSetter.SetTransform(func(obj interface{}) (interface{}, error) {
		var pod *corev1.Pod
		switch t := obj.(type) {
		case *corev1.Pod:
			pod = t
		case cache.DeletedFinalStateUnknown:
			pod, _ = t.Obj.(*corev1.Pod)
		}
		if pod == nil {
			return obj, nil
		}

		pod = pod.DeepCopy()
		for _, fn := range podTransformers {
			fn(pod)
		}

		if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok {
			unknown.Obj = pod
			return unknown, nil
		}
		return pod, nil
	})
}

func TransformSigmaIgnoreResourceContainers(pod *corev1.Pod) {
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if unified.IsContainerIgnoreResource(container) {
			for k, v := range container.Resources.Requests {
				container.Resources.Requests[k] = *resource.NewQuantity(0, v.Format)
			}
		}
	}
}

func TransformTenantPod(pod *corev1.Pod) {
	if len(pod.OwnerReferences) != 0 {
		return
	}
	if tenantOwnerReferences, ok := pod.Annotations[syncerconsts.LabelOwnerReferences]; ok {
		_ = json.Unmarshal([]byte(tenantOwnerReferences), &pod.OwnerReferences)
	}
}

func TransformNonProdPodResourceSpec(pod *corev1.Pod) {
	priorityClass := unified.GetPriorityClass(pod)
	if priorityClass == extension.PriorityNone || priorityClass == extension.PriorityProd {
		return
	}

	for _, containers := range [][]corev1.Container{pod.Spec.InitContainers, pod.Spec.Containers} {
		for i := range containers {
			container := &containers[i]
			replaceAndEraseResource(priorityClass, container.Resources.Requests, corev1.ResourceCPU)
			replaceAndEraseResource(priorityClass, container.Resources.Requests, corev1.ResourceMemory)

			replaceAndEraseResource(priorityClass, container.Resources.Limits, corev1.ResourceCPU)
			replaceAndEraseResource(priorityClass, container.Resources.Limits, corev1.ResourceMemory)
		}
	}

	if pod.Spec.Overhead != nil {
		replaceAndEraseResource(priorityClass, pod.Spec.Overhead, corev1.ResourceCPU)
		replaceAndEraseResource(priorityClass, pod.Spec.Overhead, corev1.ResourceMemory)
	}
}

func replaceAndEraseResource(priorityClass extension.PriorityClass, resourceList corev1.ResourceList, resourceName corev1.ResourceName) {
	extendResourceName := extension.ResourceNameMap[priorityClass][resourceName]
	if extendResourceName == "" {
		return
	}
	quantity, ok := resourceList[resourceName]
	if ok {
		if resourceName == corev1.ResourceCPU {
			quantity = *resource.NewQuantity(quantity.MilliValue(), resource.DecimalSI)
		}
		resourceList[extendResourceName] = quantity
		delete(resourceList, resourceName)
	}
}
