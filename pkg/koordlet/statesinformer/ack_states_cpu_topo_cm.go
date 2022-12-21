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
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"

	"gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension/cpuset"
)

/*
This code is compatible with ACK cpu topo structure in the specific configmap.
http://gitlab.alibaba-inc.com/cos/node-resource-agent/tree/master/numa

type CPUSet struct {
	Elems map[int]Cpuinfo    //cpuId -> cpuinfo
	LLC   string
}

type L3CPUTopology struct {
	Topology  CPUSet
	Allocated map[int]int    //cpuId -> count
}

1.18 cpu topo structure:  map[int]CPUSet    //numaId -> cpuset
1.20 cpu topo structure:  map[int]map[int]map[int]L3CPUTopology    //socketId -> numaId -> L3Id -> L3 topo structure

example-1:  k8s 1.20

apiVersion: v1
binaryData:
  info: |-
    {
    "0": {
        "0": {
            "0": {
                "Topology": {
                    "elems": {
                        "0": {"id": 0,"node": 0,"socket": 0,"core": 0,"l1dl1il2": "0","l3": 0,"online": "yes","size": 0},
                        "1": {"id": 1,"node": 0,"socket": 0,"core": 0,"l1dl1il2": "0","l3": 0,"online": "yes","size": 0}
                    }
                },
                "Allocated": {
                    "0": 0,
                    "1": 0
                }
            }
        },
        "1": {
            "1": {
                "Topology": {
                    "elems": {
                        "2": {"id": 2,"node": 1,"socket": 0,"core": 1,"l1dl1il2": "1","l3": 1,"online": "yes","size": 0},
                        "3": {"id": 3,"node": 1,"socket": 0,"core": 1,"l1dl1il2": "1","l3": 1,"online": "yes","size": 0}
                    }
                },
                "Allocated": {
                    "2": 0,
                    "3": 0
                }
            }
        }
    }
    }
kind: ConfigMap
metadata:
  labels:
    ack.node.cpu.schedule: topology.20
  name: node001-numa-info
  namespace: kube-system


example-1:  k8s 1.18

apiVersion: v1
binaryData:
  info: |-
    {
    "0": {
        "elems": {
            "0": {"id": 0,"node": 0,"socket": 0,"core": 0,"l1dl1il2": "","l3": 0,"online": "","size": 0},
            "1": {"id": 0,"node": 0,"socket": 0,"core": 0,"l1dl1il2": "","l3": 0,"online": "","size": 0}
        },
        "llc": "7ff"
    },
    "1": {
        "elems": {
            "2": {"id": 0,"node": 0,"socket": 0,"core": 0,"l1dl1il2": "","l3": 0,"online": "","size": 0},
            "3": {"id": 0,"node": 0,"socket": 0,"core": 0,"l1dl1il2": "","l3": 0,"online": "","size": 0}
        },
        "llc": "7ff"
    }
    }
kind: ConfigMap
metadata:
  labels:
    ack.node.cpu.schedule: topology
  name: node002-numa-info
  namespace: kube-system


ConfigMap Pattern:
  name: ${nodeName} + "-numa-info"
  namespace: {{ .util.ConfigNamespace }}
  Labels["ack.node.cpu.schedule"] = "topology" or "topology.20"
*/

const (
	cpuTopoCMPluginName = "cpuTopoCM"

	ConfigNameSpace          = "kube-system"
	CPUTopoCMNUMANodeSuffix  = "-numa-info"
	CPUTopoCMCPUInfoLabelKey = "ack.node.cpu.schedule"
	CPUTopoCMCPUInfoDataKey  = "info"
	CPUTopoCMTopology1_18    = "topology"
	CPUTopoCMTopology1_20    = "topology.20"

	syncInterval = 5 * time.Minute
)

type cpuTopoCMReporter struct {
	kubeClient   clientset.Interface
	nodeName     string
	metricCache  metriccache.MetricCache
	nodeInformer *nodeInformer

	configMapInformer cache.SharedIndexInformer
	configMapLister   listerv1.ConfigMapLister
}

func NewCPUTopoCMInformer() *cpuTopoCMReporter {
	return &cpuTopoCMReporter{}
}

