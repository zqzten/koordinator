package noderesource

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/config"

	uniext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
)

func Test_updateNodeWithReclaimedResource(t *testing.T) {
	enabledCfg := &config.ColocationCfg{
		ColocationStrategy: config.ColocationStrategy{
			Enable:                        pointer.BoolPtr(true),
			CPUReclaimThresholdPercent:    pointer.Int64Ptr(65),
			MemoryReclaimThresholdPercent: pointer.Int64Ptr(65),
			DegradeTimeMinutes:            pointer.Int64Ptr(15),
			UpdateTimeThresholdSeconds:    pointer.Int64Ptr(300),
			ResourceDiffThreshold:         pointer.Float64Ptr(0.1),
			ColocationStrategyExtender: config.ColocationStrategyExtender{
				Extensions: map[string]interface{}{
					config.ReclaimedResExtKey: &config.ReclaimedResourceConfig{
						NodeUpdate: pointer.BoolPtr(true),
					},
				},
			},
		},
	}
	disableCfg := &config.ColocationCfg{
		ColocationStrategy: config.ColocationStrategy{
			Enable:                        pointer.BoolPtr(false),
			CPUReclaimThresholdPercent:    pointer.Int64Ptr(65),
			MemoryReclaimThresholdPercent: pointer.Int64Ptr(65),
			DegradeTimeMinutes:            pointer.Int64Ptr(15),
			UpdateTimeThresholdSeconds:    pointer.Int64Ptr(300),
			ResourceDiffThreshold:         pointer.Float64Ptr(0.1),
			ColocationStrategyExtender: config.ColocationStrategyExtender{
				Extensions: map[string]interface{}{
					config.ReclaimedResExtKey: &config.ReclaimedResourceConfig{
						NodeUpdate: pointer.BoolPtr(true),
					},
				},
			},
		},
	}
	nodeNotUpdateCfg := &config.ColocationCfg{
		ColocationStrategy: config.ColocationStrategy{
			Enable:                        pointer.BoolPtr(false),
			CPUReclaimThresholdPercent:    pointer.Int64Ptr(65),
			MemoryReclaimThresholdPercent: pointer.Int64Ptr(65),
			DegradeTimeMinutes:            pointer.Int64Ptr(15),
			UpdateTimeThresholdSeconds:    pointer.Int64Ptr(300),
			ResourceDiffThreshold:         pointer.Float64Ptr(0.1),
			ColocationStrategyExtender: config.ColocationStrategyExtender{
				Extensions: map[string]interface{}{
					config.ReclaimedResExtKey: &config.ReclaimedResourceConfig{
						NodeUpdate: pointer.BoolPtr(false),
					},
				},
			},
		},
	}

	type fields struct {
		Client      client.Client
		config      *config.ColocationCfg
		SyncContext *SyncContext
	}
	type args struct {
		oldNode    *corev1.Node
		beResource *nodeBEResource
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *corev1.Node
		wantErr bool
	}{
		{
			name: "update be resource successfully",
			fields: fields{
				Client: fake.NewClientBuilder().WithRuntimeObjects(&corev1.Node{
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
				}).Build(),
				config: enabledCfg,
				SyncContext: &SyncContext{
					contextMap: map[string]time.Time{"/test-node0": time.Now()},
				},
			},
			args: args{
				oldNode: &corev1.Node{
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
				beResource: &nodeBEResource{
					MilliCPU: resource.NewQuantity(30, resource.DecimalSI),
					Memory:   resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
				},
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
			fields: fields{
				Client: fake.NewClientBuilder().WithRuntimeObjects(&corev1.Node{
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
				}).Build(),
				config: disableCfg,
				SyncContext: &SyncContext{
					contextMap: map[string]time.Time{"/test-node0": time.Now()},
				},
			},
			args: args{
				oldNode: &corev1.Node{
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
				beResource: &nodeBEResource{
					MilliCPU: nil,
					Memory:   nil,
				},
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node0",
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{},
					Capacity:    corev1.ResourceList{},
				},
			},
			wantErr: false,
		},
		{
			name: "reset be resource with enable=false config",
			fields: fields{
				Client: fake.NewClientBuilder().WithRuntimeObjects(&corev1.Node{
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
				}).Build(),
				config: nodeNotUpdateCfg,
				SyncContext: &SyncContext{
					contextMap: map[string]time.Time{"/test-node0": time.Now()},
				},
			},
			args: args{
				oldNode: &corev1.Node{
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
				beResource: &nodeBEResource{
					MilliCPU: resource.NewQuantity(30, resource.DecimalSI),
					Memory:   resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
				},
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
	}
	for _, tt := range tests {
		RegisterNodePrepareExtender(name, &ReclaimedResourcePlugin{})
		t.Run(tt.name, func(t *testing.T) {
			r := &NodeResourceReconciler{
				Client: tt.fields.Client,
				cfgCache: &FakeCfgCache{
					cfg: *tt.fields.config,
				},
				BESyncContext: SyncContext{contextMap: tt.fields.SyncContext.contextMap},
				Clock:       clock.RealClock{},
			}
			got := r.updateNodeBEResource(tt.args.oldNode, tt.args.beResource)
			assert.Equal(t, tt.wantErr, got != nil, got)
			gotNode := &corev1.Node{}
			_ = r.Client.Get(context.TODO(), types.NamespacedName{Name: tt.args.oldNode.Name}, gotNode)
			wantCPU := tt.want.Status.Allocatable[uniext.AlibabaCloudReclaimedCPU]
			gotCPU := gotNode.Status.Allocatable[uniext.AlibabaCloudReclaimedCPU]
			assert.Equal(t, wantCPU.Value(), gotCPU.Value())
			wantMem := tt.want.Status.Allocatable[uniext.AlibabaCloudReclaimedMemory]
			gotMem := gotNode.Status.Allocatable[uniext.AlibabaCloudReclaimedMemory]
			assert.Equal(t, wantMem.Value(), gotMem.Value())
		})
		UnregisterNodePrepareExtender(name)
	}
}
