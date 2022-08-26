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

package configextensions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	slocontrollerconfig "github.com/koordinator-sh/koordinator/pkg/slo-controller/config"
)

func Test_resmanager_greyControlContext(t *testing.T) {
	// prepare
	testingConfigMapEmpty := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      slocontrollerconfig.ConfigNameSpace,
			Namespace: configName,
		},
	}
	testingConfigMapEmpty1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      slocontrollerconfig.ConfigNameSpace,
			Namespace: configName,
		},
		Data: map[string]string{},
	}
	testingConfigMapInvalid := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      slocontrollerconfig.ConfigNameSpace,
			Namespace: configName,
		},
		Data: map[string]string{
			string(greyControlFeatureMemoryQOS): `{invalid}`,
		},
	}
	testingConfigMapEnable := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      slocontrollerconfig.ConfigNameSpace,
			Namespace: configName,
		},
		Data: map[string]string{
			string(greyControlFeatureMemoryQOS): `{"enabledNamespaces":["default"],"disabledNamespaces":["ns-disable"]}`,
		},
	}
	testingConfigMapDisable := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      slocontrollerconfig.ConfigNameSpace,
			Namespace: configName,
		},
		Data: map[string]string{
			string(greyControlFeatureMemoryQOS): `{"disabledNamespaces":["default","ns-disable"]}`,
		},
	}

	var expectGreyControlNamespaceControlMapNil map[greyControlFeature]greyControlNamespaceList
	expectGreyControlNamespaceControlMap := map[greyControlFeature]greyControlNamespaceList{
		greyControlFeatureMemoryQOS: {
			whiteList: map[string]struct{}{"default": {}},
			blackList: map[string]struct{}{"ns-disable": {}},
		},
	}
	expectGreyControlNamespaceControlMap1 := map[greyControlFeature]greyControlNamespaceList{
		greyControlFeatureMemoryQOS: {
			whiteList: map[string]struct{}{},
			blackList: map[string]struct{}{"default": {}, "ns-disable": {}},
		},
	}

	r := &namespaceQOSControl{}

	r.updateGreyControlContext(testingConfigMapEmpty)
	assert.Equal(t, expectGreyControlNamespaceControlMapNil, testingGetGreyControlNamespaceControlMap(t, r))

	// empty case 1
	r.updateGreyControlContext(testingConfigMapEmpty1)
	assert.Equal(t, expectGreyControlNamespaceControlMapNil, testingGetGreyControlNamespaceControlMap(t, r))

	r.updateGreyControlContext(testingConfigMapInvalid)
	assert.Equal(t, expectGreyControlNamespaceControlMapNil, testingGetGreyControlNamespaceControlMap(t, r))

	r.updateGreyControlContext(testingConfigMapEnable)
	// assert create and parse configMap successfully
	assert.Equal(t, expectGreyControlNamespaceControlMap, testingGetGreyControlNamespaceControlMap(t, r))

	r.updateGreyControlContext(testingConfigMapDisable)
	// assert update and parse configMap successfully
	assert.Equal(t, expectGreyControlNamespaceControlMap1, testingGetGreyControlNamespaceControlMap(t, r))

	// does not update context if configMaps keep equal
	testingSetGreyControlConfigMap(t, r, testingConfigMapEmpty)
	r.updateGreyControlContext(testingConfigMapEmpty)
	assert.Equal(t, expectGreyControlNamespaceControlMap1, testingGetGreyControlNamespaceControlMap(t, r))
}

func testingGetGreyControlNamespaceControlMap(t *testing.T, r *namespaceQOSControl) map[greyControlFeature]greyControlNamespaceList {
	r.greyControlContextRWMutex.RLock()
	defer r.greyControlContextRWMutex.RUnlock()
	if r.greyControlContext.NamespaceControlMap == nil {
		return nil
	}
	m := map[greyControlFeature]greyControlNamespaceList{}
	for _, feature := range greyControlFeatureList {
		v, ok := r.greyControlContext.NamespaceControlMap[feature]
		if !ok {
			continue
		}
		m[feature] = greyControlNamespaceList{whiteList: map[string]struct{}{}, blackList: map[string]struct{}{}}
		for k, vv := range v.whiteList {
			m[feature].whiteList[k] = vv
		}
		for k, vv := range v.blackList {
			m[feature].blackList[k] = vv
		}
	}
	return m
}

func testingSetGreyControlConfigMap(t *testing.T, r *namespaceQOSControl, cm *corev1.ConfigMap) {
	r.greyControlContextRWMutex.Lock()
	defer r.greyControlContextRWMutex.Unlock()
	r.greyControlContext.ConfigMap = cm
	return
}

