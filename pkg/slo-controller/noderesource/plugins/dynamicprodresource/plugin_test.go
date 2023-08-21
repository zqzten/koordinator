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

package dynamicprodresource

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/koordinator-sh/koordinator/apis/configuration"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	"github.com/koordinator-sh/koordinator/apis/thirdparty/unified"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/config"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/noderesource/framework"
)

func TestPlugin(t *testing.T) {
	t.Run("test", func(t *testing.T) {
		p := &Plugin{}
		assert.Equal(t, PluginName, p.Name())
		err := p.Setup(&framework.Option{
			Client: &fakeCtrlClient{},
		})
		assert.NoError(t, err)
		p.Reset(nil, "")
	})
}

func TestPluginNeedSyncMeta(t *testing.T) {
	testNodeWithoutLabel := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "1.2",
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	testNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "1.35",
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	testNode2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "1.35",
				uniext.LabelNodeType:            string(uniext.NodeTypeVirtualKubelet),
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	type args struct {
		strategy *configuration.ColocationStrategy
		oldNode  *corev1.Node
		newNode  *corev1.Node
	}
	tests := []struct {
		name  string
		args  args
		want  bool
		want1 string
	}{
		{
			name: "not sync when failed to parse strategy",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: 1,
						},
					},
				},
			},
			want:  false,
			want1: `failed to parse prod overcommit strategy, err: unmarshal DynamicProdResourceConfig failed, err: json: cannot unmarshal number into Go value of type unified.DynamicProdResourceConfig`,
		},
		{
			name: "skip update meta for default policy=none",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: nil,
					},
				},
				oldNode: testNode,
				newNode: testNode1,
			},
			want:  false,
			want1: "",
		},
		{
			name: "skip update meta for policy=dryRun",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy: getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyDryRun),
							},
						},
					},
				},
				oldNode: testNode,
				newNode: testNode1,
			},
			want:  false,
			want1: "",
		},
		{
			name: "need update when old node does not have label",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ResourceDiffThreshold: pointer.Float64(0.05),
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy: getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyAuto),
							},
						},
					},
				},
				oldNode: testNodeWithoutLabel,
				newNode: testNode,
			},
			want:  true,
			want1: "prod cpu over-quota resource diff is big than threshold",
		},
		{
			name: "need update when over-quota label changes",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ResourceDiffThreshold: pointer.Float64(0.05),
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy: getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyAuto),
							},
						},
					},
				},
				oldNode: testNode,
				newNode: testNode1,
			},
			want:  true,
			want1: "prod memory over-quota resource diff is big than threshold",
		},
		{
			name: "skip update when everything is unchanged",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ResourceDiffThreshold: pointer.Float64(0.05),
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy: getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyAuto),
							},
						},
					},
				},
				oldNode: testNode1,
				newNode: testNode1,
			},
			want:  false,
			want1: "",
		},
		{
			name: "skip update for a VK node",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ResourceDiffThreshold: pointer.Float64(0.01),
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy: getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyAuto),
							},
						},
					},
				},
				oldNode: testNode,
				newNode: testNode2,
			},
			want:  false,
			want1: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plugin{}
			got, got1 := p.NeedSyncMeta(tt.args.strategy, tt.args.oldNode, tt.args.newNode)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want1, got1)
		})
	}
}

type fakeCtrlClient struct {
	client.Client
	cachedObj *unified.Machine
}

func (f *fakeCtrlClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	machine, ok := obj.(*unified.Machine)
	if !ok {
		return fmt.Errorf("unexpected obj type %T", obj)
	}
	*machine = *f.cachedObj
	return nil
}

func (f *fakeCtrlClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	machine, ok := obj.(*unified.Machine)
	if !ok {
		return fmt.Errorf("unexpected obj type %T", obj)
	}

	f.cachedObj = machine
	return nil
}

