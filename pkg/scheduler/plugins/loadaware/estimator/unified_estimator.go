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

package estimator

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
)

func init() {
	Estimators[unifiedEstimatorName] = NewUnifiedEstimator
}

const (
	unifiedEstimatorName = "unifiedEstimator"
)

var _ Estimator = &UnifiedEstimator{}

type UnifiedEstimator struct {
	Estimator
}

func NewUnifiedEstimator(args *config.LoadAwareSchedulingArgs, handle framework.Handle) (Estimator, error) {
	defaultEstimator, err := NewDefaultEstimator(args, handle)
	if err != nil {
		return nil, err
	}
	return &UnifiedEstimator{
		Estimator: defaultEstimator,
	}, nil
}

func (e *UnifiedEstimator) Name() string {
	return unifiedEstimatorName
}

func (e *UnifiedEstimator) EstimateNode(node *corev1.Node) (corev1.ResourceList, error) {
	originalAllocatable, err := unified.GetOriginalNodeAllocatable(node.Annotations)
	if err != nil {
		klog.ErrorS(err, "Failed to GetOriginalNodeAllocatable", "node", node.Name)
		return node.Status.Allocatable, nil
	}
	if len(originalAllocatable) == 0 {
		return node.Status.Allocatable, nil
	}
	allocatableCopy := node.Status.Allocatable.DeepCopy()
	if allocatableCopy == nil {
		allocatableCopy = corev1.ResourceList{}
	}
	if quantity := originalAllocatable[corev1.ResourceCPU]; quantity.MilliValue() > 0 {
		allocatableCopy[corev1.ResourceCPU] = quantity.DeepCopy()
	}
	if quantity := originalAllocatable[corev1.ResourceMemory]; quantity.Value() > 0 {
		allocatableCopy[corev1.ResourceMemory] = quantity.DeepCopy()
	}
	if quantity := originalAllocatable[corev1.ResourceEphemeralStorage]; quantity.Value() > 0 {
		allocatableCopy[corev1.ResourceEphemeralStorage] = quantity.DeepCopy()
	}
	return allocatableCopy, nil
}
