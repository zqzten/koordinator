package gputopology

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/frameworkcache"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/gputopology"
)

var _ framework.PreFilterPlugin = &GPUTopology{}
var _ framework.ReservePlugin = &GPUTopology{}
var _ framework.BindPlugin = &GPUTopology{}
var _ framework.PreScorePlugin = &GPUTopology{}
var _ framework.ScorePlugin = &GPUTopology{}

type GPUTopology struct {
	FrameworkHandle framework.Handle
	PodLister       corelisters.PodLister
	frameworkCache  *frameworkcache.FrameworkCache
	cache           *GPUTopologyCache
}

var GPUTopologyName = "gputopology"

// Name returns name of the plugin. It is used in logs, etc.
func (gt *GPUTopology) Name() string {
	return GPUTopologyName
}

// New initializes a new plugin and returns it.
func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	podLister := handle.SharedInformerFactory().Core().V1().Pods().Lister()
	frameworkCache := frameworkcache.GetFrameworkCache(nil, handle)
	gt := &GPUTopology{
		FrameworkHandle: handle,
		PodLister:       podLister,
		frameworkCache:  frameworkCache,
		cache:           NewTopologyCache(frameworkCache),
	}
	if err := gt.Init(); err != nil {
		return nil, err
	}
	return gt, nil
}

func (gt *GPUTopology) Init() error {
	// 2.builds node informations
	configmaps, err := gt.FrameworkHandle.SharedInformerFactory().Core().V1().ConfigMaps().Lister().ConfigMaps("kube-system").List(labels.Everything())
	if err != nil {
		return err
	}
	pods, err := gt.FrameworkHandle.SharedInformerFactory().Core().V1().Pods().Lister().List(labels.Everything())
	if err != nil {
		return err
	}
	addConfigMap := func(configmap *v1.ConfigMap) {
		addConfigMap(configmap, gt.frameworkCache)
	}
	addPod := func(pod *v1.Pod) {
		addPod(GPUTopologyResourceName, pod, gt.cache)
	}
	// build node resource cache
	for _, configmap := range configmaps {
		addConfigMap(configmap)
	}
	// recover use resources from pod
	for _, pod := range pods {
		addPod(pod)
	}
	eventFuncs := []interface{}{
		// add pod event handler
		addPod,
		// add configmap event handler
		addConfigMap,
		// update pod event handler
		func(oldPod *v1.Pod, newPod *v1.Pod) {
			updatePod(GPUTopologyResourceName, oldPod, newPod, gt.cache)
		},
		// remove pod event handler
		func(pod *v1.Pod, _ bool) {
			removePod(GPUTopologyResourceName, pod, gt.cache)
		},
		// update node event handler
		func(oldNode *v1.Node, newNode *v1.Node) {
			updateNode(oldNode, newNode, gt.FrameworkHandle.ClientSet())
		},
		func(old, cur *v1.ConfigMap) {
			updateConfigmap(old, cur, gt.frameworkCache)
		},
		func(configmap *v1.ConfigMap, _ bool) {
			removeConfigmap(configmap, gt.frameworkCache)
		},
	}
	for _, eventFunc := range eventFuncs {
		if err := gt.frameworkCache.AddEventFunc(eventFunc); err != nil {
			return err
		}
	}
	return nil
}

/*
PreFilter is used to prepare for filtering, including:
1.Check whether the pod topology group and the replica are legal
2.Check whether the ttl set by the pod is legal
3.Take the pods that is in the same group as the current pod and has not yet been scheduled, and save them in cycleState for later plugin use
4.Try to obtain the PodTopologyAllocation of the currently scheduling pod from the GPUTopologyCache. If it exists and has not expired,
it means that the topology group of current pod has been pre-allocated with GPUs, use this allocation first.
*/

