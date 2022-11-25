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

package podconstraint

import (
	"sync"

	unischeduling "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

type PodConstraintCache struct {
	lock                       sync.RWMutex
	handle                     framework.Handle
	enableDefaultPodConstraint bool
	constraintToPodAllocations map[string]map[string]*podAllocation
	constraintStateCache       map[string]*TopologySpreadConstraintState
	allocSet                   sets.String
}

func newPodConstraintCache(handle framework.Handle, enableDefaultPodConstraint bool) *PodConstraintCache {
	return &PodConstraintCache{
		handle:                     handle,
		enableDefaultPodConstraint: enableDefaultPodConstraint,
		constraintToPodAllocations: make(map[string]map[string]*podAllocation),
		constraintStateCache:       make(map[string]*TopologySpreadConstraintState),
		allocSet:                   sets.NewString(),
	}
}

func getNamespacedName(namespace, name string) string {
	result := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	return result.String()
}

func (c *PodConstraintCache) SetPodConstraint(constraint *unischeduling.PodConstraint) {
	hasNewTopologyKey, _ := c.UpdateStateIfNeed(constraint, nil)
	if hasNewTopologyKey {
		// todo 这个还没有解决新的TopologyKey添加之后有可能出现的账本不匹配问题
		c.updateStateWithNewConstraint(getNamespacedName(constraint.Namespace, constraint.Name))
	}
}

func (c *PodConstraintCache) UpdateStateIfNeed(constraint *unischeduling.PodConstraint, spreadTypeRequired *bool) (
	hasNewTopologyKey bool, state *TopologySpreadConstraintState) {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.updateStateIfNeed(constraint, spreadTypeRequired)
}

func (c *PodConstraintCache) updateStateIfNeed(constraint *unischeduling.PodConstraint, spreadTypeRequired *bool) (
	hasNewTopologyKey bool, state *TopologySpreadConstraintState) {
	constraintKey := getNamespacedName(constraint.Namespace, constraint.Name)
	state = c.constraintStateCache[constraintKey]
	if state == nil {
		state = newTopologySpreadConstraintState(constraint, spreadTypeRequired)
		c.constraintStateCache[constraintKey] = state
	} else {
		hasNewTopologyKey = state.updateWithPodConstraint(constraint, spreadTypeRequired)
	}
	return
}

func (c *PodConstraintCache) updateStateWithNewConstraint(podConstraintKey string) {
	c.lock.Lock()
	cachedPodAllocations := c.constraintToPodAllocations[podConstraintKey]
	if len(cachedPodAllocations) == 0 {
		c.lock.Unlock()
		return
	}
	podAllocations := make(map[string]*podAllocation)
	for allocKey, cachedAllocation := range cachedPodAllocations {
		podAllocations[allocKey] = cachedAllocation
	}
	c.lock.Unlock()

	for _, allocation := range podAllocations {
		func() {
			if allocation == nil {
				return
			}
			node, err := c.handle.SharedInformerFactory().Core().V1().Nodes().Lister().Get(allocation.nodeName)
			if err != nil || node == nil {
				return
			}
			pod := allocationToPod(allocation)
			c.DeletePod(node, pod)
			c.AddPod(node, pod)
		}()
	}
}

func (c *PodConstraintCache) DelPodConstraint(constraint *unischeduling.PodConstraint) {
	c.lock.Lock()
	defer c.lock.Unlock()
	key := getNamespacedName(constraint.Namespace, constraint.Name)
	delete(c.constraintToPodAllocations, key)
	delete(c.constraintStateCache, key)
}

func (c *PodConstraintCache) AddPod(node *corev1.Node, pod *corev1.Pod) {
	if pod == nil {
		return
	}
	constraints := c.getWeightConstraints(pod)
	c.lock.Lock()
	defer c.lock.Unlock()
	c.addPod(node, pod, constraints)
}

func (c *PodConstraintCache) getWeightConstraints(pod *corev1.Pod) []extunified.WeightedPodConstraint {
	constraints := extunified.GetWeightedPodConstraints(pod)
	if len(constraints) != 0 {
		return constraints
	}
	spreadUnits := extunified.GetWeighedSpreadUnits(pod)
	if len(spreadUnits) == 0 || !c.enableDefaultPodConstraint {
		return constraints
	}
	for _, spreadUnit := range spreadUnits {
		constraintName := GetDefaultPodConstraintName(spreadUnit.Name)
		constraints = append(constraints, extunified.WeightedPodConstraint{
			Namespace: pod.Namespace,
			Name:      constraintName,
			Weight:    spreadUnit.Weight,
		})
	}
	return constraints
}

func (c *PodConstraintCache) addPod(node *corev1.Node, pod *corev1.Pod, constraints []extunified.WeightedPodConstraint) {
	if len(constraints) == 0 {
		return
	}
	podKey := getNamespacedName(pod.Namespace, pod.Name)
	if c.allocSet.Has(podKey) {
		return
	}
	c.allocSet.Insert(podKey)
	for _, constraint := range constraints {
		constraintKey := getNamespacedName(constraint.Namespace, constraint.Name)
		allocations, ok := c.constraintToPodAllocations[constraintKey]
		if !ok {
			allocations = make(map[string]*podAllocation)
			c.constraintToPodAllocations[constraintKey] = allocations
		}
		allocations[podKey] = podToAllocation(pod)
		constraintState := c.constraintStateCache[constraintKey]
		if constraintState == nil {
			if !IsConstraintNameDefault(constraint.Name) {
				continue
			}
			spreadRequiredType := extunified.IsSpreadTypeRequire(pod)
			podConstraint := BuildDefaultPodConstraint(constraint.Namespace, constraint.Name, spreadRequiredType)
			_, constraintState = c.updateStateIfNeed(podConstraint, &spreadRequiredType)
		}
		constraintState.Lock()
		constraintState.update(node, 1)
		constraintState.Unlock()
	}
}

func (c *PodConstraintCache) UpdatePod(node *corev1.Node, oldPod, newPod *corev1.Pod) {
	if oldPod == nil || newPod == nil {
		return
	}
	constraintsToDelete, constraintsToAdd := c.getConstraintsDiff(oldPod, newPod)
	if len(constraintsToDelete) == 0 && len(constraintsToAdd) == 0 {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	c.deletePod(node, oldPod, constraintsToDelete)
	c.addPod(node, newPod, constraintsToAdd)
}

func (c *PodConstraintCache) getConstraintsDiff(oldPod, newPod *corev1.Pod) (
	constraintsToDelete, constraintsToAdd []extunified.WeightedPodConstraint) {
	oldConstraints := c.getWeightConstraints(oldPod)
	oldConstraintKeys := sets.NewString()
	for _, oldConstraint := range oldConstraints {
		oldConstraintKeys.Insert(getNamespacedName(oldConstraint.Namespace, oldConstraint.Name))
	}
	newConstraints := c.getWeightConstraints(newPod)
	newConstraintKeys := sets.NewString()
	for _, newConstraint := range newConstraints {
		newConstraintKeys.Insert(getNamespacedName(newConstraint.Namespace, newConstraint.Name))
	}
	for _, oldConstraint := range oldConstraints {
		if newConstraintKeys.Has(getNamespacedName(oldConstraint.Namespace, oldConstraint.Name)) {
			continue
		}
		constraintsToDelete = append(constraintsToDelete, oldConstraint)
	}
	for _, newConstraint := range newConstraints {
		if oldConstraintKeys.Has(getNamespacedName(newConstraint.Namespace, newConstraint.Name)) {
			continue
		}
		constraintsToAdd = append(constraintsToAdd, newConstraint)
	}
	return
}

func (c *PodConstraintCache) DeletePod(node *corev1.Node, pod *corev1.Pod) {
	if pod == nil {
		return
	}
	constraints := c.getWeightConstraints(pod)
	c.lock.Lock()
	defer c.lock.Unlock()
	c.deletePod(node, pod, constraints)
}

func (c *PodConstraintCache) deletePod(node *corev1.Node, pod *corev1.Pod, constraints []extunified.WeightedPodConstraint) {
	if len(constraints) == 0 {
		return
	}
	podKey := getNamespacedName(pod.Namespace, pod.Name)
	if !c.allocSet.Has(podKey) {
		return
	}
	c.allocSet.Delete(podKey)
	for _, constraint := range constraints {
		constraintKey := getNamespacedName(constraint.Namespace, constraint.Name)

		cachedAllocation := c.constraintToPodAllocations[constraintKey]
		delete(cachedAllocation, podKey)
		if len(cachedAllocation) == 0 {
			delete(c.constraintToPodAllocations, constraintKey)
		}

		constraintState := c.constraintStateCache[constraintKey]
		if constraintState != nil {
			needDelete := false
			constraintState.Lock()
			constraintState.update(node, -1)
			if constraintState.DefaultPodConstraint {
				if len(constraintState.TpPairToMatchNum) == 0 {
					needDelete = true
				}
			}
			constraintState.Unlock()
			if needDelete {
				delete(c.constraintStateCache, constraintKey)
			}
		}
	}
}

func (c *PodConstraintCache) GetState(constraintKey string) *TopologySpreadConstraintState {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.constraintStateCache[constraintKey]
}

func (c *PodConstraintCache) GetDefaultPodConstraintState(namespace string, spreadUnit string) *TopologySpreadConstraintState {
	defaultPodConstraintName := GetDefaultPodConstraintName(spreadUnit)
	constraintKey := getNamespacedName(namespace, defaultPodConstraintName)
	return c.GetState(constraintKey)
}
