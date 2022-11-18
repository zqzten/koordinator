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

package volumebinding

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	resourcehelper "k8s.io/kubernetes/pkg/api/v1/resource"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

type podEventHandler struct {
	cache *NodeStorageInfoCache
}

func registerPodEventHandler(sharedInformerFactory informers.SharedInformerFactory, cache *NodeStorageInfoCache) {
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), sharedInformerFactory, podInformer, &podEventHandler{cache: cache})
}

func (e *podEventHandler) OnAdd(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	e.setPod(nil, pod)
}

func (e *podEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		return
	}
	pod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}

	e.setPod(oldPod, pod)
}

func (e *podEventHandler) OnDelete(obj interface{}) {
	var pod *corev1.Pod
	switch t := obj.(type) {
	case *corev1.Pod:
		pod = t
	case cache.DeletedFinalStateUnknown:
		pod, _ = t.Obj.(*corev1.Pod)
	}
	if pod == nil {
		return
	}
	e.deletePod(pod)
}

func (e *podEventHandler) setPod(oldPod *corev1.Pod, newPod *corev1.Pod) {
	if newPod.Spec.NodeName == "" {
		return
	}

	if util.IsPodTerminated(newPod) {
		e.deletePod(newPod)
		return
	}

	var oldEphemeralStorageSize resource.Quantity
	var oldLocalInlineVolumeSize int64
	if oldPod != nil {
		oldRequests, _ := resourcehelper.PodRequestsAndLimits(oldPod)
		oldEphemeralStorageSize = oldRequests[corev1.ResourceEphemeralStorage]
		oldLocalInlineVolumeSize = unified.CalcLocalInlineVolumeSize(oldPod.Spec.Volumes, nil)
	}

	requests, _ := resourcehelper.PodRequestsAndLimits(newPod)
	ephemeralStorageSize := requests[corev1.ResourceEphemeralStorage]
	localInlineVolumeSize := unified.CalcLocalInlineVolumeSize(newPod.Spec.Volumes, nil)

	e.cache.UpdateOnNode(newPod.Spec.NodeName, func(nodeStorageInfo *NodeStorageInfo) {
		if !oldEphemeralStorageSize.IsZero() || oldLocalInlineVolumeSize > 0 {
			nodeStorageInfo.DeleteLocalVolumeAlloc(oldPod, oldEphemeralStorageSize.Value(), oldLocalInlineVolumeSize)
		}
		if !ephemeralStorageSize.IsZero() || localInlineVolumeSize > 0 {
			nodeStorageInfo.AddLocalVolumeAlloc(newPod, ephemeralStorageSize.Value(), localInlineVolumeSize)
		}
	})
}

func (e *podEventHandler) deletePod(pod *corev1.Pod) {
	requests, _ := resourcehelper.PodRequestsAndLimits(pod)
	ephemeralStorageSize := requests[corev1.ResourceEphemeralStorage]
	localInlineVolumeSize := unified.CalcLocalInlineVolumeSize(pod.Spec.Volumes, nil)
	if ephemeralStorageSize.IsZero() && localInlineVolumeSize == 0 {
		return
	}
	e.cache.UpdateOnNode(pod.Spec.NodeName, func(nodeStorageInfo *NodeStorageInfo) {
		nodeStorageInfo.DeleteLocalVolumeAlloc(pod, ephemeralStorageSize.Value(), localInlineVolumeSize)
	})
}
