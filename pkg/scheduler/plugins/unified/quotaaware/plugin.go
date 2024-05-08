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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/helper"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/interpodaffinity"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeaffinity"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedlisters "github.com/koordinator-sh/koordinator/apis/thirdparty/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota"
	elasticquotacore "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota/core"
)

const (
	Name = "QuotaAware"

	ErrNoMatchingQuotaObjects = "No matching Quota objects"
)

var (
	_ framework.EnqueueExtensions = &Plugin{}
	_ framework.PreFilterPlugin   = &Plugin{}
	_ framework.FilterPlugin      = &Plugin{}
	_ framework.PreScorePlugin    = &Plugin{}
	_ framework.ScorePlugin       = &Plugin{}
	_ framework.ReservePlugin     = &Plugin{}
	_ framework.PreBindPlugin     = &Plugin{}
)

type Plugin struct {
	*elasticquota.Plugin
	handle             framework.Handle
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
	elasticQuotaLister := schedlisters.NewElasticQuotaLister(elasticQuotaInformer.GetIndexer())

	pl := &Plugin{
		Plugin:             elasticQuotaPlugin,
		handle:             handle,
		elasticQuotaLister: elasticQuotaLister,
	}

	extendedHandle := handle.(frameworkext.ExtendedHandle)
	extendedHandle.RegisterErrorHandlerFilters(pl.preErrorHandlerFilter, nil)

	return pl, nil
}

func (pl *Plugin) Name() string {
	return Name
}

func (pl *Plugin) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	if pod.Labels[LabelQuotaID] == "" {
		return pl.Plugin.PreFilter(ctx, cycleState, pod)
	}

	quotaAffinity, err := newQuotaAffinity(pod)
	if err != nil {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}

	podRequests := elasticquotacore.PodRequests(pod)
	if quotav1.IsZero(podRequests) {
		return nil, nil
	}

	elasticQuotas, err := quotaAffinity.matchElasticQuotas(pl.elasticQuotaLister, true)
	if err != nil {
		klog.ErrorS(err, "Failed to findMatchedElasticQuota", "pod", klog.KObj(pod))
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, ErrNoMatchingQuotaObjects)
	}
	if len(elasticQuotas) == 0 {
		message := generateAffinityConflictMessage(quotaAffinity, pl.elasticQuotaLister)
		if message != "" {
			return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, message)
		}
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, ErrNoMatchingQuotaObjects)
	}

	availableQuotas, status := filterGuaranteeAvailableQuotas(pod, podRequests, pl.Plugin, elasticQuotas)
	if !status.IsSuccess() {
		return nil, status
	}

	cycleState.Write(Name, &stateData{
		podRequests:     podRequests,
		availableQuotas: availableQuotas,
	})

	if quotaAffinity.affinityZones.Len() == 0 {
		addTemporaryNodeAffinity(cycleState, elasticQuotas)
	}

	return nil, nil
}

func (pl *Plugin) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

type stateData struct {
	skip              bool
	podRequests       corev1.ResourceList
	availableQuotas   []*QuotaWrapper
	maxReplicas       int
	replicasWithMin   map[string]int
	replicasWithMax   map[string]int
	selectedQuotaName string
}

func (s *stateData) Clone() framework.StateData {
	return s
}

func (s *stateData) getQuotaByNode(node *corev1.Node) *QuotaWrapper {
	for _, quota := range s.availableQuotas {
		if quota.Obj.Labels[corev1.LabelTopologyZone] == node.Labels[corev1.LabelTopologyZone] &&
			quota.Obj.Labels[corev1.LabelArchStable] == node.Labels[corev1.LabelArchStable] {
			return quota
		}
	}
	return nil
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

func (pl *Plugin) Filter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	sd := getStateData(cycleState)
	if sd.skip {
		return nil
	}
	node := nodeInfo.Node()
	quota := sd.getQuotaByNode(node)
	if quota == nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "node(s) no corresponding available Quota")
	}
	return nil
}

