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

package util

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	watchUtil "k8s.io/client-go/tools/watch"

	"github.com/koordinator-sh/koordinator/test/e2e/framework"
)

const (
	// PodContainerStartTimeout pod container start time out
	PodContainerStartTimeout = 5 * time.Minute

	// Poll How often to Poll pods, nodes and claims.
	Poll = 2 * time.Second

	// PodDeleteTimeout pod delete time out
	PodDeleteTimeout = 5 * time.Minute
)

// DeletePod delete pod by using k8s api  and with default delete pod timeout
func DeletePod(client clientset.Interface, pod *v1.Pod) error {
	return DeletePodWithTimeout(client, pod, PodDeleteTimeout)
}

func DeletePodWithTimeout(client clientset.Interface, pod *v1.Pod, timeout time.Duration) error {
	err := client.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, *metav1.NewDeleteOptions(5))
	if err != nil {
		return err
	}

	t := time.Now()
	for {
		_, err := client.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
		if err != nil && strings.Contains(err.Error(), "not found") {
			framework.Logf("pod %s has been removed", pod.Name)
			return nil
		}
		if time.Since(t) >= timeout {
			return fmt.Errorf("Gave up waiting for pod %s is removed after %v seconds",
				pod.Name, time.Since(t).Seconds())
		}
		framework.Logf("Retrying to check whether pod %s is removed", pod.Name)
		time.Sleep(5 * time.Second)
	}
}

// byFirstTimestamp sorts a slice of events by first timestamp, using their involvedObject's name as a tie breaker.
type byFirstTimestamp []v1.Event

func (o byFirstTimestamp) Len() int      { return len(o) }
func (o byFirstTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o byFirstTimestamp) Less(i, j int) bool {
	if o[i].FirstTimestamp.Equal(&o[j].FirstTimestamp) {
		return o[i].InvolvedObject.Name < o[j].InvolvedObject.Name
	}
	return o[i].FirstTimestamp.Before(&o[j].FirstTimestamp)
}

// EventsLogger is an event log helper
type EventsLogger struct {
	lastLogTime time.Time
}

// NewEventsLogger create a events logger
func NewEventsLogger() *EventsLogger {
	return &EventsLogger{lastLogTime: time.Now()}
}

// Log print events of pod
func (l *EventsLogger) Log(client clientset.Interface, pod *v1.Pod) {
	if time.Since(l.lastLogTime) > 30*time.Second {
		events, err := client.CoreV1().Events(pod.Namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			framework.Logf("List events namespace %v failed, err: %v", pod.Namespace, err)
			return
		}
		sortedEvents := events.Items
		if len(sortedEvents) > 1 {
			sort.Sort(byFirstTimestamp(sortedEvents))
		}
		var recentEvents []v1.Event
		for i := len(sortedEvents) - 1; i >= 0; i-- {
			if sortedEvents[i].InvolvedObject.Name == pod.Name {
				recentEvents = append(recentEvents, sortedEvents[i])
				if len(recentEvents) > 12 {
					break
				}
			}
		}
		for i := len(recentEvents) - 1; i >= 0; i-- {
			e := recentEvents[i]
			framework.Logf("At %v ago - event for %v: %v %v: %v", time.Since(e.FirstTimestamp.Time), e.InvolvedObject.Name, e.Source, e.Reason, e.Message)

		}
		l.lastLogTime = time.Now()
	}
}

// WaitTimeoutForPodStatus check whether the pod status is same as expected status within the timeout.
func WaitTimeoutForPodStatus(client clientset.Interface, pod *v1.Pod, expectedStatus v1.PodPhase, timeout time.Duration) error {
	watcher, err := client.CoreV1().Pods(pod.Namespace).Watch(context.TODO(), metav1.SingleObject(pod.ObjectMeta))
	defer watcher.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	eventLogger := NewEventsLogger()
	_, err = watchUtil.UntilWithoutRetry(ctx, watcher,
		func(event watch.Event) (bool, error) {
			switch pod := event.Object.(type) {
			case *v1.Pod:
				framework.Logf("pod %s status phase is %v, reason %v, message %v", pod.Name, pod.Status.Phase, pod.Status.Reason, pod.Status.Message)
				if pod.Status.Phase == expectedStatus {
					return true, nil
				}
				eventLogger.Log(client, pod)
			}
			return false, nil
		})
	if err != nil {
		p, e := client.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
		if e == nil {
			framework.Logf("pod %s status phase is %v", p.Name, p.Status.Phase)
			if p.Status.Phase == expectedStatus {
				return nil
			}
			eventLogger.Log(client, pod)
			return fmt.Errorf("pod %s status phase is %v, expected %v", p.Name, p.Status.Phase, expectedStatus)
		}
	}
	return err
}
