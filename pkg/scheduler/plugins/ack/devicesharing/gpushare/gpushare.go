package gpushare

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/devicesharing/runtime"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/frameworkcache"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/noderesource"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/policy"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

// Name of this plugin.
const GPUShareName = "gpushare"

const (
	//GPUShareAllocationFlag defines the allocation of pod
	GPUShareAllocationFlag = "scheduler.framework.gpushare.allocation"
	//CGPUPodAssignFlag represents the pod is assigned or not
	GPUSharePodAnnoAssignFlag = "scheduler.framework.gpushare.assigned"
	//CGPUPodAssumeTimeFlag represents the assume time of pod
	GPUSharePodAnnoAssumeTimeFlag = "scheduler.framework.gpushare.assume-time"
)

const (
	OldVersionGPUShareAllocationFlag  = "ALIYUN_COM_GPU_MEM_POD"
	OldVersionGPUShareDeviceIndexFlag = "ALIYUN_COM_GPU_MEM_IDX"
)

const (
	ownWholeGPULabelKey   = "ack.gpushare.placement"
	replicaDeviceLabelKey = "aliyun.com/gpu-count"
	// only support one default policy
	ErrDefaultPolicyMoreThanOne = "only supports one default policy"
	// ErrUnSupportAlgorithm defines the error message of unsupporting algorithm
	ErrGPUShareUnSupportAlgorithm = "gpushare does not support this algorithm,only support algorithm: [binpack, spread]"
	// ErrNotFoundDefaultPolicy defines the error message that gpushare needs a default policy
	ErrNotFoundDefaultPolicy = "not found default policy for gpushare,please set it"
)

// GPUShare implements allocating part gpu memory of single gpu to a pod
type GPUShare struct {
	resourceNames  []v1.ResourceName
	engine         *runtime.DeviceSharingRuntime
	frameworkCache *frameworkcache.FrameworkCache
	args           GPUShareArgs
}

// Args holds the args that are used to configure the plugin.
type GPUShareArgs struct {
	// enable only request gpu memory can be scheduled on node who owns gpu memory and gou core resources
	EnableMemoryCoreMixSchedule *bool `json:"enableMemoryCoreMixSchedule,omitempty"`
	// GPUMemoryScoreWeight is used to define the gpu memory score weight in score phase
	GPUMemoryScoreWeight *uint `json:"gpuMemoryScoreWeight,omitempty"`
	// Policy defines the policy which gpushare should use
	Policy []runtime.PolicyConfig `json:"policy,omitempty"`
}

// New initializes a new plugin and returns it.
func New(obj apiruntime.Object, handle framework.Handle) (framework.Plugin, error) {
	klog.Infof("start to create gpushare plugin")
	unknownObj, ok := obj.(*apiruntime.Unknown)
	if !ok {
		return nil, fmt.Errorf("want args to be of type *runtime.Unknown, got %T", obj)
	}
	gpushare := &GPUShare{
		args: GPUShareArgs{},
	}
	if err := frameworkruntime.DecodeInto(unknownObj, &gpushare.args); err != nil {
		return nil, err
	}
	if err := validateGPUShareArgs(gpushare.args.Policy); err != nil {
		return nil, err
	}
	if gpushare.args.GPUMemoryScoreWeight != nil && *gpushare.args.GPUMemoryScoreWeight > 100 {
		return nil, fmt.Errorf("invalid score weight,it must be less than 100")
	}
	if gpushare.args.EnableMemoryCoreMixSchedule == nil {
		enable := false
		gpushare.args.EnableMemoryCoreMixSchedule = &enable
	}
	klog.Infof("succeed to validate gpushare args")
	frameworkCache := frameworkcache.GetFrameworkCache(obj, handle)
	resourceNames := GetResourceInfoList().GetNames()
	gpushare.engine = runtime.NewDeviceSharingRuntime(GPUShareName, resourceNames, frameworkCache)
	if err := gpushare.Init(resourceNames, frameworkCache); err != nil {
		return nil, err
	}
	gpushare.resourceNames = resourceNames
	gpushare.frameworkCache = frameworkCache
	klog.Info("succeed to create gpushare plugin")
	return gpushare, nil
}