func (cr *cpuTopoCMReporter) Setup(ctx *pluginOption, state *pluginState) {
	cr.kubeClient = ctx.KubeClient
	cr.nodeName = ctx.NodeName
	cr.metricCache = state.metricCache
	nodeInformerIf := state.informerPlugins[nodeInformerName]
	nodeInformer, ok := nodeInformerIf.(*nodeInformer)
	if !ok {
		klog.Fatalf("node informer format error")
	}
	cr.nodeInformer = nodeInformer

	tweakListOptionsFunc := func(opt *metav1.ListOptions) {
		opt.FieldSelector = "metadata.name=" + GetCPUTopologyConfigName(cr.nodeName)
	}

	configMapInformer := informers.NewSharedInformerFactoryWithOptions(cr.kubeClient, 0, informers.WithTweakListOptions(tweakListOptionsFunc)).
		Core().V1().ConfigMaps()
	configMapInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			switch t := obj.(type) {
			case *v1.ConfigMap:
				if _, ok := t.Labels[CPUTopoCMCPUInfoLabelKey]; ok {
					return true
				}
				return false
			default:
				return false
			}
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { cr.runSync() },
			UpdateFunc: func(oldObj, newObj interface{}) { cr.runSync() },
			DeleteFunc: func(obj interface{}) { cr.runSync() },
		},
	})
	cr.configMapInformer = configMapInformer.Informer()
	cr.configMapLister = configMapInformer.Lister()
}

func (cr *cpuTopoCMReporter) Start(stopCh <-chan struct{}) {
	if !features.DefaultKoordletFeatureGate.Enabled(features.ConfigMapCPUTopology) {
		klog.V(3).Infof("feature %v not enalbed, no need to run module", features.ConfigMapCPUTopology)
		return
	}
	if !cache.WaitForCacheSync(stopCh, cr.nodeInformer.HasSynced) {
		klog.Fatalf("timed out waiting for node caches to sync")
	}
	if err := cr.initialize(stopCh); err != nil {
		klog.Fatalf("start cpu topo failed, error %v", err)
	}
	go wait.Until(cr.runSync, syncInterval, stopCh)
}

func (cr *cpuTopoCMReporter) HasSynced() bool {
	if !features.DefaultKoordletFeatureGate.Enabled(features.ConfigMapCPUTopology) {
		return true
	}
	if cr.configMapInformer == nil {
		return false
	}
	return cr.configMapInformer.HasSynced()
}

func (cr *cpuTopoCMReporter) initialize(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	klog.Infof("starting cpu topo reporter")

	go cr.configMapInformer.Run(stopCh)
	if !cache.WaitForCacheSync(stopCh, cr.configMapInformer.HasSynced) {
		return fmt.Errorf("timed out waiting for configmap caches to sync")
	}

	klog.Info("start cpu topo reporter successfully")
	return nil
}

func (cr *cpuTopoCMReporter) runSync() {
	backoff := wait.Backoff{
		Steps:    5,
		Duration: 1 * time.Second,
		Factor:   2.0,
		Jitter:   0.1,
	}
	retErr := retry.OnError(backoff, func(e error) bool { return e != nil }, cr.sync)
	if retErr != nil {
		klog.Errorf("failed to sync cpu topo error: %v", retErr)
	}
}

func (cr *cpuTopoCMReporter) sync() error {
	klog.Infoln("Fetching node numa info")
	var info []byte
	var topologyValue string
	version, err := cr.kubeClient.Discovery().ServerVersion()
	if err != nil {
		return err
	}
	klog.Infof("Kubernetes cluster version is  %s", version.String())
	if version.GitVersion < "v1.20.0" {
		numaInfo, err := cr.GetNUMANodeInfo()
		if err != nil {
			return fmt.Errorf("Get numa node info error %v ", err)
		}
		info, _ = json.Marshal(numaInfo)
		topologyValue = CPUTopoCMTopology1_18
	} else {
		numaInfo, err := cr.GetDetailNUMANodeInfo()
		if err != nil {
			return fmt.Errorf("Get numa node info error %v ", err)
		}
		info, _ = json.Marshal(numaInfo)
		topologyValue = CPUTopoCMTopology1_20
	}
	klog.V(3).Infof("json marshal %s", string(info))

	node := cr.nodeInformer.GetNode()
	if node == nil {
		return fmt.Errorf("get nil from nodeInformer")
	}
	cfm := &v1.ConfigMap{}
	cfm.Namespace = ConfigNameSpace
	cfm.Name = GetCPUTopologyConfigName(cr.nodeName)
	cfm.Labels = map[string]string{
		CPUTopoCMCPUInfoLabelKey: topologyValue,
	}
	cfm.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion:         "v1",
			Kind:               "Node",
			Name:               node.Name,
			UID:                node.GetUID(),
			Controller:         pointer.Bool(true),
			BlockOwnerDeletion: pointer.Bool(true),
		},
	}
	cfm.BinaryData = make(map[string][]byte)
	cfm.BinaryData[CPUTopoCMCPUInfoDataKey] = info

	cfmRemote, err := cr.configMapLister.ConfigMaps(ConfigNameSpace).Get(cfm.Name)
	if errors.IsNotFound(err) {
		_, err = cr.kubeClient.CoreV1().ConfigMaps(ConfigNameSpace).Create(context.TODO(), cfm.DeepCopy(), metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("Create %s cpu info configmap error: %v", cfm.Name, err)
		}
		klog.Infof("Create %s cpu info configmap success! CPU topo info: %s, Configmap: %v,", cfm.Name, string(info), cfm)
		return nil
	} else if err != nil {
		return fmt.Errorf("Get cfm %s error %v", cfm.Name, err)
	}

	infoRemote := cfmRemote.BinaryData[CPUTopoCMCPUInfoDataKey]
	topologyRemote := cfmRemote.Labels[CPUTopoCMCPUInfoLabelKey]
	// When k8s version is upgraded from 1.18 to 1.20, the cpu topo needs to be upgraded.
	// Delete cpu topo configmap, and recreate it.
	// TODO update configmap instead of delete and create, can be fixed after scheduler add update informer
	if topologyValue != topologyRemote || string(info) != string(infoRemote) {
		_, err = cr.kubeClient.CoreV1().ConfigMaps(ConfigNameSpace).Update(context.TODO(), cfm, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("recreate %s cpu info configmap error: %v", cfm.Name, err)
		}
		return nil
	}

	return nil
}

