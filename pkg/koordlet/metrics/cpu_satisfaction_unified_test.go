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

package metrics

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRecordPodCPUCfsSatisfaction(t *testing.T) {
	testingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test_pod",
			Namespace: "test_pod_namespace",
			UID:       "test01",
		},
	}
	t.Run("test", func(t *testing.T) {
		RecordPodCPUCfsSatisfaction(testingPod, 0.9)
		ResetPodCPUCfsSatisfaction()
	})
}

func TestRecordPodCPUSatisfaction(t *testing.T) {
	testingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test_pod",
			Namespace: "test_pod_namespace",
			UID:       "test01",
		},
	}
	t.Run("test", func(t *testing.T) {
		RecordPodCPUSatisfaction(testingPod, 0.9)
		ResetPodCPUSatisfaction()
	})
}

func TestRecordPodCPUSatisfactionWithThrottle(t *testing.T) {
	testingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test_pod",
			Namespace: "test_pod_namespace",
			UID:       "test01",
		},
	}
	t.Run("test", func(t *testing.T) {
		RecordPodCPUSatisfactionWithThrottle(testingPod, 0.9)
		ResetPodCPUSatisfactionWithThrottle()
	})
}
