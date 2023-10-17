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

package nodeaffinity

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	scheduledconfigv1beta2config "k8s.io/kube-scheduler/config/v1beta2"
	schedconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/v1beta2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	kubenodeaffinity "k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeaffinity"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"

	nodeaffinityhelper "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/nodeaffinity"
)

// NodeAffinity is a plugin that checks if a pod node selector matches the node label.
type NodeAffinity struct {
	*kubenodeaffinity.NodeAffinity
	addedNodeSelector *nodeaffinity.NodeSelector
}

var _ framework.PreFilterPlugin = &NodeAffinity{}
var _ framework.FilterPlugin = &NodeAffinity{}
var _ framework.PreScorePlugin = &NodeAffinity{}
var _ framework.ScorePlugin = &NodeAffinity{}
var _ framework.EnqueueExtensions = &NodeAffinity{}

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	Name = "UnifiedNodeAffinity"

	// preFilterStateKey is the key in CycleState to NodeAffinity pre-compute data for Filtering.
	preFilterStateKey = "PreFilter" + Name

	// ErrReasonPod is the reason for Pod's node affinity/selector not matching.
	ErrReasonPod = "node(s) didn't match Pod's node affinity/selector"

	// errReasonEnforced is the reason for added node affinity not matching.
	errReasonEnforced = "node(s) didn't match scheduler-enforced node affinity"

	// errReasonConflict is the reason for pod's conflicting affinity rules.
	errReasonConflict = "pod affinity terms conflict"
)

// Name returns name of the plugin. It is used in logs, etc.
func (pl *NodeAffinity) Name() string {
	return Name
}

type preFilterState struct {
	requiredNodeSelectorAndAffinity nodeaffinityhelper.RequiredNodeSelectorAndAffinity
}

// Clone just returns the same state because it is not affected by pod additions or deletions.
func (s *preFilterState) Clone() framework.StateData {
	return s
}

// PreFilter builds and writes cycle state used by Filter.
func (pl *NodeAffinity) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	state := &preFilterState{requiredNodeSelectorAndAffinity: nodeaffinityhelper.GetRequiredNodeAffinity(pod)}
	cycleState.Write(preFilterStateKey, state)
	affinity := pod.Spec.Affinity
	if affinity == nil ||
		affinity.NodeAffinity == nil ||
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil ||
		len(affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0 {
		return nil, nil
	}

	// Check if there is affinity to a specific node and return it.
	terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	var nodeNames sets.String
	for _, t := range terms {
		var termNodeNames sets.String
		for _, r := range t.MatchFields {
			if r.Key == metav1.ObjectNameField && r.Operator == corev1.NodeSelectorOpIn {
				// The requirements represent ANDed constraints, and so we need to
				// find the intersection of nodes.
				s := sets.NewString(r.Values...)
				if termNodeNames == nil {
					termNodeNames = s
				} else {
					termNodeNames = termNodeNames.Intersection(s)
				}
			}
		}
		if termNodeNames == nil {
			// If this term has no node.Name field affinity,
			// then all nodes are eligible because the terms are ORed.
			return nil, nil
		}
		// If the set is empty, it means the terms had affinity to different
		// sets of nodes, and since they are ANDed, then the pod will not match any node.
		if len(termNodeNames) == 0 {
			return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, errReasonConflict)
		}
		nodeNames = nodeNames.Union(termNodeNames)
	}
	if nodeNames != nil {
		return &framework.PreFilterResult{NodeNames: nodeNames}, nil
	}
	return nil, nil
}

// Filter checks if the Node matches the Pod .spec.affinity.nodeAffinity and
// the plugin's added affinity.
func (pl *NodeAffinity) Filter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if pl.addedNodeSelector != nil && !pl.addedNodeSelector.Match(node) {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, errReasonEnforced)
	}

	s, err := getPreFilterState(state)
	if err != nil {
		// Fallback to calculate requiredNodeSelector and requiredNodeAffinity
		// here when PreFilter is disabled.
		s = &preFilterState{requiredNodeSelectorAndAffinity: nodeaffinityhelper.GetRequiredNodeAffinity(pod)}
	}

	// Ignore parsing errors for backwards compatibility.
	match := s.requiredNodeSelectorAndAffinity.Match(node)
	if !match {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, ErrReasonPod)
	}
	return nil
}

// New initializes a new plugin and returns it.
func New(obj runtime.Object, h framework.Handle) (framework.Plugin, error) {
	nodeAffinityArgs, err := getArgs(obj)
	if err != nil {
		return nil, err
	}

	pl := &NodeAffinity{}
	if nodeAffinityArgs.AddedAffinity != nil {
		if ns := nodeAffinityArgs.AddedAffinity.RequiredDuringSchedulingIgnoredDuringExecution; ns != nil {
			pl.addedNodeSelector, err = nodeaffinity.NewNodeSelector(ns)
			if err != nil {
				return nil, fmt.Errorf("parsing addedAffinity.requiredDuringSchedulingIgnoredDuringExecution: %w", err)
			}
		}
	}
	internalPlugin, err := kubenodeaffinity.New(nodeAffinityArgs, h)
	if err != nil {
		return nil, err
	}
	pl.NodeAffinity = internalPlugin.(*kubenodeaffinity.NodeAffinity)
	return pl, nil
}

func getArgs(obj runtime.Object) (*schedconfig.NodeAffinityArgs, error) {
	if obj == nil {
		return getDefaultNodeAffinityArgs()
	}

	if args, ok := obj.(*schedconfig.NodeAffinityArgs); ok {
		return args, nil
	}

	unknownObj, ok := obj.(*runtime.Unknown)
	if !ok {
		return nil, fmt.Errorf("got args of type %T, want *NodeAffinityArgs", obj)
	}

	var v1beta2args scheduledconfigv1beta2config.NodeAffinityArgs
	if err := frameworkruntime.DecodeInto(unknownObj, &v1beta2args); err != nil {
		return nil, err
	}
	var nodeAffinityArgs schedconfig.NodeAffinityArgs
	err := v1beta2.Convert_v1beta2_NodeAffinityArgs_To_config_NodeAffinityArgs(&v1beta2args, &nodeAffinityArgs, nil)
	if err != nil {
		return nil, err
	}
	return &nodeAffinityArgs, nil
}

func getDefaultNodeAffinityArgs() (*schedconfig.NodeAffinityArgs, error) {
	return &schedconfig.NodeAffinityArgs{
		AddedAffinity: nil,
	}, nil
}

func getPreFilterState(cycleState *framework.CycleState) (*preFilterState, error) {
	c, err := cycleState.Read(preFilterStateKey)
	if err != nil {
		return nil, fmt.Errorf("reading %q from cycleState: %v", preFilterStateKey, err)
	}

	s, ok := c.(*preFilterState)
	if !ok {
		return nil, fmt.Errorf("invalid PreFilter state, got type %T", c)
	}
	return s, nil
}
