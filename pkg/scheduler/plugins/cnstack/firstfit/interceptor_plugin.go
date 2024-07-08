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
)

const (
	Name = "FirstFitInterceptor"

	firstFitDisableStateKey = "firstFitDisableStateKey"
)

var (
	ErrFirstFitDisabled = errors.New("disabled scheduling for firstFit")
)

var _ framework.PreFilterPlugin = &InterceptorPlugin{}

// InterceptorPlugin must be configured first
type InterceptorPlugin struct {
}

func NewInterceptorPlugin(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	return &InterceptorPlugin{}, nil
}

func (pl *InterceptorPlugin) Name() string {
	return Name
}

func (pl *InterceptorPlugin) PreFilter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod) *framework.Status {
	if pod.Labels[LabelFirstFitScheduling] == "true" {
		if _, err := state.Read(firstFitDisableStateKey); err == nil {
			return framework.AsStatus(ErrFirstFitDisabled)
		}
	}
	return nil
}

func (pl *InterceptorPlugin) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}