func (pl *Plugin) PreScore(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodes []*corev1.Node) *framework.Status {
	sd := getStateData(cycleState)
	if sd.skip {
		return nil
	}

	sd.maxReplicas = 10
	sd.replicasWithMin = map[string]int{}
	sd.replicasWithMax = map[string]int{}
	resourceNames := quotav1.ResourceNames(sd.podRequests)
	for _, quota := range sd.availableQuotas {
		min := quotav1.Max(quota.Min, quota.Guaranteed)
		used := quota.Used
		if quota.Allocated != nil {
			used = quota.Allocated
		}
		used = quotav1.Mask(used, resourceNames)
		replicas := countReplicas(min, used, sd.podRequests, sd.maxReplicas)
		if replicas > 0 {
			sd.replicasWithMin[quota.Name] = replicas
		} else {
			replicas := countReplicas(quota.Max, quota.Used, sd.podRequests, sd.maxReplicas)
			sd.replicasWithMax[quota.Name] = replicas
		}
	}
	return nil
}

func (pl *Plugin) Score(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	sd := getStateData(cycleState)
	if sd.skip {
		return 0, nil
	}

	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()

	quota := sd.getQuotaByNode(node)
	if quota == nil {
		return 0, nil
	}

	baseScore := int64(sd.maxReplicas) * framework.MaxNodeScore
	replicas, ok := sd.replicasWithMin[quota.Name]
	if ok {
		if replicas >= sd.maxReplicas {
			return baseScore, nil
		}
		return int64(sd.maxReplicas-replicas) * baseScore, nil
	}

	replicas, ok = sd.replicasWithMax[quota.Name]
	if ok {
		return int64(replicas) * framework.MaxNodeScore / 2, nil
	}
	return 0, nil
}

func (pl *Plugin) ScoreExtensions() framework.ScoreExtensions {
	return pl
}

func (pl *Plugin) NormalizeScore(ctx context.Context, cycleState *framework.CycleState, p *corev1.Pod, scores framework.NodeScoreList) *framework.Status {
	return helper.DefaultNormalizeScore(framework.MaxNodeScore, false, scores)
}

func (pl *Plugin) Reserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	sd := getStateData(cycleState)
	if sd.skip {
		return pl.Plugin.Reserve(ctx, cycleState, pod, nodeName)
	}

	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return framework.AsStatus(err)
	}
	node := nodeInfo.Node()

	quota := sd.getQuotaByNode(node)
	if quota == nil {
		return framework.NewStatus(framework.Error, "No corresponding available Quota to reserve")
	}
	sd.selectedQuotaName = quota.Name
	mgr := pl.Plugin.GetGroupQuotaManagerForQuota(sd.selectedQuotaName)
	if mgr != nil {
		mgr.OnPodAdd(sd.selectedQuotaName, pod)
	}
	return nil
}

func (pl *Plugin) Unreserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) {
	sd := getStateData(cycleState)
	if sd.skip {
		pl.Plugin.Unreserve(ctx, cycleState, pod, nodeName)
		return
	}
	mgr := pl.Plugin.GetGroupQuotaManagerForQuota(sd.selectedQuotaName)
	if mgr != nil {
		mgr.OnPodDelete(sd.selectedQuotaName, pod)
	}
	return
}

func (pl *Plugin) PreBind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	sd := getStateData(cycleState)
	if !sd.skip {
		if pod.Labels == nil {
			pod.Labels = map[string]string{}
		}
		pod.Labels[apiext.LabelQuotaName] = sd.selectedQuotaName
	}
	return nil
}