var _ framework.PreFilterPlugin = &GPUShare{}
var _ framework.FilterPlugin = &GPUShare{}
var _ framework.ScorePlugin = &GPUShare{}
var _ framework.ReservePlugin = &GPUShare{}

//var _ framework.BindPlugin = &GPUShare{}

// Name returns name of the plugin. It is used in logs, etc.
func (g *GPUShare) Name() string {
	return GPUShareName
}

func (g *GPUShare) Init(resourceNames []v1.ResourceName, frameworkCache *frameworkcache.FrameworkCache) error {
	addOrUpdateNode := func(node *v1.Node) {
		AddOrUpdateNode(resourceNames, node, frameworkCache)
	}
	addOrUpdatePod := func(pod *v1.Pod) {
		AddPod(resourceNames, pod, frameworkCache)
	}
	eventFuncs := []interface{}{
		// add pod event handler
		addOrUpdatePod,
		// update pod event handler
		func(oldPod *v1.Pod, newPod *v1.Pod) {
			UpdatePod(resourceNames, oldPod, newPod, frameworkCache)
		},
		// remove pod event handler
		func(pod *v1.Pod, _ bool) {
			RemovePod(resourceNames, pod, frameworkCache)
		},
		// update node event handler
		func(oldNode *v1.Node, newNode *v1.Node) {
			UpdateNode(resourceNames, oldNode, newNode, frameworkCache)
		},
	}
	return g.engine.Init(
		g.args.Policy,
		runtime.AddOrUpdateNode(addOrUpdateNode),
		runtime.AddOrUpdatePod(addOrUpdatePod),
		eventFuncs,
	)
}

func (g *GPUShare) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
	resourceNameMap := map[v1.ResourceName]bool{}
	for _, resourceName := range g.resourceNames {
		resourceNameMap[resourceName] = true
	}
	containerRequests := map[types.ContainerIndex]v1.ResourceList{}
	for index, container := range pod.Spec.Containers {
		requests := v1.ResourceList{}
		for resourceName, r := range container.Resources.Limits {
			if _, ok := resourceNameMap[resourceName]; ok && r.Value() > 0 {
				requests[resourceName] = r
			}
		}
		if len(requests) != 0 {
			containerRequests[types.ContainerIndex(index)] = requests
		}
	}
	// pod is not a gpushare pod
	if len(containerRequests) == 0 {
		return nil
	}
	// check request resource type
	if !runtime.ContainersRequestResourcesAreTheSame(pod, g.resourceNames...) {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "The requested gpu resource types should be consistent for all containers")
	}
	// check request device count
	replica, err := getReplicaDeviceCount(containerRequests, pod.Labels)
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	if replica == 0 {
		replica = 1
	}
	requireWholeGPU := false
	if v, ok := pod.Labels[ownWholeGPULabelKey]; ok {
		if v == "require-whole-device" {
			requireWholeGPU = true
		} else {
			err := fmt.Errorf("Invalid value of pod label %v:only support [require-whole-device]", ownWholeGPULabelKey)
			return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
		}
	}
	CreateAndSaveGPUSharePodState(state, replica, requireWholeGPU)
	return nil
}

// PreFilterExtensions returns prefilter extensions, pod add and remove.

func (g *GPUShare) PreFilterExtensions() framework.PreFilterExtensions {
	return g
}

// AddPod from pre-computed data in cycleState.
func (g *GPUShare) AddPod(ctx context.Context, cycleState *framework.CycleState, podToSchedule *v1.Pod, podToAdd *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	p := podToAdd.Pod
	if !runtime.IsMyPod(p, g.resourceNames...) {
		return framework.NewStatus(framework.Success, "")
	}
	if p.Spec.NodeName == "" {
		return framework.NewStatus(framework.Success, "")
	}
	var nodeState *GPUShareNodeState
	var err error
	nodeState, err = GetGPUShareNodeState(cycleState, p.Spec.NodeName)
	if err != nil {
		// warning: should clone the node resource
		nrm := g.frameworkCache.GetExtendNodeInfo(p.Spec.NodeName).GetNodeResourceManager().Clone()
		nodeState = &GPUShareNodeState{
			NodeResourceManager: nrm,
		}
	}
	addPod(g.resourceNames, p, func(nodeName string) *noderesource.NodeResourceManager {
		return nodeState.NodeResourceManager
	})
	CreateAndSaveGPUShareNodeState(cycleState, p.Spec.NodeName, nodeState)
	return framework.NewStatus(framework.Success, "")
}

