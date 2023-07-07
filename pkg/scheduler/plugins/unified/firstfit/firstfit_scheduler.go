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

package firstfit

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	extenderv1 "k8s.io/kube-scheduler/extender/v1"
	"k8s.io/kubernetes/pkg/scheduler"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/parallelize"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	"k8s.io/kubernetes/pkg/scheduler/metrics"
	utiltrace "k8s.io/utils/trace"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	LabelFirstFitScheduling = apiext.SchedulingDomainPrefix + "/enable-first-fit"
)

type ScheduleAlgorithmFn = func(ctx context.Context, fwk framework.Framework, state *framework.CycleState, pod *corev1.Pod) (scheduler.ScheduleResult, error)

type firstFitScheduler struct {
	scheduler          *scheduler.Scheduler
	baseScheduleFn     ScheduleAlgorithmFn
	nextStartNodeIndex int
}

func SetupFirstFitScheduleAlgorithm(sched *scheduler.Scheduler) {
	firstFitScheduler := &firstFitScheduler{
		scheduler:      sched,
		baseScheduleFn: sched.SchedulePod,
	}
	sched.SchedulePod = firstFitScheduler.Schedule
}

func (g *firstFitScheduler) Schedule(ctx context.Context, fwk framework.Framework, state *framework.CycleState, pod *corev1.Pod) (result scheduler.ScheduleResult, err error) {
	trace := utiltrace.New("FirstFitScheduling", utiltrace.Field{Key: "namespace", Value: pod.Namespace}, utiltrace.Field{Key: "name", Value: pod.Name})
	defer trace.LogIfLong(100 * time.Millisecond)

	if pod.Labels[LabelFirstFitScheduling] != "true" {
		return g.baseScheduleFn(ctx, fwk, state, pod)
	}

	prepareFirstFitState := &prepareFirstFitStateData{}
	state.Write(prepareFirstFitStateKey, prepareFirstFitState)
	result, err = g.baseScheduleFn(ctx, fwk, state, pod)
	state.Delete(prepareFirstFitStateKey)

	if err == nil {
		return result, err
	} else {
		if !errors.Is(err, ErrForcedInterrupt) && !strings.Contains(err.Error(), Name) {
			klog.Errorf("expect disable scheduling for firstFit but failed, err: %v, pod: %v", err, klog.KObj(pod))
			return result, err
		}
	}

	nodes, err := fwk.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return result, err
	}
	if len(nodes) == 0 {
		return result, scheduler.ErrNoNodesAvailable
	}

	feasibleNodes, diagnosis, err := g.findNodesThatFitPod(ctx, fwk, prepareFirstFitState.provider, state, pod)
	if err != nil {
		return result, err
	}
	trace.Step("Computing predicates done")

	if len(feasibleNodes) == 0 {
		return result, &framework.FitError{
			Pod:         pod,
			NumAllNodes: len(nodes),
			Diagnosis:   diagnosis,
		}
	}

	// When only one node after predicate, just use it.
	if len(feasibleNodes) == 1 {
		return scheduler.ScheduleResult{
			SuggestedHost:  feasibleNodes[0].Name,
			EvaluatedNodes: 1 + len(diagnosis.NodeToStatusMap),
			FeasibleNodes:  1,
		}, nil
	}

	priorityList, err := prioritizeNodes(ctx, g.scheduler.Extenders, fwk, state, pod, feasibleNodes)
	if err != nil {
		return result, err
	}

	host, err := selectHost(priorityList)
	trace.Step("Prioritizing done")

	return scheduler.ScheduleResult{
		SuggestedHost:  host,
		EvaluatedNodes: len(feasibleNodes) + len(diagnosis.NodeToStatusMap),
		FeasibleNodes:  len(feasibleNodes),
	}, err
}

