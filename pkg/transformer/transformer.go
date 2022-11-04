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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

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
		TransformSigmaIgnoreResourceContainers(pod)

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
		ignored := false
		for _, v := range container.Env {
			if v.Name == "SIGMA_IGNORE_RESOURCE" && v.Value == "true" {
				ignored = true
				break
			}
		}
		if ignored {
			for k, v := range container.Resources.Requests {
				container.Resources.Requests[k] = *resource.NewQuantity(0, v.Format)
			}
		}
	}
}