// PreFilter用于进行过滤前的准备工作，包括：
// 1.检查pod的topology group和replica数是否合法
// 2.检查pod设置的ttl是否合法
// 3.获取与该pod同组的且还没调度的pod，保存到cycleState中，供后面插件使用
// 4.尝试从TopologyCache中获取当前调度的pod的PodTopologyAllocation，如果存在且未过期，说明该pod所在的组已经进行了预分配并给该pod分配了GPU，那么优先使用这个分配方案
func (gt *GPUTopology) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
	if !IsMyPod(pod) {
		return framework.NewStatus(framework.Success, "")
	}
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	// get topology group
	_, groupName, replica, err := getPodGroupAndReplica(pod)
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	// check ttl label is ok
	_, err = getTTLFromLabel(pod)
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	podCount := 0
	topologyGroupPods := []*v1.Pod{}
	err = getTopologyGroupPods(pod, gt.PodLister, gt.cache, func(p *v1.Pod, allocatin *gputopology.PodTopologyAllocation) {
		podCount++
		// 被调度的pod包含已经binding的pod以及被reserve的pod
		// The scheduled pod includes the bound pod and the reserved pod
		if allocatin != nil && (allocatin.GetPhase() == gputopology.SchedulePhaseReserve || allocatin.GetPhase() == gputopology.SchedulePhaseFinishBind) {
			return
		}
		topologyGroupPods = append(topologyGroupPods, p)
	})
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}
	if replica != podCount {
		errMsg := fmt.Sprintf("Number(%v) of of GPUTopologyGroup(%v) pods is not equal to label replica(%v)", podCount, groupName, replica)
		return framework.NewStatus(framework.Unschedulable, errMsg)
	}
	// if found pre allocation for the pod,check it
	topologyAllocations := gt.cache.GetTopologyPodAllocations(pod.UID)
	var topologyAllocation *gputopology.PodTopologyAllocation
	if len(topologyAllocations) != 0 {
		for podUID, allocation := range topologyAllocations {
			if podUID != pod.UID {
				continue
			}
			if allocation.IsExpired() {
				klog.V(5).Infof("found pre allocation(SuggestHost: %v) for pod %v,but it is expired", allocation.NodeName, podFullName)
				continue
			}
			topologyAllocation = allocation
		}
	}
	podState := &PodTopologyState{
		PodUID:            pod.UID,
		GroupName:         groupName,
		GroupReplica:      replica,
		TopologyGroupPods: topologyGroupPods,
	}
	if topologyAllocation != nil {
		podState.Allocation = topologyAllocation
		klog.V(5).Infof("found pre allocation(SuggestHost: %v) for pod %v/%v and store it in cycleState", podState.Allocation.NodeName, pod.Namespace, pod.Name)
	}
	SetPodTopologyState(state, podState)
	return framework.NewStatus(framework.Success, "")
}

func (gt *GPUTopology) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

/*
Run filters on each node, including:
1.Get unscheduled pods in the same group from cycleState
2.Remove the GPUs allocated for these unscheduled pods from the node's GPUTopologyCache
3.Determine whether the number of GPUs available on the current node is greater than the current pod request value
*/

// 对每个节点进行过滤操作，包括：
// 1.从cycleState中获取同组未调度的pod
// 2.节点的cache中移除为这些未调度的pod分配的GPU
// 3.判断当前节点的可用GPU数是否大于当前pod请求值
func (gt *GPUTopology) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	if !IsMyPod(pod) {
		return framework.NewStatus(framework.Success, "")
	}
	// get pod state
	podState, err := GetPodTopologyState(state, pod.UID)
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}
	node := nodeInfo.Node()
	var nodeCache *gputopology.NodeGPUTopologyCache
	nodeState, err := GetNodeState(state, node.Name)
	if err == nil {
		nodeCache = nodeState.Cache
	} else {
		nodeCache = gt.cache.GetNodeTopologyCache(node.Name)
	}
	// Remove the GPUs allocated for these unscheduled pods from the node's GPUTopologyCache
	for _, pod := range podState.TopologyGroupPods {
		nodeCache.RemovePod(pod.UID)
	}
	request := GetPodRequestResource(pod, GPUTopologyResourceName)
	// Determine whether the number of GPUs available on the current node is greater than the current pod request value
	if request > nodeCache.GetAvailableGPUs().Size() {
		return framework.NewStatus(framework.Unschedulable, "not enough gpus with topology on the node")
	}
	return nil
}

