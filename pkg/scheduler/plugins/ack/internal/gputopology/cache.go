package gputopology

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager/bitmask"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/elemset"
)

var ErrNotAvailableGPUAllocation = errors.New("not available gpu topology hint was found under the given conditions")

type GPUTopologyHint struct {
	Affinity     bitmask.BitMask
	MinBandwidth float32
}

// the container index in a pod
type ContainerIndex int

// TopologyGPUs indicates a gpu group
type TopologyGPUs []string

// Difference is used to get the diffrence of d
// eg: t is [1,2,3,4] and d is [1,3],then return [2,4]
func (t TopologyGPUs) Difference(d TopologyGPUs) TopologyGPUs {
	src := elemset.NewElemSet(t...)
	dst := elemset.NewElemSet(d...)
	return src.Difference(dst).ToSlice()
}

// Clone the GPUs
func (t TopologyGPUs) Clone() TopologyGPUs {
	src := elemset.NewElemSet(t...)
	return src.Clone().ToSlice()
}

// ToPologyGPUs to slice
func (t TopologyGPUs) ToSlice() []string {
	return t
}

// union the t and d
func (t TopologyGPUs) Union(d TopologyGPUs) TopologyGPUs {
	src := elemset.NewElemSet(t...)
	dst := elemset.NewElemSet(d...)
	return src.Union(dst).ToSlice()
}

// return the size of t
func (t TopologyGPUs) Size() int {
	return len(t)
}

// SchedulePhase indicates which stage the pod is in scheduling
type SchedulePhase string

const (
	// SchedulePhaseAssumed indicates the topology group has been pre allocated gpus
	// but the pod is not scheduling
	SchedulePhaseAssumed SchedulePhase = "assumed"
	// SchedulePhaseFinishBind indicates the pod has been finished binding operation
	SchedulePhaseFinishBind SchedulePhase = "finishBind"
	// SchedulePhaseReserve indicates the pod has been processed by reservePlugins
	SchedulePhaseReserve SchedulePhase = "reserve"
)

// PodTopologyAllocation indicates the gpu topology allocation of pod
type PodTopologyAllocation struct {
	// the pod uid
	PodUID apitypes.UID
	// node that pod will be running on or is running on
	NodeName string
	// gpus allocated to the pod,value of container env CUDA_VISIBLE_DEVICES
	AllocatedGPUs map[ContainerIndex]TopologyGPUs
	// gpus the pod can visible,value of container env NVIDIA_VISIBLE_DEVICES
	VisibleGPUs TopologyGPUs
	// the time which assume gpus to pod
	assumeTime *time.Time
	// the allocation is valid in the ttl duration
	ttl *time.Duration
	// indicates the schedule phase
	phase SchedulePhase
}

func NewPodTopologyAllocation(podUID apitypes.UID) *PodTopologyAllocation {
	now := time.Now()
	return &PodTopologyAllocation{
		PodUID:        podUID,
		assumeTime:    &now,
		AllocatedGPUs: map[ContainerIndex]TopologyGPUs{},
		VisibleGPUs:   TopologyGPUs{},
		phase:         SchedulePhaseAssumed,
	}
}

// clone the pod topology allocation
func (p *PodTopologyAllocation) Clone() *PodTopologyAllocation {
	c := &PodTopologyAllocation{
		PodUID:        p.PodUID,
		NodeName:      p.NodeName,
		AllocatedGPUs: map[ContainerIndex]TopologyGPUs{},
		VisibleGPUs:   TopologyGPUs{},
		phase:         p.phase,
		assumeTime:    p.assumeTime,
		ttl:           p.ttl,
	}
	for index, containerGPUs := range p.AllocatedGPUs {
		gpus := []string{}
		gpus = append(gpus, containerGPUs...)
		c.AllocatedGPUs[index] = gpus
	}
	c.VisibleGPUs = append(c.VisibleGPUs, p.VisibleGPUs...)
	return c
}

// set the schedule phase
func (p *PodTopologyAllocation) SetPhase(phase SchedulePhase) *PodTopologyAllocation {
	p.phase = phase
	return p
}

// return the allocated gpus for the pod
func (p *PodTopologyAllocation) GetAllocatedGPUs() TopologyGPUs {
	gpus := []string{}
	for _, containerGPUs := range p.AllocatedGPUs {
		gpus = append(gpus, containerGPUs...)
	}
	return gpus
}

// return the ttl
func (p *PodTopologyAllocation) GetTTL() int64 {
	if p.ttl == nil {
		return 0
	}
	return int64(*p.ttl)
}

// set ttl
func (p *PodTopologyAllocation) SetTTL(ttl int) *PodTopologyAllocation {
	if ttl <= 0 {
		p.ttl = nil
		return p
	}
	dur := time.Duration(ttl * int(time.Second))
	p.ttl = &dur
	return p
}

