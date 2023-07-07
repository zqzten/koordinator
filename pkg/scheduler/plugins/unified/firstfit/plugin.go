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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/firstfit/nodesorting"
)

const (
	Name = "FirstFit"

	prepareFirstFitStateKey = "prepareFirstFitStateKey"
)

var (
	ErrForcedInterrupt = errors.New("forced interrupt scheduling for firstFit")
)

var _ framework.PreFilterPlugin = &Plugin{}
var _ framework.ReservePlugin = &Plugin{}

var _ frameworkext.PreFilterTransformer = &Plugin{}

type NodeCollectionProvider interface {
	GetNodeCollection() nodesorting.NodeCollection
}

// Plugin must be configured first
type Plugin struct {
	nodeCollection nodesorting.NodeCollection
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	nodeCollection := nodesorting.NewNodeCollection(handle.SharedInformerFactory())
	return &Plugin{
		nodeCollection: nodeCollection,
	}, nil
}

func (pl *Plugin) Name() string {
	return Name
}

type prepareFirstFitStateData struct {
	provider NodeCollectionProvider
}

func (s *prepareFirstFitStateData) Clone() framework.StateData {
	return s
}

func (pl *Plugin) GetNodeCollection() nodesorting.NodeCollection {
	return pl.nodeCollection
}

func (pl *Plugin) BeforePreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*corev1.Pod, bool, *framework.Status) {
	if pod.Labels[LabelFirstFitScheduling] == "true" {
		if state, err := cycleState.Read(prepareFirstFitStateKey); err == nil {
			firstFitStateData := state.(*prepareFirstFitStateData)
			firstFitStateData.provider = pl
			return nil, false, framework.AsStatus(ErrForcedInterrupt)
		}
	}
	return nil, false, nil
}

func (pl *Plugin) AfterPreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) *framework.Status {
	return nil
}

func (pl *Plugin) PreFilter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	return nil, nil
}

func (pl *Plugin) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (pl *Plugin) Reserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	node := pl.nodeCollection.GetNode(nodeName)
	if node != nil {
		node.AddPod(pod)
		pl.nodeCollection.NodeUpdated(node)
	}
	return nil
}

func (pl *Plugin) Unreserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) {
	node := pl.nodeCollection.GetNode(nodeName)
	if node != nil {
		node.DeletePod(pod)
		pl.nodeCollection.NodeUpdated(node)
	}
}