func TestPluginExecute(t *testing.T) {
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "1.2",
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	testNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "1.35",
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	testNode2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "1.35",
				uniext.LabelNodeType:            string(uniext.NodeTypeVirtualKubelet),
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	type args struct {
		strategy *configuration.ColocationStrategy
		node     *corev1.Node
		nr       *framework.NodeResource
	}
	tests := []struct {
		name      string
		args      args
		wantErr   bool
		wantField *corev1.Node
	}{
		{
			name: "failed to parse strategy",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: 1,
						},
					},
				},
				node: testNode,
				nr:   nil,
			},
			wantErr:   true,
			wantField: testNode,
		},
		{
			name: "nothing to do for default policy=none",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: nil,
					},
				},
				node: testNode,
				nr:   nil,
			},
			wantErr:   false,
			wantField: testNode,
		},
		{
			name: "nothing to do for policy=dryRun",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy: getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyDryRun),
							},
						},
					},
				},
				node: testNode,
				nr:   nil,
			},
			wantErr:   false,
			wantField: testNode,
		},
		{
			name: "nothing to do for policy unsupported",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy: getProdOvercommitPolicyPointer("unknown"),
							},
						},
					},
				},
				node: testNode,
				nr:   nil,
			},
			wantErr:   false,
			wantField: testNode,
		},
		{
			name: "record results for policy=dryRun",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy: getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyAuto),
							},
						},
					},
				},
				node: testNode,
				nr: &framework.NodeResource{
					Labels: map[string]string{
						extunified.LabelCPUOverQuota:    "1.5",
						extunified.LabelMemoryOverQuota: "1.2",
					},
				},
			},
			wantErr:   false,
			wantField: testNode,
		},
		{
			name: "prepare node labels correctly",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy: getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyAuto),
							},
						},
					},
				},
				node: testNode,
				nr: &framework.NodeResource{
					Labels: map[string]string{
						extunified.LabelCPUOverQuota:    "1.5",
						extunified.LabelMemoryOverQuota: "1.35",
					},
				},
			},
			wantErr:   false,
			wantField: testNode1,
		},
		{
			name: "skip for a VK node",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy: getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyAuto),
							},
						},
					},
				},
				node: testNode2,
				nr: &framework.NodeResource{
					Labels: map[string]string{
						extunified.LabelCPUOverQuota:    "1.5",
						extunified.LabelMemoryOverQuota: "1.35",
					},
				},
			},
			wantErr:   false,
			wantField: testNode2,
		},
	}
	oldClient := Client
	defer func() {
		Client = oldClient
	}()
	for _, tt := range tests {
		p := &Plugin{}
		Client = &fakeCtrlClient{
			cachedObj: &unified.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "machine-" + tt.args.node.Name,
				},
				Spec: unified.MachineSpec{
					LabelSpec: unified.LabelSpec{
						Labels: map[string]string{
							//extunified.LabelCPUOverQuota: "1.5",
						},
					},
				},
			},
		}
		gotErr := p.Execute(tt.args.strategy, tt.args.node, tt.args.nr)
		assert.Equal(t, tt.wantErr, gotErr != nil)
		assert.Equal(t, tt.wantField, tt.args.node)
	}
}

