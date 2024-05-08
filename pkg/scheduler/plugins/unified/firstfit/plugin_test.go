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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"
)

func TestBeforePreFilter(t *testing.T) {
	tests := []struct {
		name          string
		pod           *corev1.Pod
		needInterrupt bool
		wantErr       error
	}{
		{
			name:          "normal pod",
			pod:           &corev1.Pod{},
			needInterrupt: false,
			wantErr:       nil,
		},
		{
			name: "firstFit pod forced interrupt scheduling",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LabelFirstFitScheduling: "true",
					},
				},
			},
			needInterrupt: true,
			wantErr:       ErrForcedInterrupt,
		},
		{
			name: "firstFit pod and allow scheduling",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LabelFirstFitScheduling: "true",
					},
				},
			},
			needInterrupt: false,
			wantErr:       nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := kubefake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			registeredPlugins := []schedulertesting.RegisterPluginFunc{
				schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
				schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
			}
			fh, err := schedulertesting.NewFramework(
				context.TODO(),
				registeredPlugins,
				"koord-scheduler",
				frameworkruntime.WithInformerFactory(informerFactory),
			)
			assert.NoError(t, err)
			plugin, err := New(nil, fh)
			assert.NoError(t, err)
			assert.NotNil(t, plugin)
			pl := plugin.(*Plugin)
			state := framework.NewCycleState()
			prepareFirstFitState := &prepareFirstFitStateData{}
			if tt.needInterrupt {
				state.Write(prepareFirstFitStateKey, prepareFirstFitState)
			}
			_, _, status := pl.BeforePreFilter(context.TODO(), state, tt.pod)
			assert.Equal(t, status.AsError(), tt.wantErr)
			if tt.needInterrupt {
				assert.NotNil(t, prepareFirstFitState.provider)
			}
		})
	}
}