// Select a group of nodes from the filtered nodes to place the unscheduled pods in the topology group,including:
// 从过滤的节点中选择一组节点放置topology group中未调度的pod
func (gt *GPUTopology) PreScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodes []*v1.Node) *framework.Status {
	if !IsMyPod(pod) {
		return framework.NewStatus(framework.Success, "")
	}
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	unschedulePods := []*v1.Pod{}
	podState, _ := GetPodTopologyState(state, pod.UID)
	var topologyAllocation *gputopology.PodTopologyAllocation
	// If podState.Allocation is not empty, it means that there is already a scheme for allocating GPUs for the pod
	// and need to check whether the allocation is legal, That is, whether the node recommended by the allocation is in the filterNodes list.
	// If it is, it means that the scheme has not expired, and it can be used directly
	// if not, it means that the scheme has expired, and the recommended node cannot run the pod, and a new node needs to be selected.
	// It should be noted that in the preScore and reserve stages, it will not check whether the provided allocation is expired. This step has been checked in preFilter.

	// 如果podState.Allocation不为空，那么说明该已经存在一个为该pod分配GPU的方案，需要检查该方案是否合法，即该方案推荐的node
	// 是不是在filterNodes列表中，如果在，说明该方案没有过期，直接使用它；如果不在，说明该方案已经过期了，推荐的节点不能运行该pod，需要重新选择节点
	// 需要说明一点的是，在preScore和reserve阶段，不会检查提供的方案是否过期，这一步在preFilter已经检查了。
	if podState != nil {
		if podState.Allocation != nil {
			// found pre allocation for current pod,but we should check the node is in filter nodes collection
			for _, node := range nodes {
				if node.Name == podState.Allocation.NodeName {
					topologyAllocation = podState.Allocation
					klog.V(5).Infof("pod %v SuggestHost(%v) is found in filterNodes collection,will use it", podFullName, node.Name)
					break
				}
			}
			// suggestHost is existed but is expired
			if topologyAllocation == nil {
				klog.V(5).Infof("pod %v SuggestHost %v is expired,because it is not in filterNodes collection", podFullName, podState.Allocation.NodeName)
			}
		}
	} else {
		_, groupName, replica, _ := getPodGroupAndReplica(pod)
		podState = &PodTopologyState{
			PodUID:       pod.UID,
			GroupName:    groupName,
			GroupReplica: replica,
		}
	}
	if topologyAllocation == nil {
		allocateAllPods := true
		suggestHosts := map[string]int{}
		totalRequestGPUs := 0
		err := getTopologyGroupPods(pod, gt.PodLister, gt.cache, func(p *v1.Pod, allocatin *gputopology.PodTopologyAllocation) {
			// 如果整个topology group已经有pod已经分配过了，那么为当前pod调度时不考虑其他的未被调度的pod
			if allocatin != nil && (allocatin.GetPhase() == gputopology.SchedulePhaseReserve || allocatin.GetPhase() == gputopology.SchedulePhaseFinishBind) {
				suggestHosts[allocatin.NodeName]++
				allocateAllPods = false
				return
			}
			totalRequestGPUs += GetPodRequestResource(p, GPUTopologyResourceName)
			unschedulePods = append(unschedulePods, p)
		})
		if err != nil {
			return framework.NewStatus(framework.Unschedulable, err.Error())
		}
		if !allocateAllPods {
			unschedulePods = []*v1.Pod{pod}
		}
		if len(suggestHosts) != 0 {
			klog.V(5).Infof("schedule pod %v/%v found suggest hosts: %v", pod.Namespace, pod.Name, suggestHosts)
		}
		activateUnschedulePods(state, pod, unschedulePods)
		allocation, status := gt.aussmePods(state, podState.GroupName, pod, unschedulePods, nodes, suggestHosts)
		if allocation == nil {
			return status
		}
		topologyAllocation = allocation
	}
	podState.Allocation = topologyAllocation
	SetPodTopologyState(state, podState)
	return nil
}

// 节点打分，如果当前节点与pod在preScore阶段给出的最优节点一致，那么给出100份，否则给0分
// 为保证score后最终挑选出的节点为在preScore阶段给出的最优节点，需要保证gputopology的score插件具有很高的权重
func (gt *GPUTopology) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	if !IsMyPod(pod) {
		return 0, framework.NewStatus(framework.Success, "")
	}
	podState, err := GetPodTopologyState(state, pod.UID)
	if err != nil {
		return 0, framework.NewStatus(framework.Unschedulable, "not found pod topology allocation in state")
	}
	if podState.Allocation.NodeName == nodeName {
		return 100, nil
	}
	return 0, nil
}

func (gt *GPUTopology) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