// set assume time
func (p *PodTopologyAllocation) SetAssumeTime(timestamp int64) *PodTopologyAllocation {
	if timestamp == 0 {
		return p
	}
	assumeAt := time.Unix(0, timestamp)
	p.assumeTime = &assumeAt
	return p
}

func (p *PodTopologyAllocation) GetAssumeTime() int64 {
	return p.assumeTime.UnixNano()
}

// check the pod topology allocation is expired or not
func (p *PodTopologyAllocation) IsExpired() bool {
	if p.phase != SchedulePhaseAssumed || p.ttl == nil {
		return false
	}
	return p.assumeTime.Add(*p.ttl).Before(time.Now())
}

func (p *PodTopologyAllocation) GetPhase() SchedulePhase {
	return p.phase
}

type NodeGPUTopologyCache struct {
	// bandwidth matrix indicates the preffered combination of gpus
	bandwidthMatrix [][]float32
	// total gpus of the node
	allGPUs TopologyGPUs
	// unhealthy gpus of the node
	unhealthyGPUs TopologyGPUs
	// topology pod allocations of node
	pods map[apitypes.UID]*PodTopologyAllocation
	// preffered gpu combination hint
	hints []GPUTopologyHint
	lock  *sync.RWMutex
}

func NewNodeGPUTopologyCache() *NodeGPUTopologyCache {
	return &NodeGPUTopologyCache{
		bandwidthMatrix: [][]float32{},
		allGPUs:         TopologyGPUs{},
		unhealthyGPUs:   TopologyGPUs{},
		pods:            map[apitypes.UID]*PodTopologyAllocation{},
		hints:           []GPUTopologyHint{},
		lock:            new(sync.RWMutex),
	}
}

// Print the node cache
func (g *NodeGPUTopologyCache) String(showBandwidth bool) string {
	g.lock.RLock()
	defer g.lock.RUnlock()
	type NodeCache struct {
		BandwidthMatrix [][]float32               `json:"BandwidthMatrix,omitempty"`
		AllGPUs         []string                  `json:"AllGPUs"`
		UnhealthyGPUs   []string                  `json:"UnhealthyGPUs"`
		Pods            map[apitypes.UID][]string `json:"Pods"`
		AvailableGPUs   []string                  `json:"AvailableGPUs"`
	}
	pods := map[apitypes.UID][]string{}
	for uid, pod := range g.pods {
		if len(pod.AllocatedGPUs) == 0 || pod.IsExpired() {
			continue
		}
		pods[uid] = pod.GetAllocatedGPUs()
	}
	avaiableGPUs := g.getAvailableGPUs()
	c := &NodeCache{
		AllGPUs:       g.allGPUs,
		UnhealthyGPUs: g.unhealthyGPUs,
		Pods:          pods,
		AvailableGPUs: avaiableGPUs,
	}
	if showBandwidth {
		c.BandwidthMatrix = g.bandwidthMatrix
	}
	data, err := json.Marshal(c)
	if err != nil {
		klog.Errorf("failed to marshal node gputopology cache: %v", err)
		return ""
	}
	return string(data)
}

// clone the node cache
func (g *NodeGPUTopologyCache) Clone() *NodeGPUTopologyCache {
	g.lock.RLock()
	defer g.lock.RUnlock()
	result := NewNodeGPUTopologyCache()
	for i := 0; i < len(g.bandwidthMatrix); i++ {
		row := []float32{}
		for j := 0; j < len(g.bandwidthMatrix[i]); j++ {
			row = append(row, g.bandwidthMatrix[i][j])
		}
		result.bandwidthMatrix = append(result.bandwidthMatrix, row)
	}
	result.bandwidthMatrix = g.bandwidthMatrix
	result.hints = append(result.hints, g.hints...)
	result.allGPUs = g.allGPUs.Clone()
	result.unhealthyGPUs = g.unhealthyGPUs.Clone()
	for id, pod := range g.pods {
		result.pods[id] = pod.Clone()
	}
	return result
}

// set BandwidthMatrix and get topology hints
func (g *NodeGPUTopologyCache) SetBandwidthMatrix(bandwidthMatrix [][]float32) error {
	g.lock.Lock()
	defer g.lock.Unlock()
	if err := checkBandwidthMatrixIsValid(bandwidthMatrix); err != nil {
		return err
	}
	hints := []GPUTopologyHint{}
	gpus := []int{}
	// get gpu index ids
	for i := 0; i < len(bandwidthMatrix); i++ {
		gpus = append(gpus, i)
	}
	errs := []string{}
	// generate bandwidth topology hints
	iterateGPUS(gpus, func(gpuCombination []int) {
		bitMask, err := bitmask.NewBitMask(gpuCombination...)
		if err != nil {
			errs = append(errs, err.Error())
			return
		}
		hint := GPUTopologyHint{
			Affinity:     bitMask,
			MinBandwidth: math.MaxFloat32,
		}
		if len(gpuCombination) == 1 {
			hint.MinBandwidth = bandwidthMatrix[gpuCombination[0]][gpuCombination[0]]
		}
		getGPULinkMinBandwidth(gpuCombination, func(gpus []int) {
			if gpus[0] < len(bandwidthMatrix) && gpus[1] < len(bandwidthMatrix[gpus[0]]) && bandwidthMatrix[gpus[0]][gpus[1]] < hint.MinBandwidth {
				hint.MinBandwidth = bandwidthMatrix[gpus[0]][gpus[1]]
			}
		})
		hints = append(hints, hint)
	})
	g.hints = hints
	g.bandwidthMatrix = bandwidthMatrix
	return nil
}