// Filters the nodes to find the ones that fit the pod based on the framework
// filter plugins and filter extenders.
func (g *firstFitScheduler) findNodesThatFitPod(ctx context.Context, fwk framework.Framework, ncp NodeCollectionProvider, state *framework.CycleState, pod *corev1.Pod) ([]*corev1.Node, framework.Diagnosis, error) {
	diagnosis := framework.Diagnosis{
		NodeToStatusMap:      make(framework.NodeToStatusMap),
		UnschedulablePlugins: sets.NewString(),
	}

	// Run "prefilter" plugins.
	preRes, s := fwk.RunPreFilterPlugins(ctx, state, pod)
	allNodes, err := fwk.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return nil, diagnosis, err
	}
	if !s.IsSuccess() {
		if !s.IsUnschedulable() {
			return nil, diagnosis, s.AsError()
		}
		// All nodes will have the same status. Some non trivial refactoring is
		// needed to avoid this copy.
		for _, n := range allNodes {
			diagnosis.NodeToStatusMap[n.Node().Name] = s
		}
		// Status satisfying IsUnschedulable() gets injected into diagnosis.UnschedulablePlugins.
		if s.FailedPlugin() != "" {
			diagnosis.UnschedulablePlugins.Insert(s.FailedPlugin())
		}
		return nil, diagnosis, nil
	}

	// "NominatedNodeName" can potentially be set in a previous scheduling cycle as a result of preemption.
	// This node is likely the only candidate that will fit the pod, and hence we try it first before iterating over all nodes.
	if len(pod.Status.NominatedNodeName) > 0 {
		feasibleNodes, err := g.evaluateNominatedNode(ctx, pod, fwk, state, diagnosis)
		if err != nil {
			klog.ErrorS(err, "Evaluation failed on nominated node", "pod", klog.KObj(pod), "node", pod.Status.NominatedNodeName)
		}
		// Nominated node passes all the filters, scheduler is good to assign this node to the pod.
		if len(feasibleNodes) != 0 {
			return feasibleNodes, diagnosis, nil
		}
	}

	nodes := allNodes
	if !preRes.AllNodes() {
		nodes = make([]*framework.NodeInfo, 0, len(preRes.NodeNames))
		for n := range preRes.NodeNames {
			nInfo, err := fwk.SnapshotSharedLister().NodeInfos().Get(n)
			if err != nil {
				return nil, diagnosis, err
			}
			nodes = append(nodes, nInfo)
		}
	}
	feasibleNodes, err := g.findNodesThatPassFilters(ctx, fwk, ncp, state, pod, diagnosis, nodes, preRes)
	if err != nil {
		return nil, diagnosis, err
	}

	feasibleNodes, err = findNodesThatPassExtenders(g.scheduler.Extenders, pod, feasibleNodes, diagnosis.NodeToStatusMap)
	if err != nil {
		return nil, diagnosis, err
	}
	return feasibleNodes, diagnosis, nil
}

func (g *firstFitScheduler) evaluateNominatedNode(ctx context.Context, pod *corev1.Pod, fwk framework.Framework, state *framework.CycleState, diagnosis framework.Diagnosis) ([]*corev1.Node, error) {
	nnn := pod.Status.NominatedNodeName
	nodeInfo, err := fwk.SnapshotSharedLister().NodeInfos().Get(nnn)
	if err != nil {
		return nil, err
	}
	node := []*framework.NodeInfo{nodeInfo}
	feasibleNodes, err := g.findNodesThatPassFilters(ctx, fwk, nil, state, pod, diagnosis, node, nil)
	if err != nil {
		return nil, err
	}

	feasibleNodes, err = findNodesThatPassExtenders(g.scheduler.Extenders, pod, feasibleNodes, diagnosis.NodeToStatusMap)
	if err != nil {
		return nil, err
	}

	return feasibleNodes, nil
}

