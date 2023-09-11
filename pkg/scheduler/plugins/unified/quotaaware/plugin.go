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

package quotaaware

import (
	"context"
	"fmt"
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"
	apiresource "k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling"
	schedlisters "sigs.k8s.io/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota"
)

const (
	Name = "QuotaAware"
)

var _ framework.EnqueueExtensions = &Plugin{}
var _ framework.PreFilterPlugin = &Plugin{}
var _ framework.PostFilterPlugin = &Plugin{}
var _ framework.ReservePlugin = &Plugin{}
var _ framework.PreBindPlugin = &Plugin{}

type Plugin struct {
	internalPlugin     *elasticquota.Plugin
	handle             framework.Handle
	quotaCache         *QuotaCache
	podInfoCache       *podInfoCache
	elasticQuotaLister schedlisters.ElasticQuotaLister
}

func New(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	args, err := getElasticQuotaArgs(obj)
	if err != nil {
		return nil, err
	}
	internalPlugin, err := elasticquota.New(args, handle)
	if err != nil {
		return nil, err
	}
	elasticQuotaPlugin := internalPlugin.(*elasticquota.Plugin)
	elasticQuotaInformer := elasticQuotaPlugin.GetElasticQuotaInformer()

	quotaCache := newQuotaCache()
	registerElasticQuotaEventHandler(quotaCache, elasticQuotaInformer)
	elasticQuotaLister := schedlisters.NewElasticQuotaLister(elasticQuotaInformer.GetIndexer())

	podInfoCache := newPodInfoCache()
	registerPodEventHandler(quotaCache, podInfoCache, handle.SharedInformerFactory())

	pl := &Plugin{
		internalPlugin:     internalPlugin.(*elasticquota.Plugin),
		handle:             handle,
		quotaCache:         quotaCache,
		podInfoCache:       podInfoCache,
		elasticQuotaLister: elasticQuotaLister,
	}
	extendedHandle := handle.(frameworkext.ExtendedHandle)
	extendedHandle.RegisterErrorHandlerFilters(nil, func(info *framework.QueuedPodInfo, err error) bool {
		if info.Pod.Labels[LabelQuotaID] == "" {
			return false
		}
		pi := pl.podInfoCache.getPendingPodInfo(info.Pod.UID)
		if pi == nil {
			return false
		}
		extendedHandle.Scheduler().GetSchedulingQueue().MoveAllToActiveOrBackoffQueue(framework.ClusterEvent{Resource: framework.WildCard, ActionType: framework.All}, func(pod *corev1.Pod) bool {
			if pod.UID != info.Pod.UID {
				return false
			}
			if pi.processedQuotas.Len() > 0 && pi.pendingQuotas.Len() > 0 && pi.processedQuotas.Equal(pi.pendingQuotas) {
				klog.V(4).InfoS("Pod has retried multiple available Quotas, but scheduling still failed.", "pod", klog.KObj(pod), "processedQuotas", pi.processedQuotas.List())
				pi.resetTrackState()
				return false
			}
			return true
		})
		return true
	})

	return pl, nil
}

func (pl *Plugin) Name() string {
	return Name
}

func (pl *Plugin) NewControllers() ([]frameworkext.Controller, error) {
	return pl.internalPlugin.NewControllers()
}

func (pl *Plugin) EventsToRegister() []framework.ClusterEvent {
	// To register a custom event, follow the naming convention at:
	// https://git.k8s.io/kubernetes/pkg/scheduler/eventhandlers.go#L403-L410
	eqGVK := fmt.Sprintf("elasticquotas.v1alpha1.%v", scheduling.GroupName)
	return []framework.ClusterEvent{
		{Resource: framework.Pod, ActionType: framework.Delete},
		{Resource: framework.GVK(eqGVK), ActionType: framework.All},
	}
}

func (pl *Plugin) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	if pod.Labels[LabelQuotaID] == "" {
		return pl.internalPlugin.PreFilter(ctx, cycleState, pod)
	}

	pi := pl.podInfoCache.getPendingPodInfo(pod.UID)
	if pi == nil {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, "waiting sync state")
	}

	podAffinity, err := newNodeAffinity(pod)
	if err != nil {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}

	podRequests, _ := apiresource.PodRequestsAndLimits(pod)
	if quotav1.IsZero(podRequests) {
		return nil, nil
	}

	elasticQuotas, err := podAffinity.matchElasticQuotas(pl.elasticQuotaLister)
	if err != nil {
		klog.ErrorS(err, "Failed to findMatchedElasticQuota", "pod", klog.KObj(pod))
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, "No matching Quota objects")
	}
	if len(elasticQuotas) == 0 {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, "No matching Quota objects")
	}

	availableQuotas, frozenQuotas := filterAvailableQuotas(podRequests, pl.quotaCache, elasticQuotas, pi.frozenQuotas)
	if len(availableQuotas) == 0 {
		if len(frozenQuotas) == 0 {
			return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, "No available Quotas")
		}
		availableQuotas = frozenQuotas
		pi.frozenQuotas = sets.NewString()
	}

	if pi.pendingQuotas.Len() == 0 {
		for _, v := range availableQuotas {
			pi.pendingQuotas.Insert(v.name)
		}
	}

	minSufficientQuotas, minInsufficientQuotas := filterReplicasSufficientQuotas(podRequests, availableQuotas, pi.selectedQuotaName, true, 100, true)
	candidateQuota, candidateNodes := pl.selectCandidateQuotaAndNodes(pod, podRequests, minSufficientQuotas)
	if candidateQuota == nil {
		minInsufficientQuotas, _ = filterReplicasSufficientQuotas(podRequests, minInsufficientQuotas, pi.selectedQuotaName, false, 100, false)
		candidateQuota, candidateNodes = pl.selectCandidateQuotaAndNodes(pod, podRequests, minInsufficientQuotas)
	}
	if len(candidateNodes) == 0 {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, "No available nodes")
	}

	pi.selectedQuotaName = candidateQuota.quotaObj.Name

	cycleState.Write(Name, &stateData{
		quotaName:   candidateQuota.quotaObj.Name,
		podRequests: podRequests,
	})

	return &framework.PreFilterResult{NodeNames: candidateNodes}, nil
}

