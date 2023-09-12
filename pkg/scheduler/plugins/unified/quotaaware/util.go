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
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	schedlisters "sigs.k8s.io/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config/v1beta2"
)

type nodeAffinity struct {
	userID         string
	quotaID        string
	podType        string
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
	podType := pod.Labels[LabelPodType]
	if podType == "" {
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
		podType:        podType,
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
				Key:      LabelQuotaPodType,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{p.podType},
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

func filterAvailableQuotas(requests corev1.ResourceList, cache *QuotaCache, quotas []*schedv1alpha1.ElasticQuota, frozen sets.String) ([]*QuotaObject, []*QuotaObject) {
	//
	// NOTE: 关于 Frozen
	// 只有 Filter 失败后才会冻结选中的 Quota 对象；如果在 Unreserve 阶段，则不冻结 Quota 对象。
	// 原因是 Unreserve 证明之前一定是 Filter 成功，Quota 满足，资源满足，但中途 Reserve/PreBind 失败了，但这些失败
	// 可能是因为外部链路有异常，比如 webhook 拦截、APIServer 不可用、云盘挂载异常等等，
	// 因此不但不能冻结，还需要重新调度时优先使用这个 Quota 对象。
	// 另外即使在 Filter 阶段失败了，还有几种情况需要考虑：
	// 1. 选中的 Quota 对象的 min 是足够的，但还是 Filter 失败了；
	// 2. 选中的 Quota 对象的 min 是不够的，runtime 满足，但现在可能变成 min 又满足了，或者 runtime 又不够了。
	// 站在用户的角度，从成本角度考虑肯定是希望优先使用 min 满足 Quota 对象，但当有多个 min 满足时，一个 min 足够的Quota 对象
	// 却调度失败了，最好是尝试换个 min 满足的 Quota 对象；但如果只有一个 min 满足Quota时，同时还有可以使用弹性 Quota 的对象，
	// 就很纠结了，从资源交付角度看，我们应该尽可能的满足用户的Pod诉求，选择弹性 Quota 充足的对象，
	// 但用户在收到账单时，一定有个疑问：为什么 min 有剩余但没有使用呢？
	// 现在这一期先按照完全冻结的角度考虑，后面再进行优化，尽可能的使用用户的min。
	//
	var availableQuotas []*QuotaObject
	var filteredByFrozen []*QuotaObject
	for _, v := range quotas {
		quotaObj := cache.getQuota(v.Name)
		if quotaObj != nil {
			used := quotav1.Add(requests, quotaObj.used)
			if usedLessThan(used, quotaObj.max) {
				if frozen.Has(v.Name) {
					filteredByFrozen = append(filteredByFrozen, quotaObj)
					continue
				}
				availableQuotas = append(availableQuotas, quotaObj)
			}
		}
	}
	return availableQuotas, filteredByFrozen
}

func filterReplicasSufficientQuotas(requests corev1.ResourceList, quotas []*QuotaObject, preferredQuotaName string, byMin bool, minReplicas int, ascending bool) (sufficientQuotas, insufficientQuotas []*QuotaObject) {
	quotaReplicas := map[string]int{}
	var preferredQuota *QuotaObject
	for _, quotaObj := range quotas {
		upper := quotaObj.max
		if byMin {
			upper = quotaObj.min
		}
		used := quotav1.Add(quotaObj.used, requests)
		replicas := countReplicas(upper, used, minReplicas)
		if replicas > 0 {
			if preferredQuotaName != "" && quotaObj.name == preferredQuotaName {
				preferredQuota = quotaObj
			} else {
				sufficientQuotas = append(sufficientQuotas, quotaObj)
			}
		} else {
			insufficientQuotas = append(insufficientQuotas, quotaObj)
		}
		quotaReplicas[quotaObj.name] = replicas
	}
	sortQuotas(sufficientQuotas, quotaReplicas, ascending)
	if preferredQuota != nil {
		if len(sufficientQuotas) == 0 {
			sufficientQuotas = []*QuotaObject{preferredQuota}
		} else {
			sufficientQuotas = append([]*QuotaObject{preferredQuota}, sufficientQuotas...)
		}
	}
	return
}

func sortQuotas(quotaObjects []*QuotaObject, quotaReplicas map[string]int, ascending bool) {
	sort.Slice(quotaObjects, func(i, j int) bool {
		iQuotaReplicas := quotaReplicas[quotaObjects[i].name]
		jQuotaReplicas := quotaReplicas[quotaObjects[j].name]
		if iQuotaReplicas != jQuotaReplicas {
			if ascending {
				return iQuotaReplicas < jQuotaReplicas
			} else {
				return iQuotaReplicas > jQuotaReplicas
			}
		}
		iQuotaZone := quotaObjects[i].quotaObj.Labels[corev1.LabelTopologyZone]
		jQuotaZone := quotaObjects[j].quotaObj.Labels[corev1.LabelTopologyZone]
		return iQuotaZone < jQuotaZone
	})
}

func usedLessThan(used, max corev1.ResourceList) bool {
	satisfied, _ := quotav1.LessThanOrEqual(used, max)
	return satisfied
}

func countReplicas(remaining, requests corev1.ResourceList, minReplicas int) int {
	replicas := 0
	for usedLessThan(requests, remaining) {
		replicas++
		if minReplicas > 0 && replicas >= minReplicas {
			break
		}
		remaining = quotav1.SubtractWithNonNegativeResult(remaining, requests)
	}
	return replicas
}

func isParentQuota(quota *schedv1alpha1.ElasticQuota) bool {
	if val, exist := quota.Labels[apiext.LabelQuotaIsParent]; exist {
		isParent, err := strconv.ParseBool(val)
		if err == nil {
			return isParent
		}
	}

	return false
}

func getQuotaRuntime(quota *schedv1alpha1.ElasticQuota) (corev1.ResourceList, error) {
	runtimeAnnotation, ok := quota.Annotations[apiext.AnnotationRuntime]
	if ok {
		resourceList := make(corev1.ResourceList)
		err := json.Unmarshal([]byte(runtimeAnnotation), &resourceList)
		if err != nil {
			return nil, err
		}
		return resourceList, nil
	}
	return nil, nil
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