// Reserve阶段主要有3件事：
// 1.如果当前节点与preScore阶段提供的最优节点一致，那么将pod的topology allocation的phase设置为Reserve
// 2.如果当前节点与preScore阶段提供的最优节点不一致，那么需要重新给出分配方案
// 3.有一种情况也需要考虑，那就是如果在经过filter阶段后，整个集群只有一个节点符合条件，那么preScore和Score阶段就被跳过，此时将拿不到preScore给定的分配方案，需要重新分配
func (gt *GPUTopology) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	if !IsMyPod(pod) {
		return framework.NewStatus(framework.Success, "")
	}
	unschedulePods := []*v1.Pod{}
	suggestHosts := map[string]int{}
	podState, _ := GetPodTopologyState(state, pod.UID)
	if podState != nil && podState.Allocation != nil {
		if podState.Allocation.NodeName == nodeName {
			if err := gt.patchPodAnnotations(pod, podState.Allocation); err != nil {
				return framework.NewStatus(framework.Unschedulable, err.Error())
			}
			gt.cache.SetPhase(pod.UID, gputopology.SchedulePhaseReserve)
			return framework.NewStatus(framework.Success, "")
		} else {
			unschedulePods = append(unschedulePods, pod)
		}
	} else {
		_, groupName, replica, _ := getPodGroupAndReplica(pod)
		podState = &PodTopologyState{
			PodUID:       pod.UID,
			GroupName:    groupName,
			GroupReplica: replica,
		}
	}
	if len(unschedulePods) == 0 {
		allocateAllPods := true
		totalRequestGPUs := 0
		err := getTopologyGroupPods(pod, gt.PodLister, gt.cache, func(p *v1.Pod, allocatin *gputopology.PodTopologyAllocation) {
			// 如果整个topology group已经有pod已经分配过了，那么为当前pod调度时不考虑其他的未被调度的pod
			if allocatin != nil && (allocatin.GetPhase() == gputopology.SchedulePhaseReserve || allocatin.GetPhase() == gputopology.SchedulePhaseFinishBind) {
				suggestHosts[allocatin.NodeName]++
				allocateAllPods = false
				return
			}
			totalRequestGPUs += GetPodRequestResource(p, GPUTopologyResourceName)
			unschedulePods = append(unschedulePods, p)
		})
		if err != nil {
			return framework.NewStatus(framework.Unschedulable, err.Error())
		}
		if !allocateAllPods {
			unschedulePods = []*v1.Pod{pod}
		}
		if len(suggestHosts) != 0 {
			klog.V(5).Infof("schedule pod %v/%v found suggest hosts: %v", pod.Namespace, pod.Name, suggestHosts)
		}
	}
	activateUnschedulePods(state, pod, unschedulePods)
	node, err := gt.FrameworkHandle.ClientSet().CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	topologyAllocation, status := gt.aussmePods(state, podState.GroupName, pod, unschedulePods, []*v1.Node{node}, suggestHosts)
	if topologyAllocation == nil {
		return status
	}
	podState.Allocation = topologyAllocation
	SetPodTopologyState(state, podState)
	if err := gt.patchPodAnnotations(pod, podState.Allocation); err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}
	gt.cache.SetPhase(pod.UID, gputopology.SchedulePhaseReserve)
	return nil
}

// Unreserve mainly sets the Phase of the Topology Allocation of the current pod to SchedulePhaseAssumed
// indicating that the allocation is not final and may still be replaced
func (gt *GPUTopology) Unreserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	if !IsMyPod(pod) {
		return
	}
	gt.cache.SetPhase(pod.UID, gputopology.SchedulePhaseAssumed)
	klog.V(5).Infof("topology pod %v/%v is unreserved,set SchedulerPhase with Assumed", pod.Namespace, pod.Name)
}

func (gt *GPUTopology) Bind(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	if !IsMyPod(pod) {
		return framework.NewStatus(framework.Skip, "")
	}
	gt.cache.SetPhase(pod.UID, gputopology.SchedulePhaseFinishBind)
	return framework.NewStatus(framework.Skip, "")
}

func (gt *GPUTopology) patchPodAnnotations(pod *v1.Pod, topologyAllocation *gputopology.PodTopologyAllocation) error {
	annotations := map[string]string{}
	allocation, err := json.Marshal(&AllocationInfo{
		AllocatedGPUs: topologyAllocation.AllocatedGPUs,
		VisibleGPUs:   topologyAllocation.VisibleGPUs,
		Assigned:      false,
		AssumeTime:    time.Now().UnixNano(),
	})
	if err != nil {
		return err
	}
	// update pod annotations
	annotations[GPUTopologyAllocationKey] = string(allocation)
	if err := PatchPodAnnotations(gt.FrameworkHandle.ClientSet(), annotations, pod.Namespace, pod.Name); err != nil {
		return fmt.Errorf("failed to update pod annotation: %v", err)
	}
	return nil
}