func TestPluginCalculate(t *testing.T) {
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "1.2",
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	testNodeVK := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "1.2",
				uniext.LabelNodeType:            string(uniext.NodeTypeVirtualKubelet),
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	testNodeMetric := &slov1alpha1.NodeMetric{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: slov1alpha1.NodeMetricStatus{
			UpdateTime: &metav1.Time{Time: time.Now()},
			ProdReclaimableMetric: &slov1alpha1.ReclaimableMetric{
				Resource: slov1alpha1.ResourceMap{
					ResourceList: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("20"),
						corev1.ResourceMemory: resource.MustParse("40Gi"),
					},
				},
			},
		},
	}
	type args struct {
		strategy *configuration.ColocationStrategy
		node     *corev1.Node
		podList  *corev1.PodList
		metrics  *framework.ResourceMetrics
	}
	tests := []struct {
		name    string
		args    args
		want    []framework.ResourceItem
		wantErr bool
	}{
		{
			name: "failed to parse strategy",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: 1,
						},
					},
				},
				node: testNode,
			},
			wantErr: true,
		},
		{
			name: "skip calculation when policy=none",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy: getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyNone),
							},
						},
					},
				},
				node: testNode,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "calculate when policy=static",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy:               getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyStatic),
								ProdCPUOvercommitDefaultPercent:    pointer.Int64(150),
								ProdMemoryOvercommitDefaultPercent: pointer.Int64(135),
							},
						},
					},
				},
				node: testNode,
			},
			want: []framework.ResourceItem{
				{
					Labels: map[string]string{
						extunified.LabelCPUOverQuota:    "1.5",
						extunified.LabelMemoryOverQuota: "1.35",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "calculate when policy=static but cpu percent is illegal",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy:               getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyStatic),
								ProdCPUOvercommitDefaultPercent:    pointer.Int64(200),
								ProdCPUOvercommitMaxPercent:        pointer.Int64(180),
								ProdMemoryOvercommitDefaultPercent: pointer.Int64(135),
							},
						},
					},
				},
				node: testNode,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "calculate when policy=static but memory percent is illegal",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy:               getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyStatic),
								ProdCPUOvercommitDefaultPercent:    pointer.Int64(200),
								ProdMemoryOvercommitDefaultPercent: pointer.Int64(135),
								ProdMemoryOvercommitMinPercent:     pointer.Int64(150),
							},
						},
					},
				},
				node: testNode,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "calculate when policy=dryRun",
			args: args{
				strategy: &configuration.ColocationStrategy{
					DegradeTimeMinutes: pointer.Int64(10),
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy:               getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyDryRun),
								ProdCPUOvercommitDefaultPercent:    pointer.Int64(150),
								ProdMemoryOvercommitDefaultPercent: pointer.Int64(135),
							},
						},
					},
				},
				node: testNode,
				metrics: &framework.ResourceMetrics{
					NodeMetric: testNodeMetric,
				},
			},
			want: []framework.ResourceItem{
				{
					Labels: map[string]string{
						extunified.LabelCPUOverQuota:    "1.2",
						extunified.LabelMemoryOverQuota: "1.2",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "calculate when policy=auto but metric is expired",
			args: args{
				strategy: &configuration.ColocationStrategy{
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy:               getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyAuto),
								ProdCPUOvercommitDefaultPercent:    pointer.Int64(150),
								ProdMemoryOvercommitDefaultPercent: pointer.Int64(135),
							},
						},
					},
				},
				node: testNode,
				metrics: &framework.ResourceMetrics{
					NodeMetric: &slov1alpha1.NodeMetric{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-node",
						},
						Status: slov1alpha1.NodeMetricStatus{
							UpdateTime:            &metav1.Time{Time: time.Now()},
							ProdReclaimableMetric: nil,
						},
					},
				},
			},
			want: []framework.ResourceItem{
				{
					Labels: map[string]string{
						extunified.LabelCPUOverQuota:    "1.5",
						extunified.LabelMemoryOverQuota: "1.35",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "calculate when policy=auto",
			args: args{
				strategy: &configuration.ColocationStrategy{
					DegradeTimeMinutes: pointer.Int64(10),
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy:               getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyAuto),
								ProdCPUOvercommitDefaultPercent:    pointer.Int64(150),
								ProdMemoryOvercommitDefaultPercent: pointer.Int64(135),
							},
						},
					},
				},
				node: testNode,
				metrics: &framework.ResourceMetrics{
					NodeMetric: testNodeMetric,
				},
			},
			want: []framework.ResourceItem{
				{
					Labels: map[string]string{
						extunified.LabelCPUOverQuota:    "1.2",
						extunified.LabelMemoryOverQuota: "1.2",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "calculate when policy=auto and result is capped",
			args: args{
				strategy: &configuration.ColocationStrategy{
					DegradeTimeMinutes: pointer.Int64(10),
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy:               getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyAuto),
								ProdCPUOvercommitDefaultPercent:    pointer.Int64(150),
								ProdCPUOvercommitMinPercent:        pointer.Int64(150),
								ProdCPUOvercommitMaxPercent:        pointer.Int64(200),
								ProdMemoryOvercommitDefaultPercent: pointer.Int64(135),
								ProdMemoryOvercommitMinPercent:     pointer.Int64(100),
								ProdMemoryOvercommitMaxPercent:     pointer.Int64(110),
							},
						},
					},
				},
				node: testNode,
				metrics: &framework.ResourceMetrics{
					NodeMetric: testNodeMetric,
				},
			},
			want: []framework.ResourceItem{
				{
					Labels: map[string]string{
						extunified.LabelCPUOverQuota:    "1.5",
						extunified.LabelMemoryOverQuota: "1.1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "calculate for a VK node",
			args: args{
				strategy: &configuration.ColocationStrategy{
					DegradeTimeMinutes: pointer.Int64(10),
					ColocationStrategyExtender: configuration.ColocationStrategyExtender{
						Extensions: map[string]interface{}{
							config.DynamicProdResourceExtKey: &extunified.DynamicProdResourceConfig{
								ProdOvercommitPolicy:               getProdOvercommitPolicyPointer(extunified.ProdOvercommitPolicyAuto),
								ProdCPUOvercommitDefaultPercent:    pointer.Int64(150),
								ProdMemoryOvercommitDefaultPercent: pointer.Int64(135),
							},
						},
					},
				},
				node: testNodeVK,
				metrics: &framework.ResourceMetrics{
					NodeMetric: testNodeMetric,
				},
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plugin{}
			got, gotErr := p.Calculate(tt.args.strategy, tt.args.node, tt.args.podList, tt.args.metrics)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
		})
	}
}

