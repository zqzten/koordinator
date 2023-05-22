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

package framework

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
)

func TestRuntimeClassPodFilter(t *testing.T) {
	type fields struct {
		whitelist      []string
		blacklist      []string
		allowByDefault bool
	}
	tests := []struct {
		name   string
		fields fields
		args   *statesinformer.PodMeta
		want   bool
		want1  string
	}{
		{
			name: "default allow",
			fields: fields{
				allowByDefault: true,
			},
			args: &statesinformer.PodMeta{
				Pod: &corev1.Pod{},
			},
			want:  false,
			want1: "pod runtime class [runc] unmatched, AllowByDefault=true",
		},
		{
			name: "default reject",
			fields: fields{
				allowByDefault: false,
			},
			args: &statesinformer.PodMeta{
				Pod: &corev1.Pod{},
			},
			want:  true,
			want1: "pod runtime class [runc] unmatched, AllowByDefault=false",
		},
		{
			name: "allow whitelist runtime",
			fields: fields{
				whitelist:      []string{extunified.PodRuntimeTypeRunc},
				allowByDefault: true,
			},
			args: &statesinformer.PodMeta{
				Pod: &corev1.Pod{},
			},
			want:  false,
			want1: "pod runtime class [runc] belongs to the whitelist",
		},
		{
			name: "reject blacklist runtime",
			fields: fields{
				whitelist:      []string{extunified.PodRuntimeTypeRunc},
				blacklist:      []string{extunified.PodRuntimeTypeRund},
				allowByDefault: false,
			},
			args: &statesinformer.PodMeta{
				Pod: &corev1.Pod{
					Spec: corev1.PodSpec{
						RuntimeClassName: pointer.String(extunified.PodRuntimeTypeRund),
					},
				},
			},
			want:  true,
			want1: "pod runtime class [rund] belongs to the blacklist",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewRuntimeClassPodFilter(tt.fields.whitelist, tt.fields.blacklist, tt.fields.allowByDefault)
			got, got1 := f.FilterPod(tt.args)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want1, got1)
		})
	}
}