func (gt *GPUTopology) aussmePods(state *framework.CycleState, groupName string, currentPod *v1.Pod, unscheduledPods []*v1.Pod, filterNodes []*v1.Node, suggestHosts map[string]int) (*gputopology.PodTopologyAllocation, *framework.Status) {
	var topologyAllocation *gputopology.PodTopologyAllocation
	// 获取同组pod，注意只获取为调度的pod，已经调度完成(finishBind)或者经过Reserve阶段还没Bind的pod不在考虑范围内
	// Obtain pods in the same topology group. Note that only pods that are scheduled are obtained.
	// Pods that have been scheduled (finishBind) or have not yet been bound after the Reserve stage are not considered.
	nodeCaches := map[string]*gputopology.NodeGPUTopologyCache{}
	// Clone and save a copy of the topology cache of each node, so that the original cache will not be affected
	// The cache of each node clears the gpu previously allocated to the pods in the group
	// 对每个节点的topology cache克隆一份并保存，这样不会影响原cache
	// 每个节点的cache中清除之前为group中的pod分配的gpu
	for _, node := range filterNodes {
		nodeCache := gt.frameworkCache.GetExtendNodeInfo(node.Name).GetGPUTopologyCache().Clone()
		for _, p := range unscheduledPods {
			nodeCache.RemovePod(p.UID)
		}
		nodeCaches[node.Name] = nodeCache
	}
	// 寻找一组节点，用于运行这一组未调度的pod
	// Find a set of nodes to run this set of unscheduled pods
	allocationHints := gt.packPodsOnNodes(currentPod, unscheduledPods, filterNodes, suggestHosts, nodeCaches, state)
	if len(allocationHints) == 0 {
		return nil, framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Insufficient gpus to allocate for the topology group %v", groupName))
	}
	// 从这组节点上挑选合适的GPU分配为这一组未调度的pod
	// Select the appropriate GPU allocation from this set of nodes for this set of unscheduled pods
	podTopologyAllocations, err := gt.getTopologyAllocations(groupName, nodeCaches, unscheduledPods, allocationHints)
	if err != nil {
		return nil, framework.NewStatus(framework.Error, err.Error())
	}
	// 检查新分配方案中是否包含当前pod的分配信息
	// Check whether the new allocations contain the allocation information of the current pod
	if allocation, ok := podTopologyAllocations[currentPod.UID]; !ok {
		return nil, framework.NewStatus(framework.Unschedulable, fmt.Sprintf("pod(%v/%v) topology allocation is in the new allocations", currentPod.Namespace, currentPod.Name))
	} else {
		topologyAllocation = allocation.Clone()
	}
	// 将新分配方案更新到topology cache中，预占GPU生效
	// Update the new allocations to the topology cache, and pre-occupy the GPU to take effect
	for _, podTopologyAllocation := range podTopologyAllocations {
		gt.cache.AddPod(podTopologyAllocation)
	}
	return topologyAllocation, nil
}

func activateUnschedulePods(state *framework.CycleState, currentPod *v1.Pod, pods []*v1.Pod) {
	if len(pods) == 1 {
		return
	}
	data, err := state.Read(framework.PodsToActivateKey)
	if err != nil {
		klog.Errorf("failed to read data with key %v from cycle: %v", framework.PodsToActivateKey, err)
		return
	}
	podToActive := data.(*framework.PodsToActivate)
	podToActive.Lock()
	defer podToActive.Unlock()
	if len(podToActive.Map) == 0 {
		podToActive.Map = map[string]*v1.Pod{}
	}
	for _, pod := range pods {
		if pod.UID == currentPod.UID {
			continue
		}
		podToActive.Map[fmt.Sprintf("%v/%v/%v", GPUTopologyNamespaceKey, pod.Namespace, pod.Name)] = pod
	}
}

func getTopologyGroupPods(currentPod *v1.Pod, podLister corelisters.PodLister, cache *GPUTopologyCache, callback func(p *v1.Pod, allocation *gputopology.PodTopologyAllocation)) error {
	podUIDs := []apitypes.UID{}
	topologyGroupPods := []*v1.Pod{}
	groupLabelKey, groupName, _, err := getPodGroupAndReplica(currentPod)
	if err != nil {
		return err
	}
	var getPods = func() ([]*v1.Pod, error) {
		selector := labels.Set(labels.Set{groupLabelKey: groupName}).AsSelector()
		pods, err := podLister.Pods(currentPod.Namespace).List(selector)
		if err != nil {
			return nil, err
		}
		filterPods := []*v1.Pod{}
		for _, pod := range pods {
			if pod.DeletionTimestamp != nil {
				continue
			}
			filterPods = append(filterPods, pod)
		}
		return filterPods, nil
	}
	if groupName == string(currentPod.UID) {
		topologyGroupPods = append(topologyGroupPods, currentPod)
		podUIDs = append(podUIDs, currentPod.UID)
	} else {
		pods, err := getPods()
		if err != nil {
			return fmt.Errorf("failed to get pods with group label: %v", err)
		}
		for _, pod := range pods {
			topologyGroupPods = append(topologyGroupPods, pod)
			podUIDs = append(podUIDs, pod.UID)
		}
	}
	topologyAllocations := cache.GetTopologyPodAllocations(podUIDs...)
	for _, pod := range topologyGroupPods {
		topologyAllocation, ok := topologyAllocations[pod.UID]
		if ok {
			callback(pod, topologyAllocation)
		} else {
			callback(pod, nil)
		}
	}
	return nil
}

