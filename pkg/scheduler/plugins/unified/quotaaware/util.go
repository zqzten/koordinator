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
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	schedlisters "sigs.k8s.io/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config/v1beta2"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota"
	elasticquotacore "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota/core"
	nodeaffinityhelper "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/nodeaffinity"
)

type nodeAffinity struct {
	userID         string
	quotaID        string
	instanceType   string
	affinityZones  sets.String
	affinityArches sets.String
}

func newNodeAffinity(pod *corev1.Pod) (*nodeAffinity, error) {
	userID := pod.Labels[LabelUserAccountId]
	if userID == "" {
		return nil, fmt.Errorf("missing user account id")
	}
	quotaID := pod.Labels[LabelQuotaID]
	if quotaID == "" {
		return nil, fmt.Errorf("missing quota id")
	}
	instanceType := pod.Labels[LabelInstanceType]
	if instanceType == "" {
		return nil, fmt.Errorf("missing pod type")
	}

	affinityZones := sets.NewString()
	affinityArches := sets.NewString()
	parseNodeAffinity(pod, func(key string, operator corev1.NodeSelectorOperator, values []string) bool {
		if (key == corev1.LabelTopologyZone || key == corev1.LabelZoneFailureDomain) &&
			operator == corev1.NodeSelectorOpIn {
			affinityZones.Insert(values...)
		}

		if key == corev1.LabelArchStable && operator == corev1.NodeSelectorOpIn {
			affinityArches.Insert(values...)
		}
		return true
	})
	if affinityArches.Len() == 0 {
		affinityArches.Insert("amd64")
	}
	return &nodeAffinity{
		userID:         userID,
		quotaID:        quotaID,
		instanceType:   instanceType,
		affinityZones:  affinityZones,
		affinityArches: affinityArches,
	}, nil
}

func (p *nodeAffinity) matchElasticQuotas(elasticQuotaLister schedlisters.ElasticQuotaLister) ([]*schedv1alpha1.ElasticQuota, error) {
	labelSelector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      LabelUserAccountId,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{p.userID},
			},
			{
				Key:      LabelQuotaID,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{p.quotaID},
			},
			{
				Key:      LabelInstanceType,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{p.instanceType},
			},
			{
				Key:      corev1.LabelArchStable,
				Operator: metav1.LabelSelectorOpIn,
				Values:   p.affinityArches.UnsortedList(),
			},
			{
				Key:      apiext.LabelQuotaIsParent,
				Operator: metav1.LabelSelectorOpNotIn,
				Values:   []string{"true"},
			},
		},
	}
	if p.affinityZones.Len() > 0 {
		labelSelector.MatchExpressions = append(labelSelector.MatchExpressions, metav1.LabelSelectorRequirement{
			Key:      corev1.LabelTopologyZone,
			Operator: metav1.LabelSelectorOpIn,
			Values:   p.affinityZones.UnsortedList(),
		})
	}
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return nil, err
	}
	return elasticQuotaLister.List(selector)
}

func parseNodeAffinity(pod *corev1.Pod, fn func(key string, operator corev1.NodeSelectorOperator, values []string) bool) {
	for k, v := range pod.Spec.NodeSelector {
		if !fn(k, corev1.NodeSelectorOpIn, []string{v}) {
			break
		}
	}
	if pod.Spec.Affinity != nil &&
		pod.Spec.Affinity.NodeAffinity != nil &&
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		for _, term := range pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			for _, expr := range term.MatchExpressions {
				if !fn(expr.Key, expr.Operator, expr.Values) {
					break
				}
			}
		}
	}
}

type QuotaWrapper struct {
	Name       string
	Used       corev1.ResourceList
	Min        corev1.ResourceList
	Max        corev1.ResourceList
	Guaranteed corev1.ResourceList
	Allocated  corev1.ResourceList
	Obj        *schedv1alpha1.ElasticQuota
}