// RemovePod from pre-computed data in cycleState.
func (g *GPUShare) RemovePod(ctx context.Context, cycleState *framework.CycleState, podToSchedule *v1.Pod, podToRemove *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	p := podToRemove.Pod
	if !runtime.IsMyPod(p, g.resourceNames...) {
		return framework.NewStatus(framework.Success, "")
	}
	if p.Spec.NodeName == "" {
		return framework.NewStatus(framework.Success, "")
	}
	var nodeState *GPUShareNodeState
	var err error
	nodeState, err = GetGPUShareNodeState(cycleState, p.Spec.NodeName)
	if err != nil {
		// warning: should clone the node resource
		nrm := g.frameworkCache.GetExtendNodeInfo(p.Spec.NodeName).GetNodeResourceManager().Clone()
		nodeState = &GPUShareNodeState{
			NodeResourceManager: nrm,
		}
	}
	removePod(g.resourceNames, p, func(nodeName string) *noderesource.NodeResourceManager {
		return nodeState.NodeResourceManager
	})
	CreateAndSaveGPUShareNodeState(cycleState, p.Spec.NodeName, nodeState)
	return framework.NewStatus(framework.Success, "")
}

// Filter invoked at the filter extension point.
// It checks whether the given node has enough gpu memory in single device for the pod
func (g *GPUShare) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	// if not found the node,return an error
	node := nodeInfo.Node()
	if node == nil {
		klog.Errorf("node not found,it may be deleted")
		return framework.NewStatus(framework.Error, "node not found")
	}
	return g.engine.FilterNode(
		state,
		runtime.FilterNodeArgs{
			Node:                   node,
			SchedulePod:            pod,
			SearchNodePolicy:       g.searchPolicyForNode,
			FitlerNodeByPolicy:     g.filterNodeByPolicy,
			GetNodeResourceManager: g.getNodeResourceManager,
		},
	)
}

func (g *GPUShare) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	return g.engine.ScoreNode(
		state,
		pod,
		nodeName,
		runtime.AllocateDeviceResources(g.allocateDeviceResources),
	)
}

func (g *GPUShare) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func (g *GPUShare) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	return g.engine.ReserveResources(
		state,
		pod,
		nodeName,
		runtime.AllocateDeviceResources(g.allocateDeviceResources),
		runtime.BuildPodAnnotations(buildPodAnnotations),
	)
}

/*
func (g *GPUShare) Bind(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	return g.engine.BindPod(
		pod,
		nodeName,
		runtime.BuildPodAnnotations(buildPodAnnotations),
	)
}
*/

func (g *GPUShare) Unreserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	g.engine.UnReserveResources(state, pod, nodeName)
}

// searchPolicyForNode is used to search a policy for the node
func (g *GPUShare) searchPolicyForNode(pod *v1.Pod, node *v1.Node, policyInstances []runtime.PolicyInstance) (runtime.PolicyInstance, error) {
	result := []runtime.PolicyInstance{}
	policyType := policy.SingleDevice
	// if found replica device label in pod,set the policy type with ReplicaDevice
	if _, ok := pod.Labels[replicaDeviceLabelKey]; ok {
		policyType = policy.ReplicaDevice
	}
	// filter the policyTypes whose name is we need
	for _, p := range policyInstances {
		if p.Name() != policyType {
			continue
		}
		result = append(result, p)
	}
	// matchScore is used to score the two policies
	// if one policy is more matched than the another
	// it gets hight score.
	var matchScore = func(selectors, labels map[string]string) int {
		score := 0
		levelScore := 99999
		unUseLevelScore := -99999999
		if len(selectors) == 0 {
			return score
		}
		for key, value := range selectors {
			if _, ok := labels[key]; !ok {
				score = unUseLevelScore
				break
			}
			if value == "" {
				score += levelScore / 10
				continue
			}
			if labels[key] != value {
				score = unUseLevelScore
				break
			}
			score += levelScore
		}
		return score
	}
	// sort the policies by scores
	nodeLabels := node.ObjectMeta.Labels
	sort.Slice(result, func(i, j int) bool {
		pi := result[i]
		pj := result[j]
		piScore := matchScore(pi.Config.NodeSelectors, nodeLabels)
		pjScore := matchScore(pj.Config.NodeSelectors, nodeLabels)
		return piScore > pjScore
	})
	//if matchScore(vaildPolicyInstances[0].Config.NodeSelectors,pod.Spec.NodeSelector) < 0 {
	//	return PolicyInstance{},ErrPolicyIsNotMatched
	//}
	return result[0], nil
}