// 获取节点组以后，在这些节点上挑选最优组合的GPU分配给pod
// After obtaining the node group, select the optimal combination of GPUs on these nodes and assign them to pods
func (gt *GPUTopology) getTopologyAllocations(groupName string, nodeCaches map[string]*gputopology.NodeGPUTopologyCache, pods []*v1.Pod, allocationHints map[string][]string) (map[apitypes.UID]*gputopology.PodTopologyAllocation, error) {
	result := map[apitypes.UID]*gputopology.PodTopologyAllocation{}
	if len(allocationHints) == 0 {
		return result, nil
	}
	podMap := map[apitypes.UID]*v1.Pod{}
	for _, pod := range pods {
		podMap[pod.UID] = pod
	}
	for nodeName, podUIDs := range allocationHints {
		nodeTopoCache := nodeCaches[nodeName].Clone()
		nodeAvailableGPUs := nodeTopoCache.GetAvailableGPUs()
		req := 0
		for _, podUID := range podUIDs {
			p := podMap[apitypes.UID(podUID)]
			req += GetPodRequestResource(p, GPUTopologyResourceName)
		}
		// 1.对需要运行在该节点上的这组pod，给出一个最优GPU组合，具体哪个pod使用哪些GPU将从这个GPU组合中再次挑选
		// 1.For the group of pods that need to run on this node, an optimal GPU combination is given, and which GPUs are used by the pod will be selected again from this GPU combination
		visibleGPUs, minxBandwidth, err := gputopology.GetGPUsAndMinBandwidth(nodeAvailableGPUs, nodeTopoCache.GetTopologyHints(), req)
		if err != nil {
			return nil, err
		}
		klog.V(5).Infof("nodeName: %v,topologyGroup: %v,Request: %v,visibleGPUs: %v,minBandwidth: %v", nodeName, groupName, req, visibleGPUs, minxBandwidth)
		// 注意需要克隆一份visibleGPUs
		// Note that you need to clone a copy of visibleGPUs
		nodeVisibleGPUs := visibleGPUs.Clone()
		// 2.从最优GPU组合中为每个pod挑选GPU
		// 2.Pick GPUs for each pod from the optimal GPU combination
		for _, podUID := range podUIDs {
			allocateGPUs := map[gputopology.ContainerIndex]gputopology.TopologyGPUs{}
			p := podMap[apitypes.UID(podUID)]
			podReq := GetPodRequestResource(p, GPUTopologyResourceName)
			podGPUs, podMinBandwidth, err := gputopology.GetGPUsAndMinBandwidth(visibleGPUs, nodeTopoCache.GetTopologyHints(), podReq)
			if err != nil {
				return nil, err
			}
			klog.V(5).Infof("nodeName: %v,pod: %v/%v,Request: %v,allocatedGPUs: %v,minBandwidth: %v", nodeName, p.Namespace, p.Name, podReq, podGPUs, podMinBandwidth)
			// 每次pod挑选GPU后，visibleGPUs中需要除去这些GPU
			// After each pod selects GPUs, these GPUs need to be removed from visibleGPUs
			visibleGPUs = visibleGPUs.Difference(podGPUs).Clone()
			// 3.为每个pod挑选一组GPU以后，pod中的容器从这一组GPU中再进行挑选
			// 3.After selecting a set of GPUs for each pod, the containers in the pod are selected from this set of GPUs
			for index, containerReq := range GetContainerRequestResourceByName(GPUTopologyResourceName, p) {
				if len(podGPUs) == containerReq {
					allocateGPUs[index] = podGPUs
					continue
				}
				containerGPUs, _, err := gputopology.GetGPUsAndMinBandwidth(podGPUs, nodeTopoCache.GetTopologyHints(), containerReq)
				if err != nil {
					return nil, err
				}
				allocateGPUs[index] = containerGPUs
				// 每个容器挑选GPU后，为pod分配的GPU集合中需要除去这些GPU
				// After each container selects GPUs, these GPUs need to be removed from the GPU set allocated to the pod
				podGPUs = podGPUs.Difference(containerGPUs).Clone()
			}
			// 获取每个Pod的分配方案的TTL，即给定的分配方案有效期为多长时间
			// Get the TTL of each Pod's allocation plan, that is, how long a given allocation is valid
			ttl, err := getTTLFromLabel(p)
			if err != nil {
				klog.Errorf("%v", err)
				return nil, err
			}

			podAllocation := gputopology.NewPodTopologyAllocation(apitypes.UID(podUID))
			podAllocation.AllocatedGPUs = allocateGPUs
			podAllocation.VisibleGPUs = nodeVisibleGPUs
			podAllocation.NodeName = nodeName
			if ttl > 0 {
				podAllocation.SetTTL(ttl)
			}
			result[podAllocation.PodUID] = podAllocation
		}
	}
	return result, nil
}

