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

package podtopologyaware

import (
	"reflect"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

type cacheManager struct {
	lock            sync.Mutex
	constraintInfos map[types.NamespacedName]*constraintInfo
}

func newCacheManager() *cacheManager {
	return &cacheManager{
		constraintInfos: map[types.NamespacedName]*constraintInfo{},
	}
}

func (m *cacheManager) getConstraintInfo(namespace, name string) *constraintInfo {
	m.lock.Lock()
	defer m.lock.Unlock()
	namespacedName := types.NamespacedName{Namespace: namespace, Name: name}
	return m.constraintInfos[namespacedName]
}

func (m *cacheManager) updatePod(oldPod, newPod *corev1.Pod) {
	var oldConstraint *apiext.TopologyAwareConstraint
	if oldPod != nil {
		var err error
		oldConstraint, err = apiext.GetTopologyAwareConstraint(oldPod.Annotations)
		if err != nil {
			klog.ErrorS(err, "Failed to GetTopologyAwareConstraint from old pod", "pod", klog.KObj(oldPod))
			return
		}
	}
	constraint, err := apiext.GetTopologyAwareConstraint(newPod.Annotations)
	if err != nil {
		klog.ErrorS(err, "Failed to GetTopologyAwareConstraint from pod", "pod", klog.KObj(newPod))
		return
	}

	if oldConstraint == nil && constraint == nil {
		return
	}

	if oldConstraint != nil && constraint != nil {
		if reflect.DeepEqual(oldConstraint, constraint) {
			return
		}
	}

	m.lock.Lock()
	defer m.lock.Unlock()
	if oldConstraint != nil && validateTopologyAwareConstraint(oldConstraint) {
		m.removeConstraintInfo(oldPod, oldConstraint)
	}
	if constraint != nil && validateTopologyAwareConstraint(constraint) {
		m.addConstraintInfo(newPod, constraint)
	}
}

func (m *cacheManager) deletePod(pod *corev1.Pod) {
	constraint, err := apiext.GetTopologyAwareConstraint(pod.Annotations)
	if err != nil || !validateTopologyAwareConstraint(constraint) {
		return
	}

	m.lock.Lock()
	defer m.lock.Unlock()
	m.removeConstraintInfo(pod, constraint)
}

func (m *cacheManager) addConstraintInfo(pod *corev1.Pod, constraint *apiext.TopologyAwareConstraint) {
	podKey, err := framework.GetPodKey(pod)
	if err != nil {
		return
	}

	namespacedName := types.NamespacedName{Namespace: pod.Namespace, Name: constraint.Name}
	constraintInfo := m.constraintInfos[namespacedName]
	if constraintInfo == nil {
		constraintInfo = newTopologyAwareConstraintInfo(pod.Namespace, constraint)
		m.constraintInfos[namespacedName] = constraintInfo
	}
	constraintInfo.pods.Insert(podKey)
}

func (m *cacheManager) removeConstraintInfo(pod *corev1.Pod, constraint *apiext.TopologyAwareConstraint) {
	podKey, err := framework.GetPodKey(pod)
	if err != nil {
		return
	}

	namespacedName := types.NamespacedName{Namespace: pod.Namespace, Name: constraint.Name}
	constraintInfo := m.constraintInfos[namespacedName]
	if constraintInfo != nil {
		constraintInfo.pods.Delete(podKey)
		if constraintInfo.pods.Len() == 0 {
			delete(m.constraintInfos, namespacedName)
		}
	}
}

func validateTopologyAwareConstraint(constraint *apiext.TopologyAwareConstraint) bool {
	return constraint != nil && constraint.Name != "" && constraint.Required != nil
}