// set node gpus
func (g *NodeGPUTopologyCache) SetGPUs(allGPUs, unhealthyGPUs TopologyGPUs) *NodeGPUTopologyCache {
	g.lock.Lock()
	defer g.lock.Unlock()
	if len(allGPUs) != 0 {
		g.allGPUs = allGPUs.Clone()
	}
	if len(unhealthyGPUs) != 0 {
		g.unhealthyGPUs = unhealthyGPUs.Clone()
	}
	return g
}

// get available GPUs
func (g *NodeGPUTopologyCache) GetAvailableGPUs() TopologyGPUs {
	g.lock.RLock()
	defer g.lock.RUnlock()
	return g.getAvailableGPUs()
}

func (g *NodeGPUTopologyCache) GetTopologyHints() []GPUTopologyHint {
	g.lock.RLock()
	defer g.lock.RUnlock()
	return g.hints
}

func (g *NodeGPUTopologyCache) getAvailableGPUs() TopologyGPUs {
	if g.allGPUs == nil {
		return []string{}
	}
	totalGPUs := g.allGPUs.Clone()
	allocatedGPUs := TopologyGPUs{}
	for _, pod := range g.pods {
		if len(pod.AllocatedGPUs) == 0 {
			continue
		}
		if pod.IsExpired() {
			continue
		}
		allocatedGPUs = allocatedGPUs.Union(pod.GetAllocatedGPUs())
	}
	return totalGPUs.Difference(g.unhealthyGPUs).Difference(allocatedGPUs).ToSlice()
}

/*
func (g *NodeGPUTopologyCache) GetAvailableGPUCount() int {
	g.lock.RLock()
	defer g.lock.RUnlock()
	if g.allGPUs == nil {
		return 0
	}
	totalGPUs := g.allGPUs.Clone()
	allocatedGPUCount := 0
	for _, pod := range g.pods {
		if len(pod.AllocatedGPUs) == 0 {
			continue
		}
		if pod.IsExpired() {
			continue
		}
		allocatedGPUCount += len(pod.GetAllocatedGPUs())
	}
	availableCount := totalGPUs.Size() - g.unhealthyGPUs.Size() - allocatedGPUCount
	if availableCount < 0 {
		availableCount = 0
	}
	return availableCount
}
*/

func (g *NodeGPUTopologyCache) RemovePod(podUID apitypes.UID) {
	g.lock.Lock()
	defer g.lock.Unlock()
	delete(g.pods, podUID)
}

func (g *NodeGPUTopologyCache) AddPod(podAllocations ...*PodTopologyAllocation) {
	g.lock.Lock()
	defer g.lock.Unlock()
	for _, pod := range podAllocations {
		g.pods[pod.PodUID] = pod.Clone()
	}
}

func (g *NodeGPUTopologyCache) GetPod(podUID apitypes.UID) *PodTopologyAllocation {
	g.lock.RLock()
	defer g.lock.RUnlock()
	allocation, ok := g.pods[podUID]
	if !ok {
		return nil
	}
	return allocation.Clone()
}

func (g *NodeGPUTopologyCache) SetPodPhase(podUID apitypes.UID, phase SchedulePhase) error {
	g.lock.Lock()
	defer g.lock.Unlock()
	_, ok := g.pods[podUID]
	if !ok {
		return fmt.Errorf("not found PodInfo in node gpu topology cache")
	}
	g.pods[podUID].SetPhase(phase)
	return nil
}

func (g *NodeGPUTopologyCache) PodIsCached(podUID apitypes.UID) bool {
	g.lock.RLock()
	defer g.lock.RUnlock()
	_, ok := g.pods[podUID]
	return ok
}

func (g *NodeGPUTopologyCache) CleanExpiredPods() {
	g.lock.Lock()
	defer g.lock.Unlock()
	for id, podInfo := range g.pods {
		if podInfo.IsExpired() {
			delete(g.pods, id)
		}
	}
}

func (g *NodeGPUTopologyCache) Reset() {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.allGPUs = TopologyGPUs{}
	g.unhealthyGPUs = TopologyGPUs{}
	g.hints = []GPUTopologyHint{}
	g.bandwidthMatrix = [][]float32{}
}
