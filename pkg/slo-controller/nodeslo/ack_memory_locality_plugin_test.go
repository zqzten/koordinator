package nodeslo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	ackapis "github.com/koordinator-sh/koordinator/apis/ackplugins"
	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/config"
)

func Test_mergeMemoryLocalityCfg(t *testing.T) {
	plugin := MemoryLocalityPlugin{}
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
			extension.ResourceQOSConfigKey: "{\"clusterStrategy\":{\"beClass\":{\"memoryLocality\":{\"policy\":\"bestEffort\"}}}}",
		},
	}
	expectTestingConfig1 := &ackapis.MemoryLocality{}
	expectTestingConfig1.Policy = ackapis.MemoryLocalityPolicyBesteffort.Pointer()
	expextTestingQos1 := &ackapis.MemoryLocalityQOS{MemoryLocality: expectTestingConfig1}
	expectTestingExtCfg1 := extension.ExtensionCfg{}
	expectTestingExtCfg1.ClusterStrategy = &ackapis.MemoryLocalityStrategy{BEClass: expextTestingQos1}
	expectTestingCfg1 := oldSLOCfg.DeepCopy()
	if expectTestingCfg1.ExtensionCfgMerged.Object == nil {
		expectTestingCfg1.ExtensionCfgMerged.Object = make(map[string]extension.ExtensionCfg)
	}
	expectTestingCfg1.ExtensionCfgMerged.Object[ackapis.MemoryLocalityExtKey] = expectTestingExtCfg1
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

func Test_getMemoryLocalityNodeSLOExtension(t *testing.T) {
	plugin := MemoryLocalityPlugin{}
	oldSLOCfg := DefaultSLOCfg()

	testingCfg := oldSLOCfg.DeepCopy()
	// expectTestingExtStrategy is nil

	testingConfig1 := &ackapis.MemoryLocality{}
	testingConfig1.Policy = ackapis.MemoryLocalityPolicyBesteffort.Pointer()
	testingQos1 := &ackapis.MemoryLocalityQOS{MemoryLocality: testingConfig1}
	testingExtCfg1 := extension.ExtensionCfg{}
	testingExtCfg1.ClusterStrategy = &ackapis.MemoryLocalityStrategy{BEClass: testingQos1}
	testingCfg1 := oldSLOCfg.DeepCopy()
	if testingCfg1.ExtensionCfgMerged.Object == nil {
		testingCfg1.ExtensionCfgMerged.Object = make(map[string]extension.ExtensionCfg)
	}
	testingCfg1.ExtensionCfgMerged.Object[ackapis.MemoryLocalityExtKey] = testingExtCfg1
	expectTestingExtStrategy1 := &ackapis.MemoryLocalityStrategy{BEClass: testingQos1.DeepCopy()}

	type args struct {
		oldCfg *SLOCfg
	}
	tests := []struct {
		name         string
		args         args
		wantStr      string
		wantStrategy *ackapis.MemoryLocalityStrategy
	}{
		{
			name:         "no config info",
			args:         args{oldCfg: testingCfg},
			wantStr:      ackapis.MemoryLocalityExtKey,
			wantStrategy: nil,
		},
		{
			name:         "config successfully",
			args:         args{oldCfg: testingCfg1},
			wantStr:      ackapis.MemoryLocalityExtKey,
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
			gotStrategy, ok := gotIf.(*ackapis.MemoryLocalityStrategy)
			assert.Equal(t, true, ok, "change interface to ext strategy")
			assert.Equal(t, tt.wantStrategy, gotStrategy, "get ext strategy")
		})
	}
}
