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

package cpusetallocator

import (
	"context"
	"encoding/json"

	uniapiext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	"gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension/cpuset"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/apis/extension"
	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
)

type nodeResourceTopologySyncer struct {
	cpuTopologyManager nodenumaresource.CPUTopologyManager
}

func syncCPUTopologyConfigMap(handle framework.Handle, cpuTopologyManager nodenumaresource.CPUTopologyManager) {
	syncer := &nodeResourceTopologySyncer{
		cpuTopologyManager: cpuTopologyManager,
	}
	configMapInformer := handle.SharedInformerFactory().Core().V1().ConfigMaps().Informer()
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), handle.SharedInformerFactory(), configMapInformer,
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.ConfigMap:
					return uniapiext.IsCPUTopology1_20(t)
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    syncer.addConfigMap,
				DeleteFunc: syncer.deleteConfigMap,
			},
		})
}

func (n *nodeResourceTopologySyncer) addConfigMap(obj interface{}) {
	config := obj.(*v1.ConfigMap)
	nodeName := config.Name[0 : len(config.Name)-len(uniapiext.ConfigMapNUMANodeSuffix)]

	klog.Infof("process ConfigMap %s/%s", config.Namespace, config.Name)

	info := config.BinaryData[uniapiext.DataConfigMapCPUInfo]
	var nodeCPUTopology = make(cpuset.NodeCPUTopology)
	err := json.Unmarshal(info, &nodeCPUTopology)
	if err != nil {
		klog.Errorf("Unmarshal err %v", err.Error())
	}

	cpuTopology := convertToCPUTopology(nodeCPUTopology)

	n.cpuTopologyManager.UpdateCPUTopologyOptions(nodeName, func(options *nodenumaresource.CPUTopologyOptions) {
		options.CPUTopology = convertCPUTopology(&cpuTopology)
	})
}

func convertToCPUTopology(topology cpuset.NodeCPUTopology) extension.CPUTopology {
	var cpuTopology extension.CPUTopology
	for _, socket := range topology {
		for _, numa := range socket {
			for _, l3 := range numa {
				for cpuIndex := range l3.Topology.Elems {
					cpuInfo := l3.Topology.Elems[cpuIndex]

					cpuTopology.Detail = append(cpuTopology.Detail, extension.CPUInfo{
						ID:     int32(cpuInfo.Id),
						Core:   int32(cpuInfo.Core),
						Socket: int32(cpuInfo.Socket),
						Node:   int32(cpuInfo.Node),
					})
				}
			}
		}
	}
	return cpuTopology
}

func (n *nodeResourceTopologySyncer) deleteConfigMap(obj interface{}) {
	config := obj.(*v1.ConfigMap)
	nodeName := config.Name[0 : len(config.Name)-len(uniapiext.ConfigMapNUMANodeSuffix)]
	n.cpuTopologyManager.Delete(nodeName)
}

func convertCPUTopology(reportedCPUTopology *extension.CPUTopology) *nodenumaresource.CPUTopology {
	builder := nodenumaresource.NewCPUTopologyBuilder()
	for _, info := range reportedCPUTopology.Detail {
		builder.AddCPUInfo(int(info.Socket), int(info.Node), int(info.Core), int(info.ID))
	}
	return builder.Result()
}
