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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/validation"
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
func (pl *NodeAffinity) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) *framework.Status {
	state := &preFilterState{requiredNodeSelectorAndAffinity: nodeaffinityhelper.GetRequiredNodeAffinity(pod)}
	cycleState.Write(preFilterStateKey, state)
	return nil
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
func New(plArgs runtime.Object, h framework.Handle) (framework.Plugin, error) {
	nodeAffinityArgs, err := getArgs(plArgs)
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

func getArgs(obj runtime.Object) (*config.NodeAffinityArgs, error) {
	nodeAffinityArgs, err := getDefaultNodeAffinityArgs()
	if err != nil {
		return nil, err
	}

	if obj != nil {
		switch t := obj.(type) {
		case *config.NodeAffinityArgs:
			nodeAffinityArgs = t
		case *runtime.Unknown:
			unknownObj := t
			if err := frameworkruntime.DecodeInto(unknownObj, nodeAffinityArgs); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("got args of type %T, want *NodeAffinityArgs", obj)
		}
	}
	return nodeAffinityArgs, validation.ValidateNodeAffinityArgs(nil, nodeAffinityArgs)
}

func getDefaultNodeAffinityArgs() (*config.NodeAffinityArgs, error) {
	return &config.NodeAffinityArgs{
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
