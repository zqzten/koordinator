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

package asiquotaadaptor

import (
	"context"
	"fmt"

	"github.com/spf13/pflag"
	asiquotav1 "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"
	uniclientset "gitlab.alibaba-inc.com/unischeduler/api/client/clientset/versioned"
	uniexternalversions "gitlab.alibaba-inc.com/unischeduler/api/client/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	schedclientset "sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned"
	schedinformer "sigs.k8s.io/scheduler-plugins/pkg/generated/informers/externalversions"
	schedlister "sigs.k8s.io/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
)

const (
	Name = "ASIQuotaAdaptor"
)

const (
	LabelQuotaSkipCheck string = "alibabacloud.com/skip-quota-check"
)

var (
	asiQuotaNamespace            = "asi-quota"
	enableSyncASIQuota           = true
	enableCompatibleWithASIQuota = false
	isLeader                     = false
)

func init() {
	pflag.BoolVar(&enableCompatibleWithASIQuota, "enable-compatible-with-asi-quota", enableCompatibleWithASIQuota, "Enable compatible with ASIQuota")
	pflag.BoolVar(&enableSyncASIQuota, "enable-sync-asi-quota", enableSyncASIQuota, "Enable sync from ASIQuota to ElasticQuota")
	pflag.StringVar(&asiQuotaNamespace, "asi-quota-namespace", "asi-quota", "The namespace of the elasticQuota created by ASIQuota")
}

var _ framework.PreFilterPlugin = &Plugin{}
var _ framework.ReservePlugin = &Plugin{}

type Plugin struct {
	cache                   *ASIQuotaCache
	schedClient             schedclientset.Interface
	elasticQuotaLister      schedlister.ElasticQuotaLister
	asiQuotaInformerFactory uniexternalversions.SharedInformerFactory
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	client, ok := handle.(uniclientset.Interface)
	if !ok {
		kubeConfig := handle.KubeConfig()
		kubeConfig.ContentType = runtime.ContentTypeJSON
		kubeConfig.AcceptContentTypes = runtime.ContentTypeJSON
		client = uniclientset.NewForConfigOrDie(kubeConfig)
	}
	asiQuotaInformerFactory := uniexternalversions.NewSharedInformerFactory(client, 0)

	schedClient, ok := handle.(schedclientset.Interface)
	if !ok {
		kubeConfig := *handle.KubeConfig()
		kubeConfig.ContentType = runtime.ContentTypeJSON
		kubeConfig.AcceptContentTypes = runtime.ContentTypeJSON
		schedClient = schedclientset.NewForConfigOrDie(&kubeConfig)
	}
	schedInformFactory := schedinformer.NewSharedInformerFactory(schedClient, 0)
	elasticQuotaLister := schedInformFactory.Scheduling().V1alpha1().ElasticQuotas().Lister()

	cache := newASIQuotaCache()
	pl := &Plugin{
		cache:                   cache,
		schedClient:             schedClient,
		elasticQuotaLister:      elasticQuotaLister,
		asiQuotaInformerFactory: asiQuotaInformerFactory,
	}

	if enableCompatibleWithASIQuota {
		registerASIQuotaEventHandler(cache, schedClient, elasticQuotaLister, asiQuotaInformerFactory)
		registerPodEventHandler(cache, handle.SharedInformerFactory())
	}

	if enableSyncASIQuota {
		if err := createASIQuotaNamespace(handle.ClientSet(), asiQuotaNamespace); err != nil {
			klog.ErrorS(err, "Failed to create ASIQuotaNamespace")
			return nil, err
		}
		ctx := context.TODO()
		schedInformFactory.Start(ctx.Done())
		schedInformFactory.WaitForCacheSync(ctx.Done())
	}

	return pl, nil
}

func (pl *Plugin) Name() string {
	return Name
}

func (pl *Plugin) NewControllers() ([]frameworkext.Controller, error) {
	return []frameworkext.Controller{pl}, nil
}

func (pl *Plugin) Start() {
	isLeader = true
}

func (pl *Plugin) EventsToRegister() []framework.ClusterEvent {
	// To register a custom event, follow the naming convention at:
	// https://github.com/kubernetes/kubernetes/blob/e1ad9bee5bba8fbe85a6bf6201379ce8b1a611b1/pkg/scheduler/eventhandlers.go#L415-L422
	gvk := fmt.Sprintf("quotas.%v.%v", asiquotav1.GroupVersion.Version, asiquotav1.GroupVersion.Group)
	return []framework.ClusterEvent{
		{Resource: framework.GVK(gvk), ActionType: framework.Add | framework.Update | framework.Delete},
	}
}

type stateData struct {
	skip      bool
	quotaName string
}

func (s *stateData) Clone() framework.StateData {
	return s
}

func getStateData(cycleState *framework.CycleState) *stateData {
	s, _ := cycleState.Read(Name)
	sd, _ := s.(*stateData)
	if sd == nil {
		sd = &stateData{
			skip: true,
		}
	}
	return sd
}

func (pl *Plugin) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	if pod.Labels[LabelQuotaSkipCheck] == "true" ||
		!k8sfeature.DefaultFeatureGate.Enabled(features.QuotaRunTime) {
		return nil, nil
	}

	quotaName := pod.Labels[asiquotav1.LabelQuotaName]
	quota := pl.cache.getQuota(quotaName)
	if quota == nil {
		if k8sfeature.DefaultFeatureGate.Enabled(features.RejectQuotaNotExist) {
			return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, "quota node exist")
		}
		return nil, nil
	}

	podRequests, _ := resource.PodRequestsAndLimits(pod)
	if quotav1.IsZero(podRequests) {
		return nil, nil
	}

	if apiext.GetPodQoSClass(pod) == apiext.QoSBE {
		podRequests = convertToBatchRequests(podRequests)
	}

	used := quotav1.Add(podRequests, quota.used)
	available := quota.getAvailable()

	if isLessEqual, exceedDimensions := quotav1.LessThanOrEqual(used, available); !isLessEqual {
		remained := quotav1.SubtractWithNonNegativeResult(available, quota.used)
		return nil, framework.NewStatus(framework.Unschedulable, fmt.Sprintf(
			"Insufficient quotas, quotaName: %v, available: %v, remained: %v, used: %v, pod's request: %v, exceedDimensions: %v",
			quotaName, marshalResourceList(available), marshalResourceList(remained),
			marshalResourceList(quota.used), marshalResourceList(podRequests), exceedDimensions))
	}

	cycleState.Write(Name, &stateData{
		skip:      false,
		quotaName: quotaName,
	})

	return nil, nil
}

func (pl *Plugin) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (pl *Plugin) Reserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	sd := getStateData(cycleState)
	if sd.skip {
		return nil
	}
	pl.cache.assumePod(pod)
	return nil
}

func (pl *Plugin) Unreserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) {
	sd := getStateData(cycleState)
	if sd.skip {
		return
	}
	pl.cache.forgetPod(pod)
}
