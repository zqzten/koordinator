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

package resourcepolicy

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	listerschedulingv1alpha1 "github.com/koordinator-sh/koordinator/pkg/client/listers/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/util"
	reservationutil "github.com/koordinator-sh/koordinator/pkg/util/reservation"
)

var (
	_ framework.PreBindPlugin = &Plugin{}
	_ framework.ScorePlugin   = &Plugin{}
)

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	Name = "UnifiedResourcePolicy"
)

type Plugin struct {
	handle               framework.Handle
	resourcePolicyLister listerschedulingv1alpha1.ResourcePolicyLister
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	extendedHandle, ok := handle.(frameworkext.ExtendedHandle)
	if !ok {
		return nil, fmt.Errorf("want handle to be of type frameworkext.ExtendedHandle, got %T", handle)
	}
	koordSharedInformerFactory := extendedHandle.KoordinatorSharedInformerFactory()
	lister := koordSharedInformerFactory.Scheduling().V1alpha1().ResourcePolicies().Lister()
	return &Plugin{
		handle:               handle,
		resourcePolicyLister: lister,
	}, nil
}

func (p *Plugin) Name() string { return Name }

func (p *Plugin) Score(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()
	if node == nil {
		return 0, framework.NewStatus(framework.Error, "node not found")
	}
	resourcePolicy, err := p.getLatestResourcePolicy(node)
	if err != nil {
		return 0, framework.AsStatus(err)
	}
	score, err := scoreNodeByResourcePolicy(resourcePolicy, node)
	if err != nil {
		return 0, framework.AsStatus(err)
	}
	return score, nil
}

func (p *Plugin) getLatestResourcePolicy(node *corev1.Node) (resourcePolicy *schedulingv1alpha1.ResourcePolicy, err error) {
	resourcePolicies, err := p.resourcePolicyLister.List(labels.Everything())
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, candidateResourcePolicy := range resourcePolicies {
		if resourcePolicy == nil || resourcePolicy.CreationTimestamp.Before(&candidateResourcePolicy.CreationTimestamp) {
			resourcePolicy = candidateResourcePolicy
		}
	}
	return resourcePolicy, nil
}

func scoreNodeByResourcePolicy(resourcePolicy *schedulingv1alpha1.ResourcePolicy, node *corev1.Node) (int64, error) {
	if resourcePolicy == nil {
		return 0, nil
	}
	units := resourcePolicy.Spec.Units
	result := int64(len(units))
	for index, unit := range units {
		nodeSelector, err := metav1.LabelSelectorAsSelector(unit.NodeSelector)
		if err != nil {
			klog.Errorf("Fail to convert label selector for resourcePolicy %s, unit: %s: %v", err, resourcePolicy.Name, unit.Name)
			return 0, err
		}
		if nodeSelector.Matches(labels.Set(node.Labels)) {
			result = int64(index)
			break
		}
	}
	score := framework.MaxNodeScore - framework.MaxNodeScore*result/int64(len(units))
	return score, nil
}

func (p *Plugin) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func (p *Plugin) PreBind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	if reservationutil.IsReservePod(pod) {
		return nil
	}
	if _, ok := pod.Annotations[core.PodDeletionCost]; ok {
		return nil
	}
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	resourcePolicy, err := p.getLatestResourcePolicy(node)
	if err != nil {
		return framework.AsStatus(err)
	}

	score, err := scoreNodeByResourcePolicy(resourcePolicy, node)
	if err != nil {
		return framework.AsStatus(err)
	}

	podOriginal := pod
	pod = pod.DeepCopy()
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[core.PodDeletionCost] = strconv.FormatInt(score, 10)
	err = retry.OnError(
		retry.DefaultRetry,
		errors.IsTooManyRequests,
		func() error {
			_, err = util.PatchPod(p.handle.ClientSet(), podOriginal, pod)
			return err
		})
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	return nil
}