func (g *GPUShare) getNodeResourceManager(state *framework.CycleState, nodeName string) (*noderesource.NodeResourceManager, error) {
	if ns, err := GetGPUShareNodeState(state, nodeName); err == nil && ns.NodeResourceManager != nil {
		return ns.NodeResourceManager, nil
	}
	nrm := g.frameworkCache.GetExtendNodeInfo(nodeName).GetNodeResourceManager()
	return nrm, nil
}

func (g *GPUShare) filterNodeByPolicy(state *framework.CycleState, pod *v1.Pod, node *v1.Node, policyInstance runtime.PolicyInstance, nrm *noderesource.NodeResourceManager) (bool, error) {
	podState, err := GetGPUSharePodState(state)
	if err != nil {
		return false, err
	}
	podRequests := runtime.GetPodRequestResourcesByNames(pod, g.resourceNames...)
	nodeResources := runtime.GetNodeResourcesByNames(node, g.resourceNames...)
	args := g.args
	maxPodsPerDevice, err := getMaxPodsPerDevice(node.Labels)
	if err != nil {
		return false, err
	}
	if maxPodsPerDevice != -1 {
		if err := checkPodsPerDevice(maxPodsPerDevice, nrm); err != nil {
			return false, err
		}
	}
	nodeState := &GPUShareNodeState{
		MaxPodsPerDevice:    maxPodsPerDevice,
		PolicyInstance:      policyInstance,
		NodeResourceManager: nrm,
	}
	if err := checkNodeIsValid(args, podRequests, nodeResources); err != nil {
		return false, err
	}
	CreateAndSaveGPUShareNodeState(state, node.Name, nodeState)
	totalPolicyArgs := buildPolicyArgs(podRequests, podState.RequestGPUs, nodeState.NodeResourceManager, nodeState.MaxPodsPerDevice, podState.RequireWholeGPU)
	// if some resources are not available,return false
	if len(totalPolicyArgs) != len(podRequests) {
		return false, nil
	}
	for _, policyArgs := range totalPolicyArgs {
		satified, err := nodeState.PolicyInstance.Filter(policyArgs)
		if err != nil {
			return false, err
		}
		if !satified {
			return false, nil
		}
	}
	// if only request aliyun.com/gpu-mem,return the result
	if len(podRequests) == 1 {
		return true, nil
	}
	allCandidateAllocations := [][]policy.CandidateAllocation{}
	resourceNames := []v1.ResourceName{}
	for resourceName := range podRequests {
		// search the available allocations with specified resource
		candidateAllocations, err := nodeState.PolicyInstance.Allocate(totalPolicyArgs[resourceName])
		if err != nil {
			return false, err
		}
		if len(candidateAllocations) == 0 {
			return false, nil
		}
		allCandidateAllocations = append(allCandidateAllocations, candidateAllocations)
		resourceNames = append(resourceNames, resourceName)
	}
	// combine the all resources and search the preffered candidate allocations
	klog.V(6).Infof("allCandidateAllocations: %v,allResourceNames: %v", allCandidateAllocations, resourceNames)
	prefferedCandidateAllocations, _ := findBestCandidateAllocationCombination(args.GPUMemoryScoreWeight, allCandidateAllocations, resourceNames)
	return len(prefferedCandidateAllocations) > 0, nil
}

