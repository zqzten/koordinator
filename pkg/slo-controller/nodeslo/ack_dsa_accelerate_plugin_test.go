package nodeslo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"

	ackapis "github.com/koordinator-sh/koordinator/apis/ackplugins"
	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/config"
)

func Test_mergeDsaAccelerateCfg(t *testing.T) {
	plugin := DsaAcceleratePlugin{}
	oldSLOCfg := DefaultSLOCfg()

	expectTestingCfg := oldSLOCfg.DeepCopy()

	testingConfigMap1 := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.SLOCtrlConfigMap,
			Namespace: config.ConfigNameSpace,
		},
		Data: map[string]string{
			ackapis.DsaAccelerateConfigKey: "{\"clusterStrategy\":{\"enable\":false}}",
		},
	}
	expectTestingExtCfg1 := extension.ExtensionCfg{}
	expectTestingExtCfg1.ClusterStrategy = &ackapis.DsaAccelerateStrategy{Enable: pointer.BoolPtr(false)}
	expectTestingCfg1 := oldSLOCfg.DeepCopy()
	if expectTestingCfg1.ExtensionCfgMerged.Object == nil {
		expectTestingCfg1.ExtensionCfgMerged.Object = make(map[string]extension.ExtensionCfg)
	}
	expectTestingCfg1.ExtensionCfgMerged.Object[ackapis.DsaAccelerateExtKey] = expectTestingExtCfg1
	type args struct {
		configMap *corev1.ConfigMap
	}
	tests := []struct {
		name string
		args args
		want *SLOCfg
	}{
		{
			name: "no config info",
			args: args{configMap: &corev1.ConfigMap{}},
			want: expectTestingCfg,
		},
		{
			name: "config successfully",
			args: args{configMap: testingConfigMap1},
			want: expectTestingCfg1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeRecorder := record.NewFakeRecorder(1024)
			got, err := plugin.MergeNodeSLOExtension(oldSLOCfg.DeepCopy().ExtensionCfgMerged, tt.args.configMap, fakeRecorder)
			assert.NoError(t, err)
			assert.Equal(t, tt.want.ExtensionCfgMerged, got, "merge node slo extension")
		})
	}
}

func Test_getDsaAccelerateNodeSLOExtension(t *testing.T) {
	plugin := DsaAcceleratePlugin{}
	oldSLOCfg := DefaultSLOCfg()

	testingCfg := oldSLOCfg.DeepCopy()

	testingExtCfg1 := extension.ExtensionCfg{}
	testingExtCfg1.ClusterStrategy = &ackapis.DsaAccelerateStrategy{Enable: pointer.BoolPtr(false)}
	testingCfg1 := oldSLOCfg.DeepCopy()
	if testingCfg1.ExtensionCfgMerged.Object == nil {
		testingCfg1.ExtensionCfgMerged.Object = make(map[string]extension.ExtensionCfg)
	}
	testingCfg1.ExtensionCfgMerged.Object[ackapis.DsaAccelerateExtKey] = testingExtCfg1
	expectTestingExtStrategy1 := &ackapis.DsaAccelerateStrategy{}
	expectTestingExtStrategy1.Enable = pointer.BoolPtr(false)

	type args struct {
		oldCfg *SLOCfg
	}
	tests := []struct {
		name         string
		args         args
		wantStr      string
		wantStrategy *ackapis.DsaAccelerateStrategy
	}{
		{
			name:         "no config info",
			args:         args{oldCfg: testingCfg},
			wantStr:      ackapis.DsaAccelerateExtKey,
			wantStrategy: nil,
		},
		{
			name:         "config successfully",
			args:         args{oldCfg: testingCfg1},
			wantStr:      ackapis.DsaAccelerateExtKey,
			wantStrategy: expectTestingExtStrategy1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStr, gotIf, err := plugin.GetNodeSLOExtension(&corev1.Node{}, &tt.args.oldCfg.DeepCopy().ExtensionCfgMerged)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStr, gotStr, "get ext key")
			if gotIf == nil {
				assert.Empty(t, tt.wantStrategy)
				return
			}
			gotStrategy, ok := gotIf.(*ackapis.DsaAccelerateStrategy)
			assert.Equal(t, true, ok, "change interface to ext strategy")
			assert.Equal(t, tt.wantStrategy, gotStrategy, "get ext strategy")
		})
	}
}