// findNodesThatPassFilters finds the nodes that fit the filter plugins.
func (g *firstFitScheduler) findNodesThatPassFilters(
	ctx context.Context,
	fwk framework.Framework,
	ncp NodeCollectionProvider,
	state *framework.CycleState,
	pod *corev1.Pod,
	diagnosis framework.Diagnosis,
	nodes []*framework.NodeInfo,
	preRes *framework.PreFilterResult,
) ([]*corev1.Node, error) {
	// NOTE(joseph): FirstFit just need one node
	numNodesToFind := int32(1)

	// Create feasible list with enough space to avoid growing it
	// and allow assigning.
	feasibleNodes := make([]*corev1.Node, numNodesToFind)

	if !fwk.HasFilterPlugins() {
		length := len(nodes)
		for i := range feasibleNodes {
			feasibleNodes[i] = nodes[(g.nextStartNodeIndex+i)%length].Node()
		}
		g.nextStartNodeIndex = (g.nextStartNodeIndex + len(feasibleNodes)) % length
		return feasibleNodes, nil
	}

	if ncp != nil && len(nodes) > 1 {
		feasibleNodes, err := g.findNodesThatPassFiltersViaNodeCollection(ctx, fwk, ncp, state, pod, diagnosis, preRes)
		if err != nil {
			return nil, err
		}
		if len(feasibleNodes) > 0 {
			return feasibleNodes, nil
		}
	}

	errCh := parallelize.NewErrorChannel()
	var statusesLock sync.Mutex
	var feasibleNodesLen int32
	ctx, cancel := context.WithCancel(ctx)
	checkNode := func(i int) {
		// We check the nodes starting from where we left off in the previous scheduling cycle,
		// this is to make sure all nodes have the same chance of being examined across pods.
		nodeInfo := nodes[(g.nextStartNodeIndex+i)%len(nodes)]
		status := fwk.RunFilterPluginsWithNominatedPods(ctx, state, pod, nodeInfo)
		if status.Code() == framework.Error {
			errCh.SendErrorWithCancel(status.AsError(), cancel)
			return
		}
		if status.IsSuccess() {
			length := atomic.AddInt32(&feasibleNodesLen, 1)
			if length > numNodesToFind {
				cancel()
				atomic.AddInt32(&feasibleNodesLen, -1)
			} else {
				feasibleNodes[length-1] = nodeInfo.Node()
			}
		} else {
			statusesLock.Lock()
			diagnosis.NodeToStatusMap[nodeInfo.Node().Name] = status
			diagnosis.UnschedulablePlugins.Insert(status.FailedPlugin())
			statusesLock.Unlock()
		}
	}

	beginCheckNode := time.Now()
	statusCode := framework.Success
	defer func() {
		// We record Filter extension point latency here instead of in framework.go because framework.RunFilterPlugins
		// function is called for each node, whereas we want to have an overall latency for all nodes per scheduling cycle.
		// Note that this latency also includes latency for `addNominatedPods`, which calls framework.RunPreFilterAddPod.
		metrics.FrameworkExtensionPointDuration.WithLabelValues(frameworkruntime.Filter, statusCode.String(), fwk.ProfileName()).Observe(metrics.SinceInSeconds(beginCheckNode))
	}()

	// Stops searching for more nodes once the configured number of feasible nodes
	// are found.
	fwk.Parallelizer().Until(ctx, len(nodes), checkNode)
	processedNodes := int(feasibleNodesLen) + len(diagnosis.NodeToStatusMap)
	g.nextStartNodeIndex = (g.nextStartNodeIndex + processedNodes) % len(nodes)

	feasibleNodes = feasibleNodes[:feasibleNodesLen]
	if err := errCh.ReceiveError(); err != nil {
		statusCode = framework.Error
		return nil, err
	}
	return feasibleNodes, nil
}

func findNodesThatPassExtenders(extenders []framework.Extender, pod *corev1.Pod, feasibleNodes []*corev1.Node, statuses framework.NodeToStatusMap) ([]*corev1.Node, error) {
	// Extenders are called sequentially.
	// Nodes in original feasibleNodes can be excluded in one extender, and pass on to the next
	// extender in a decreasing manner.
	for _, extender := range extenders {
		if len(feasibleNodes) == 0 {
			break
		}
		if !extender.IsInterested(pod) {
			continue
		}

		// Status of failed nodes in failedAndUnresolvableMap will be added or overwritten in <statuses>,
		// so that the scheduler framework can respect the UnschedulableAndUnresolvable status for
		// particular nodes, and this may eventually improve preemption efficiency.
		// Note: users are recommended to configure the extenders that may return UnschedulableAndUnresolvable
		// status ahead of others.
		feasibleList, failedMap, failedAndUnresolvableMap, err := extender.Filter(pod, feasibleNodes)
		if err != nil {
			if extender.IsIgnorable() {
				klog.InfoS("Skipping extender as it returned error and has ignorable flag set", "extender", extender, "err", err)
				continue
			}
			return nil, err
		}

		for failedNodeName, failedMsg := range failedAndUnresolvableMap {
			var aggregatedReasons []string
			if _, found := statuses[failedNodeName]; found {
				aggregatedReasons = statuses[failedNodeName].Reasons()
			}
			aggregatedReasons = append(aggregatedReasons, failedMsg)
			statuses[failedNodeName] = framework.NewStatus(framework.UnschedulableAndUnresolvable, aggregatedReasons...)
		}

		for failedNodeName, failedMsg := range failedMap {
			if _, found := failedAndUnresolvableMap[failedNodeName]; found {
				// failedAndUnresolvableMap takes precedence over failedMap
				// note that this only happens if the extender returns the node in both maps
				continue
			}
			if _, found := statuses[failedNodeName]; !found {
				statuses[failedNodeName] = framework.NewStatus(framework.Unschedulable, failedMsg)
			} else {
				statuses[failedNodeName].AppendReason(failedMsg)
			}
		}

		feasibleNodes = feasibleList
	}
	return feasibleNodes, nil
}

