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

package nodesorting

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

type Node struct {
	Name        string
	allocatable corev1.ResourceList
	requested   corev1.ResourceList
	pods        sets.String
	lock        sync.RWMutex
}

func NewNode(node *corev1.Node) *Node {
	return &Node{
		Name:        node.Name,
		allocatable: node.Status.Allocatable,
		pods:        sets.NewString(),
	}
}

func (n *Node) GetResourceUsageShares() map[string]float64 {
	n.lock.RLock()
	defer n.lock.RUnlock()
	res := make(map[string]float64)
	if n.allocatable == nil {
		// no resources present, so no usage
		return res
	}
	for k, v := range n.allocatable {
		requested := n.requested[k]
		available := v.MilliValue() - requested.MilliValue()
		if available < 0 {
			available = 0
		}
		res[string(k)] = float64(1) - (float64(available) / float64(v.MilliValue()))
	}
	return res
}

func (n *Node) Update(node *corev1.Node) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.allocatable = node.Status.Allocatable
}

func (n *Node) AddPod(pod *corev1.Pod) {
	n.lock.Lock()
	defer n.lock.Unlock()

	podKey, err := framework.GetPodKey(pod)
	if err != nil {
		return
	}
	if n.pods.Has(podKey) {
		return
	}

	requests := resource.PodRequests(pod, resource.PodResourcesOptions{})
	n.requested = quotav1.Add(n.requested, requests)
	n.pods.Insert(podKey)
}

func (n *Node) UpdatePod(oldPod, newPod *corev1.Pod) {
	n.lock.Lock()
	defer n.lock.Unlock()

	podKey, err := framework.GetPodKey(newPod)
	if err != nil {
		return
	}
	if !n.pods.Has(podKey) {
		return
	}

	if oldPod != nil {
		requests := resource.PodRequests(oldPod, resource.PodResourcesOptions{})
		n.requested = quotav1.SubtractWithNonNegativeResult(n.requested, requests)
	}

	requests := resource.PodRequests(newPod, resource.PodResourcesOptions{})
	n.requested = quotav1.Add(n.requested, requests)
}

func (n *Node) DeletePod(pod *corev1.Pod) {
	n.lock.Lock()
	defer n.lock.Unlock()

	podKey, err := framework.GetPodKey(pod)
	if err != nil {
		return
	}
	if !n.pods.Has(podKey) {
		return
	}

	requests := resource.PodRequests(pod, resource.PodResourcesOptions{})
	n.requested = quotav1.SubtractWithNonNegativeResult(n.requested, requests)
}
