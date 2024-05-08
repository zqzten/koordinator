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
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/configuration"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func TestMergeNodeSLOExtension(t *testing.T) {
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
						unified.DeadlineEvictConfigKey: `{"ClusterStrategy": {"enable": true, "deadlineDuration": "60s"}}`,
					},
				},
				recorder: &record.FakeRecorder{},
			},
			want: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{
					unified.DeadlineEvictExtKey: {
						ClusterStrategy: &unified.DeadlineEvictStrategy{
							Enable: pointer.Bool(true),
							DeadlineEvictConfig: unified.DeadlineEvictConfig{
								DeadlineDuration: &metav1.Duration{
									Duration: time.Second * 60,
								},
							},
						},
						NodeStrategies: make([]configuration.NodeExtensionStrategy, 0),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "merge with old ExtCfgMap",
			args: args{
				oldExtCfg: configuration.ExtensionCfgMap{
					Object: map[string]configuration.ExtensionCfg{
						unified.DeadlineEvictExtKey: {
							ClusterStrategy: &unified.DeadlineEvictStrategy{
								Enable: pointer.Bool(true),
								DeadlineEvictConfig: unified.DeadlineEvictConfig{
									DeadlineDuration: &metav1.Duration{
										Duration: time.Second * 30,
									},
								},
							},
						},
					},
				},
				configMap: &corev1.ConfigMap{
					Data: map[string]string{
						unified.DeadlineEvictConfigKey: `{"ClusterStrategy": {"enable": true, "deadlineDuration": "60s"}}`,
					},
				},
				recorder: &record.FakeRecorder{},
			},
			want: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{
					unified.DeadlineEvictExtKey: {
						ClusterStrategy: &unified.DeadlineEvictStrategy{
							Enable: pointer.Bool(true),
							DeadlineEvictConfig: unified.DeadlineEvictConfig{
								DeadlineDuration: &metav1.Duration{
									Duration: time.Second * 60,
								},
							},
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
						unified.DeadlineEvictExtKey: {
							ClusterStrategy: &unified.DeadlineEvictStrategy{
								Enable: pointer.Bool(true),
								DeadlineEvictConfig: unified.DeadlineEvictConfig{
									DeadlineDuration: &metav1.Duration{
										Duration: time.Second * 60,
									},
								},
							},
						},
					},
				},
				configMap: &corev1.ConfigMap{
					Data: map[string]string{
						unified.DeadlineEvictConfigKey: `invalid json format`,
					},
				},
				recorder: &record.FakeRecorder{},
			},
			want: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{
					unified.DeadlineEvictExtKey: {
						ClusterStrategy: &unified.DeadlineEvictStrategy{
							Enable: pointer.Bool(true),
							DeadlineEvictConfig: unified.DeadlineEvictConfig{
								DeadlineDuration: &metav1.Duration{
									Duration: time.Second * 60,
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PodDeadlineEvictPlugin{}
			got, err := p.MergeNodeSLOExtension(tt.args.oldExtCfg, tt.args.configMap, tt.args.recorder)
			if tt.wantErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestGetNodeSLOExtension(t *testing.T) {
	second := metav1.Duration{
		Duration: time.Second,
	}
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
						unified.DeadlineEvictExtKey: {
							NodeStrategies: []configuration.NodeExtensionStrategy{
								{
									NodeCfgProfile: configuration.NodeCfgProfile{
										NodeSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"label-1": "value-1",
											},
										},
									},
									NodeStrategy: &unified.DeadlineEvictStrategy{
										Enable: pointer.Bool(true),
										DeadlineEvictConfig: unified.DeadlineEvictConfig{
											DeadlineDuration: &second,
										},
									},
								},
							},
						},
					},
				},
			},
			wantKey: unified.DeadlineEvictExtKey,
			wantValue: &unified.DeadlineEvictStrategy{
				Enable: pointer.Bool(true),
				DeadlineEvictConfig: unified.DeadlineEvictConfig{
					DeadlineDuration: &second,
				},
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
						unified.DeadlineEvictExtKey: {
							ClusterStrategy: &unified.DeadlineEvictStrategy{
								Enable: pointer.Bool(false),
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
									NodeStrategy: &unified.DeadlineEvictStrategy{
										Enable: pointer.Bool(true),
										DeadlineEvictConfig: unified.DeadlineEvictConfig{
											DeadlineDuration: &second,
										},
									},
								},
							},
						},
					},
				},
			},
			wantKey: unified.DeadlineEvictExtKey,
			wantValue: &unified.DeadlineEvictStrategy{
				Enable: pointer.Bool(false),
			},
			wantErr: false,
		},
		{
			name: "nil input",
			args: args{
				node:   nil,
				cfgMap: &configuration.ExtensionCfgMap{},
			},
			wantKey:   unified.DeadlineEvictExtKey,
			wantValue: nil,
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PodDeadlineEvictPlugin{}
			gotKey, gotValue, err := p.GetNodeSLOExtension(tt.args.node, tt.args.cfgMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetNodeSLOExtension() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotKey != tt.wantKey || !assert.Equal(t, tt.wantValue, gotValue) {
				t.Errorf("GetNodeSLOExtension() gotKey = %v, want %v; gotValue = %v, want %v", gotKey, tt.wantKey, gotValue, tt.wantValue)
			}
		})
	}
}