func (pl *Plugin) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

type stateData struct {
	skip        bool
	quotaName   string
	podRequests corev1.ResourceList
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

func (pl *Plugin) PostFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, filteredNodeStatusMap framework.NodeToStatusMap) (*framework.PostFilterResult, *framework.Status) {
	sd := getStateData(cycleState)
	if !sd.skip {
		pi := pl.podInfoCache.getPendingPodInfo(pod.UID)
		if pi != nil {
			pi.processedQuotas.Insert(pi.selectedQuotaName)
			pi.frozenQuotas.Insert(pi.selectedQuotaName)
			pi.selectedQuotaName = ""
		}
	}
	return nil, framework.NewStatus(framework.Unschedulable)
}

func (pl *Plugin) Reserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	sd := getStateData(cycleState)
	if !sd.skip {
		pi := pl.podInfoCache.getPendingPodInfo(pod.UID)
		if pi != nil {
			pl.internalPlugin.ReserveQuota(pod, pi.selectedQuotaName)
			pl.quotaCache.assumePod(pod, sd.podRequests)
		}
	}
	return nil
}

func (pl *Plugin) Unreserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) {
	sd := getStateData(cycleState)
	if !sd.skip {
		pi := pl.podInfoCache.getPendingPodInfo(pod.UID)
		if pi != nil {
			pi.processedQuotas.Insert(pi.selectedQuotaName)
			pl.internalPlugin.UnreserveQuota(pod, pi.selectedQuotaName)
			pl.quotaCache.forgetPod(pod, sd.podRequests)
		}
	}
}

func (pl *Plugin) PreBind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	sd := getStateData(cycleState)
	if !sd.skip {
		pi := pl.podInfoCache.getPendingPodInfo(pod.UID)
		if pi != nil {
			if pod.Labels == nil {
				pod.Labels = map[string]string{}
			}
			pod.Labels[apiext.LabelQuotaName] = pi.selectedQuotaName
		}
	}
	return nil
}

func (pl *Plugin) selectCandidateQuotaAndNodes(pod *corev1.Pod, podRequests corev1.ResourceList, quotas []*QuotaObject) (*QuotaObject, sets.String) {
	var (
		candidateQuota *QuotaObject
		candidateNodes sets.String
	)
	for _, quota := range quotas {
		pl.internalPlugin.TryAdd(pod, quota.name)
		_, status := pl.internalPlugin.FitQuota(podRequests, quota.name)
		if !status.IsSuccess() {
			pl.internalPlugin.Forget(pod, quota.name)
			if status.Code() == framework.Error {
				klog.ErrorS(status.AsError(), "Failed to FitQuota", "pod", klog.KObj(pod), "quota", klog.KObj(quota.quotaObj))
			} else {
				klog.V(4).InfoS("Failed to FitQuota", "pod", klog.KObj(pod), "quota", klog.KObj(quota.quotaObj), "reason", status.Message())
			}
			continue
		}

		var err error
		if candidateNodes, err = pl.filterNodeInfosByQuota(quota); err != nil {
			pl.internalPlugin.Forget(pod, quota.name)
			klog.ErrorS(err, "Failed to filterNodeInfosByQuota", "pod", klog.KObj(pod), "quota", klog.KObj(quota.quotaObj))
			continue
		}
		if len(candidateNodes) > 0 {
			klog.V(4).InfoS("Pod will schedule in nodes by quota",
				"pod", klog.KObj(pod), "quota", klog.KObj(quota.quotaObj), "nodes", len(candidateNodes), "zone", quota.quotaObj.Labels[corev1.LabelTopologyZone])
			candidateQuota = quota
			break
		}
		pl.internalPlugin.Forget(pod, quota.name)
	}
	return candidateQuota, candidateNodes
}

func (pl *Plugin) filterNodeInfosByQuota(quota *QuotaObject) (sets.String, error) {
	zone := quota.quotaObj.Labels[corev1.LabelTopologyZone]
	arch := quota.quotaObj.Labels[corev1.LabelArchStable]
	nodeInfos, err := pl.handle.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return nil, err
	}
	targetNodes := make([]*framework.NodeInfo, len(nodeInfos))
	var index int32
	pl.handle.Parallelizer().Until(context.TODO(), len(nodeInfos), func(piece int) {
		nodeInfo := nodeInfos[piece]
		node := nodeInfo.Node()
		if node.Labels[corev1.LabelTopologyZone] == zone && node.Labels[corev1.LabelArchStable] == arch {
			next := atomic.AddInt32(&index, 1)
			targetNodes[next-1] = nodeInfo
		}
	})
	if len(targetNodes) == 0 {
		return nil, nil
	}
	nodes := sets.NewString()
	for _, v := range targetNodes[:index] {
		nodes.Insert(v.Node().Name)
	}
	return nodes, nil
}
