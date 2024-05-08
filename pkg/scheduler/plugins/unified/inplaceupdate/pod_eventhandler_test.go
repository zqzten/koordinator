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

package inplaceupdate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_podHasUnhandledUpdateReq(t *testing.T) {
	tests := []struct {
		name                string
		resourceUpdateSpec  *uniext.ResourceUpdateSpec
		resourceUpdateState *uniext.ResourceUpdateSchedulerState
		want                bool
	}{
		{
			name: "pod specify update spec, but scheduler state not patched",
			resourceUpdateSpec: &uniext.ResourceUpdateSpec{
				Version: "2",
			},
			want: true,
		},
		{
			name: "pod specify update spec, but scheduler state not updated",
			resourceUpdateSpec: &uniext.ResourceUpdateSpec{
				Version: "2",
			},
			resourceUpdateState: &uniext.ResourceUpdateSchedulerState{
				Version: "1",
			},
			want: true,
		},
		{
			name: "pod specify update spec, but scheduler state not updated",
			resourceUpdateSpec: &uniext.ResourceUpdateSpec{
				Version: "2",
			},
			resourceUpdateState: &uniext.ResourceUpdateSchedulerState{
				Version: "2",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}}
			if tt.resourceUpdateSpec != nil {
				assert.NoError(t, uniext.SetResourceUpdateSpec(pod.Annotations, tt.resourceUpdateSpec))
			}
			if tt.resourceUpdateState != nil {
				assert.NoError(t, uniext.SetResourceUpdateSchedulerState(pod.Annotations, tt.resourceUpdateState))
			}
			if got := podHasUnhandledUpdateReq(pod); got != tt.want {
				t.Errorf("podHasUnhandledUpdateReq() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_podHasUpdateReq(t *testing.T) {
	tests := []struct {
		name                  string
		oldResourceUpdateSpec *uniext.ResourceUpdateSpec
		newResourceUpdateSpec *uniext.ResourceUpdateSpec
		want                  bool
	}{
		{
			name: "pod specify update spec at first",
			oldResourceUpdateSpec: &uniext.ResourceUpdateSpec{
				Version: "1",
			},
			want: true,
		},
		{
			name: "pod update updateSpec",
			oldResourceUpdateSpec: &uniext.ResourceUpdateSpec{
				Version: "1",
			},
			newResourceUpdateSpec: &uniext.ResourceUpdateSpec{
				Version: "2",
			},
			want: true,
		},
		{
			name: "updateSpec unchanged",
			oldResourceUpdateSpec: &uniext.ResourceUpdateSpec{
				Version: "1",
			},
			newResourceUpdateSpec: &uniext.ResourceUpdateSpec{
				Version: "1",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			}
			if tt.oldResourceUpdateSpec != nil {
				assert.NoError(t, uniext.SetResourceUpdateSpec(oldPod.Annotations, tt.oldResourceUpdateSpec))
			}
			newPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			}
			if tt.newResourceUpdateSpec != nil {
				assert.NoError(t, uniext.SetResourceUpdateSpec(newPod.Annotations, tt.newResourceUpdateSpec))
			}
			if got := podHasUpdateReq(oldPod, newPod); got != tt.want {
				t.Errorf("podHasUpdateReq() = %v, want %v", got, tt.want)
			}
		})
	}
}