func TestPlugin_getNodeDynamicProdResourceConfig(t *testing.T) {
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "1.2",
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	testNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "1.2",
			},
			Annotations: map[string]string{
				extunified.AnnotationDynamicProdConfig: "invalid",
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	testNode2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "1.2",
			},
			Annotations: map[string]string{
				extunified.AnnotationDynamicProdConfig: `{"prodOvercommitPolicy": "none"}`,
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	testNode3 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				extunified.LabelCPUOverQuota:    "1.5",
				extunified.LabelMemoryOverQuota: "1.2",
			},
			Annotations: map[string]string{},
		},
		Status: corev1.NodeStatus{
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("200Gi"),
			},
		},
	}
	testPolicyAuto := extunified.ProdOvercommitPolicyAuto
	testPolicyNone := extunified.ProdOvercommitPolicyNone
	type args struct {
		cfg  *extunified.DynamicProdResourceConfig
		node *corev1.Node
	}
	tests := []struct {
		name string
		args args
		want *extunified.DynamicProdResourceConfig
	}{
		{
			name: "skip node without node config",
			args: args{
				cfg: &extunified.DynamicProdResourceConfig{
					ProdOvercommitPolicy: &testPolicyAuto,
				},
				node: testNode,
			},
			want: &extunified.DynamicProdResourceConfig{
				ProdOvercommitPolicy: &testPolicyAuto,
			},
		},
		{
			name: "abort merging when node config is invalid",
			args: args{
				cfg: &extunified.DynamicProdResourceConfig{
					ProdOvercommitPolicy: &testPolicyAuto,
				},
				node: testNode1,
			},
			want: &extunified.DynamicProdResourceConfig{
				ProdOvercommitPolicy: &testPolicyAuto,
			},
		},
		{
			name: "merged node config successfully",
			args: args{
				cfg: &extunified.DynamicProdResourceConfig{
					ProdOvercommitPolicy: &testPolicyAuto,
				},
				node: testNode2,
			},
			want: &extunified.DynamicProdResourceConfig{
				ProdOvercommitPolicy: &testPolicyNone,
			},
		},
		{
			name: "skip node without node config 1",
			args: args{
				cfg: &extunified.DynamicProdResourceConfig{
					ProdOvercommitPolicy: &testPolicyNone,
				},
				node: testNode3,
			},
			want: &extunified.DynamicProdResourceConfig{
				ProdOvercommitPolicy: &testPolicyNone,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getNodeDynamicProdResourceConfig(tt.args.cfg, tt.args.node)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPlugin_isDegradeNeeded(t *testing.T) {
	const degradeTimeoutMinutes = 10
	type fields struct {
		Clock clock.Clock
	}
	type args struct {
		strategy   *configuration.ColocationStrategy
		nodeMetric *slov1alpha1.NodeMetric
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "empty NodeMetric should degrade",
			args: args{
				nodeMetric: nil,
			},
			want: true,
		},
		{
			name: "empty NodeMetric status should degrade",
			args: args{
				nodeMetric: &slov1alpha1.NodeMetric{},
			},
			want: true,
		},
		{
			name: "outdated NodeMetric status should degrade",
			args: args{
				strategy: &configuration.ColocationStrategy{
					Enable:             pointer.Bool(true),
					DegradeTimeMinutes: pointer.Int64(degradeTimeoutMinutes),
				},
				nodeMetric: &slov1alpha1.NodeMetric{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
					},
					Status: slov1alpha1.NodeMetricStatus{
						UpdateTime: &metav1.Time{
							Time: time.Now().Add(-(degradeTimeoutMinutes + 1) * time.Minute),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "outdated NodeMetric status should degrade 1",
			fields: fields{
				Clock: clock.NewFakeClock(time.Now().Add(time.Minute * (degradeTimeoutMinutes + 2))),
			},
			args: args{
				strategy: &configuration.ColocationStrategy{
					Enable:             pointer.Bool(true),
					DegradeTimeMinutes: pointer.Int64(degradeTimeoutMinutes),
				},
				nodeMetric: &slov1alpha1.NodeMetric{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node0",
						Labels: map[string]string{
							"xxx": "yy",
						},
					},
					Status: slov1alpha1.NodeMetricStatus{
						UpdateTime: &metav1.Time{
							Time: time.Now(),
						},
						ProdReclaimableMetric: &slov1alpha1.ReclaimableMetric{
							Resource: slov1alpha1.ResourceMap{
								ResourceList: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10"),
									corev1.ResourceMemory: resource.MustParse("20Gi"),
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "NodeMetric without prod reclaimable should degrade",
			args: args{
				strategy: &configuration.ColocationStrategy{
					Enable:             pointer.Bool(true),
					DegradeTimeMinutes: pointer.Int64(degradeTimeoutMinutes),
				},
				nodeMetric: &slov1alpha1.NodeMetric{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
					},
					Status: slov1alpha1.NodeMetricStatus{
						UpdateTime: &metav1.Time{
							Time: time.Now(),
						},
						NodeMetric: &slov1alpha1.NodeMetricInfo{
							NodeUsage: slov1alpha1.ResourceMap{
								ResourceList: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("20"),
									corev1.ResourceMemory: resource.MustParse("40Gi"),
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "valid NodeMetric status should not degrade",
			args: args{
				strategy: &configuration.ColocationStrategy{
					Enable:             pointer.Bool(true),
					DegradeTimeMinutes: pointer.Int64(degradeTimeoutMinutes),
				},
				nodeMetric: &slov1alpha1.NodeMetric{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
					},
					Status: slov1alpha1.NodeMetricStatus{
						UpdateTime: &metav1.Time{
							Time: time.Now(),
						},
						NodeMetric: &slov1alpha1.NodeMetricInfo{
							NodeUsage: slov1alpha1.ResourceMap{
								ResourceList: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("20"),
									corev1.ResourceMemory: resource.MustParse("40Gi"),
								},
							},
						},
						ProdReclaimableMetric: &slov1alpha1.ReclaimableMetric{
							Resource: slov1alpha1.ResourceMap{
								ResourceList: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10"),
									corev1.ResourceMemory: resource.MustParse("20Gi"),
								},
							},
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.fields.Clock != nil {
				oldClock := Clock
				Clock = tt.fields.Clock
				defer func() {
					Clock = oldClock
				}()
			}

			p := &Plugin{}
			assert.Equal(t, tt.want, p.isDegradeNeeded(tt.args.strategy, tt.args.nodeMetric))
		})
	}
}

func getProdOvercommitPolicyPointer(policy extunified.ProdOvercommitPolicy) *extunified.ProdOvercommitPolicy {
	return &policy
}
