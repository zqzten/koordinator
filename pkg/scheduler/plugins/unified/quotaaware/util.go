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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	schedlisters "sigs.k8s.io/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config/v1beta2"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota"
	elasticquotacore "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota/core"
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
	Name string
	Used corev1.ResourceList
	Min  corev1.ResourceList
	Max  corev1.ResourceList
	Obj  *schedv1alpha1.ElasticQuota
}

func filterGuaranteeAvailableQuotas(requests corev1.ResourceList, cache *elasticquota.Plugin, quotas []*schedv1alpha1.ElasticQuota) []*QuotaWrapper {
	var availableQuotas []*QuotaWrapper
	for _, v := range quotas {
		qm := cache.GetGroupQuotaManagerForQuota(v.Name)
		if qm == nil {
			continue
		}
		quotaInfo := qm.GetQuotaInfoByName(v.Name)
		if quotaInfo != nil {
			if !checkGuarantee(qm, quotaInfo, requests) {
				klog.V(4).InfoS("Quota is unavailable", "quota", quotaInfo.Name)
				continue
			}

			availableQuotas = append(availableQuotas, &QuotaWrapper{
				Name: quotaInfo.Name,
				Used: quotaInfo.GetUsed(),
				Min:  quotaInfo.GetMin(),
				Max:  quotaInfo.GetMax(),
				Obj:  v,
			})
		}
	}
	return availableQuotas
}

func checkGuarantee(qm *elasticquotacore.GroupQuotaManager, quotaInfo *elasticquotacore.QuotaInfo, requests corev1.ResourceList) bool {
	if quotaInfo.Name == apiext.RootQuotaName {
		return true
	}

	if quotaInfo.IsParent && (quotaInfo.ParentName == "" || quotaInfo.ParentName == apiext.RootQuotaName) {
		totalResource := qm.GetClusterTotalResource()
		used := quotav1.Add(requests, quotaInfo.GetAllocated())
		return usedLessThan(used, totalResource)
	} else {
		allocated := quotaInfo.GetAllocated()
		used := quotav1.Add(requests, allocated)
		if !usedLessThan(used, quotaInfo.GetMax()) {
			return false
		}
		if usedLessThan(used, quotaInfo.GetGuaranteed()) {
			return true
		}
		requests = quotav1.SubtractWithNonNegativeResult(used, allocated)
	}

	parent := qm.GetQuotaInfoByName(quotaInfo.ParentName)
	if parent == nil {
		return false
	}
	return checkGuarantee(qm, parent, requests)
}

func usedLessThan(used, max corev1.ResourceList) bool {
	if !quotav1.IsZero(used) && quotav1.IsZero(max) {
		return false
	}
	satisfied, _ := quotav1.LessThanOrEqual(used, max)
	return satisfied
}

func countReplicas(remaining, used, requests corev1.ResourceList, maxReplicas int) int {
	replicas := 0
	used = quotav1.Add(used, requests)
	for usedLessThan(used, remaining) {
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
