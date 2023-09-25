//go:build !github
// +build !github

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

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
)

func TestGetPodCgroupParentDirUnified(t *testing.T) {
	type args struct {
		pod *corev1.Pod
	}
	tests := []struct {
		name         string
		cgroupDriver system.CgroupDriverType
		args         args
		want         string
	}{
		{
			name:         "guaranteed pod",
			cgroupDriver: system.Cgroupfs,
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
						UID:  "1",
					},
					Status: corev1.PodStatus{
						QOSClass: corev1.PodQOSGuaranteed,
					},
				},
			},
			want: "kubepods/pod1",
		},
		{
			name:         "besteffort pod",
			cgroupDriver: system.Cgroupfs,
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
						UID:  "1",
					},
					Status: corev1.PodStatus{
						QOSClass: corev1.PodQOSBestEffort,
					},
				},
			},
			want: "kubepods/besteffort/pod1",
		},
		{
			name:         "systemd burstable pod",
			cgroupDriver: system.Systemd,
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
						UID:  "1",
					},
					Status: corev1.PodStatus{
						QOSClass: corev1.PodQOSBurstable,
					},
				},
			},
			want: "kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod1.slice",
		},
		{
			name:         "custom cgroup pod",
			cgroupDriver: system.Cgroupfs,
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
						UID:  "2",
						Annotations: map[string]string{
							uniext.AnnotationPodCgroup: "/kubepods/pod2",
						},
					},
					Status: corev1.PodStatus{
						QOSClass: corev1.PodQOSBurstable,
					},
				},
			},
			want: "kubepods/pod2",
		},
		{
			name:         "custom cgroup pod 1",
			cgroupDriver: system.Cgroupfs,
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
						UID:  "2",
						Labels: map[string]string{
							uniext.LabelPodQoSClass: string(uniext.QoSSYSTEM),
						},
						Annotations: map[string]string{
							uniext.AnnotationPodCgroup: "/system/pod2",
						},
					},
					Status: corev1.PodStatus{
						QOSClass: corev1.PodQOSGuaranteed,
					},
				},
			},
			want: "system/pod2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			system.SetupCgroupPathFormatter(tt.cgroupDriver)

			got := GetPodCgroupParentDir(tt.args.pod)
			assert.Equal(t, tt.want, got)
		})
	}
}