func (g *GPUShare) allocateDeviceResources(state *framework.CycleState, pod *v1.Pod, node *v1.Node, policyInstances []runtime.PolicyInstance) (map[v1.ResourceName]noderesource.PodResource, int, error) {
	args := g.args
	result := map[v1.ResourceName]noderesource.PodResource{}
	podRequests := runtime.GetPodRequestResourcesByNames(pod, g.resourceNames...)
	podState, err := GetGPUSharePodState(state)
	if err != nil {
		return nil, 0, err
	}
	nodeState, err := GetGPUShareNodeState(state, node.Name)
	// ocur when pod has nominate node
	if err != nil {
		nodeState = &GPUShareNodeState{
			NodeResourceManager: g.frameworkCache.GetExtendNodeInfo(node.Name).GetNodeResourceManager(),
		}
		policyInstance, err := g.searchPolicyForNode(pod, node, policyInstances)
		if err != nil {
			return nil, 0, err
		}
		nodeState.PolicyInstance = policyInstance
		maxPodsPerDevice, err := getMaxPodsPerDevice(node.Labels)
		if err != nil {
			return nil, 0, err
		}
		if maxPodsPerDevice != -1 {
			if err := checkPodsPerDevice(maxPodsPerDevice, nodeState.NodeResourceManager); err != nil {
				return nil, 0, err
			}
		}
		nodeState.MaxPodsPerDevice = maxPodsPerDevice
		CreateAndSaveGPUShareNodeState(state, node.Name, nodeState)
	}
	totalPolicyArgs := buildPolicyArgs(podRequests, podState.RequestGPUs, nodeState.NodeResourceManager, nodeState.MaxPodsPerDevice, podState.RequireWholeGPU)
	// if some resources are not available,return false
	if len(totalPolicyArgs) != len(podRequests) {
		return result, 0, nil
	}
	allCandidateAllocations := [][]policy.CandidateAllocation{}
	resourceNames := []v1.ResourceName{}

	for resourceName, policyArgs := range totalPolicyArgs {
		candidateAllocations, err := nodeState.PolicyInstance.Allocate(policyArgs)
		if err != nil {
			return result, 0, err
		}
		if len(candidateAllocations) == 0 {
			return result, 0, nil
		}
		allCandidateAllocations = append(allCandidateAllocations, candidateAllocations)
		resourceNames = append(resourceNames, resourceName)
	}
	candidateAllocationCombinations, score := findBestCandidateAllocationCombination(args.GPUMemoryScoreWeight, allCandidateAllocations, resourceNames)
	bestCandidateAllocationCombination := map[v1.ResourceName]policy.CandidateAllocation{}
	for index, candicateAllocation := range candidateAllocationCombinations {
		bestCandidateAllocationCombination[resourceNames[index]] = candicateAllocation
	}
	if len(bestCandidateAllocationCombination) == 0 {
		return result, 0, fmt.Errorf("not found available devices to allocate gpu memory or gpu cores")
	}
	f := policy.AllocateForContainers
	if podState.RequestGPUs > 1 {
		f = policy.AllocateForContainersWithReplica
	}
	for resourceName, bestCandidateAllocation := range bestCandidateAllocationCombination {
		totalDeviceResourceCount, _ := getNodeDeviceResourceCount(resourceName, nodeState.NodeResourceManager, nodeState.MaxPodsPerDevice)
		containerRequests := runtime.GetContainerRequestResourceByName(resourceName, pod)
		containerCandidateAllocations, err := f(totalDeviceResourceCount, bestCandidateAllocation, containerRequests, podState.RequestGPUs)
		if err != nil {
			return nil, 0, err
		}
		allContainerAllocatedResources := map[types.ContainerIndex]noderesource.ContainerAllocatedResources{}
		for containerIndex, candidateAllocation := range containerCandidateAllocations {
			containerAllocatedResources := noderesource.ContainerAllocatedResources{}
			for devId, allocation := range candidateAllocation.Hint {
				containerAllocatedResources[devId] = noderesource.NewNoneIDResources(allocation)
			}
			allContainerAllocatedResources[containerIndex] = containerAllocatedResources
		}
		result[resourceName] = noderesource.PodResource{
			PodUID:              pod.UID,
			PodName:             pod.Name,
			PodNamespace:        pod.Namespace,
			DeviceResourceCount: totalDeviceResourceCount,
			AllocatedResources:  allContainerAllocatedResources,
		}
	}
	return result, int(score), nil
}

