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

	apiext "github.com/koordinator-sh/koordinator/apis/extension"

	uniapiext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
)

func Test_IsPodCfsQuotaNeedUnset(t *testing.T) {
	type args struct {
		podAnnotations map[string]string
		podAlloc       *apiext.ResourceStatus
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "need unset for cpuset pod",
			args: args{
				podAnnotations: map[string]string{},
				podAlloc: &apiext.ResourceStatus{
					CPUSet: "2-4",
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "no need to unset for cpushare pod",
			args: args{
				podAnnotations: map[string]string{},
				podAlloc:       &apiext.ResourceStatus{},
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "not unset for ack non-cpuset pod",
			args: args{
				podAnnotations: map[string]string{
					uniapiext.AnnotationPodCPUSetScheduler: string(uniapiext.CPUSetSchedulerNone),
				},
			},
			want: false,
		},
		{
			name: "not unset for parsing ack cpuset failed",
			args: args{
				podAnnotations: map[string]string{
					uniapiext.AnnotationPodCPUSetScheduler: string(uniapiext.CPUSetSchedulerTrue),
					uniapiext.AnnotationPodCPUSet:          "",
				},
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "unset for ack cpuset pod",
			args: args{
				podAnnotations: map[string]string{
					uniapiext.AnnotationPodCPUSetScheduler: string(uniapiext.CPUSetSchedulerTrue),
					uniapiext.AnnotationPodCPUSet: `
{"container1":{"0":{"elems":{"0":{},"1":{},"2":{},"3":{}}}},"container2":{"1":{"elems":{"26":{},"27":{},"28":{},"29":{}}}}}
`,
				},
			},
			want: true,
		},
		{
			name: "not unset for ack static-burst pod whose cpu limit not equal to cpuset size",
			args: args{
				podAnnotations: map[string]string{
					uniapiext.AnnotationPodCPUSetScheduler: string(uniapiext.CPUSetSchedulerTrue),
					uniapiext.AnnotationPodCPUSet: `
{"container1":{"0":{"elems":{"0":{},"1":{},"2":{},"3":{}}}},"container2":{"1":{"elems":{"26":{},"27":{},"28":{},"29":{}}}}}
`,
					uniapiext.AnnotationPodCPUPolicy: string(uniapiext.CPUPolicyStaticBurst),
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.podAlloc != nil {
				podAllocJson := DumpJSON(tt.args.podAlloc)
				tt.args.podAnnotations[apiext.AnnotationResourceStatus] = podAllocJson
			}
			got, err := IsPodCfsQuotaNeedUnset(tt.args.podAnnotations)
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsPodCPUBurstable(t *testing.T) {
	type args struct {
		pod *corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "LSR pod is not burstable",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							apiext.LabelPodQoS: string(apiext.QoSLSR),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "BE pod is not burstable",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							apiext.LabelPodQoS: string(apiext.QoSBE),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "LS pod is burstable",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							apiext.LabelPodQoS: string(apiext.QoSLS),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "ack BE pod is not burstable",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							uniapiext.AnnotationPodQOSClass: string(uniapiext.QoSBE),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "ack LS pod is burstable",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							uniapiext.AnnotationPodQOSClass: string(uniapiext.QoSLS),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "ack LS pod is burstable",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							uniapiext.AnnotationPodQOSClass: string(uniapiext.QoSLS),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "ack cpuset (expect static-burst) pod is not burstable",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							uniapiext.AnnotationPodCPUSetScheduler: string(uniapiext.CPUSetSchedulerTrue),
							uniapiext.AnnotationPodCPUSet: `
{"container1":{"0":{"elems":{"0":{},"1":{},"2":{},"3":{}}}},"container2":{"1":{"elems":{"26":{},"27":{},"28":{},"29":{}}}}}
`,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "ack static-burst cpuset pod is burstable",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							uniapiext.AnnotationPodCPUSetScheduler: string(uniapiext.CPUSetSchedulerTrue),
							uniapiext.AnnotationPodCPUSet: `
{"container1":{"0":{"elems":{"0":{},"1":{},"2":{},"3":{}}}},"container2":{"1":{"elems":{"26":{},"27":{},"28":{},"29":{}}}}}
`,
							uniapiext.AnnotationPodCPUPolicy: string(uniapiext.CPUPolicyStaticBurst),
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPodCPUBurstable(tt.args.pod)
			assert.Equal(t, tt.want, got)
		})
	}
}
