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

package statesinformer

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclient "k8s.io/client-go/kubernetes/fake"

	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	mock_metriccache "github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache/mockmetriccache"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

var info = &metriccache.NodeCPUInfo{
	BasicInfo: util.CPUBasicInfo{CatL3CbmMask: "7ff"},
	ProcessorInfos: []util.ProcessorInfo{
		{CPUID: 0, CoreID: 0, SocketID: 0, NodeID: 0, L1dl1il2: "0", L3: 0, Online: "yes"},
		{CPUID: 1, CoreID: 0, SocketID: 0, NodeID: 0, L1dl1il2: "0", L3: 0, Online: "yes"},
		{CPUID: 2, CoreID: 1, SocketID: 0, NodeID: 0, L1dl1il2: "1", L3: 0, Online: "yes"},
		{CPUID: 3, CoreID: 1, SocketID: 0, NodeID: 0, L1dl1il2: "1", L3: 0, Online: "yes"},
	},
}
var info2 = &metriccache.NodeCPUInfo{
	BasicInfo: util.CPUBasicInfo{CatL3CbmMask: "7ff"},
	ProcessorInfos: []util.ProcessorInfo{
		{CPUID: 0, CoreID: 0, SocketID: 0, NodeID: 0, L1dl1il2: "0", L3: 0, Online: "yes"},
		{CPUID: 1, CoreID: 0, SocketID: 0, NodeID: 0, L1dl1il2: "0", L3: 0, Online: "yes"},
		{CPUID: 2, CoreID: 1, SocketID: 0, NodeID: 1, L1dl1il2: "1", L3: 1, Online: "yes"},
		{CPUID: 3, CoreID: 1, SocketID: 0, NodeID: 1, L1dl1il2: "1", L3: 1, Online: "yes"},
	},
}

const lscpu_1_json = `{"0":{"0":{"0":{"Topology":{"elems":{"0":{"id":0,"node":0,"socket":0,"core":0,"l1dl1il2":"0","l3":0,"online":"yes","size":0},"1":{"id":1,"node":0,"socket":0,"core":0,"l1dl1il2":"0","l3":0,"online":"yes","size":0},"2":{"id":2,"node":0,"socket":0,"core":1,"l1dl1il2":"1","l3":0,"online":"yes","size":0},"3":{"id":3,"node":0,"socket":0,"core":1,"l1dl1il2":"1","l3":0,"online":"yes","size":0}}},"Allocated":{"0":0,"1":0,"2":0,"3":0}}}}}`

const lscpu_2_json = `{"0":{"0":{"0":{"Topology":{"elems":{"0":{"id":0,"node":0,"socket":0,"core":0,"l1dl1il2":"0","l3":0,"online":"yes","size":0},"1":{"id":1,"node":0,"socket":0,"core":0,"l1dl1il2":"0","l3":0,"online":"yes","size":0}}},"Allocated":{"0":0,"1":0}}},"1":{"1":{"Topology":{"elems":{"2":{"id":2,"node":1,"socket":0,"core":1,"l1dl1il2":"1","l3":1,"online":"yes","size":0},"3":{"id":3,"node":1,"socket":0,"core":1,"l1dl1il2":"1","l3":1,"online":"yes","size":0}}},"Allocated":{"2":0,"3":0}}}}}`

const devcie_1_json = `{"0":{"elems":{"0":{"id":0,"node":0,"socket":0,"core":0,"l1dl1il2":"","l3":0,"online":"","size":0},"1":{"id":0,"node":0,"socket":0,"core":0,"l1dl1il2":"","l3":0,"online":"","size":0},"2":{"id":0,"node":0,"socket":0,"core":0,"l1dl1il2":"","l3":0,"online":"","size":0},"3":{"id":0,"node":0,"socket":0,"core":0,"l1dl1il2":"","l3":0,"online":"","size":0}},"llc":"7ff"}}`

const devcie_2_json = `{"0":{"elems":{"0":{"id":0,"node":0,"socket":0,"core":0,"l1dl1il2":"","l3":0,"online":"","size":0},"1":{"id":0,"node":0,"socket":0,"core":0,"l1dl1il2":"","l3":0,"online":"","size":0}},"llc":"7ff"},"1":{"elems":{"2":{"id":0,"node":0,"socket":0,"core":0,"l1dl1il2":"","l3":0,"online":"","size":0},"3":{"id":0,"node":0,"socket":0,"core":0,"l1dl1il2":"","l3":0,"online":"","size":0}},"llc":"7ff"}}`

func generateConfigMap(nodeName, version, info string, isDel bool) *v1.ConfigMap {
	cm := &v1.ConfigMap{}
	cm.Namespace = ConfigNameSpace
	cm.Name = GetCPUTopologyConfigName(nodeName)
	cm.Labels = map[string]string{}
	cm.BinaryData = map[string][]byte{
		CPUTopoCMCPUInfoDataKey: []byte(info),
	}
	if version < "v1.20.0" {
		cm.Labels[CPUTopoCMCPUInfoLabelKey] = CPUTopoCMTopology1_18
	} else {
		cm.Labels[CPUTopoCMCPUInfoLabelKey] = CPUTopoCMTopology1_20
	}
	if isDel {
		cm.DeletionTimestamp = &metav1.Time{}
	}

	return cm
}

