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

package podconstraint

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	unifiedfake "gitlab.alibaba-inc.com/unischeduler/api/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	scheduledconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
	"k8s.io/utils/pointer"

	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config/v1beta2"
	schedulingconfigv1beta2 "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config/v1beta2"
)

var _ framework.SharedLister = &testSharedLister{}

type testSharedLister struct {
	nodes       []*corev1.Node
	nodeInfos   []*framework.NodeInfo
	nodeInfoMap map[string]*framework.NodeInfo
}

func newTestSharedLister(pods []*corev1.Pod, nodes []*corev1.Node) *testSharedLister {
	nodeInfoMap := make(map[string]*framework.NodeInfo)
	nodeInfos := make([]*framework.NodeInfo, 0)
	for _, pod := range pods {
		nodeName := pod.Spec.NodeName
		if _, ok := nodeInfoMap[nodeName]; !ok {
			nodeInfoMap[nodeName] = framework.NewNodeInfo()
		}
		nodeInfoMap[nodeName].AddPod(pod)
	}
	for _, node := range nodes {
		if _, ok := nodeInfoMap[node.Name]; !ok {
			nodeInfoMap[node.Name] = framework.NewNodeInfo()
		}

		nodeInfoMap[node.Name].SetNode(node)
		nodeInfos = append(nodeInfos, nodeInfoMap[node.Name])
	}

	return &testSharedLister{
		nodes:       nodes,
		nodeInfos:   nodeInfos,
		nodeInfoMap: nodeInfoMap,
	}
}

func (f *testSharedLister) NodeInfos() framework.NodeInfoLister {
	return f
}

func (f *testSharedLister) List() ([]*framework.NodeInfo, error) {
	return f.nodeInfos, nil
}

func (f *testSharedLister) HavePodsWithAffinityList() ([]*framework.NodeInfo, error) {
	return nil, nil
}

func (f *testSharedLister) HavePodsWithRequiredAntiAffinityList() ([]*framework.NodeInfo, error) {
	return nil, nil
}

func (f *testSharedLister) Get(nodeName string) (*framework.NodeInfo, error) {
	return f.nodeInfoMap[nodeName], nil
}

type frameworkHandleExtender struct {
	framework.Handle
	*unifiedfake.Clientset
}

func proxyPluginFactory(fakeClientSet *unifiedfake.Clientset, factory runtime.PluginFactory) runtime.PluginFactory {
	return func(configuration apiruntime.Object, f framework.Handle) (framework.Plugin, error) {
		return factory(configuration, &frameworkHandleExtender{
			Handle:    f,
			Clientset: fakeClientSet,
		})
	}
}

type pluginTestSuit struct {
	framework.Handle
	unifiedClientSet *unifiedfake.Clientset
	proxyNew         runtime.PluginFactory
	args             apiruntime.Object
}

func newPluginTestSuit(t *testing.T, nodes []*corev1.Node, pods []*corev1.Pod) *pluginTestSuit {
	var v1beta2args v1beta2.UnifiedPodConstraintArgs
	schedulingconfigv1beta2.SetDefaults_UnifiedPodConstraintArgs(&v1beta2args)
	var unifiedPodConstraintArgs schedulingconfig.UnifiedPodConstraintArgs
	err := schedulingconfigv1beta2.Convert_v1beta2_UnifiedPodConstraintArgs_To_config_UnifiedPodConstraintArgs(&v1beta2args, &unifiedPodConstraintArgs, nil)
	assert.NoError(t, err)
	unifiedPodConstraintArgs.EnableDefaultPodConstraint = pointer.BoolPtr(true)
	unifiedCPUSetAllocatorPluginConfig := scheduledconfig.PluginConfig{
		Name: Name,
		Args: &unifiedPodConstraintArgs,
	}

	unifiedClientSet := unifiedfake.NewSimpleClientset()

	proxyNew := proxyPluginFactory(unifiedClientSet, New)

	registeredPlugins := []st.RegisterPluginFunc{
		func(reg *runtime.Registry, profile *scheduledconfig.KubeSchedulerProfile) {
			profile.PluginConfig = []scheduledconfig.PluginConfig{
				unifiedCPUSetAllocatorPluginConfig,
			}
		},
		st.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
		st.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
	}

	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newTestSharedLister(pods, nodes)
	fh, err := st.NewFramework(
		registeredPlugins,
		"koord-scheduler",
		runtime.WithClientSet(cs),
		runtime.WithInformerFactory(informerFactory),
		runtime.WithSnapshotSharedLister(snapshot),
	)
	assert.Nil(t, err)
	return &pluginTestSuit{
		Handle:           fh,
		unifiedClientSet: unifiedClientSet,
		proxyNew:         proxyNew,
		args:             &unifiedPodConstraintArgs,
	}
}

func (p *pluginTestSuit) start() {
	ctx := context.TODO()
	p.Handle.SharedInformerFactory().Start(ctx.Done())
	p.Handle.SharedInformerFactory().WaitForCacheSync(ctx.Done())
}
