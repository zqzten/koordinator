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

package interpodaffinity

import (
	"fmt"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	listersv1 "k8s.io/client-go/listers/core/v1"
	scheduledconfigv1beta2config "k8s.io/kube-scheduler/config/v1beta2"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/v1beta2"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/validation"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	plfeature "k8s.io/kubernetes/pkg/scheduler/framework/plugins/feature"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/interpodaffinity"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
)

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	Name = "UnifiedInterPodAffinity"
)

var _ framework.PreFilterPlugin = &InterPodAffinity{}
var _ framework.FilterPlugin = &InterPodAffinity{}
var _ framework.PreScorePlugin = &InterPodAffinity{}
var _ framework.ScorePlugin = &InterPodAffinity{}
var _ framework.EnqueueExtensions = &InterPodAffinity{}

// InterPodAffinity is a plugin that checks inter pod affinity
type InterPodAffinity struct {
	*interpodaffinity.InterPodAffinity
	handle                  framework.Handle
	sharedLister            framework.SharedLister
	nsLister                listersv1.NamespaceLister
	enableNamespaceSelector bool
}

// Name returns name of the plugin. It is used in logs, etc.
func (pl *InterPodAffinity) Name() string {
	return Name
}

// New initializes a new plugin and returns it.
func New(plArgs runtime.Object, h framework.Handle) (framework.Plugin, error) {
	fts := plfeature.Features{
		EnablePodAffinityNamespaceSelector: utilfeature.DefaultFeatureGate.Enabled(features.PodAffinityNamespaceSelector),
	}
	return NewWithFeature(plArgs, h, fts)
}

func NewWithFeature(plArgs runtime.Object, h framework.Handle, fts plfeature.Features) (framework.Plugin, error) {
	if h.SnapshotSharedLister() == nil {
		return nil, fmt.Errorf("SnapshotSharedlister is nil")
	}
	args, err := getArgs(plArgs)
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateInterPodAffinityArgs(nil, &args); err != nil {
		return nil, err
	}
	internalPlugin, err := interpodaffinity.New(&args, h)
	if err != nil {
		return nil, err
	}
	pl := &InterPodAffinity{
		InterPodAffinity:        internalPlugin.(*interpodaffinity.InterPodAffinity),
		handle:                  h,
		sharedLister:            h.SnapshotSharedLister(),
		enableNamespaceSelector: true,
	}

	if pl.enableNamespaceSelector {
		pl.nsLister = h.SharedInformerFactory().Core().V1().Namespaces().Lister()
	}
	return pl, nil
}

func getArgs(obj runtime.Object) (config.InterPodAffinityArgs, error) {
	defaultInterPodAffinityArgs, err := getDefaultInterPodAffinityArgs()
	if err != nil {
		return config.InterPodAffinityArgs{}, err
	}

	if obj != nil {
		switch t := obj.(type) {
		case *config.InterPodAffinityArgs:
			defaultInterPodAffinityArgs = t
		case *runtime.Unknown:
			unknownObj := t
			if err := frameworkruntime.DecodeInto(unknownObj, defaultInterPodAffinityArgs); err != nil {
				return config.InterPodAffinityArgs{}, err
			}
		default:
			return config.InterPodAffinityArgs{}, fmt.Errorf("got args of type %T, want *InterPodAffinityArgs", obj)
		}
	}
	return *defaultInterPodAffinityArgs, nil
}

func getDefaultInterPodAffinityArgs() (*config.InterPodAffinityArgs, error) {
	var v1beta2args scheduledconfigv1beta2config.InterPodAffinityArgs
	v1beta2.SetDefaults_InterPodAffinityArgs(&v1beta2args)
	var interPodAffinityArgs config.InterPodAffinityArgs
	err := v1beta2.Convert_v1beta2_InterPodAffinityArgs_To_config_InterPodAffinityArgs(&v1beta2args, &interPodAffinityArgs, nil)
	if err != nil {
		return nil, err
	}
	return &interPodAffinityArgs, nil
}

// Updates Namespaces with the set of namespaces identified by NamespaceSelector.
// If successful, NamespaceSelector is set to nil.
// The assumption is that the term is for an incoming pod, in which case
// namespaceSelector is either unrolled into Namespaces (and so the selector
// is set to Nothing()) or is Empty(), which means match everything. Therefore,
// there when matching against this term, there is no need to lookup the existing
// pod's namespace labels to match them against term's namespaceSelector explicitly.
func (pl *InterPodAffinity) mergeAffinityTermNamespacesIfNotEmpty(at *framework.AffinityTerm) error {
	if at.NamespaceSelector.Empty() {
		return nil
	}
	ns, err := pl.nsLister.List(at.NamespaceSelector)
	if err != nil {
		return err
	}
	for _, n := range ns {
		at.Namespaces.Insert(n.Name)
	}
	at.NamespaceSelector = labels.Nothing()
	return nil
}
