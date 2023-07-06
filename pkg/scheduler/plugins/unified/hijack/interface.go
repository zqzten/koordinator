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

package hijack

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

var (
	targetPodKey framework.StateKey = "koordinator.sh/target-pod"
)

type targetPodState struct {
	targetPod *corev1.Pod
}

func (s *targetPodState) Clone() framework.StateData {
	return s
}

func SetTargetPod(cycleState *framework.CycleState, targetPod *corev1.Pod) {
	if targetPod != nil {
		cycleState.Write(targetPodKey, &targetPodState{targetPod: targetPod})
	}
}

func GetTargetPod(cycleState *framework.CycleState) *corev1.Pod {
	state, err := cycleState.Read(targetPodKey)
	if err != nil {
		return nil
	}
	return state.(*targetPodState).targetPod
}
