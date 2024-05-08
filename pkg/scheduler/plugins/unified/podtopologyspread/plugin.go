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

package podtopologyspread

import (
	"context"
	"fmt"

	dummyworkloadclientset "gitlab.alibaba-inc.com/serverlessinfra/dummy-workload/pkg/client/clientset/versioned"
	dummyworkloadinformers "gitlab.alibaba-inc.com/serverlessinfra/dummy-workload/pkg/client/informers/externalversions"
	dummyworkloadlister "gitlab.alibaba-inc.com/serverlessinfra/dummy-workload/pkg/client/listers/acs/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	scheduledconfigv1beta3 "k8s.io/kube-scheduler/config/v1beta3"
	schedconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/v1beta3"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/validation"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"

	"github.com/koordinator-sh/koordinator/pkg/features"
)

const (
	// ErrReasonConstraintsNotMatch is used for PodTopologySpread filter error.
	ErrReasonConstraintsNotMatch = "node(s) didn't match pod topology spread constraints"
	// ErrReasonNodeLabelNotMatch is used when the node doesn't hold the required label.
	ErrReasonNodeLabelNotMatch = ErrReasonConstraintsNotMatch + " (missing required label)"
)

var systemDefaultConstraints = []v1.TopologySpreadConstraint{
	{
		TopologyKey:       v1.LabelHostname,
		WhenUnsatisfiable: v1.ScheduleAnyway,
		MaxSkew:           3,
	},
	{
		TopologyKey:       v1.LabelTopologyZone,
		WhenUnsatisfiable: v1.ScheduleAnyway,
		MaxSkew:           5,
	},
}

// PodTopologySpread is a plugin that ensures pod's topologySpreadConstraints is satisfied.
type PodTopologySpread struct {
	systemDefaulted     bool
	handle              framework.Handle
	defaultConstraints  []v1.TopologySpreadConstraint
	dummyWorkloadLister dummyworkloadlister.DummyWorkloadLister
	sharedLister        framework.SharedLister
	services            corelisters.ServiceLister
	replicationCtrls    corelisters.ReplicationControllerLister
	replicaSets         appslisters.ReplicaSetLister
	statefulSets        appslisters.StatefulSetLister
}

var _ framework.PreFilterPlugin = &PodTopologySpread{}
var _ framework.FilterPlugin = &PodTopologySpread{}
var _ framework.PreScorePlugin = &PodTopologySpread{}
var _ framework.ScorePlugin = &PodTopologySpread{}
var _ framework.EnqueueExtensions = &PodTopologySpread{}

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	Name = "UnifiedPodTopologySpread"
)

// Name returns name of the plugin. It is used in logs, etc.
func (pl *PodTopologySpread) Name() string {
	return Name
}

// New initializes a new plugin and returns it.
func New(obj runtime.Object, h framework.Handle) (framework.Plugin, error) {
	if h.SnapshotSharedLister() == nil {
		return nil, fmt.Errorf("SnapshotSharedlister is nil")
	}
	args, err := getArgs(obj)
	if err != nil {
		return nil, err
	}
	if err := validation.ValidatePodTopologySpreadArgs(nil, args); err != nil {
		return nil, err
	}
	pl := &PodTopologySpread{
		handle:             h,
		sharedLister:       h.SnapshotSharedLister(),
		defaultConstraints: args.DefaultConstraints,
	}
	if args.DefaultingType == schedconfig.SystemDefaulting {
		pl.defaultConstraints = systemDefaultConstraints
		pl.systemDefaulted = true
	}
	if len(pl.defaultConstraints) != 0 {
		if h.SharedInformerFactory() == nil {
			return nil, fmt.Errorf("SharedInformerFactory is nil")
		}
		pl.setListers(h.SharedInformerFactory())
	}
	if k8sfeature.DefaultFeatureGate.Enabled(features.EnableACSDefaultSpread) {
		kubeConfig := *h.KubeConfig()
		kubeConfig.ContentType = runtime.ContentTypeJSON
		kubeConfig.AcceptContentTypes = runtime.ContentTypeJSON
		client, err := dummyworkloadclientset.NewForConfig(&kubeConfig)
		if err != nil {
			return nil, err
		}
		informerFactory := dummyworkloadinformers.NewSharedInformerFactory(client, 0)
		dummyWorkloadLister := informerFactory.Acs().V1alpha1().DummyWorkloads().Lister()
		informerFactory.Start(context.Background().Done())
		informerFactory.WaitForCacheSync(context.Background().Done())
		pl.dummyWorkloadLister = dummyWorkloadLister
	}
	return pl, nil
}

