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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

type pendingPodInfo struct {
	uid               types.UID
	namespace         string
	name              string
	selectedQuotaName string
	frozenQuotas      sets.String
	pendingQuotas     sets.String
	processedQuotas   sets.String

	lastProcessedQuotas sets.String
}

func newPendingPodInfo(pod *corev1.Pod) *pendingPodInfo {
	pi := &pendingPodInfo{
		uid:             pod.UID,
		namespace:       pod.Namespace,
		name:            pod.Name,
		frozenQuotas:    sets.NewString(),
		pendingQuotas:   sets.NewString(),
		processedQuotas: sets.NewString(),
	}
	return pi
}

func (p *pendingPodInfo) clone() *pendingPodInfo {
	return &pendingPodInfo{
		uid:                 p.uid,
		namespace:           p.namespace,
		name:                p.name,
		selectedQuotaName:   p.selectedQuotaName,
		frozenQuotas:        sets.NewString(p.frozenQuotas.UnsortedList()...),
		pendingQuotas:       sets.NewString(p.pendingQuotas.UnsortedList()...),
		processedQuotas:     sets.NewString(p.processedQuotas.UnsortedList()...),
		lastProcessedQuotas: sets.NewString(p.lastProcessedQuotas.UnsortedList()...),
	}
}

func (p *pendingPodInfo) resetTrackState() {
	p.lastProcessedQuotas = p.processedQuotas
	p.frozenQuotas = sets.NewString()
	p.pendingQuotas = sets.NewString()
	p.processedQuotas = sets.NewString()
}