// 寻找一组节点存放group中未调度的pod（包含当前正在调度的pod）
// Find a group of nodes to store unscheduled pods in the group (including pods currently being scheduled)
func (gt *GPUTopology) packPodsOnNodes(currentPod *v1.Pod, pods []*v1.Pod, nodes []*v1.Node, suggestedHosts map[string]int, nodeCaches map[string]*gputopology.NodeGPUTopologyCache, state *framework.CycleState) map[string][]string {
	result := map[string][]string{}
	type Candidate struct {
		hints map[string][]string
		score float32
	}
	totalAvailableGPUs := 0
	totalRequestGPUs := 0
	nodeAvailableGPUs := map[string]int{}
	for _, node := range nodes {
		nodeCache := nodeCaches[node.Name]
		nodeAvailableGPUs[node.Name] = nodeCache.GetAvailableGPUs().Size()
		totalAvailableGPUs += nodeCache.GetAvailableGPUs().Size()
	}
	for host := range suggestedHosts {
		if _, ok := nodeCaches[host]; !ok {
			delete(suggestedHosts, host)
		}
	}
	podRequestGPUs := map[string]int{}
	for _, pod := range pods {
		req := GetPodRequestResource(pod, GPUTopologyResourceName)
		podRequestGPUs[string(pod.UID)] = req
		totalRequestGPUs += req
	}
	if totalAvailableGPUs < totalRequestGPUs {
		return result
	}
	podMap := map[apitypes.UID]*v1.Pod{}
	for _, pod := range pods {
		podMap[pod.UID] = pod
	}
	// runFilter is used to run the filter plugins
	// runFilter用于验证选出的这组pod能不能运行在给定的节点上
	// runFilter is used to verify whether the selected set of pods can run on a given node
	runFilter := func(nodeName string, nodePods []*v1.Pod) (bool, error) {
		// must clone the topology info
		topologyCache := nodeCaches[nodeName].Clone()
		ni, err := gt.FrameworkHandle.SnapshotSharedLister().NodeInfos().Get(nodeName)
		if err != nil {
			return false, fmt.Errorf("get node info failed: %v", err)
		}
		// must clone the nodeInfo
		nodeInfo := ni.Clone()
		nodeState := &NodeState{
			NodeName: nodeName,
			Cache:    topologyCache,
		}
		SetGPUTopologyNodeState(state, nodeState)
		for _, p := range nodePods {
			podState := &PodTopologyState{
				PodUID:            p.UID,
				TopologyGroupPods: pods,
			}
			SetPodTopologyState(state, podState)
			status := gt.FrameworkHandle.RunFilterPlugins(context.TODO(), state, p, nodeInfo)
			if status.Merge().IsSuccess() {
				nodeInfo.AddPod(p)
			} else {
				return false, nil
			}
		}
		return true, nil
	}
	// 按组合方案中节点个数寻找一个比较优的节点组合，比如假设有3个pod，那么不理想的情况是用3个节点来存放这3个pod
	// 那么首先寻找1个节点能不能放下这3个pod，如果不能，就试一下用2个节点能不能，如果2个节点也不行，那么就去寻找3个节点能不能放下该pod

	// Find an optimal node combination according to the number of nodes in the given node combination
	// For example, if there are 3 pods, the unideal situation is to use 3 nodes to store the 3 pods.
	// Then first find out whether 1 node can put down the 3 pods Pod, if not, try to see if it can be done with 2 nodes,
	// if 2 nodes are not enough, then go to find 3 nodes to see if the pod can be put down
	for i := 1; i <= len(pods); i++ {
		candidates := []Candidate{}
		Iterate(nodeAvailableGPUs, podRequestGPUs, i, func(item map[string][]string, score float32) {
			if len(item) == 1 && len(suggestedHosts) != 0 {
				for nodeName := range item {
					score += float32(suggestedHosts[nodeName])
				}
			}
			candidates = append(candidates, Candidate{
				hints: item,
				score: score,
			})
		})
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].score >= candidates[j].score
		})
		for _, candidate := range candidates {
			// 如果只有一个pod，那么不运行Filter插件,直接返回
			// If there is only one pod, then do not run the FilterPlugins and return directly
			if len(pods) == 1 && pods[0].UID == currentPod.UID {
				return candidate.hints
			}
			satisfied := true
			for nodeName, podUIDs := range candidate.hints {
				nodePods := []*v1.Pod{}
				for _, uid := range podUIDs {
					nodePods = append(nodePods, podMap[apitypes.UID(uid)])
				}
				succeeded, err := runFilter(nodeName, nodePods)
				if err != nil {
					klog.Errorf("failed to run filter plugins on node %v for gpu topology pods: %v", nodeName, err)
					satisfied = false
					break
				} else if !succeeded {
					satisfied = false
					break
				}
			}
			if satisfied {
				return candidate.hints
			}
		}
	}
	return result
}

