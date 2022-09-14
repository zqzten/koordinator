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

package orchestratingslo

import (
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getPodName(pod *v1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}

func makePod(name, version, namespace string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       namespace,
			Name:            name,
			ResourceVersion: version,
		},
	}
}

func verifyPod(cache PodAssumeCache, name string, expectedPod *v1.Pod) error {
	pod, err := cache.GetPod(name)
	if err != nil {
		return err
	}
	if pod != expectedPod {
		return fmt.Errorf("GetPod() returned %p, expected %p", pod, expectedPod)
	}
	return nil
}

func TestAssumePod(t *testing.T) {
	scenarios := map[string]struct {
		oldPod        *v1.Pod
		newPod        *v1.Pod
		shouldSucceed bool
	}{
		"success-same-version": {
			oldPod:        makePod("pod1", "5", "ns1"),
			newPod:        makePod("pod1", "5", "ns1"),
			shouldSucceed: true,
		},
		"success-new-higher-version": {
			oldPod:        makePod("pod1", "5", "ns1"),
			newPod:        makePod("pod1", "6", "ns1"),
			shouldSucceed: true,
		},
		"fail-old-not-found": {
			oldPod:        makePod("pod2", "5", "ns1"),
			newPod:        makePod("pod1", "5", "ns1"),
			shouldSucceed: false,
		},
		"fail-new-lower-version": {
			oldPod:        makePod("pod1", "5", "ns1"),
			newPod:        makePod("pod1", "4", "ns1"),
			shouldSucceed: false,
		},
		"fail-new-bad-version": {
			oldPod:        makePod("pod1", "5", "ns1"),
			newPod:        makePod("pod1", "a", "ns1"),
			shouldSucceed: false,
		},
		"fail-old-bad-version": {
			oldPod:        makePod("pod1", "a", "ns1"),
			newPod:        makePod("pod1", "5", "ns1"),
			shouldSucceed: false,
		},
	}

	for name, scenario := range scenarios {
		cache := NewPodAssumeCache(nil)
		internalCache, ok := cache.(*podAssumeCache).AssumeCache.(*assumeCache)
		if !ok {
			t.Fatalf("Failed to get internal cache")
		}

		// Add oldPod to cache
		internalCache.add(scenario.oldPod)
		if err := verifyPod(cache, getPodName(scenario.oldPod), scenario.oldPod); err != nil {
			t.Errorf("Failed to GetPod() after initial update: %v", err)
			continue
		}

		// Assume newPod
		err := cache.Assume(scenario.newPod)
		if scenario.shouldSucceed && err != nil {
			t.Errorf("Test %q failed: Assume() returned error %v", name, err)
		}
		if !scenario.shouldSucceed && err == nil {
			t.Errorf("Test %q failed: Assume() returned success but expected error", name)
		}

		// Check that GetPod returns correct Pod
		expectedPod := scenario.newPod
		if !scenario.shouldSucceed {
			expectedPod = scenario.oldPod
		}
		if err := verifyPod(cache, getPodName(scenario.oldPod), expectedPod); err != nil {
			t.Errorf("Failed to GetPod() after initial update: %v", err)
		}
	}
}

func TestRestorePod(t *testing.T) {
	cache := NewPodAssumeCache(nil)
	internalCache, ok := cache.(*podAssumeCache).AssumeCache.(*assumeCache)
	if !ok {
		t.Fatalf("Failed to get internal cache")
	}

	oldPod := makePod("pod1", "5", "ns1")
	newPod := makePod("pod1", "5", "ns1")

	// Restore Pod that doesn't exist
	cache.Restore("nothing")

	// Add oldPod to cache
	internalCache.add(oldPod)
	if err := verifyPod(cache, getPodName(oldPod), oldPod); err != nil {
		t.Fatalf("Failed to GetPod() after initial update: %v", err)
	}

	// Restore Pod
	cache.Restore(getPodName(oldPod))
	if err := verifyPod(cache, getPodName(oldPod), oldPod); err != nil {
		t.Fatalf("Failed to GetPod() after initial restore: %v", err)
	}

	// Assume newPod
	if err := cache.Assume(newPod); err != nil {
		t.Fatalf("Assume() returned error %v", err)
	}
	if err := verifyPod(cache, getPodName(oldPod), newPod); err != nil {
		t.Fatalf("Failed to GetPod() after Assume: %v", err)
	}

	// Restore Pod
	cache.Restore(getPodName(oldPod))
	if err := verifyPod(cache, getPodName(oldPod), oldPod); err != nil {
		t.Fatalf("Failed to GetPod() after restore: %v", err)
	}
}

func TestAssumeUpdatePodCache(t *testing.T) {
	cache := NewPodAssumeCache(nil)
	internalCache, ok := cache.(*podAssumeCache).AssumeCache.(*assumeCache)
	if !ok {
		t.Fatalf("Failed to get internal cache")
	}

	podName := "test-pod0"
	podNamespace := "test-ns"

	// Add a Pod
	pod := makePod(podName, "1", podNamespace)
	internalCache.add(pod)
	if err := verifyPod(cache, getPodName(pod), pod); err != nil {
		t.Fatalf("failed to get Pod: %v", err)
	}

	// Assume Pod
	newPod := pod.DeepCopy()
	newPod.Annotations = map[string]string{
		"test": "test-node",
	}
	if err := cache.Assume(newPod); err != nil {
		t.Fatalf("failed to assume Pod: %v", err)
	}
	if err := verifyPod(cache, getPodName(pod), newPod); err != nil {
		t.Fatalf("failed to get Pod after assume: %v", err)
	}

	// Add old Pod
	internalCache.add(pod)
	if err := verifyPod(cache, getPodName(pod), newPod); err != nil {
		t.Fatalf("failed to get Pod after old Pod added: %v", err)
	}
}