// validateGPUShareArgs validates that policy is valid,we only support SingleDevice policy
func validateGPUShareArgs(policies []runtime.PolicyConfig) error {
	if len(policies) == 0 {
		return fmt.Errorf("not found policy config")
	}
	defaultPolicyCount := 0
	for _, policyConfig := range policies {
		if len(policyConfig.NodeSelectors) == 0 {
			defaultPolicyCount++
			if defaultPolicyCount > 1 {
				return fmt.Errorf(ErrDefaultPolicyMoreThanOne)
			}
		}
		switch policy.AlgorithmType(policyConfig.AlgorithmName) {
		case policy.Binpack:
			break
		case policy.Spread:
			break
		default:
			return fmt.Errorf("%v", ErrGPUShareUnSupportAlgorithm)
		}
	}
	if defaultPolicyCount == 0 {
		return fmt.Errorf(ErrNotFoundDefaultPolicy)
	}
	return nil
}

func checkNodeIsValid(args GPUShareArgs, podRequests, nodeResources map[v1.ResourceName]int) error {
	// only support following:
	// 1.support request aliyun.com/gpu-mem
	// 2.support request aliyun.com/gpu-mem + aliyun.com/gpu-core.*
	if len(podRequests) > 2 {
		return fmt.Errorf("invalid reqeust for gpushare resources,only support reqeusts 2 resources at most")
	}
	// must include aliyun.com/gpu-mem
	if podRequests[GPUShareResourceMemoryName] == 0 {
		return fmt.Errorf("reqeust resources must include %v", GPUShareResourceMemoryName)
	}
	// if enable mix schedule mode,the pod only requests aliyun.com/gpu-mem can be
	// scheduled on node who owns aliyun.com/gpu-mem and aliyun.com/gpu-core.*
	if args.EnableMemoryCoreMixSchedule != nil && !*args.EnableMemoryCoreMixSchedule {
		if len(podRequests) != len(nodeResources) {
			return fmt.Errorf("node(s) owns gpu resources are not matched that pod requests")
		}
		for resourceName := range podRequests {
			if nodeResources[resourceName] == 0 {
				return fmt.Errorf("node(s) owns gpu resources are not matched that pod requests")
			}
		}
	}
	return nil
}

func getMaxPodsPerDevice(nodeLabels map[string]string) (int64, error) {
	v, ok := nodeLabels[GPUShareMaxPodPerDeviceLabelKey]
	if !ok {
		return -1, nil
	}
	maxPodPerDevice, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse max pod per device from node label: %v", err)
	}
	if maxPodPerDevice < 0 {
		return 0, fmt.Errorf("value(%v) of max pod per device is invalid,it must be >= 0", maxPodPerDevice)
	}
	return maxPodPerDevice, nil
}

func checkPodsPerDevice(maxPodsPerDevice int64, nodeResourceManager *noderesource.NodeResourceManager) error {
	devices := nodeResourceManager.GetNodeResourceCache(GPUShareResourceMemoryName).GetAllDevices()
	available := map[types.DeviceId]bool{}
	for _, dev := range devices {
		if len(dev.GetPods()) >= int(maxPodsPerDevice) {
			continue
		}
		available[dev.GetId()] = true
	}
	if len(available) > 0 {
		return nil
	}
	return fmt.Errorf("The number of pods per gpu device has reached the maximum allowed")
}

func getNodeDeviceResourceCount(resourceName v1.ResourceName, nodeResourceManager *noderesource.NodeResourceManager, maxPodsPerDevice int64) (map[types.DeviceId]int, map[types.DeviceId]int) {
	totalDeviceCount := map[types.DeviceId]int{}
	availableDeviceCount := map[types.DeviceId]int{}
	devices := nodeResourceManager.GetNodeResourceCache(resourceName).GetAllDevices()
	for _, dev := range devices {
		total := dev.GetTotal().Size()
		if total > 0 {
			totalDeviceCount[dev.GetId()] = total
			available := dev.GetAvailableResources().Size()
			if maxPodsPerDevice != -1 && len(dev.GetPods()) >= int(maxPodsPerDevice) {
				continue
			}
			if available != 0 {
				availableDeviceCount[dev.GetId()] = available
			}
		}
	}
	return totalDeviceCount, availableDeviceCount
}