type NUMANodeInfo map[int]cpuset.CPUSet

// GetNUMANodeInfo return a map of NUMANode id to the list of
// CPUs associated with that NUMANode.
//
func (cr *cpuTopoCMReporter) GetNUMANodeInfo() (NUMANodeInfo, error) {
	// Get the possible NUMA nodes on this machine. If reading this file
	// is not possible, this is not an error. Instead, we just return a
	// nil NUMANodeInfo, indicating that no NUMA information is available
	// on this machine. This should implicitly be interpreted as having a
	// single NUMA node with id 0 for all CPUs.
	info := make(NUMANodeInfo)
	nodeCPUInfo, err := cr.metricCache.GetNodeCPUInfo(&metriccache.QueryParam{})
	if err != nil {
		return nil, err
	}
	if nodeCPUInfo == nil {
		return nil, fmt.Errorf("get nodeCPUInfo error!")
	}
	llc := nodeCPUInfo.BasicInfo.CatL3CbmMask
	if len(llc) == 0 {
		llc = "Not support l3 cache allocation"
	}
	for _, val := range nodeCPUInfo.ProcessorInfos {
		if _, ok := info[int(val.NodeID)]; !ok {
			cpu := cpuset.NewCPUSet(int(val.CPUID))
			cpu.LLC = llc
			info[int(val.NodeID)] = cpu
		} else {
			cpu := info[int(val.NodeID)].Union(cpuset.NewCPUSet(int(val.CPUID)))
			cpu.LLC = llc
			info[int(val.NodeID)] = cpu
		}
	}

	return info, nil
}

// GetDetailNUMANodeInfo return detail cpu topo info
func (cr *cpuTopoCMReporter) GetDetailNUMANodeInfo() (cpuset.NodeCPUTopology, error) {
	var nodeInfo cpuset.NodeCPUTopology = make(cpuset.NodeCPUTopology)
	nodeCPUInfo, err := cr.metricCache.GetNodeCPUInfo(&metriccache.QueryParam{})
	if err != nil {
		return nil, err
	}
	if nodeCPUInfo == nil {
		return nil, fmt.Errorf("get nodeCPUInfo error!")
	}

	for _, val := range nodeCPUInfo.ProcessorInfos {
		cpu := cpuset.CPUInfo{}
		cpu.Id = int(val.CPUID)
		cpu.Node = int(val.NodeID)
		cpu.Core = int(val.CoreID)
		cpu.Socket = int(val.SocketID)
		cpu.L1dl1il2 = val.L1dl1il2
		cpu.L3 = int(val.L3)
		cpu.Online = val.Online

		if _, ok := nodeInfo[cpu.Socket]; !ok {
			nodeInfo[cpu.Socket] = make(map[int]cpuset.NUMANodeCPUTopology)
		}
		socketInfo := nodeInfo[cpu.Socket]

		if _, ok := socketInfo[cpu.Node]; !ok {
			socketInfo[cpu.Node] = make(map[int]cpuset.L3CPUTopology)
		}
		numaNodeInfo := socketInfo[cpu.Node]

		if _, ok := numaNodeInfo[cpu.L3]; !ok {
			numaNodeInfo[cpu.L3] = cpuset.L3CPUTopology{
				Topology: cpuset.CPUSet{
					Elems: make(map[int]cpuset.CPUInfo),
				},
				Allocated: make(map[int]int),
			}
		}

		l3info := numaNodeInfo[cpu.L3]
		allocated := numaNodeInfo[cpu.L3].Allocated
		allocated[cpu.Id] = 0
		l3info.Allocated = allocated
		elem := numaNodeInfo[cpu.L3].Topology.Elems
		elem[cpu.Id] = cpu
		l3info.Topology.Elems = elem
		numaNodeInfo[cpu.L3] = l3info
	}
	return nodeInfo, nil
}

func GetCPUTopologyConfigName(nodeName string) string {
	return nodeName + CPUTopoCMNUMANodeSuffix
}
