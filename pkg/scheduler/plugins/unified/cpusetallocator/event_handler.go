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

package cpusetallocator

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/koordinator-sh/koordinator/pkg/util"
)

func (p *Plugin) onPodUpdate(oldObj, newObj interface{}) {
	pod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}
	if pod.Spec.NodeName == "" || !util.IsPodTerminated(pod) {
		return
	}
	p.GetCPUManager().Free(pod.Spec.NodeName, pod.UID)
	p.cpuSharePoolUpdater.asyncUpdate(pod.Spec.NodeName)
}

func (p *Plugin) onPodDelete(obj interface{}) {
	var pod *corev1.Pod
	switch t := obj.(type) {
	case *corev1.Pod:
		pod = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		pod, ok = t.Obj.(*corev1.Pod)
		if !ok {
			return
		}
	default:
		break
	}
	if pod == nil {
		return
	}
	if pod.Spec.NodeName == "" {
		return
	}
	p.GetCPUManager().Free(pod.Spec.NodeName, pod.UID)
	p.cpuSharePoolUpdater.asyncUpdate(pod.Spec.NodeName)
}