// prioritizeNodes prioritizes the nodes by running the score plugins,
// which return a score for each node from the call to RunScorePlugins().
// The scores from each plugin are added together to make the score for that node, then
// any extenders are run as well.
// All scores are finally combined (added) to get the total weighted scores of all nodes
func prioritizeNodes(
	ctx context.Context,
	extenders []framework.Extender,
	fwk framework.Framework,
	state *framework.CycleState,
	pod *corev1.Pod,
	nodes []*corev1.Node,
) (framework.NodeScoreList, error) {
	// If no priority configs are provided, then all nodes will have a score of one.
	// This is required to generate the priority list in the required format
	if len(extenders) == 0 && !fwk.HasScorePlugins() {
		result := make(framework.NodeScoreList, 0, len(nodes))
		for i := range nodes {
			result = append(result, framework.NodeScore{
				Name:  nodes[i].Name,
				Score: 1,
			})
		}
		return result, nil
	}

	// Run PreScore plugins.
	preScoreStatus := fwk.RunPreScorePlugins(ctx, state, pod, nodes)
	if !preScoreStatus.IsSuccess() {
		return nil, preScoreStatus.AsError()
	}

	// Run the Score plugins.
	scoresMap, scoreStatus := fwk.RunScorePlugins(ctx, state, pod, nodes)
	if !scoreStatus.IsSuccess() {
		return nil, scoreStatus.AsError()
	}

	// Additional details logged at level 10 if enabled.
	klogV := klog.V(10)
	if klogV.Enabled() {
		for plugin, nodeScoreList := range scoresMap {
			for _, nodeScore := range nodeScoreList {
				klogV.InfoS("Plugin scored node for pod", "pod", klog.KObj(pod), "plugin", plugin, "node", nodeScore.Name, "score", nodeScore.Score)
			}
		}
	}

	// Summarize all scores.
	result := make(framework.NodeScoreList, 0, len(nodes))

	for i := range nodes {
		result = append(result, framework.NodeScore{Name: nodes[i].Name, Score: 0})
		for j := range scoresMap {
			result[i].Score += scoresMap[j][i].Score
		}
	}

	if len(extenders) != 0 && nodes != nil {
		var mu sync.Mutex
		var wg sync.WaitGroup
		combinedScores := make(map[string]int64, len(nodes))
		for i := range extenders {
			if !extenders[i].IsInterested(pod) {
				continue
			}
			wg.Add(1)
			go func(extIndex int) {
				metrics.SchedulerGoroutines.WithLabelValues(metrics.PrioritizingExtender).Inc()
				defer func() {
					metrics.SchedulerGoroutines.WithLabelValues(metrics.PrioritizingExtender).Dec()
					wg.Done()
				}()
				prioritizedList, weight, err := extenders[extIndex].Prioritize(pod, nodes)
				if err != nil {
					// Prioritization errors from extender can be ignored, let k8s/other extenders determine the priorities
					klog.V(5).InfoS("Failed to run extender's priority function. No score given by this extender.", "error", err, "pod", klog.KObj(pod), "extender", extenders[extIndex].Name())
					return
				}
				mu.Lock()
				for i := range *prioritizedList {
					host, score := (*prioritizedList)[i].Host, (*prioritizedList)[i].Score
					if klogV.Enabled() {
						klogV.InfoS("Extender scored node for pod", "pod", klog.KObj(pod), "extender", extenders[extIndex].Name(), "node", host, "score", score)
					}
					combinedScores[host] += score * weight
				}
				mu.Unlock()
			}(i)
		}
		// wait for all go routines to finish
		wg.Wait()
		for i := range result {
			// MaxExtenderPriority may diverge from the max priority used in the scheduler and defined by MaxNodeScore,
			// therefore we need to scale the score returned by extenders to the score range used by the scheduler.
			result[i].Score += combinedScores[result[i].Name] * (framework.MaxNodeScore / extenderv1.MaxExtenderPriority)
		}
	}

	if klogV.Enabled() {
		for i := range result {
			klogV.InfoS("Calculated node's final score for pod", "pod", klog.KObj(pod), "node", result[i].Name, "score", result[i].Score)
		}
	}
	return result, nil
}

