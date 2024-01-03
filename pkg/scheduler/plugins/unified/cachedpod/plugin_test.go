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

package cachedpod

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	cachepodapis "gitlab.alibaba-inc.com/cache/api/pb/generated"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	koordfake "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned/fake"
	koordinatorinformers "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config/v1beta2"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	frameworkexttesting "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/testing"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/hijack"
)

var _ framework.SharedLister = &fakeSharedLister{}

type fakeSharedLister struct {
	nodes       []*corev1.Node
	nodeInfos   []*framework.NodeInfo
	nodeInfoMap map[string]*framework.NodeInfo
	listErr     bool
}

func newFakeSharedLister(pods []*corev1.Pod, nodes []*corev1.Node, listErr bool) *fakeSharedLister {
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
	}

	for _, v := range nodeInfoMap {
		nodeInfos = append(nodeInfos, v)
	}

	return &fakeSharedLister{
		nodes:       nodes,
		nodeInfos:   nodeInfos,
		nodeInfoMap: nodeInfoMap,
		listErr:     listErr,
	}
}

func (f *fakeSharedLister) NodeInfos() framework.NodeInfoLister {
	return f
}

func (f *fakeSharedLister) List() ([]*framework.NodeInfo, error) {
	if f.listErr {
		return nil, fmt.Errorf("list error")
	}
	return f.nodeInfos, nil
}

func (f *fakeSharedLister) HavePodsWithAffinityList() ([]*framework.NodeInfo, error) {
	return nil, nil
}

func (f *fakeSharedLister) HavePodsWithRequiredAntiAffinityList() ([]*framework.NodeInfo, error) {
	return nil, nil
}

func (f *fakeSharedLister) Get(nodeName string) (*framework.NodeInfo, error) {
	return f.nodeInfoMap[nodeName], nil
}

type pluginTestSuit struct {
	fw              framework.Framework
	pluginFactory   func(args *config.CachedPodArgs) (framework.Plugin, error)
	extenderFactory *frameworkext.FrameworkExtenderFactory
}

func newPluginTestSuitWith(t testing.TB, pods []*corev1.Pod, nodes []*corev1.Node) *pluginTestSuit {
	koordClientSet := koordfake.NewSimpleClientset()
	koordSharedInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordClientSet, 0)
	extenderFactory, _ := frameworkext.NewFrameworkExtenderFactory(
		frameworkext.WithKoordinatorClientSet(koordClientSet),
		frameworkext.WithKoordinatorSharedInformerFactory(koordSharedInformerFactory),
		frameworkext.WithReservationNominator(frameworkexttesting.NewFakeReservationNominator()),
	)
	extenderFactory.InitScheduler(frameworkext.NewFakeScheduler())
	proxyNew := frameworkext.PluginFactoryProxy(extenderFactory, New)

	registeredPlugins := []schedulertesting.RegisterPluginFunc{
		schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
		schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
	}

	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	snapshot := newFakeSharedLister(pods, nodes, false)

	fakeRecorder := record.NewFakeRecorder(1024)
	eventRecorder := record.NewEventRecorderAdapter(fakeRecorder)

	fw, err := schedulertesting.NewFramework(
		registeredPlugins,
		"koord-scheduler",
		frameworkruntime.WithClientSet(cs),
		frameworkruntime.WithInformerFactory(informerFactory),
		frameworkruntime.WithSnapshotSharedLister(snapshot),
		frameworkruntime.WithEventRecorder(eventRecorder),
	)
	assert.NoError(t, err)

	factory := func(args *config.CachedPodArgs) (framework.Plugin, error) {
		return proxyNew(args, fw)
	}

	return &pluginTestSuit{
		fw:              fw,
		pluginFactory:   factory,
		extenderFactory: extenderFactory,
	}
}

func newPluginTestSuit(t *testing.T) *pluginTestSuit {
	return newPluginTestSuitWith(t, nil, nil)
}

func (s *pluginTestSuit) start() {
	s.fw.SharedInformerFactory().Start(nil)
	s.extenderFactory.KoordinatorSharedInformerFactory().Start(nil)
	s.fw.SharedInformerFactory().WaitForCacheSync(nil)
	s.extenderFactory.KoordinatorSharedInformerFactory().WaitForCacheSync(nil)
}

func getDefaultArgsForTest(t *testing.T) *config.CachedPodArgs {
	var v1beta2args v1beta2.CachedPodArgs
	v1beta2args.Address = ":0"
	v1beta2.SetDefaults_CachedPodArgs(&v1beta2args)
	var cachedPodArgs config.CachedPodArgs
	err := v1beta2.Convert_v1beta2_CachedPodArgs_To_config_CachedPodArgs(&v1beta2args, &cachedPodArgs, nil)
	assert.NoError(t, err)
	return &cachedPodArgs
}

type fakeServer struct {
	cachepodapis.UnimplementedCacheSchedulerServer
	completeRequestPod *corev1.Pod
	cachedPod          *corev1.Pod
	err                error
}

func (s *fakeServer) AllocateCachedPods(ctx context.Context, req *cachepodapis.AllocateCachedPodsRequest) (*cachepodapis.AllocateCachedPodsResponse, error) {
	return nil, nil
}

func (s *fakeServer) completeRequest(requestPod, cachedPod *corev1.Pod, err error) {
	s.completeRequestPod = requestPod
	s.cachedPod = cachedPod
	s.err = err
}