func (pl *Plugin) preErrorHandlerFilter(ctx context.Context, fwk framework.Framework, podInfo *framework.QueuedPodInfo, status *framework.Status, nominatingInfo *framework.NominatingInfo, start time.Time) bool {
	pod := podInfo.Pod
	if pod.Labels[LabelQuotaID] == "" {
		return false
	}

	scheduleErr := status.AsError()
	fitErr, ok := scheduleErr.(*framework.FitError)
	if !ok {
		// TODO:后续需要等 Coscheduling 的PreFilter error 纠正后这里要加上一个判断，专门处理 framework.Error 的情况。
		return false
	}

	quotaAffinity, err := newQuotaAffinity(pod)
	if err != nil {
		klog.Warningf("Unexpected scheduling error, pod: %q, err: %v", klog.KObj(pod), "failed to parse pod")
		UnexpectedSchedulingError.WithLabelValues(fwk.ProfileName(), "", "", "", "InvalidQuotaMeta").Inc()
		return false
	}

	recordSchedulingErrorFn := func(reason string) {
		UnexpectedSchedulingError.WithLabelValues(fwk.ProfileName(), quotaAffinity.userID, quotaAffinity.quotaID, quotaAffinity.instanceType, reason).Inc()
	}

	//  先确认是否是特殊场景：用户创建的 Pod 却没有匹配的 ElasticQuota，只有链路上有问题才会到这种情况出现。
	for _, status := range fitErr.Diagnosis.NodeToStatusMap {
		if status.IsUnschedulable() && status.Message() == ErrNoMatchingQuotaObjects {
			klog.Warningf("Unexpected scheduling error, pod: %q, err: %v", klog.KObj(pod), ErrNoMatchingQuotaObjects)
			recordSchedulingErrorFn("NoMatchingQuota")
			return false
		}
		break
	}

	elasticQuotas, err := quotaAffinity.matchElasticQuotas(pl.elasticQuotaLister, true)
	if err != nil {
		klog.ErrorS(err, "Failed to findMatchedElasticQuota", "pod", klog.KObj(pod))
		return false
	}

	podRequests := elasticquotacore.PodRequests(pod)
	requestsNames := quotav1.ResourceNames(podRequests)
	reservedZones := sets.NewString()
	for _, v := range elasticQuotas {
		if qm := pl.Plugin.GetGroupQuotaManagerForQuota(v.Name); qm != nil {
			if quotaInfo := qm.GetQuotaInfoByName(v.Name); quotaInfo != nil {
				if enough := checkMin(qm, quotaInfo, podRequests, requestsNames); enough {
					reservedZones.Insert(v.Labels[corev1.LabelTopologyZone])
				}
			}
		}
	}

	// 没有预留资源的 ElasticQuota，不再需要识别是否是非预期的调度失败行为
	if reservedZones.Len() == 0 {
		return false
	}

	abnormalZones := sets.NewString()
	for nodeName, status := range fitErr.Diagnosis.NodeToStatusMap {
		if !status.IsUnschedulable() {
			continue
		}
		message := status.Message()
		if message == nodeaffinity.ErrReasonPod ||
			message == interpodaffinity.ErrReasonExistingAntiAffinityRulesNotMatch ||
			message == interpodaffinity.ErrReasonAffinityRulesNotMatch ||
			message == interpodaffinity.ErrReasonAntiAffinityRulesNotMatch {
			// 这类错误都是节点不符合 NodeAffinity 规则的 或者 不符合 InterPodAffinity 规则的，这类直接 skip 掉
			// 目前 InterPodAffinity 也不支持按照 Node 粒度亲和或者反亲和，都统一处理了，后续支持了，还得细化。
			continue
		}

		// 到了这里都是可能因为资源不足、节点不可用/不可调度、打散等原因导致的
		nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
		if err != nil {
			continue
		}
		node := nodeInfo.Node()
		az := node.Labels[corev1.LabelTopologyZone]
		abnormalZones.Insert(az)
	}

	zones := abnormalZones.Intersection(reservedZones)
	if zones.Len() > 0 {
		klog.Warningf("Unexpected scheduling error, pod: %q, err: %v", klog.KObj(podInfo.Pod), "reserved resources but scheduling failed")
		UnexpectedSchedulingError.WithLabelValues(fwk.ProfileName(), quotaAffinity.userID, quotaAffinity.quotaID, quotaAffinity.instanceType, "FailedWithReserveResource").Inc()
	}
	return false
}
