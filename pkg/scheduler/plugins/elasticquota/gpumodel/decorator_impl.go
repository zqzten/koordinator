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

package gpumodel

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	schedulingv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned"
	koordinatorinformers "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota/core"
)

func init() {
	core.RegisterDecorator(&GPUModelCache{})
}

func (gm *GPUModelCache) Init(handle framework.Handle) error {
	var koordSharedInformerFactory koordinatorinformers.SharedInformerFactory
	client, ok := handle.(frameworkext.ExtendedHandle)
	if !ok {
		kubeConfig := *handle.KubeConfig()
		kubeConfig.ContentType = runtime.ContentTypeJSON
		kubeConfig.AcceptContentTypes = runtime.ContentTypeJSON
		koordClientSet := versioned.NewForConfigOrDie(&kubeConfig)
		koordSharedInformerFactory = koordinatorinformers.NewSharedInformerFactory(koordClientSet, 0)
	} else {
		koordSharedInformerFactory = client.KoordinatorSharedInformerFactory()
	}
	if GlobalGPUModelCache == nil {
		GlobalGPUModelCache = &GPUModelCache{
			koordSharedInformerFactory: koordSharedInformerFactory,
			ModelToMemCapacity:         make(map[string]int64),
			NodeToGpuModel:             make(map[string]string),
		}
	}
	return nil
}

func (gm *GPUModelCache) DecorateNode(node *corev1.Node) *corev1.Node {
	if NodeAllocatableContainsGpu(node.Status.Allocatable) {
		node.Status.Allocatable = ParseGPUResourcesByModel(node.Name, node.Status.Allocatable, node.Labels)
	}
	return node
}

func (gm *GPUModelCache) DecoratePod(pod *corev1.Pod) *corev1.Pod {
	gpuModel := GetPodGPUModel(pod)
	for _, container := range pod.Spec.Containers {
		NormalizeGPUResourcesToCardRatioForPod(container.Resources.Requests, gpuModel)
		NormalizeGPUResourcesToCardRatioForPod(container.Resources.Limits, gpuModel)
	}
	for _, container := range pod.Spec.InitContainers {
		NormalizeGPUResourcesToCardRatioForPod(container.Resources.Requests, gpuModel)
		NormalizeGPUResourcesToCardRatioForPod(container.Resources.Limits, gpuModel)
	}
	if pod.Spec.Overhead != nil {
		NormalizeGPUResourcesToCardRatioForPod(pod.Spec.Overhead, gpuModel)
	}
	return pod
}

func (gm *GPUModelCache) DecorateElasticQuota(quota *schedulingv1alpha1.ElasticQuota) *schedulingv1alpha1.ElasticQuota {
	quota.Spec.Max = NormalizeGpuResourcesToCardRatioForQuota(quota.Spec.Max)
	quota.Spec.Min = NormalizeGpuResourcesToCardRatioForQuota(quota.Spec.Min)
	if quota.Annotations != nil {
		quota.Annotations[extension.AnnotationSharedWeight] =
			NormalizeGpuResourcesToCardRatioForQuotaAnnotations(quota.Annotations[extension.AnnotationSharedWeight])
	}
	return quota
}