func TestNamespaceQOSControl_injectCPUBurst(t *testing.T) {
	type fields struct {
		greyControlContext greyControlContext
	}
	type args struct {
		pod      *corev1.Pod
		nsConfig interface{}
	}
	type wants struct {
		hasErr   bool
		injected bool
		burstCfg *slov1alpha1.CPUBurstConfig
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		wants  wants
	}{
		{
			name: "inject by allowed ns list",
			fields: fields{
				greyControlContext: greyControlContext{
					NamespaceControlMap: map[greyControlFeature]greyControlNamespaceList{
						greyControlFeatureCPUBurst: {
							whiteList: map[string]struct{}{
								"allow-ns": {},
							},
							blackList: map[string]struct{}{
								"block-ns": {},
							},
						},
					},
				},
			},
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "allow-ns",
					},
				},
				nsConfig: &slov1alpha1.CPUBurstConfig{},
			},
			wants: wants{
				hasErr:   false,
				injected: true,
				burstCfg: &slov1alpha1.CPUBurstConfig{
					Policy: slov1alpha1.CPUBurstAuto,
				},
			},
		},
		{
			name: "inject by block ns list",
			fields: fields{
				greyControlContext: greyControlContext{
					NamespaceControlMap: map[greyControlFeature]greyControlNamespaceList{
						greyControlFeatureCPUBurst: {
							whiteList: map[string]struct{}{
								"allow-ns": {},
							},
							blackList: map[string]struct{}{
								"block-ns": {},
							},
						},
					},
				},
			},
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "block-ns",
					},
				},
				nsConfig: &slov1alpha1.CPUBurstConfig{},
			},
			wants: wants{
				hasErr:   false,
				injected: true,
				burstCfg: &slov1alpha1.CPUBurstConfig{
					Policy: slov1alpha1.CPUBurstNone,
				},
			},
		},
		{
			name: "not inject for other ns",
			fields: fields{
				greyControlContext: greyControlContext{
					NamespaceControlMap: map[greyControlFeature]greyControlNamespaceList{
						greyControlFeatureCPUBurst: {
							whiteList: map[string]struct{}{
								"allow-ns": {},
							},
							blackList: map[string]struct{}{
								"block-ns": {},
							},
						},
					},
				},
			},
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "other-ns",
					},
				},
				nsConfig: &slov1alpha1.CPUBurstConfig{},
			},
			wants: wants{
				hasErr:   false,
				injected: false,
				burstCfg: &slov1alpha1.CPUBurstConfig{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &namespaceQOSControl{
				greyControlContext: tt.fields.greyControlContext,
			}
			injected, err := c.InjectPodPolicy(tt.args.pod, QOSPolicyCPUBurst, &tt.args.nsConfig)
			hasErr := err != nil
			assert.Equal(t, tt.wants.hasErr, hasErr)
			assert.Equal(t, tt.wants.injected, injected, "pod cpu burst field injected")
			assert.Equal(t, tt.wants.burstCfg, tt.args.nsConfig, "inject pod cpu burst policy")
		})
	}
}

func TestNamespaceQOSControl_injectMemoryQOS(t *testing.T) {
	type fields struct {
		greyControlContext greyControlContext
	}
	type args struct {
		pod      *corev1.Pod
		nsConfig interface{}
	}
	type wants struct {
		hasErr    bool
		injected  bool
		memQOSCfg *slov1alpha1.PodMemoryQOSConfig
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		wants  wants
	}{
		{
			name: "inject by allowed ns list",
			fields: fields{
				greyControlContext: greyControlContext{
					NamespaceControlMap: map[greyControlFeature]greyControlNamespaceList{
						greyControlFeatureMemoryQOS: {
							whiteList: map[string]struct{}{
								"allow-ns": {},
							},
							blackList: map[string]struct{}{
								"block-ns": {},
							},
						},
					},
				},
			},
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "allow-ns",
					},
				},
				nsConfig: &slov1alpha1.PodMemoryQOSConfig{},
			},
			wants: wants{
				hasErr:   false,
				injected: true,
				memQOSCfg: &slov1alpha1.PodMemoryQOSConfig{
					Policy: slov1alpha1.PodMemoryQOSPolicyAuto,
				},
			},
		},
		{
			name: "inject by block ns list",
			fields: fields{
				greyControlContext: greyControlContext{
					NamespaceControlMap: map[greyControlFeature]greyControlNamespaceList{
						greyControlFeatureMemoryQOS: {
							whiteList: map[string]struct{}{
								"allow-ns": {},
							},
							blackList: map[string]struct{}{
								"block-ns": {},
							},
						},
					},
				},
			},
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "block-ns",
					},
				},
				nsConfig: &slov1alpha1.PodMemoryQOSConfig{},
			},
			wants: wants{
				hasErr:   false,
				injected: true,
				memQOSCfg: &slov1alpha1.PodMemoryQOSConfig{
					Policy: slov1alpha1.PodMemoryQOSPolicyNone,
				},
			},
		},
		{
			name: "not inject for other ns",
			fields: fields{
				greyControlContext: greyControlContext{
					NamespaceControlMap: map[greyControlFeature]greyControlNamespaceList{
						greyControlFeatureMemoryQOS: {
							whiteList: map[string]struct{}{
								"allow-ns": {},
							},
							blackList: map[string]struct{}{
								"block-ns": {},
							},
						},
					},
				},
			},
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "other-ns",
					},
				},
				nsConfig: &slov1alpha1.PodMemoryQOSConfig{},
			},
			wants: wants{
				hasErr:    false,
				injected:  false,
				memQOSCfg: &slov1alpha1.PodMemoryQOSConfig{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &namespaceQOSControl{
				greyControlContext: tt.fields.greyControlContext,
			}
			injected, err := c.InjectPodPolicy(tt.args.pod, QOSPolicyMemoryQOS, &tt.args.nsConfig)
			hasErr := err != nil
			assert.Equal(t, tt.wants.hasErr, hasErr)
			assert.Equal(t, tt.wants.injected, injected, "pod cpu burst field injected")
			assert.Equal(t, tt.wants.memQOSCfg, tt.args.nsConfig, "inject pod memmory qos policy")
		})
	}
}