func Test_cpuTopoCMReporter(t *testing.T) {
	type fields struct {
		nodeName string
		version  string
		objs     []runtime.Object
		info     *metriccache.NodeCPUInfo
		infoErr  error
	}
	tests := []struct {
		name   string
		fields fields
		want   *v1.ConfigMap
	}{
		{
			name: "v1.20, no configmap, get topo error",
			fields: fields{
				nodeName: "node-1",
				version:  "v1.20.4",
				objs:     []runtime.Object{},
				info:     nil,
				infoErr:  fmt.Errorf("get topo error"),
			},
		},
		{
			name: "v1.20, no configmap, get topo nil",
			fields: fields{
				nodeName: "node-1",
				version:  "v1.20.4",
				objs:     []runtime.Object{},
				info:     nil,
				infoErr:  nil,
			},
		},
		{
			name: "v1.18, no configmap, get topo error",
			fields: fields{
				nodeName: "node-1",
				version:  "v1.18.5",
				objs:     []runtime.Object{},
				info:     nil,
				infoErr:  fmt.Errorf("get topo error"),
			},
		},
		{
			name: "v1.18, no configmap, get topo nil",
			fields: fields{
				nodeName: "node-1",
				version:  "v1.18.5",
				objs:     []runtime.Object{},
				info:     nil,
				infoErr:  nil,
			},
		},
		{
			name: "v1.20, no configmap, create new",
			fields: fields{
				nodeName: "node-1",
				version:  "v1.20.4",
				objs:     []runtime.Object{},
				info:     info,
				infoErr:  nil,
			},
			want: generateConfigMap("node-1", "v1.20.4", lscpu_1_json, false),
		},
		{
			name: "v1.20, no configmap, create new 2",
			fields: fields{
				nodeName: "node-1",
				version:  "v1.20.4",
				objs:     []runtime.Object{},
				info:     info2,
				infoErr:  nil,
			},
			want: generateConfigMap("node-1", "v1.20.4", lscpu_2_json, false),
		},
		{
			name: "v1.18, no configmap, create new",
			fields: fields{
				nodeName: "node-2",
				version:  "v1.18.5",
				objs:     []runtime.Object{},
				info:     info,
				infoErr:  nil,
			},
			want: generateConfigMap("node-2", "v1.18.5", devcie_1_json, false),
		},
		{
			name: "v1.18, no configmap, create new 2",
			fields: fields{
				nodeName: "node-2",
				version:  "v1.18.5",
				objs:     []runtime.Object{},
				info:     info2,
				infoErr:  nil,
			},
			want: generateConfigMap("node-2", "v1.18.5", devcie_2_json, false),
		},
		{
			name: "v1.20, replace cm",
			fields: fields{
				nodeName: "node-1",
				version:  "v1.20.4",
				objs: []runtime.Object{
					generateConfigMap("node-1", "v1.18.5", devcie_1_json, false),
				},
				info:    info,
				infoErr: nil,
			},
			want: generateConfigMap("node-1", "v1.20.4", lscpu_1_json, false),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			metricCache := mock_metriccache.NewMockMetricCache(ctrl)
			metricCache.EXPECT().GetNodeCPUInfo(gomock.Any()).Return(tt.fields.info, tt.fields.infoErr).AnyTimes()
			cs := fakeclient.NewSimpleClientset(tt.fields.objs...)
			fakeDiscovery, _ := cs.Discovery().(*fakediscovery.FakeDiscovery)
			fakeDiscovery.FakedServerVersion = &version.Info{GitVersion: tt.fields.version}

			cr := NewCPUTopoCMInformer()
			opt := &pluginOption{
				KubeClient: cs,
				NodeName:   tt.fields.nodeName,
			}
			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.fields.nodeName,
					UID:  types.UID(tt.fields.nodeName + "uid"),
				},
			}
			stat := &pluginState{
				metricCache: metricCache,
				informerPlugins: map[pluginName]informerPlugin{
					nodeInformerName: &nodeInformer{
						node: node,
					},
				},
			}
			cr.Setup(opt, stat)
			cr.initialize(make(<-chan struct{}))
			cr.runSync()
			cm, _ := cs.CoreV1().ConfigMaps(ConfigNameSpace).Get(context.TODO(), GetCPUTopologyConfigName(tt.fields.nodeName), metav1.GetOptions{})
			if (cm != nil) != (tt.want != nil) {
				t.Errorf("runSync() = %v, want %v", cm, tt.want)
				return
			}
			if cm == nil && tt.want == nil {
				return
			}
			if cm.Labels[CPUTopoCMCPUInfoLabelKey] != tt.want.Labels[CPUTopoCMCPUInfoLabelKey] ||
				string(cm.BinaryData[CPUTopoCMCPUInfoDataKey]) != string(tt.want.BinaryData[CPUTopoCMCPUInfoDataKey]) {
				t.Errorf("runSync() = %v, want %v", cm, tt.want)
			}
		})
	}
}
