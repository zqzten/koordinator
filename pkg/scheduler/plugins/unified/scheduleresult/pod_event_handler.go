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

package scheduleresult

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

func registersPodEventHandler(handle framework.Handle) {
	podInformer := handle.SharedInformerFactory().Core().V1().Pods().Informer()
	eventHandler := &podEventHandler{}
	podInformer.AddEventHandler(eventHandler)
	handle.SharedInformerFactory().Start(context.TODO().Done())
	handle.SharedInformerFactory().WaitForCacheSync(context.TODO().Done())
}

type podEventHandler struct {
}

func (p *podEventHandler) OnAdd(obj interface{}) {
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
	_, oldCond := pod.GetPodCondition(&oldPod.Status, corev1.PodScheduled)
	_, newCond := pod.GetPodCondition(&newPod.Status, corev1.PodScheduled)
	if newCond != nil {
		if newCond.Status == corev1.ConditionTrue {
			if oldCond == nil {
				// 首次调度成功
				scheduleAttempts.WithLabelValues("first", "Success").Inc()
			} else if oldCond.Status == corev1.ConditionFalse {
				// 重试调度成功
				scheduleAttempts.WithLabelValues("retry", "Success").Inc()
			}
			// condition 无变化，不做处理
		} else if oldCond == nil {
			// 首次调度失败
			scheduleAttempts.WithLabelValues("first", newCond.Reason).Inc()
		}
		// 重试调度失败
		scheduleAttempts.WithLabelValues("retry", newCond.Reason).Inc()
	}
	// condition 无变化，不做处理
}

func (p *podEventHandler) OnDelete(obj interface{}) {
}
