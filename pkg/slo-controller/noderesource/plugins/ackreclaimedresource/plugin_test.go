package noderesource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/config"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/noderesource/framework"

	uniext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
)

func TestPluginExecute(t *testing.T) {
	enabledCfg := &apiext.ColocationStrategy{
		Enable:                        pointer.BoolPtr(true),
		CPUReclaimThresholdPercent:    pointer.Int64Ptr(65),
		MemoryReclaimThresholdPercent: pointer.Int64Ptr(65),
		DegradeTimeMinutes:            pointer.Int64Ptr(15),
		UpdateTimeThresholdSeconds:    pointer.Int64Ptr(300),
		ResourceDiffThreshold:         pointer.Float64Ptr(0.1),
		ColocationStrategyExtender: apiext.ColocationStrategyExtender{
			Extensions: map[string]interface{}{
				config.ReclaimedResExtKey: &config.ReclaimedResourceConfig{
					NodeUpdate: pointer.BoolPtr(true),
				},
			},
		},
	}
	disableCfg := &apiext.ColocationStrategy{
		Enable:                        pointer.BoolPtr(false),
		CPUReclaimThresholdPercent:    pointer.Int64Ptr(65),
		MemoryReclaimThresholdPercent: pointer.Int64Ptr(65),
		DegradeTimeMinutes:            pointer.Int64Ptr(15),
		UpdateTimeThresholdSeconds:    pointer.Int64Ptr(300),
		ResourceDiffThreshold:         pointer.Float64Ptr(0.1),
		ColocationStrategyExtender: apiext.ColocationStrategyExtender{
			Extensions: map[string]interface{}{
				config.ReclaimedResExtKey: &config.ReclaimedResourceConfig{
					NodeUpdate: pointer.BoolPtr(true),
				},
			},
		},
	}
	nodeNotUpdateCfg := &apiext.ColocationStrategy{
		Enable:                        pointer.BoolPtr(false),
		CPUReclaimThresholdPercent:    pointer.Int64Ptr(65),
		MemoryReclaimThresholdPercent: pointer.Int64Ptr(65),
		DegradeTimeMinutes:            pointer.Int64Ptr(15),
		UpdateTimeThresholdSeconds:    pointer.Int64Ptr(300),
		ResourceDiffThreshold:         pointer.Float64Ptr(0.1),
		ColocationStrategyExtender: apiext.ColocationStrategyExtender{
			Extensions: map[string]interface{}{
				config.ReclaimedResExtKey: &config.ReclaimedResourceConfig{
					NodeUpdate: pointer.BoolPtr(false),
				},
			},
		},
	}
	unmarshalFailedCfg := &apiext.ColocationStrategy{
		Enable:                        pointer.BoolPtr(false),
		CPUReclaimThresholdPercent:    pointer.Int64Ptr(65),
		MemoryReclaimThresholdPercent: pointer.Int64Ptr(65),
		DegradeTimeMinutes:            pointer.Int64Ptr(15),
		UpdateTimeThresholdSeconds:    pointer.Int64Ptr(300),
		ResourceDiffThreshold:         pointer.Float64Ptr(0.1),
		ColocationStrategyExtender: apiext.ColocationStrategyExtender{
			Extensions: map[string]interface{}{
				config.ReclaimedResExtKey: "invalidContent",
			},
		},
	}

	type args struct {
		strategy *apiext.ColocationStrategy
		node     *corev1.Node
		nr       *framework.NodeResource
	}
	tests := []struct {
		name    string
		args    args
		want    *corev1.Node
		wantErr bool
	}{
		{
			name: "update be resource successfully",
			args: args{
				strategy: enabledCfg,
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node0",
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							apiext.BatchCPU:    resource.MustParse("20"),
							apiext.BatchMemory: resource.MustParse("40G"),
						},
						Capacity: corev1.ResourceList{
							apiext.BatchCPU:    resource.MustParse("20"),
							apiext.BatchMemory: resource.MustParse("40G"),
						},
					},
				},
				nr: framework.NewNodeResource([]framework.ResourceItem{
					{
						Name:     apiext.BatchCPU,
						Quantity: resource.NewQuantity(30, resource.DecimalSI),
					},
					{
						Name:     apiext.BatchMemory,
						Quantity: resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
					},
				}...),
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node0",
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						apiext.BatchCPU:                    *resource.NewQuantity(30, resource.DecimalSI),
						apiext.BatchMemory:                 *resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
						uniext.AlibabaCloudReclaimedCPU:    *resource.NewQuantity(30, resource.DecimalSI),
						uniext.AlibabaCloudReclaimedMemory: *resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
					},
					Capacity: corev1.ResourceList{
						apiext.BatchCPU:                    *resource.NewQuantity(30, resource.DecimalSI),
						apiext.BatchMemory:                 *resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
						uniext.AlibabaCloudReclaimedCPU:    *resource.NewQuantity(30, resource.DecimalSI),
						uniext.AlibabaCloudReclaimedMemory: *resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "clear reclaimed resource with nodeUpdate=false config",
			args: args{
				strategy: nodeNotUpdateCfg,
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node0",
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							apiext.BatchCPU:                    resource.MustParse("20"),
							apiext.BatchMemory:                 resource.MustParse("40G"),
							uniext.AlibabaCloudReclaimedCPU:    resource.MustParse("20"),
							uniext.AlibabaCloudReclaimedMemory: resource.MustParse("40G"),
						},
						Capacity: corev1.ResourceList{
							apiext.BatchCPU:                    resource.MustParse("20"),
							apiext.BatchMemory:                 resource.MustParse("40G"),
							uniext.AlibabaCloudReclaimedCPU:    resource.MustParse("20"),
							uniext.AlibabaCloudReclaimedMemory: resource.MustParse("40G"),
						},
					},
				},
				nr: framework.NewNodeResource([]framework.ResourceItem{
					{
						Name:     apiext.BatchCPU,
						Quantity: resource.NewQuantity(30, resource.DecimalSI),
					},
					{
						Name:     apiext.BatchMemory,
						Quantity: resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
					},
				}...),
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node0",
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						apiext.BatchCPU:    *resource.NewQuantity(30, resource.DecimalSI),
						apiext.BatchMemory: *resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
					},
					Capacity: corev1.ResourceList{
						apiext.BatchCPU:    *resource.NewQuantity(30, resource.DecimalSI),
						apiext.BatchMemory: *resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "reset be resource with enable=false config",
			args: args{
				strategy: disableCfg,
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node0",
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							apiext.BatchCPU:                    resource.MustParse("20"),
							apiext.BatchMemory:                 resource.MustParse("40G"),
							uniext.AlibabaCloudReclaimedCPU:    resource.MustParse("20"),
							uniext.AlibabaCloudReclaimedMemory: resource.MustParse("40G"),
						},
						Capacity: corev1.ResourceList{
							apiext.BatchCPU:                    resource.MustParse("20"),
							apiext.BatchMemory:                 resource.MustParse("40G"),
							uniext.AlibabaCloudReclaimedCPU:    resource.MustParse("20"),
							uniext.AlibabaCloudReclaimedMemory: resource.MustParse("40G"),
						},
					},
				},
				nr: framework.NewNodeResource([]framework.ResourceItem{
					{
						Name:  apiext.BatchCPU,
						Reset: true,
					},
					{
						Name:  apiext.BatchMemory,
						Reset: true,
					},
				}...),
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node0",
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						apiext.BatchCPU:    *resource.NewQuantity(30, resource.DecimalSI),
						apiext.BatchMemory: *resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
					},
					Capacity: corev1.ResourceList{
						apiext.BatchCPU:    *resource.NewQuantity(30, resource.DecimalSI),
						apiext.BatchMemory: *resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "throw an error for reclaimed config unmarshalled failed",
			args: args{
				strategy: unmarshalFailedCfg,
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node0",
					},
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							apiext.BatchCPU:                    resource.MustParse("20"),
							apiext.BatchMemory:                 resource.MustParse("40G"),
							uniext.AlibabaCloudReclaimedCPU:    resource.MustParse("20"),
							uniext.AlibabaCloudReclaimedMemory: resource.MustParse("40G"),
						},
						Capacity: corev1.ResourceList{
							apiext.BatchCPU:                    resource.MustParse("20"),
							apiext.BatchMemory:                 resource.MustParse("40G"),
							uniext.AlibabaCloudReclaimedCPU:    resource.MustParse("20"),
							uniext.AlibabaCloudReclaimedMemory: resource.MustParse("40G"),
						},
					},
				},
				nr: framework.NewNodeResource([]framework.ResourceItem{
					{
						Name:     apiext.BatchCPU,
						Quantity: resource.NewQuantity(30, resource.DecimalSI),
					},
					{
						Name:     apiext.BatchMemory,
						Quantity: resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
					},
				}...),
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node0",
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						apiext.BatchCPU:                    *resource.NewQuantity(30, resource.DecimalSI),
						apiext.BatchMemory:                 *resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
						uniext.AlibabaCloudReclaimedCPU:    *resource.NewQuantity(30, resource.DecimalSI),
						uniext.AlibabaCloudReclaimedMemory: *resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
					},
					Capacity: corev1.ResourceList{
						apiext.BatchCPU:                    *resource.NewQuantity(30, resource.DecimalSI),
						apiext.BatchMemory:                 *resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
						uniext.AlibabaCloudReclaimedCPU:    *resource.NewQuantity(30, resource.DecimalSI),
						uniext.AlibabaCloudReclaimedMemory: *resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plugin{}
			gotErr := p.Execute(tt.args.strategy, tt.args.node, tt.args.nr)
			assert.Equal(t, tt.wantErr, gotErr != nil)
			gotNode := tt.args.node
			wantCPU := tt.want.Status.Allocatable[uniext.AlibabaCloudReclaimedCPU]
			gotCPU := gotNode.Status.Allocatable[uniext.AlibabaCloudReclaimedCPU]
			assert.Equal(t, wantCPU.Value(), gotCPU.Value())
			wantMem := tt.want.Status.Allocatable[uniext.AlibabaCloudReclaimedMemory]
			gotMem := gotNode.Status.Allocatable[uniext.AlibabaCloudReclaimedMemory]
			assert.Equal(t, wantMem.Value(), gotMem.Value())
		})
	}
}