func filterGuaranteeAvailableQuotas(pod *corev1.Pod, requests corev1.ResourceList, cache *elasticquota.Plugin, quotas []*schedv1alpha1.ElasticQuota) ([]*QuotaWrapper, *framework.Status) {
	if len(quotas) == 0 {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, "No available quotas")
	}
	var availableQuotas []*QuotaWrapper
	status := framework.NewStatus(framework.Unschedulable)
	requestsNames := quotav1.ResourceNames(requests)
	for _, v := range quotas {
		qm := cache.GetGroupQuotaManagerForQuota(v.Name)
		if qm == nil {
			continue
		}
		quotaInfo := qm.GetQuotaInfoByName(v.Name)
		if quotaInfo != nil {
			enough, guaranteed, allocated, sts := checkGuarantee(qm, quotaInfo, requests, requestsNames)
			if !enough {
				if !sts.IsSuccess() {
					for _, reason := range sts.Reasons() {
						status.AppendReason(reason)
					}
				}
				klog.V(4).InfoS("Failed to checkGuarantee", "pod", klog.KObj(pod), "quota", quotaInfo.Name, "requests", sprintResourceList(requests, nil), "reasons", sts.Message())
				continue
			}

			availableQuotas = append(availableQuotas, &QuotaWrapper{
				Name:       quotaInfo.Name,
				Used:       quotaInfo.GetUsed(),
				Min:        quotaInfo.GetMin(),
				Max:        quotaInfo.GetMax(),
				Guaranteed: guaranteed,
				Allocated:  allocated,
				Obj:        v,
			})
		}
	}
	if len(availableQuotas) > 0 {
		return availableQuotas, nil
	}
	return nil, status
}

func checkGuarantee(qm *elasticquotacore.GroupQuotaManager, quotaInfo *elasticquotacore.QuotaInfo, requests corev1.ResourceList, requestsNames []corev1.ResourceName) (bool, corev1.ResourceList, corev1.ResourceList, *framework.Status) {
	if quotaInfo.Name == apiext.RootQuotaName {
		return true, nil, nil, nil
	}

	if quotaInfo.IsParent && (quotaInfo.ParentName == "" || quotaInfo.ParentName == apiext.RootQuotaName) {
		totalResource := quotav1.Mask(qm.GetClusterTotalResource(), quotav1.ResourceNames(quotaInfo.GetMin()))
		allocated := quotaInfo.GetAllocated()
		used := quotav1.Add(requests, allocated)
		used = quotav1.Mask(used, requestsNames)
		enough, exceedDimensions := usedLessThanOrEqual(used, totalResource)
		if !enough {
			status := framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Insufficient inventory %q, %s", quotaInfo.Name, generateQuotaExceedMaxMessage(totalResource, allocated, exceedDimensions)))
			return false, nil, nil, status
		}
		return true, nil, nil, nil
	} else {
		allocated := quotaInfo.GetAllocated()
		max := quotaInfo.GetMax()
		used := quotav1.Add(requests, allocated)
		used = quotav1.Mask(used, requestsNames)
		if enough, exceedDimensions := usedLessThanOrEqual(used, max); !enough {
			status := framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Insufficient Quotas %q, %s", quotaInfo.Name, generateQuotaExceedMaxMessage(max, allocated, exceedDimensions)))
			return false, nil, nil, status
		}
		guaranteed := quotaInfo.GetGuaranteed()
		if enough, _ := usedLessThanOrEqual(used, guaranteed); enough {
			return true, guaranteed, allocated, nil
		}
		requests = quotav1.SubtractWithNonNegativeResult(used, allocated)
	}

	parent := qm.GetQuotaInfoByName(quotaInfo.ParentName)
	if parent == nil {
		return false, nil, nil, framework.NewStatus(framework.Unschedulable, "missing parent quota")
	}
	return checkGuarantee(qm, parent, requests, requestsNames)
}

func checkMin(qm *elasticquotacore.GroupQuotaManager, quotaInfo *elasticquotacore.QuotaInfo, requests corev1.ResourceList, requestsNames []corev1.ResourceName) bool {
	if quotaInfo.Name == apiext.RootQuotaName {
		return true
	}

	if quotaInfo.IsParent && (quotaInfo.ParentName == "" || quotaInfo.ParentName == apiext.RootQuotaName) {
		// Here skip checking inventory
		return false
	}

	allocated := quotaInfo.GetAllocated()
	used := quotav1.Add(requests, allocated)
	used = quotav1.Mask(used, requestsNames)
	guaranteed := quotaInfo.GetGuaranteed()
	if enough, _ := usedLessThanOrEqual(used, guaranteed); enough {
		return true
	}

	parent := qm.GetQuotaInfoByName(quotaInfo.ParentName)
	if parent == nil {
		return false
	}
	requests = quotav1.SubtractWithNonNegativeResult(used, allocated)
	return checkMin(qm, parent, requests, requestsNames)
}

