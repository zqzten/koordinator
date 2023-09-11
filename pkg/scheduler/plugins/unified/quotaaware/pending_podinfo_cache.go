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

package quotaaware

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type podInfoCache struct {
	lock     sync.Mutex
	podInfos map[types.UID]*pendingPodInfo
}

func newPodInfoCache() *podInfoCache {
	return &podInfoCache{
		podInfos: map[types.UID]*pendingPodInfo{},
	}
}

func (m *podInfoCache) getPendingPodInfo(uid types.UID) *pendingPodInfo {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.podInfos[uid]
}

func (m *podInfoCache) getAndClonePendingPodInfo(uid types.UID) *pendingPodInfo {
	m.lock.Lock()
	defer m.lock.Unlock()
	pi := m.podInfos[uid]
	if pi == nil {
		return nil
	}
	return pi.clone()
}

func (m *podInfoCache) updatePod(oldPod, newPod *corev1.Pod) {
	if newPod.Spec.NodeName != "" {
		m.deletePod(newPod)
		return
	}

	m.lock.Lock()
	defer m.lock.Unlock()
	pi := m.podInfos[newPod.UID]
	if pi == nil {
		pi = newPendingPodInfo(newPod)
		m.podInfos[newPod.UID] = pi
	}
}

func (m *podInfoCache) deletePod(pod *corev1.Pod) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.podInfos, pod.UID)
}