func Iterate(nodeAvailableGPUs map[string]int, podRequests map[string]int, nodeLength int, callback func(item map[string][]string, score float32)) {
	type NodeAvailableGPUs struct {
		NodeName      string
		AvailableGPUs int
	}
	type PodRequestGPUs struct {
		UID         string
		RequestGPUs int
	}
	nodeGPUList := []NodeAvailableGPUs{}
	for name, count := range nodeAvailableGPUs {
		nodeGPUList = append(nodeGPUList, NodeAvailableGPUs{NodeName: name, AvailableGPUs: count})
	}
	podRequestList := []PodRequestGPUs{}
	for uid, count := range podRequests {
		podRequestList = append(podRequestList, PodRequestGPUs{UID: uid, RequestGPUs: count})
	}
	// 对pod申请GPU的值按从大到小排序
	// Sort the pod application GPU value in descending order
	sort.Slice(podRequestList, func(i, j int) bool {
		return podRequestList[i].RequestGPUs > podRequestList[j].RequestGPUs
	})
	totalReq := 0
	for _, count := range podRequests {
		totalReq += count
	}
	// search用于寻找给定的节点组中，假定为该pod分配GPU后，碎片最少的节点
	// search is used to find the node with the least fragmentation in a given node group, assuming that the GPU is allocated to the pod
	search := func(nodeGPUs []NodeAvailableGPUs, req int) ([]NodeAvailableGPUs, string) {
		MatchNode := ""
		minDiff := 99999999
		for _, available := range nodeGPUs {
			diff := available.AvailableGPUs - req
			if diff >= 0 && diff < minDiff {
				minDiff = diff
				MatchNode = available.NodeName
			}
		}
		result := []NodeAvailableGPUs{}
		if MatchNode != "" {
			for _, available := range nodeGPUs {
				if available.NodeName == MatchNode {
					available.AvailableGPUs -= req
				}
				result = append(result, available)
			}
		}
		return result, MatchNode
	}
	getPrefferedCombination := func(nodeGPUs []NodeAvailableGPUs) {
		totalNodes := len(nodeGPUs)
		totalRequests := 0
		nodeMap := map[string]int{}
		result := map[string][]string{}
		for _, podReq := range podRequestList {
			// Find an node for the Pod
			newNodeGPUs, nodeName := search(nodeGPUs, podReq.RequestGPUs)
			// 如果nodeName为空，那么nodeGPUs提供的节点组合方案不可行
			// If nodeName is empty, the node combination scheme provided by nodeGPUs is not feasible
			if nodeName == "" {
				return
			}
			nodeGPUs = newNodeGPUs
			if result[nodeName] == nil {
				result[nodeName] = []string{}
			}
			result[nodeName] = append(result[nodeName], podReq.UID)
			nodeMap[nodeName] = nodeAvailableGPUs[nodeName]
			totalRequests += podReq.RequestGPUs
		}
		// 如果提供m个节点组合，但是却拿到了n个节点的组合，认为该方案不可用
		// If m node combinations are provided, but n node combinations are obtained, it is considered unavailable
		if len(nodeMap) != totalNodes {
			return
		}
		// Calculate the node combination score, the formula is as follows:
		// score = RequestGPUsOfPods / AvailableGPUsOfNodes
		// 计算节点组合分数，公式如下：
		// score = pod组请求GPU数 / 节点组可用GPU数总和
		score := float32(0)
		totalAvailable := 0
		for _, count := range nodeMap {
			totalAvailable += count
		}
		if totalAvailable != 0 {
			score = float32(totalRequests) / float32(totalAvailable)
		}
		// 回调函数
		// callback function
		callback(result, score)
	}

	var iterate func(availables []NodeAvailableGPUs, combination []NodeAvailableGPUs)
	iterate = func(availables []NodeAvailableGPUs, combination []NodeAvailableGPUs) {
		if len(combination) == nodeLength {
			getPrefferedCombination(combination)
			return
		}
		for i := range availables {
			iterate(availables[i+1:], append(combination, availables[i]))
		}
	}
	iterate(nodeGPUList, []NodeAvailableGPUs{})
}
