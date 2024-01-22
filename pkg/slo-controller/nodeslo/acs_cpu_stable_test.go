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

package nodeslo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/configuration"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/util/sloconfig"
)

func TestCPUStablePluginMergeNodeSLOExtension(t *testing.T) {
	type args struct {
		oldExtCfg configuration.ExtensionCfgMap
		configMap *corev1.ConfigMap
		recorder  record.EventRecorder
	}
	tests := []struct {
		name    string
		args    args
		want    configuration.ExtensionCfgMap
		wantErr bool
	}{
		{
			name: "empty old ExtCfgMap",
			args: args{
				oldExtCfg: configuration.ExtensionCfgMap{},
				configMap: &corev1.ConfigMap{
					Data: map[string]string{
						unified.CPUStableConfigKey: `{"ClusterStrategy": {"policy": "auto", "podMetricWindowSeconds": 30}}`,
					},
				},
				recorder: &record.FakeRecorder{},
			},
			want: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{
					unified.CPUStableExtKey: {
						ClusterStrategy: &unified.CPUStableStrategy{
							Policy:                 sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
							PodMetricWindowSeconds: pointer.Int64(30),
						},
						NodeStrategies: make([]configuration.NodeExtensionStrategy, 0),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "parse configmap",
			args: args{
				oldExtCfg: configuration.ExtensionCfgMap{
					Object: map[string]configuration.ExtensionCfg{
						unified.CPUStableExtKey: {
							ClusterStrategy: &unified.CPUStableStrategy{
								Policy:                 sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
								PodMetricWindowSeconds: pointer.Int64(10),
							},
							NodeStrategies: make([]configuration.NodeExtensionStrategy, 0),
						},
					},
				},
				configMap: &corev1.ConfigMap{
					Data: map[string]string{
						unified.CPUStableConfigKey: `{"ClusterStrategy": {"policy": "auto", "podMetricWindowSeconds": 30}}`,
					},
				},
				recorder: &record.FakeRecorder{},
			},
			want: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{
					unified.CPUStableExtKey: {
						ClusterStrategy: &unified.CPUStableStrategy{
							Policy:                 sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
							PodMetricWindowSeconds: pointer.Int64(30),
						},
						NodeStrategies: make([]configuration.NodeExtensionStrategy, 0),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid json format",
			args: args{
				oldExtCfg: configuration.ExtensionCfgMap{
					Object: map[string]configuration.ExtensionCfg{
						unified.CPUStableExtKey: {
							ClusterStrategy: &unified.CPUStableStrategy{
								Policy:                 sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
								PodMetricWindowSeconds: pointer.Int64(10),
							},
							NodeStrategies: make([]configuration.NodeExtensionStrategy, 0),
						},
					},
				},
				configMap: &corev1.ConfigMap{
					Data: map[string]string{
						unified.CPUStableConfigKey: `invalid json format`,
					},
				},
				recorder: &record.FakeRecorder{},
			},
			want: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{
					unified.CPUStableExtKey: {
						ClusterStrategy: &unified.CPUStableStrategy{
							Policy:                 sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
							PodMetricWindowSeconds: pointer.Int64(10),
						},
						NodeStrategies: make([]configuration.NodeExtensionStrategy, 0),
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &CPUStablePlugin{}
			got, gotErr := p.MergeNodeSLOExtension(tt.args.oldExtCfg, tt.args.configMap, tt.args.recorder)
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCPUStablePluginGetNodeSLOExtension(t *testing.T) {
	type args struct {
		node   *corev1.Node
		cfgMap *configuration.ExtensionCfgMap
	}
	tests := []struct {
		name      string
		args      args
		wantKey   string
		wantValue interface{}
		wantErr   bool
	}{
		{
			name: "valid input",
			args: args{
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
						Labels: map[string]string{
							"label-1": "value-1",
						},
					},
				},
				cfgMap: &configuration.ExtensionCfgMap{
					Object: map[string]configuration.ExtensionCfg{
						unified.CPUStableExtKey: {
							NodeStrategies: []configuration.NodeExtensionStrategy{
								{
									NodeCfgProfile: configuration.NodeCfgProfile{
										NodeSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"label-1": "value-1",
											},
										},
									},
									NodeStrategy: &unified.CPUStableStrategy{
										Policy:                 sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
										PodMetricWindowSeconds: pointer.Int64(10),
									},
								},
							},
						},
					},
				},
			},
			wantKey: unified.CPUStableExtKey,
			wantValue: &unified.CPUStableStrategy{
				Policy:                 sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
				PodMetricWindowSeconds: pointer.Int64(10),
			},
			wantErr: false,
		},
		{
			name: "no matching strategy found, use cluster",
			args: args{
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
						Labels: map[string]string{
							"label-2": "value-2",
						},
					},
				},
				cfgMap: &configuration.ExtensionCfgMap{
					Object: map[string]configuration.ExtensionCfg{
						unified.CPUStableExtKey: {
							ClusterStrategy: &unified.CPUStableStrategy{
								Policy:                 sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
								PodMetricWindowSeconds: pointer.Int64(30),
							},
							NodeStrategies: []configuration.NodeExtensionStrategy{
								{
									NodeCfgProfile: configuration.NodeCfgProfile{
										NodeSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"label-1": "value-1",
											},
										},
									},
									NodeStrategy: &unified.CPUStableStrategy{
										Policy:                 sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
										PodMetricWindowSeconds: pointer.Int64(10),
									},
								},
							},
						},
					},
				},
			},
			wantKey: unified.CPUStableExtKey,
			wantValue: &unified.CPUStableStrategy{
				Policy:                 sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
				PodMetricWindowSeconds: pointer.Int64(30),
			},
			wantErr: false,
		},
		{
			name: "nil input",
			args: args{
				node:   nil,
				cfgMap: &configuration.ExtensionCfgMap{},
			},
			wantKey:   unified.CPUStableExtKey,
			wantValue: nil,
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &CPUStablePlugin{}
			gotKey, gotValue, gotErr := p.GetNodeSLOExtension(tt.args.node, tt.args.cfgMap)
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
			assert.Equal(t, tt.wantKey, gotKey)
			assert.Equal(t, tt.wantValue, gotValue)
		})
	}
}
