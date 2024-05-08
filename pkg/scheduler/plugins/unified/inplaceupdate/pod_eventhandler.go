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

package inplaceupdate

import (
	"context"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

type podEventHandler struct {
	updateReqHandler *controller
	profileName      string
}

func registersPodEventHandler(handle framework.Handle, inplaceUpdateController *controller) {
	podInformer := handle.SharedInformerFactory().Core().V1().Pods().Informer()
	eventHandler := cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			switch t := obj.(type) {
			case *corev1.Pod:
				return assignedPod(t)
			case cache.DeletedFinalStateUnknown:
				if pod, ok := t.Obj.(*corev1.Pod); ok {
					return assignedPod(pod)
				}
				return false
			default:
				return false
			}
		},
		Handler: &podEventHandler{
			updateReqHandler: inplaceUpdateController,
			profileName:      handle.(framework.Framework).ProfileName(),
		},
	}
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), handle.SharedInformerFactory(), podInformer, eventHandler)
}

func assignedPod(pod *corev1.Pod) bool {
	return pod.Spec.NodeName != ""
}

func (p *podEventHandler) OnAdd(obj interface{}, _ bool) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	if pod.Spec.SchedulerName != p.profileName {
		return
	}
	if util.IsPodTerminated(pod) {
		// 因为出队之后会从 Informer 里面判断一下是否继续后续流程，所以此处无需从队列里面 Done 掉
		return
	}
	if podHasUnhandledUpdateReq(pod) {
		// 这里异步将 Pod 入 Queue，因为当接收到事件的时候，scheduler可能还没有初始化完成, 不能直接加入schedulingQueue
		p.updateReqHandler.asyncAddToQ(apimachinerytypes.NamespacedName{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		})
	}
}

func podHasUnhandledUpdateReq(pod *corev1.Pod) bool {
	podAnnotations := pod.Annotations
	if podAnnotations == nil {
		podAnnotations = map[string]string{}
	}
	resourceUpdateSpec, err := uniext.GetResourceUpdateSpec(podAnnotations)
	if err != nil {
		return false
	}
	resourceUpdateState, err := uniext.GetResourceUpdateSchedulerState(podAnnotations)
	if err != nil {
		return false
	}
	if resourceUpdateSpec.Version == resourceUpdateState.Version {
		return false
	}
	return true
}

func (p *podEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		return
	}
	newPod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}
	if newPod.Spec.SchedulerName != p.profileName {
		return
	}
	if util.IsPodTerminated(newPod) {
		// 因为出队之后会从 Informer 里面判断一下是否继续后续流程，所以此处无需从队列里面 Done 掉
		return
	}
	if podHasUpdateReq(oldPod, newPod) {
		// 这里异步将 Pod 入 Queue，因为当接收到事件的时候，scheduler可能还没有初始化完成, 不能直接加入schedulingQueue
		p.updateReqHandler.asyncAddToQ(apimachinerytypes.NamespacedName{
			Namespace: newPod.Namespace,
			Name:      newPod.Name,
		})
	}
}

func podHasUpdateReq(oldPod, newPod *corev1.Pod) bool {
	oldPodAnnotations := oldPod.Annotations
	if oldPodAnnotations == nil {
		oldPodAnnotations = map[string]string{}
	}
	newPodAnnotations := newPod.Annotations
	if newPodAnnotations == nil {
		newPodAnnotations = map[string]string{}
	}
	oldResourceUpdateSpec, err := uniext.GetResourceUpdateSpec(oldPodAnnotations)
	if err != nil {
		return false
	}
	newResourceUpdateSpec, err := uniext.GetResourceUpdateSpec(newPodAnnotations)
	if err != nil {
		return false
	}
	if oldResourceUpdateSpec.Version == newResourceUpdateSpec.Version {
		return false
	}
	return true
}

func (p *podEventHandler) OnDelete(obj interface{}) {
	// 因为出队之后会从 Informer 里面判断一下是否继续后续流程，所以此处无需从队列里面 Done 掉
}