func getArgs(obj runtime.Object) (*schedconfig.PodTopologySpreadArgs, error) {
	if obj == nil {
		return getDefaultPodTopologySpreadArgs()
	}

	if args, ok := obj.(*schedconfig.PodTopologySpreadArgs); ok {
		return args, nil
	}

	unknownObj, ok := obj.(*runtime.Unknown)
	if !ok {
		return nil, fmt.Errorf("got args of type %T, want *PodTopologySpreadArgs", obj)
	}

	var v1beta3args scheduledconfigv1beta3.PodTopologySpreadArgs
	v1beta3.SetDefaults_PodTopologySpreadArgs(&v1beta3args)
	if err := frameworkruntime.DecodeInto(unknownObj, &v1beta3args); err != nil {
		return nil, err
	}
	var podTopologySpreadArgs schedconfig.PodTopologySpreadArgs
	err := v1beta3.Convert_v1beta3_PodTopologySpreadArgs_To_config_PodTopologySpreadArgs(&v1beta3args, &podTopologySpreadArgs, nil)
	if err != nil {
		return nil, err
	}
	return &podTopologySpreadArgs, nil
}

func getDefaultPodTopologySpreadArgs() (*schedconfig.PodTopologySpreadArgs, error) {
	var v1beta3args scheduledconfigv1beta3.PodTopologySpreadArgs
	v1beta3.SetDefaults_PodTopologySpreadArgs(&v1beta3args)
	var podTopologySpreadArgs schedconfig.PodTopologySpreadArgs
	err := v1beta3.Convert_v1beta3_PodTopologySpreadArgs_To_config_PodTopologySpreadArgs(&v1beta3args, &podTopologySpreadArgs, nil)
	if err != nil {
		return nil, err
	}
	return &podTopologySpreadArgs, nil
}

func (pl *PodTopologySpread) setListers(factory informers.SharedInformerFactory) {
	pl.services = factory.Core().V1().Services().Lister()
	pl.replicationCtrls = factory.Core().V1().ReplicationControllers().Lister()
	pl.replicaSets = factory.Apps().V1().ReplicaSets().Lister()
	pl.statefulSets = factory.Apps().V1().StatefulSets().Lister()
}

// EventsToRegister returns the possible events that may make a Pod
// failed by this plugin schedulable.
func (pl *PodTopologySpread) EventsToRegister() []framework.ClusterEventWithHint {
	return []framework.ClusterEventWithHint{
		// All ActionType includes the following events:
		// - Add. An unschedulable Pod may fail due to violating topology spread constraints,
		// adding an assigned Pod may make it schedulable.
		// - Update. Updating on an existing Pod's labels (e.g., removal) may make
		// an unschedulable Pod schedulable.
		// - Delete. An unschedulable Pod may fail due to violating an existing Pod's topology spread constraints,
		// deleting an existing Pod may make it schedulable.
		{Event: framework.ClusterEvent{Resource: framework.Pod, ActionType: framework.All}},
		// Node add|delete|updateLabel maybe lead an topology key changed,
		// and make these pod in scheduling schedulable or unschedulable.
		{Event: framework.ClusterEvent{Resource: framework.Node, ActionType: framework.Add | framework.Delete | framework.UpdateNodeLabel}},
	}
}
