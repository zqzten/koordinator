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

package podtopologyspread

import (
	"fmt"
	"strings"

	"gitlab.alibaba-inc.com/serverlessinfra/dummy-workload/pkg/client/listers/acs/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	v1helper "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/helper"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/features"
	tainttolerationhelper "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/tainttoleration"
)

type topologyPair struct {
	key   string
	value string
}

// topologySpreadConstraint is an internal version for v1.TopologySpreadConstraint
// and where the selector is parsed.
// Fields are exported for comparison during testing.
type topologySpreadConstraint struct {
	MaxSkew     int32
	TopologyKey string
	Selector    labels.Selector
}

// buildDefaultConstraints builds the constraints for a pod using
// .DefaultConstraints and the selectors from the services, replication
// controllers, replica sets and stateful sets that match the pod.
func (pl *PodTopologySpread) buildDefaultConstraints(p *v1.Pod, action v1.UnsatisfiableConstraintAction) ([]topologySpreadConstraint, error) {
	constraints, err := filterTopologySpreadConstraints(pl.defaultConstraints, p.Labels, p.Annotations, action)
	if err != nil || len(constraints) == 0 {
		return nil, err
	}
	var selector labels.Selector
	if k8sfeature.DefaultFeatureGate.Enabled(features.EnableACSDefaultSpread) {
		selector = defaultSelectorFromDummyWorkload(p, pl.dummyWorkloadLister)
	} else {
		selector = helper.DefaultSelector(p, pl.services, pl.replicationCtrls, pl.replicaSets, pl.statefulSets)
	}
	if selector == nil || selector.Empty() {
		return nil, nil
	}
	for i := range constraints {
		constraints[i].Selector = selector
	}
	return constraints, nil
}

func defaultSelectorFromDummyWorkload(pod *v1.Pod, lister v1alpha1.DummyWorkloadLister) labels.Selector {
	ownerReferences, err := extunified.GetTenancyOwnerReferences(pod.Annotations)
	if err != nil {
		klog.ErrorS(err, "Failed to unmarshal ownerReferences from Pod", "pod", klog.KObj(pod))
		return nil
	}
	if len(ownerReferences) == 0 {
		return nil
	}
	var workloadName string
	for _, ref := range ownerReferences {
		if ref.Controller != nil && *ref.Controller == true {
			workloadName = fmt.Sprintf("%s-%s", ref.Name, strings.ToLower(ref.Kind))
			break
		}
	}
	if workloadName == "" {
		return nil
	}
	dummyWorkload, err := lister.DummyWorkloads(pod.Namespace).Get(workloadName)
	if err != nil {
		if klog.V(5).Enabled() {
			klog.ErrorS(err, "Failed to get dummyWorkload", "name", workloadName)
		}
		return nil
	}
	if dummyWorkload.Status.LabelSelector == "" {
		return nil
	}
	selector, err := labels.Parse(dummyWorkload.Status.LabelSelector)
	if err != nil {
		klog.ErrorS(err, "Failed to parse labelSelector", "dummyWorkload", workloadName)
		return nil
	}
	return selector
}

// nodeLabelsMatchSpreadConstraints checks if ALL topology keys in spread Constraints are present in node labels.
func nodeLabelsMatchSpreadConstraints(nodeLabels map[string]string, constraints []topologySpreadConstraint) bool {
	for _, c := range constraints {
		if _, ok := nodeLabels[c.TopologyKey]; !ok {
			return false
		}
	}
	return true
}

func filterTopologySpreadConstraints(constraints []v1.TopologySpreadConstraint, podLabels, podAnnotations map[string]string, action v1.UnsatisfiableConstraintAction) ([]topologySpreadConstraint, error) {
	var result []topologySpreadConstraint
	for _, c := range constraints {
		if c.WhenUnsatisfiable == action {
			selector, err := metav1.LabelSelectorAsSelector(c.LabelSelector)
			if err != nil {
				return nil, err
			}
			if k8sfeature.DefaultFeatureGate.Enabled(features.EnableMatchLabelKeysInPodTopologySpread) {
				podMatchLabelKeys, err := extunified.GetMatchLabelKeysInPodTopologySpread(podAnnotations)
				if err != nil {
					return nil, err
				}
				if len(podMatchLabelKeys) > 0 {
					matchLabels := make(labels.Set)
					for _, labelKey := range podMatchLabelKeys {
						if value, ok := podLabels[labelKey]; ok {
							matchLabels[labelKey] = value
						}
					}
					if len(matchLabels) > 0 {
						selector = mergeLabelSetWithSelector(matchLabels, selector)
					}
				}

			}
			result = append(result, topologySpreadConstraint{
				MaxSkew:     c.MaxSkew,
				TopologyKey: c.TopologyKey,
				Selector:    selector,
			})
		}
	}
	return result, nil
}

func mergeLabelSetWithSelector(matchLabels labels.Set, s labels.Selector) labels.Selector {
	mergedSelector := labels.SelectorFromSet(matchLabels)

	requirements, ok := s.Requirements()
	if !ok {
		return s
	}

	for _, r := range requirements {
		mergedSelector = mergedSelector.Add(r)
	}

	return mergedSelector
}

func countPodsMatchSelector(podInfos []*framework.PodInfo, selector labels.Selector, ns string) int {
	count := 0
	for _, p := range podInfos {
		// Bypass terminating Pod (see #87621).
		if p.Pod.DeletionTimestamp != nil || p.Pod.Namespace != ns {
			continue
		}
		if selector.Matches(labels.Set(p.Pod.Labels)) {
			count++
		}
	}
	return count
}

func tolerationTolerateNodeFn(pod *v1.Pod) func(node *v1.Node) bool {
	if !k8sfeature.DefaultFeatureGate.Enabled(features.DefaultHonorTaintTolerationInTopologySpread) {
		return func(node *v1.Node) bool {
			return true
		}
	}
	tolerations := pod.Spec.Tolerations
	tolerationsToleratesECI := tainttolerationhelper.TolerationsToleratesECI(pod)
	podAffinityECI := extunified.AffinityECI(pod)
	return func(node *v1.Node) bool {
		tolerationsMakeEffect := tolerations
		if podAffinityECI && extunified.IsVirtualKubeletNode(node) {
			tolerationsMakeEffect = tolerationsToleratesECI
		}
		_, isUnTolerated := v1helper.FindMatchingUntoleratedTaint(node.Spec.Taints, tolerationsMakeEffect, func(t *v1.Taint) bool {
			// PodToleratesNodeTaints is only interested in NoSchedule and NoExecute taints.
			return t.Effect == v1.TaintEffectNoSchedule || t.Effect == v1.TaintEffectNoExecute
		})
		return !isUnTolerated
	}
}
