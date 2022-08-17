package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_syncColocationConfigWithReclaimedResourceIfChanged(t *testing.T) {
	RegisterDefaultColocationExtension(ReclaimedResExtKey, defaultReclaimedResourceConfig)
	oldCfg := *NewDefaultColocationCfg()
	oldCfg.MemoryReclaimThresholdPercent = pointer.Int64Ptr(40)

	type fields struct {
		config *colocationCfgCache
	}
	type args struct {
		configMap *corev1.ConfigMap
	}
	type wants struct {
		available    bool
		errorStatus  bool
		reclaimedCfg *ReclaimedResourceConfig
	}
	tests := []struct {
		name        string
		fields      fields
		args        args
		wantChanged bool
		wantField   *wants
	}{
		{
			name:   "no colocation config in configmap, cache have no old cfg, cfg will be changed to use default cfg",
			fields: fields{config: &colocationCfgCache{}},
			args: args{configMap: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      SLOCtrlConfigMap,
					Namespace: ConfigNameSpace,
				},
				Data: map[string]string{},
			}},
			wantChanged: true,
			wantField:   &wants{reclaimedCfg: defaultReclaimedResourceConfig, available: true, errorStatus: false},
		},
		{
			name:   "no colocation config in configmap, cache have old cfg, cfg will be changed to use default cfg",
			fields: fields{config: &colocationCfgCache{colocationCfg: oldCfg}},
			args: args{configMap: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      SLOCtrlConfigMap,
					Namespace: ConfigNameSpace,
				},
				Data: map[string]string{},
			}},
			wantChanged: true,
			wantField:   &wants{reclaimedCfg: defaultReclaimedResourceConfig, available: true, errorStatus: false},
		},
		{
			name:   "no colocation config in configmap, cache has been set default cfg ,so not changed",
			fields: fields{config: &colocationCfgCache{colocationCfg: *NewDefaultColocationCfg()}},
			args: args{configMap: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      SLOCtrlConfigMap,
					Namespace: ConfigNameSpace,
				},
				Data: map[string]string{},
			}},
			wantChanged: false,
			wantField:   &wants{reclaimedCfg: defaultReclaimedResourceConfig, available: true, errorStatus: false},
		},
		{
			name: "validate and merge partial config with the default",
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
					Name:      SLOCtrlConfigMap,
					Namespace: ConfigNameSpace,
				},
				Data: map[string]string{
					ColocationConfigKey: "{\"metricAggregateDurationSeconds\":60,\"cpuReclaimThresholdPercent\":70," +
						"\"memoryReclaimThresholdPercent\":70,\"updateTimeThresholdSeconds\":100," +
						"\"metricReportIntervalSeconds\":20," +
						"\"extensions\":{\"reclaimedResource\":{\"NodeUpdate\":false}}}",
				},
			}},
			wantChanged: true,
			wantField: &wants{
				reclaimedCfg: &ReclaimedResourceConfig{NodeUpdate: pointer.BoolPtr(false)},
				available:    true,
				errorStatus:  false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			p := NewColocationHandlerForConfigMapEvent(fakeClient, *NewDefaultColocationCfg())
			p.cfgCache = colocationCfgCache{available: tt.fields.config.available, colocationCfg: tt.fields.config.colocationCfg}
			got := p.syncColocationCfgIfChanged(tt.args.configMap)
			assert.Equal(t, tt.wantChanged, got)
			assert.Equal(t, tt.wantField.available, p.cfgCache.IsAvailable())
			assert.Equal(t, tt.wantField.errorStatus, p.cfgCache.IsErrorStatus())

			gotStrategy := p.cfgCache.GetCfgCopy().ColocationStrategy
			gotReclaimedExtCfg, err := ParseReclaimedResourceConfig(&gotStrategy)
			assert.NoError(t, err, "parse reclaimed resource config failed")

			assert.Equal(t, tt.wantField.reclaimedCfg, gotReclaimedExtCfg)
		})
	}
	UnregisterDefaultColocationExtension(ReclaimedResExtKey)
}
