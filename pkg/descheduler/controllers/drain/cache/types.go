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

package cache

import (
	"sync"

	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type NodeInfoMap map[v1alpha1.DrainNodePhase][]*NodeInfo

type Cache interface {
	GetNodeInfo(nodeName string) *NodeInfo
	GetPods(nodeName string) []*PodInfo
}

type CacheEventHandler interface {
	NodeEventHandler
	PodEventHandler
	ReservationEventHandler
}

type NodeEventHandler interface {
	OnNodeAdd(obj interface{})
	OnNodeUpdate(oldObj, newObj interface{})
	OnNodeDelete(obj interface{})
}

type PodEventHandler interface {
	OnPodAdd(obj interface{})
	OnPodUpdate(oldObj, newObj interface{})
	OnPodDelete(obj interface{})
}

type ReservationEventHandler interface {
	OnReservationAdd(obj interface{})
	OnReservationUpdate(oldObj, newObj interface{})
	OnReservationDelete(obj interface{})
}

type NodeInfo struct {
	sync.RWMutex
	Name        string
	Allocatable corev1.ResourceList
	Free        corev1.ResourceList
	Score       int64
	Pods        map[types.UID]*PodInfo
	Reservation map[string]struct{}
	Drainable   bool
}

type PodInfo struct {
	types.NamespacedName
	UID        types.UID
	Request    corev1.ResourceList
	Ignore     bool
	Migratable bool
	Pod        *corev1.Pod
}
