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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/koordinator-sh/koordinator/apis/configuration"
	"github.com/koordinator-sh/koordinator/pkg/util/sloconfig"
)

func Test_syncColocationConfigWithKataResourceIfChanged(t *testing.T) {
	sloconfig.RegisterDefaultColocationExtension(KataResExtKey, defaultKataResourceConfig)
	oldCfg := *sloconfig.NewDefaultColocationCfg()
	oldCfg.MemoryReclaimThresholdPercent = pointer.Int64Ptr(40)

	type fields struct {
		config *colocationCfgCache
	}
	type args struct {
		configMap *corev1.ConfigMap
	}
	type wants struct {
		available   bool
		errorStatus bool
		kataCfg     *KataResourceConfig
	}
	tests := []struct {
		name        string
		fields      fields
		args        args
		wantChanged bool
		wantField   *wants
	}{
		{
			name:   "no colocation config in configMap, no old cfg in cache, cfg will be changed to default cfg",
			fields: fields{config: &colocationCfgCache{}},
			args: args{configMap: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      sloconfig.SLOCtrlConfigMap,
					Namespace: sloconfig.ConfigNameSpace,
				},
				Data: map[string]string{},
			}},
			wantChanged: true,
			wantField: &wants{
				available:   true,
				errorStatus: false,
				kataCfg:     defaultKataResourceConfig,
			},
		},
		{
			name:   "no colocation config in configMap, old cfg is in cache, cfg will be changed to default cfg",
			fields: fields{config: &colocationCfgCache{colocationCfg: oldCfg}},
			args: args{configMap: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      sloconfig.SLOCtrlConfigMap,
					Namespace: sloconfig.ConfigNameSpace,
				},
				Data: map[string]string{},
			}},
			wantChanged: true,
			wantField: &wants{
				available:   true,
				errorStatus: false,
				kataCfg:     defaultKataResourceConfig,
			},
		},
		{
			name:   "no colocation config in configmap, cache has been set to default cfg",
			fields: fields{config: &colocationCfgCache{colocationCfg: *sloconfig.NewDefaultColocationCfg()}},
			args: args{configMap: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      sloconfig.SLOCtrlConfigMap,
					Namespace: sloconfig.ConfigNameSpace,
				},
				Data: map[string]string{},
			}},
			wantChanged: false,
			wantField: &wants{
				available:   true,
				errorStatus: false,
				kataCfg:     defaultKataResourceConfig,
			},
		},
		{
			name: "validate and merge partial config with the default config",
			fields: fields{config: &colocationCfgCache{
				colocationCfg: oldCfg,
				available:     true,
			}},
			args: args{configMap: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      sloconfig.SLOCtrlConfigMap,
					Namespace: sloconfig.ConfigNameSpace,
				},
				Data: map[string]string{
					configuration.ColocationConfigKey: "{\"metricAggregateDurationSeconds\":60,\"cpuReclaimThresholdPercent\":70," +
						"\"memoryReclaimThresholdPercent\":70,\"updateTimeThresholdSeconds\":100," +
						"\"metricReportIntervalSeconds\":20," +
						"\"extensions\":{\"kataResource\":{\"enable\":true}}}",
				},
			}},
			wantChanged: true,
			wantField: &wants{
				kataCfg:     &KataResourceConfig{Enable: pointer.BoolPtr(true)},
				available:   true,
				errorStatus: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			p := NewColocationHandlerForConfigMapEvent(fakeClient, *sloconfig.NewDefaultColocationCfg(), &record.FakeRecorder{})
			p.cfgCache = colocationCfgCache{
				colocationCfg: tt.fields.config.colocationCfg,
				available:     tt.fields.config.available,
			}
			got := p.syncColocationCfgIfChanged(tt.args.configMap)
			assert.Equal(t, tt.wantChanged, got)
			assert.Equal(t, tt.wantField.available, p.IsCfgAvailable())
			assert.Equal(t, tt.wantField.errorStatus, p.IsErrorStatus())
			gotStrategy := p.GetCfgCopy().ColocationStrategy
			gotKataExtCfg, err := ParseKataResourceConfig(&gotStrategy)
			assert.NoError(t, err, "parse kata resource config failed")
			assert.Equal(t, tt.wantField.kataCfg, gotKataExtCfg)
		})
	}
	sloconfig.UnregisterDefaultColocationExtension(KataResExtKey)
}