func TestErrorHandler(t *testing.T) {
	fs := &fakeServer{}
	pl := &Plugin{
		server: fs,
	}
	requestPod := NewFakePod("123456", &corev1.PodTemplateSpec{}, 0)
	queuedPodInfo := &framework.QueuedPodInfo{
		PodInfo: framework.NewPodInfo(requestPod),
	}
	err := fmt.Errorf("fake error")
	assert.True(t, pl.ErrorHandler(queuedPodInfo, err))

	assert.Equal(t, requestPod, fs.completeRequestPod)
	assert.Nil(t, fs.cachedPod)
	assert.Equal(t, err, fs.err)

	assert.False(t, pl.ErrorHandler(&framework.QueuedPodInfo{PodInfo: framework.NewPodInfo(&corev1.Pod{})}, err))
}

func TestPostFilter(t *testing.T) {
	suit := newPluginTestSuit(t)
	p, err := suit.pluginFactory(getDefaultArgsForTest(t))
	assert.NoError(t, err)
	pl := p.(*Plugin)
	suit.start()

	_, status := pl.PostFilter(context.TODO(), framework.NewCycleState(), &corev1.Pod{}, nil)
	assert.Equal(t, framework.NewStatus(framework.Unschedulable), status)

	requestPod := NewFakePod("123456", &corev1.PodTemplateSpec{}, 0)
	_, status = pl.PostFilter(context.TODO(), framework.NewCycleState(), requestPod, nil)
	assert.True(t, status.Code() == framework.Error)
}

func TestReserve(t *testing.T) {
	suit := newPluginTestSuit(t)
	p, err := suit.pluginFactory(getDefaultArgsForTest(t))
	assert.NoError(t, err)
	pl := p.(*Plugin)
	suit.start()

	status := pl.Reserve(context.TODO(), framework.NewCycleState(), &corev1.Pod{}, "")
	assert.True(t, status.IsSuccess())

	// no nominated reservation
	cycleState := framework.NewCycleState()
	requestPod := NewFakePod("123456", &corev1.PodTemplateSpec{}, 0)
	status = pl.Reserve(context.TODO(), cycleState, requestPod, "")
	assert.False(t, status.IsSuccess())

	// nominated reservation is reservation
	pl.handle.GetReservationNominator().AddNominatedReservation(requestPod, "test-node", frameworkext.NewReservationInfo(&schedulingv1alpha1.Reservation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-r",
		},
		Spec: schedulingv1alpha1.ReservationSpec{
			Template: &corev1.PodTemplateSpec{},
		},
	}))
	status = pl.Reserve(context.TODO(), cycleState, requestPod, "test-node")
	assert.False(t, status.IsSuccess())

	targetPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Labels: map[string]string{
				apiext.LabelPodOperatingMode: string(apiext.ReservationPodOperatingMode),
			},
		},
	}
	pl.handle.GetReservationNominator().AddNominatedReservation(requestPod, "test-node", frameworkext.NewReservationInfoFromPod(targetPod))
	status = pl.Reserve(context.TODO(), cycleState, requestPod, "test-node")
	targetPodInState := hijack.GetTargetPod(cycleState)
	assert.Equal(t, targetPod, targetPodInState)
	assert.True(t, status.IsSuccess())
}

func TestBind(t *testing.T) {
	suit := newPluginTestSuit(t)
	p, err := suit.pluginFactory(getDefaultArgsForTest(t))
	assert.NoError(t, err)
	pl := p.(*Plugin)
	suit.start()
	fs := &fakeServer{}
	pl.server = fs
	status := pl.Bind(context.TODO(), framework.NewCycleState(), &corev1.Pod{}, "nodeName")
	assert.True(t, status.Code() == framework.Skip)

	// no nominated reservation
	cycleState := framework.NewCycleState()
	requestPod := NewFakePod("123456", &corev1.PodTemplateSpec{}, 0)
	assert.Equal(t, corev1.NamespaceDefault, requestPod.Namespace)
	assert.Equal(t, corev1.DefaultSchedulerName, requestPod.Spec.SchedulerName)
	assert.NotNil(t, requestPod.Spec.Priority)
	status = pl.Bind(context.TODO(), cycleState, requestPod, "")
	assert.False(t, status.IsSuccess())

	// nominated reservation is reservation
	pl.handle.GetReservationNominator().AddNominatedReservation(requestPod, "test-node", frameworkext.NewReservationInfo(&schedulingv1alpha1.Reservation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-r",
		},
		Spec: schedulingv1alpha1.ReservationSpec{
			Template: &corev1.PodTemplateSpec{},
		},
	}))
	status = pl.Bind(context.TODO(), cycleState, requestPod, "test-node")
	assert.False(t, status.IsSuccess())

	cachedPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Labels: map[string]string{
				apiext.LabelPodOperatingMode: string(apiext.ReservationPodOperatingMode),
			},
		},
	}
	pl.handle.GetReservationNominator().AddNominatedReservation(requestPod, "test-node", frameworkext.NewReservationInfoFromPod(cachedPod))
	status = pl.Bind(context.TODO(), cycleState, requestPod, "test-node")
	assert.True(t, status.IsSuccess())

	assert.Equal(t, requestPod, fs.completeRequestPod)
	assert.Equal(t, cachedPod, fs.cachedPod)
	assert.Nil(t, fs.err)
}
