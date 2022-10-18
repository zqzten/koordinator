package resourcesummary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

func Test_getStandardRequested(t *testing.T) {
	type args struct {
		resourceList      corev1.ResourceList
		priorityClassType uniext.PriorityClass
	}
	tests := []struct {
		name string
		args args
		want corev1.ResourceList
	}{
		{
			name: "normal priority prod",
			args: args{
				resourceList: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50"),
					corev1.ResourceMemory: resource.MustParse("20Gi"),
				},
				priorityClassType: uniext.PriorityProd,
			},
			want: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50"),
				corev1.ResourceMemory: resource.MustParse("20Gi"),
			},
		},
		{
			name: "unified priority prod",
			args: args{
				resourceList: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50"),
					corev1.ResourceMemory: resource.MustParse("20Gi"),
				},
				priorityClassType: uniext.PriorityProd,
			},
			want: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50"),
				corev1.ResourceMemory: resource.MustParse("20Gi"),
			},
		},
		{
			name: "koord priority batch",
			args: args{
				resourceList: corev1.ResourceList{
					extension.BatchCPU:    resource.MustParse("20000"),
					extension.BatchMemory: resource.MustParse("10000Gi"),
				},
				priorityClassType: uniext.PriorityBatch,
			},
			want: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("20"),
				corev1.ResourceMemory: resource.MustParse("10Gi"),
			},
		},
		{
			name: "koord priority batch, small cpu value",
			args: args{
				resourceList: corev1.ResourceList{
					extension.BatchCPU:    resource.MustParse("20"),
					extension.BatchMemory: resource.MustParse("10000Mi"),
				},
				priorityClassType: uniext.PriorityBatch,
			},
			want: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("20m"),
				corev1.ResourceMemory: resource.MustParse("10Mi"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, quotav1.Equals(tt.want, priorityPodRequestedToNormal(tt.args.resourceList, tt.args.priorityClassType)))
		})
	}
}

func TestGetNodePriorityResource(t *testing.T) {
	type args struct {
		resourceList      corev1.ResourceList
		priorityClassType uniext.PriorityClass
	}
	tests := []struct {
		name string
		args args
		want corev1.ResourceList
	}{
		{
			name: "normal priority prod",
			args: args{
				resourceList: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50"),
					corev1.ResourceMemory: resource.MustParse("20Gi"),
					extension.BatchCPU:    resource.MustParse("20"),
					extension.BatchMemory: resource.MustParse("10Gi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
				priorityClassType: uniext.PriorityProd,
			},
			want: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50"),
				corev1.ResourceMemory: resource.MustParse("20Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
		{
			name: "normal priority batch",
			args: args{
				resourceList: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50"),
					corev1.ResourceMemory: resource.MustParse("20Gi"),
					extension.BatchCPU:    resource.MustParse("20000"),
					extension.BatchMemory: resource.MustParse("10000Gi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
				priorityClassType: uniext.PriorityBatch,
			},
			want: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("20"),
				corev1.ResourceMemory: resource.MustParse("10Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
		{
			name: "normal priority free",
			args: args{
				resourceList: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50"),
					corev1.ResourceMemory: resource.MustParse("20Gi"),
					extension.BatchCPU:    resource.MustParse("20"),
					extension.BatchMemory: resource.MustParse("10Gi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
				priorityClassType: uniext.PriorityFree,
			},
			want: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("110"),
			},
		},
		{
			name: "normal priority mid",
			args: args{
				resourceList: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50"),
					corev1.ResourceMemory: resource.MustParse("20Gi"),
					extension.BatchCPU:    resource.MustParse("20"),
					extension.BatchMemory: resource.MustParse("10Gi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
				priorityClassType: uniext.PriorityMid,
			},
			want: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("110"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, quotav1.Equals(tt.want, GetNodePriorityResource(tt.args.resourceList, tt.args.priorityClassType)))
		})
	}
}
