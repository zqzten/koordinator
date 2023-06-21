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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func TestParseDynamicProdResourceConfig(t *testing.T) {
	testPolicy := unified.ProdOvercommitPolicyAuto
	testCfg := DefaultDynamicProdResourceConfig.DeepCopy()
	testCfg.ProdOvercommitPolicy = &testPolicy
	tests := []struct {
		name    string
		arg     *extension.ColocationStrategy
		want    *unified.DynamicProdResourceConfig
		wantErr bool
	}{
		{
			name:    "failed to parse nil colocation strategy",
			arg:     nil,
			wantErr: true,
		},
		{
			name:    "use the default when the extension is empty",
			arg:     &extension.ColocationStrategy{},
			want:    DefaultDynamicProdResourceConfig,
			wantErr: false,
		},
		{
			name: "use the default when the config data is empty",
			arg: &extension.ColocationStrategy{
				ColocationStrategyExtender: extension.ColocationStrategyExtender{
					Extensions: map[string]interface{}{},
				},
			},
			want:    DefaultDynamicProdResourceConfig,
			wantErr: false,
		},
		{
			name: "failed to convert the config data",
			arg: &extension.ColocationStrategy{
				ColocationStrategyExtender: extension.ColocationStrategyExtender{
					Extensions: map[string]interface{}{
						DynamicProdResourceExtKey: 1,
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "failed to parse the config data",
			arg: &extension.ColocationStrategy{
				ColocationStrategyExtender: extension.ColocationStrategyExtender{
					Extensions: map[string]interface{}{
						DynamicProdResourceExtKey: "invalidField",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "parse the config data successfully",
			arg: &extension.ColocationStrategy{
				ColocationStrategyExtender: extension.ColocationStrategyExtender{
					Extensions: map[string]interface{}{
						DynamicProdResourceExtKey: &unified.DynamicProdResourceConfig{
							ProdOvercommitPolicy: &testPolicy,
						},
					},
				},
			},
			want:    testCfg,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := ParseDynamicProdResourceConfig(tt.arg)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantErr, gotErr != nil)
		})
	}
}
