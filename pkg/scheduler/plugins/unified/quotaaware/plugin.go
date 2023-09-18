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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/helper"
	schedlisters "sigs.k8s.io/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota"
	elasticquotacore "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota/core"
)

const (
	Name = "QuotaAware"
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
	return pl, nil
}

func (pl *Plugin) Name() string {
	return Name
}

func (pl *Plugin) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	if pod.Labels[LabelQuotaID] == "" {
		return pl.Plugin.PreFilter(ctx, cycleState, pod)
	}

	podNodeAffinity, err := newNodeAffinity(pod)
	if err != nil {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}

	podRequests, _ := elasticquotacore.PodRequestsAndLimits(pod)
	if quotav1.IsZero(podRequests) {
		return nil, nil
	}

	elasticQuotas, err := podNodeAffinity.matchElasticQuotas(pl.elasticQuotaLister)
	if err != nil {
		klog.ErrorS(err, "Failed to findMatchedElasticQuota", "pod", klog.KObj(pod))
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, "No matching Quota objects")
	}
	if len(elasticQuotas) == 0 {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, "No matching Quota objects")
	}

	availableQuotas := filterGuaranteeAvailableQuotas(podRequests, pl.Plugin, elasticQuotas)
	if len(availableQuotas) == 0 {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, "No available Quotas")
	}

	cycleState.Write(Name, &stateData{
		podRequests:     podRequests,
		availableQuotas: availableQuotas,
	})

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
	for _, quota := range sd.availableQuotas {
		replicas := countReplicas(quota.Min, quota.Used, sd.podRequests, sd.maxReplicas)
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