func TestGetDeadlineEvictConfigSpec(t *testing.T) {
	type testCase struct {
		name            string
		node            *corev1.Node
		cfg             *unified.DeadlineEvictCfg
		expectedResult  *unified.DeadlineEvictStrategy
		expectedError   bool
		nodeSelector    map[string]string
		nodeLabelValues map[string]string
	}

	cases := []testCase{
		{
			name: "no matching node strategy",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						"label-1": "value-1",
						"label-2": "value-2",
					},
				},
			},
			cfg: &unified.DeadlineEvictCfg{
				NodeStrategies: []unified.NodeDeadlineEvictStrategy{
					{
						NodeCfgProfile: configuration.NodeCfgProfile{
							Name:         "",
							NodeSelector: nil,
						},
						DeadlineEvictStrategy: nil,
					},
					{
						NodeCfgProfile: configuration.NodeCfgProfile{
							NodeSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "label-1",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"value-3"},
									},
								},
							},
						},
						DeadlineEvictStrategy: &unified.DeadlineEvictStrategy{
							Enable:              pointer.Bool(true),
							DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 30 * time.Second}},
						},
					},
				},
				ClusterStrategy: &unified.DeadlineEvictStrategy{
					Enable:              pointer.Bool(false),
					DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 60 * time.Second}},
				},
			},
			expectedResult: &unified.DeadlineEvictStrategy{
				Enable:              pointer.Bool(false),
				DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 60 * time.Second}},
			},
			expectedError: false,
			nodeSelector:  map[string]string{"label-1": "value-3"},
			nodeLabelValues: map[string]string{
				"label-1": "value-1",
				"label-2": "value-2",
			},
		},
		{
			name: "matching node strategy",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						"label-1": "value-1",
						"label-2": "value-2",
					},
				},
			},
			cfg: &unified.DeadlineEvictCfg{
				NodeStrategies: []unified.NodeDeadlineEvictStrategy{
					{},
					{
						NodeCfgProfile: configuration.NodeCfgProfile{
							NodeSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "label-1",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"value-1"},
									},
								},
							},
						},
						DeadlineEvictStrategy: &unified.DeadlineEvictStrategy{
							Enable:              pointer.Bool(true),
							DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 30 * time.Second}},
						},
					},
				},
				ClusterStrategy: &unified.DeadlineEvictStrategy{
					Enable:              pointer.Bool(false),
					DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 60 * time.Second}},
				},
			},
			expectedResult: &unified.DeadlineEvictStrategy{
				Enable:              pointer.Bool(true),
				DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 30 * time.Second}},
			},
			expectedError: false,
			nodeSelector:  map[string]string{"label-1": "value-1"},
			nodeLabelValues: map[string]string{
				"label-1": "value-1",
				"label-2": "value-2",
			},
		},
		{
			name: "error parsing node selector",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						"label-1": "value-1",
						"label-2": "value-2",
					},
				},
			},
			cfg: &unified.DeadlineEvictCfg{
				NodeStrategies: []unified.NodeDeadlineEvictStrategy{
					{
						NodeCfgProfile: configuration.NodeCfgProfile{
							NodeSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "label-1",
										Operator: metav1.LabelSelectorOperator("bad-operator"),
										Values:   []string{"value-1"},
									},
								},
							},
						},
						DeadlineEvictStrategy: &unified.DeadlineEvictStrategy{
							Enable:              pointer.Bool(true),
							DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 30 * time.Second}},
						},
					},
				},
				ClusterStrategy: &unified.DeadlineEvictStrategy{
					Enable:              pointer.Bool(false),
					DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 60 * time.Second}},
				},
			},
			expectedResult: &unified.DeadlineEvictStrategy{
				Enable:              pointer.Bool(false),
				DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 60 * time.Second}},
			},
			expectedError: false,
			nodeSelector:  map[string]string{"label-1": "value-1"},
			nodeLabelValues: map[string]string{
				"label-1": "value-1",
				"label-2": "value-2",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := tc.node
			result, err := getDeadlineEvictConfigSpec(node, tc.cfg)

			assert.Equal(t, err != nil, tc.expectedError)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestCalculateDeadlineEvictCfgMerged(t *testing.T) {
	type testCase struct {
		name           string
		oldExtCfg      configuration.ExtensionCfgMap
		configMap      *corev1.ConfigMap
		expectedResult configuration.ExtensionCfgMap
		expectedError  bool
	}

	cases := []testCase{
		{
			name: "empty configmap",
			oldExtCfg: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{
					unified.DeadlineEvictExtKey: {
						ClusterStrategy: &unified.DeadlineEvictStrategy{
							Enable:              pointer.Bool(true),
							DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 5 * time.Minute}},
						},
						NodeStrategies: []configuration.NodeExtensionStrategy{
							{
								NodeCfgProfile: configuration.NodeCfgProfile{Name: "node-1"},
								NodeStrategy: &unified.DeadlineEvictStrategy{
									Enable:              pointer.Bool(false),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 10 * time.Minute}},
								},
							},
							{
								NodeCfgProfile: configuration.NodeCfgProfile{Name: "node-2"},
								NodeStrategy: &unified.DeadlineEvictStrategy{
									Enable:              pointer.Bool(true),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 15 * time.Minute}},
								},
							},
						},
					},
				},
			},
			configMap:      &corev1.ConfigMap{},
			expectedResult: configuration.ExtensionCfgMap{Object: map[string]configuration.ExtensionCfg{}},
			expectedError:  false,
		},
		{
			name: "invalid json",
			oldExtCfg: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{
					unified.DeadlineEvictExtKey: {
						ClusterStrategy: &unified.DeadlineEvictStrategy{
							Enable:              pointer.Bool(true),
							DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 5 * time.Minute}},
						},
						NodeStrategies: []configuration.NodeExtensionStrategy{
							{
								NodeCfgProfile: configuration.NodeCfgProfile{Name: "node-1"},
								NodeStrategy: &unified.DeadlineEvictStrategy{
									Enable:              pointer.Bool(false),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 10 * time.Minute}},
								},
							},
							{
								NodeCfgProfile: configuration.NodeCfgProfile{Name: "node-2"},
								NodeStrategy: &unified.DeadlineEvictStrategy{
									Enable:              pointer.Bool(true),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 15 * time.Minute}},
								},
							},
						},
					},
				},
			},
			configMap: &corev1.ConfigMap{
				Data: map[string]string{
					unified.DeadlineEvictConfigKey: "{invalid json}",
				},
			},
			expectedResult: configuration.ExtensionCfgMap{},
			expectedError:  true,
		},
		{
			name: "valid configmap",
			oldExtCfg: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{
					unified.DeadlineEvictExtKey: {
						ClusterStrategy: &unified.DeadlineEvictStrategy{
							Enable:              pointer.Bool(true),
							DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 5 * time.Minute}},
						},
						NodeStrategies: []configuration.NodeExtensionStrategy{
							{
								NodeCfgProfile: configuration.NodeCfgProfile{Name: "node-1"},
								NodeStrategy: &unified.DeadlineEvictStrategy{
									Enable:              pointer.Bool(false),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 10 * time.Minute}},
								},
							},
							{
								NodeCfgProfile: configuration.NodeCfgProfile{Name: "node-2"},
								NodeStrategy: &unified.DeadlineEvictStrategy{
									Enable:              pointer.Bool(true),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 15 * time.Minute}},
								},
							},
						},
					},
					"other-config": {
						ClusterStrategy: "other-config-val",
					},
				},
			},
			configMap: &corev1.ConfigMap{
				Data: map[string]string{
					//unified.DeadlineEvictConfigKey: fmt.Sprintf(`{
					//	"clusterStrategy": {
					//		"enable": true,
					//		"deadlineDuration": "7m"
					//	},
					//	"nodeStrategies": [
					//		{
					//			"name": "node-1",
					//			"strategy": {
					//				"enable": false,
					//				"deadlineDuration": "9m"
					//			}
					//		},
					//		{
					//			"name": "node-2",
					//			"strategy": {
					//				"enable": true,
					//				"deadlineDuration": "16m"
					//			}
					//		}
					//	]
					//}`),
					unified.DeadlineEvictConfigKey: fmt.Sprintf(`{
						"clusterStrategy": {
							"enable": true,
							"deadlineDuration": "7m"
						},
						"nodeStrategies": [
							{
								"name": "node-1",
									"enable": false,
									"deadlineDuration": "9m"
							},
							{
								"name": "node-2",
									"enable": true,
									"deadlineDuration": "16m"
							}
						]
					}`),
				},
			},
			expectedResult: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{
					unified.DeadlineEvictExtKey: {
						ClusterStrategy: &unified.DeadlineEvictStrategy{
							Enable:              pointer.Bool(true),
							DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 7 * time.Minute}},
						},
						NodeStrategies: []configuration.NodeExtensionStrategy{
							{
								NodeCfgProfile: configuration.NodeCfgProfile{Name: "node-1"},
								NodeStrategy: &unified.DeadlineEvictStrategy{
									Enable:              pointer.Bool(false),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 9 * time.Minute}},
								},
							},
							{
								NodeCfgProfile: configuration.NodeCfgProfile{Name: "node-2"},
								NodeStrategy: &unified.DeadlineEvictStrategy{
									Enable:              pointer.Bool(true),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 16 * time.Minute}},
								},
							},
						},
					},
					"other-config": {
						ClusterStrategy: "other-config-val",
					},
				},
			},
			expectedError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := calculateDeadlineEvictCfg(c.oldExtCfg, c.configMap)
			if c.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.expectedResult, result)
			}
		})
	}
}