func generateQuotaExceedMaxMessage(capacity corev1.ResourceList, allocated corev1.ResourceList, exceedDimensions []corev1.ResourceName) string {
	var sb strings.Builder
	sort.Slice(exceedDimensions, func(i, j int) bool {
		return exceedDimensions[i] < exceedDimensions[j]
	})
	for i, resourceName := range exceedDimensions {
		if i != 0 {
			_, _ = fmt.Fprintf(&sb, "; ")
		}
		c := capacity[resourceName]
		a := allocated[resourceName]
		_, _ = fmt.Fprintf(&sb, "%s capacity %s, allocated: %s", resourceName, c.String(), a.String())
	}
	return sb.String()
}

func usedLessThanOrEqual(used, max corev1.ResourceList) (bool, []corev1.ResourceName) {
	if !quotav1.IsZero(used) && quotav1.IsZero(max) {
		return false, nil
	}
	satisfied, exceedDimensions := quotav1.LessThanOrEqual(used, max)
	return satisfied, exceedDimensions
}

func countReplicas(remaining, used, requests corev1.ResourceList, maxReplicas int) int {
	replicas := 0
	used = quotav1.Add(used, requests)
	for {
		satisfied, _ := usedLessThanOrEqual(used, remaining)
		if !satisfied {
			break
		}
		replicas++
		if maxReplicas > 0 && replicas >= maxReplicas {
			break
		}
		used = quotav1.Add(used, requests)
	}
	return replicas
}

func getElasticQuotaArgs(obj runtime.Object) (*schedulingconfig.ElasticQuotaArgs, error) {
	if obj == nil {
		return getDefaultElasticQuotaArgs()
	}

	unknownObj, ok := obj.(*runtime.Unknown)
	if !ok {
		return nil, fmt.Errorf("got args of type %T, want *DeviceShareArgs", obj)
	}
	var v1beta2args v1beta2.ElasticQuotaArgs
	v1beta2.SetDefaults_ElasticQuotaArgs(&v1beta2args)
	if err := frameworkruntime.DecodeInto(unknownObj, &v1beta2args); err != nil {
		return nil, err
	}
	var args schedulingconfig.ElasticQuotaArgs
	err := v1beta2.Convert_v1beta2_ElasticQuotaArgs_To_config_ElasticQuotaArgs(&v1beta2args, &args, nil)
	if err != nil {
		return nil, err
	}
	return &args, nil
}

func getDefaultElasticQuotaArgs() (*schedulingconfig.ElasticQuotaArgs, error) {
	var v1beta2args v1beta2.ElasticQuotaArgs
	v1beta2.SetDefaults_ElasticQuotaArgs(&v1beta2args)
	var elasticQuotaArgs schedulingconfig.ElasticQuotaArgs
	err := v1beta2.Convert_v1beta2_ElasticQuotaArgs_To_config_ElasticQuotaArgs(&v1beta2args, &elasticQuotaArgs, nil)
	if err != nil {
		return nil, err
	}
	return &elasticQuotaArgs, nil
}

func addTemporaryNodeAffinity(cycleState *framework.CycleState, elasticQuotas []*schedv1alpha1.ElasticQuota) {
	zones := sets.NewString()
	for _, eq := range elasticQuotas {
		if zone := eq.Labels[corev1.LabelTopologyZone]; zone != "" {
			zones.Insert(zone)
		}
	}
	if zones.Len() > 0 {
		nodeaffinityhelper.SetTemporaryNodeAffinity(cycleState, &nodeaffinityhelper.TemporaryNodeAffinity{
			NodeSelector: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      corev1.LabelTopologyZone,
								Operator: corev1.NodeSelectorOpIn,
								Values:   zones.List(),
							},
						},
					},
				},
			},
		})
	}
}

func sprintResourceList(resourceList corev1.ResourceList, resourceNames []corev1.ResourceName) string {
	if len(resourceNames) > 0 {
		resourceList = quotav1.Mask(resourceList, resourceNames)
	}
	res := make([]string, 0)
	for k, v := range resourceList {
		tmp := string(k) + ":" + v.String()
		res = append(res, tmp)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i] < res[j]
	})
	return strings.Join(res, ",")
}