// selectHost takes a prioritized list of nodes and then picks one
// in a reservoir sampling manner from the nodes that had the highest score.
func selectHost(nodeScoreList framework.NodeScoreList) (string, error) {
	if len(nodeScoreList) == 0 {
		return "", fmt.Errorf("empty priorityList")
	}
	maxScore := nodeScoreList[0].Score
	selected := nodeScoreList[0].Name
	cntOfMaxScore := 1
	for _, ns := range nodeScoreList[1:] {
		if ns.Score > maxScore {
			maxScore = ns.Score
			selected = ns.Name
			cntOfMaxScore = 1
		} else if ns.Score == maxScore {
			cntOfMaxScore++
			if rand.Intn(cntOfMaxScore) == 0 {
				// Replace the candidate with probability of 1/cntOfMaxScore
				selected = ns.Name
			}
		}
	}
	return selected, nil
}

// findNodesThatPassFilters finds the nodes that fit the filter plugins.
func (g *firstFitScheduler) findNodesThatPassFiltersViaNodeCollection(
	ctx context.Context,
	fwk framework.Framework,
	ncp NodeCollectionProvider,
	state *framework.CycleState,
	pod *corev1.Pod,
	diagnosis framework.Diagnosis,
	preRes *framework.PreFilterResult) ([]*corev1.Node, error) {

	nodeCollection := ncp.GetNodeCollection()
	nodeIterator := nodeCollection.GetNodeIterator()

	// NOTE(joseph): FirstFit just need one node
	numNodesToFind := int32(1)

	// Create feasible list with enough space to avoid growing it
	// and allow assigning.
	feasibleNodes := make([]*corev1.Node, numNodesToFind)

	errCh := parallelize.NewErrorChannel()
	var statusesLock sync.Mutex
	var feasibleNodesLen int32
	ctx, cancel := context.WithCancel(ctx)
	checkNode := func(i int) {
		if !nodeIterator.HasNext() {
			cancel()
			return
		}

		node := nodeIterator.Next()
		if preRes != nil && !preRes.NodeNames.Has(node.Name) {
			return
		}

		nodeInfo, err := fwk.SnapshotSharedLister().NodeInfos().Get(node.Name)
		if err != nil {
			return
		}

		status := fwk.RunFilterPluginsWithNominatedPods(ctx, state, pod, nodeInfo)
		if status.Code() == framework.Error {
			errCh.SendErrorWithCancel(status.AsError(), cancel)
			return
		}
		if status.IsSuccess() {
			length := atomic.AddInt32(&feasibleNodesLen, 1)
			if length > numNodesToFind {
				cancel()
				atomic.AddInt32(&feasibleNodesLen, -1)
			} else {
				feasibleNodes[length-1] = nodeInfo.Node()
			}
		} else {
			statusesLock.Lock()
			diagnosis.NodeToStatusMap[nodeInfo.Node().Name] = status
			diagnosis.UnschedulablePlugins.Insert(status.FailedPlugin())
			statusesLock.Unlock()
		}
	}

	beginCheckNode := time.Now()
	statusCode := framework.Success
	defer func() {
		// We record Filter extension point latency here instead of in framework.go because framework.RunFilterPlugins
		// function is called for each node, whereas we want to have an overall latency for all nodes per scheduling cycle.
		// Note that this latency also includes latency for `addNominatedPods`, which calls framework.RunPreFilterAddPod.
		metrics.FrameworkExtensionPointDuration.WithLabelValues(frameworkruntime.Filter, statusCode.String(), fwk.ProfileName()).Observe(metrics.SinceInSeconds(beginCheckNode))
	}()

	// Stops searching for more nodes once the configured number of feasible nodes are found.
	fwk.Parallelizer().Until(ctx, int(nodeIterator.Size()), checkNode)
	feasibleNodes = feasibleNodes[:feasibleNodesLen]
	if err := errCh.ReceiveError(); err != nil {
		statusCode = framework.Error
		return nil, err
	}
	return feasibleNodes, nil
}
