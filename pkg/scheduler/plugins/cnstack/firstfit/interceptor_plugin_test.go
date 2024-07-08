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
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

func TestInterceptorPreFilter(t *testing.T) {
	tests := []struct {
		name        string
		pod         *corev1.Pod
		needDisable bool
		wantErr     error
	}{
		{
			name:        "normal pod",
			pod:         &corev1.Pod{},
			needDisable: false,
			wantErr:     nil,
		},
		{
			name: "firstFit pod and disable scheduling",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LabelFirstFitScheduling: "true",
					},
				},
			},
			needDisable: true,
			wantErr:     ErrFirstFitDisabled,
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
			needDisable: false,
			wantErr:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin, err := NewInterceptorPlugin(nil, nil)
			assert.NoError(t, err)
			assert.NotNil(t, plugin)
			pl := plugin.(*InterceptorPlugin)
			state := framework.NewCycleState()
			if tt.needDisable {
				state.Write(firstFitDisableStateKey, nil)
			}
			status := pl.PreFilter(context.TODO(), state, tt.pod)
			assert.Equal(t, status.AsError(), tt.wantErr)
		})
	}
}