func TestParseDeadlineEvictCfg(t *testing.T) {
	type testCase struct {
		name           string
		oldCfg         configuration.ExtensionCfgMap
		expectedResult *unified.DeadlineEvictCfg
		expectedError  bool
	}

	cases := []testCase{
		{
			name: "nil config",
			oldCfg: configuration.ExtensionCfgMap{
				Object: nil,
			},
			expectedResult: nil,
			expectedError:  false,
		},
		{
			name: "empty config",
			oldCfg: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{},
			},
			expectedResult: nil,
			expectedError:  false,
		},
		{
			name: "invalid json",
			oldCfg: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{
					unified.DeadlineEvictExtKey: {
						ClusterStrategy: "invalid json",
					},
				},
			},
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name: "valid config with node strategies",
			oldCfg: configuration.ExtensionCfgMap{
				Object: map[string]configuration.ExtensionCfg{
					unified.DeadlineEvictExtKey: {
						ClusterStrategy: &unified.DeadlineEvictStrategy{
							Enable:              pointer.Bool(true),
							DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 5 * time.Minute}},
						},
						NodeStrategies: []configuration.NodeExtensionStrategy{
							{
								NodeCfgProfile: configuration.NodeCfgProfile{
									Name: "node-1",
									NodeSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"test-key": "test-value",
										},
									},
								},
								NodeStrategy: &unified.DeadlineEvictStrategy{
									Enable:              pointer.Bool(false),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 10 * time.Minute}},
								},
							},
						},
					},
				},
			},
			expectedResult: &unified.DeadlineEvictCfg{
				ClusterStrategy: &unified.DeadlineEvictStrategy{
					Enable:              pointer.Bool(true),
					DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 5 * time.Minute}},
				},
				NodeStrategies: []unified.NodeDeadlineEvictStrategy{
					{
						NodeCfgProfile: configuration.NodeCfgProfile{
							Name: "node-1",
							NodeSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"test-key": "test-value",
								},
							},
						},
						DeadlineEvictStrategy: &unified.DeadlineEvictStrategy{
							Enable:              pointer.Bool(false),
							DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 10 * time.Minute}},
						},
					},
				},
			},
			expectedError: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := parseDeadlineEvictCfg(c.oldCfg)
			if c.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.expectedResult, result)
			}
		})
	}
}
