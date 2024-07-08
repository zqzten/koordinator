package gpuoversell

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/devicesharing/runtime"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const GPUOversellStateKeyNamespace = "gpuoversell"

type NodeCache struct {
	mu        sync.RWMutex
	nodeInfos map[string]*GPUOversellNodeState
}

func newNodeCache() *NodeCache {
	return &NodeCache{
		nodeInfos: make(map[string]*GPUOversellNodeState),
	}
}

func (nc *NodeCache) NodeIsInCache(nodeName string) bool {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	_, ok := nc.nodeInfos[nodeName]
	return ok
}

func (nc *NodeCache) GetNodeState(nodeName string) *GPUOversellNodeState {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	ns, ok := nc.nodeInfos[nodeName]
	if ok {
		return ns
	}
	ns = NewGPUOversellNodeState()
	nc.nodeInfos[nodeName] = ns
	return ns
}

type GPUOversellNodeState struct {
	mu            sync.RWMutex
	caches        map[corev1.ResourceName]*NodeResource
	oversellRatio int
	devices       int
}

func NewGPUOversellNodeState() *GPUOversellNodeState {
	s := &GPUOversellNodeState{
		caches: make(map[corev1.ResourceName]*NodeResource),
	}
	return s
}

func (s *GPUOversellNodeState) getNodeResourceWithoutLock(resourceName corev1.ResourceName) *NodeResource {
	cache, ok := s.caches[resourceName]
	if !ok {
		cache = NewNodeResourceCache(resourceName)
		s.caches[resourceName] = cache
	}
	return cache
}

func (s *GPUOversellNodeState) GetNodeResource(resourceName corev1.ResourceName) *NodeResource {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getNodeResourceWithoutLock(resourceName)
}

func (s *GPUOversellNodeState) CloneState() *GPUOversellNodeState {
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

func (s *GPUOversellNodeState) SetOversellRatio(ratio int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.oversellRatio = ratio
}

func (s *GPUOversellNodeState) GetOversellRatio() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.oversellRatio
}

func (s *GPUOversellNodeState) SetDevices(devices int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices = devices
}

func (s *GPUOversellNodeState) GetDevices() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.devices
}

func (s *GPUOversellNodeState) Reset(node *corev1.Node, resources []corev1.ResourceName) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices = getNodeGPUCount(node)
	s.oversellRatio = getOversellRatio(node)
	resMap := runtime.GetNodeResourcesByNames(node, resources...)
	for k, v := range resMap {
		s.getNodeResourceWithoutLock(k).SetTotal(v)
	}
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

func getOversellRatio(node *corev1.Node) int {
	if node == nil {
		return 1
	}
	v, ok := node.Labels["ack.node.gpu.schedule.oversell"]
	if !ok {
		return 1
	}
	r, err := strconv.Atoi(v)
	if err != nil {
		return 2
	}
	return r
}

func getPodStateKey() framework.StateKey {
	return framework.StateKey(fmt.Sprintf("%v/podstate", GPUOversellStateKeyNamespace))
}

func GetGPUOversellPodState(cycleState *framework.CycleState) (*GPUOversellPodState, error) {
	key := getPodStateKey()
	c, err := cycleState.Read(key)
	if err != nil {
		return nil, fmt.Errorf("error reading %q from cycleState: %v", key, err)
	}
	s, ok := c.(*GPUOversellPodState)
	if !ok {
		return nil, fmt.Errorf("%+v  convert to GPUOversellPodState error", c)
	}
	return s, nil
}

func CreateAndSaveGPUOversellPodState(state *framework.CycleState, RequestGPUs int, RequestResources map[corev1.ResourceName]int) {
	key := getPodStateKey()
	s := &GPUOversellPodState{
		RequestGPUs:      RequestGPUs,
		RequestResources: RequestResources,
	}
	state.Write(key, s)
}

type GPUOversellPodState struct {
	RequestGPUs      int
	RequestResources map[corev1.ResourceName]int
}

// Clone the pod state.
func (s *GPUOversellPodState) Clone() framework.StateData {
	ns := &GPUOversellPodState{
		RequestResources: make(map[corev1.ResourceName]int, len(s.RequestResources)),
		RequestGPUs:      s.RequestGPUs,
	}
	for k, v := range s.RequestResources {
		ns.RequestResources[k] = v
	}
	return ns
}