func buildPolicyArgs(podRequests map[v1.ResourceName]int, requestDeviceCount int, nodeResourceManager *noderesource.NodeResourceManager, maxPodsPerDevice int64, requireWholeGPU bool) map[v1.ResourceName]policy.PolicyArgs {
	result := map[v1.ResourceName]policy.PolicyArgs{}
	for resourceName, podRequestCount := range podRequests {
		totalDeviceResourceCount, availableDeviceResourceCount := getNodeDeviceResourceCount(resourceName, nodeResourceManager, maxPodsPerDevice)
		// if not avaiable devices,skip it
		if len(totalDeviceResourceCount) == 0 || len(availableDeviceResourceCount) == 0 {
			continue
		}
		// detect the node is satified the pod request
		policyArgs := policy.PolicyArgs{
			TotalDevices:                totalDeviceResourceCount,
			AvailableDevices:            availableDeviceResourceCount,
			PodRequest:                  podRequestCount,
			WantDeviceCountInAllocation: requestDeviceCount,
			RequireWholeGPU:             requireWholeGPU,
		}
		result[resourceName] = policyArgs
	}
	return result
}

func findBestCandidateAllocationCombination(scoreWeight *uint, candidateAllocations [][]policy.CandidateAllocation, resourceNames []v1.ResourceName) ([]policy.CandidateAllocation, float32) {
	result := []policy.CandidateAllocation{}
	maxScore := float32(0)
	// getScoreWeight gets the score weight for every resource
	var getScoreWeight = func(resourceName v1.ResourceName, length int) float32 {
		// if not set gpu memory weight,every resource weight is equal
		// if only one resource,return weight 1
		if length == 1 || scoreWeight == nil {
			return float32(1) / float32(length)
		}
		// if resource name is aliyun.com/gpu-mem,return the specified weight
		gpuMemoryWeight := float32(*scoreWeight) / float32(100)
		if string(resourceName) == GPUShareResourceMemoryName {
			return gpuMemoryWeight
		}
		return (1 - gpuMemoryWeight) / float32(length-1)
	}
	policy.IterateAllCandidateAllocations(candidateAllocations, func(combination []policy.CandidateAllocation) {
		alloc := map[types.DeviceId]int{}
		s := float32(0)
		n := len(combination)
		for index, candidateAllocation := range combination {
			for id, count := range candidateAllocation.Hint {
				alloc[id] = count
			}
			weight := getScoreWeight(resourceNames[index], n)
			s += candidateAllocation.Score * weight
		}
		for _, candidateAllocation := range combination {
			if len(candidateAllocation.Hint) != len(alloc) {
				return
			}
		}
		if s > maxScore {
			maxScore = s
			result = combination
		}
	})
	return result, maxScore
}

func buildPodAnnotations(pod *v1.Pod, podResources map[v1.ResourceName]noderesource.PodResource, nodeName string) (map[string]string, error) {
	annotations := map[string]string{}
	assumeTime := fmt.Sprintf("%d", time.Now().UnixNano())
	annotations[GPUShareV2PodAnnoAssumeTimeFlag] = fmt.Sprintf("%v", assumeTime)
	annotations[GPUShareV2PodAnnoAssignFlag] = "false"
	resourceInfos := GetResourceInfoList()
	for resourceName, podResource := range podResources {
		key := resourceInfos.GetPodAnnotationKey(resourceName)
		if key == "" {
			continue
		}
		annotations[key] = podResource.AllocatedResourcesToString()
		if resourceName == GPUShareResourceMemoryName {
			annotations[GPUShareV3DeviceCountFlag] = podResource.DeviceCountToString()
		}
	}
	return annotations, nil
}

// get replica from pod labels
func getReplicaDeviceCount(containerRequests map[types.ContainerIndex]v1.ResourceList, labels map[string]string) (int, error) {
	val, ok := labels[replicaDeviceLabelKey]
	// not specify the replica
	if !ok {
		return 0, nil
	}
	replica, err := strconv.Atoi(val)
	// invalid replica
	if err != nil {
		return 0, fmt.Errorf("failed to parse request device count from pod label %v,reason: %v", replicaDeviceLabelKey, err)
	}
	if replica < 0 {
		return 0, fmt.Errorf("request device count must be large than 0,but got %v", replica)
	}
	if replica == 0 {
		return 0, nil
	}
	for _, resourceList := range containerRequests {
		for resourceName, r := range resourceList {
			requestCount := int(r.Value())
			size := requestCount / replica
			if size*replica != requestCount {
				return 0, fmt.Errorf("invalid replica for resource %v,because %v / %v = %.1f", resourceName, requestCount, replica, float32(requestCount)/float32(replica))
			}
		}
	}
	return replica, nil
}
