package gpuoversell

import (
	"fmt"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/devicesharing/runtime"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sync"
)

const GPUOversellStateKeyNamespace = "gpushare"

type GPUOversellNodeState struct {
	mu            sync.RWMutex
	caches        map[corev1.ResourceName]*NodeResource
	oversellRatio int64
}

func NewGPUOversellNodeState(ratio int64) *GPUOversellNodeState {
	return &GPUOversellNodeState{
		caches:        make(map[corev1.ResourceName]*NodeResource),
		oversellRatio: ratio,
	}
}

func (s *GPUOversellNodeState) GetNodeResource(resourceName corev1.ResourceName) *NodeResource {
	s.mu.Lock()
	defer s.mu.Unlock()
	cache, ok := s.caches[resourceName]
	if !ok {
		cache = NewNodeResourceCache(resourceName)
		s.caches[resourceName] = cache
	}
	return cache
}

func (s *GPUOversellNodeState) Clone() framework.StateData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	new := &GPUOversellNodeState{
		caches:        make(map[corev1.ResourceName]*NodeResource, len(s.caches)),
		oversellRatio: s.oversellRatio,
	}
	for r, nr := range s.caches {
		new.caches[r] = nr.Clone()
	}
	return new
}

func getGPUOversellNodeState(cycleState *framework.CycleState, nodeName string) (*GPUOversellNodeState, error) {
	key := getGPUOverSellNodeStateKey(nodeName)
	c, err := cycleState.Read(key)
	if err != nil {
		return nil, fmt.Errorf("error reading %q from cycleState: %v", key, err)
	}
	s, ok := c.(*GPUOversellNodeState)
	if !ok {
		return nil, fmt.Errorf("%+v  convert to GPUShareNodeState error", c)
	}
	return s, nil
}

func getGPUOverSellNodeStateKey(nodeName string) framework.StateKey {
	return framework.StateKey(fmt.Sprintf("%v/allocation/%v", GPUOversellStateKeyNamespace, nodeName))
}

func addPod(resourceNames []corev1.ResourceName, pod *corev1.Pod, state *GPUOversellNodeState) {
	// if pod is not requested target resources,skip it
	if !runtime.IsMyPod(pod, resourceNames...) {
		return
	}
	// if node name is null,skip it
	if pod.Spec.NodeName == "" {
		return
	}
	// if pod is completed,skip it
	if runtime.IsCompletedPod(pod) {
		return
	}
	// if not found allocation,skip to handle it
	for _, resouceName := range resourceNames {
		state.GetNodeResource(resouceName).AddPodResource(pod)
	}
}
