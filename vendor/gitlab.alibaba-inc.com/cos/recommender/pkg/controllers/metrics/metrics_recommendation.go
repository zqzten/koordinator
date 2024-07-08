package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	conf "gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/config"
	recv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
)

const (
	NamespaceKey            = "namespace"
	RecommendationNameKey   = "recommendation_name"
	WorkloadKindKey         = "workload_kind"
	WorkloadNameKey         = "workload_name"
	WorkloadAPIVersionKey   = "workload_api_version"
	ContainerNameKey        = "container_name"
	ResourceTypeKey         = "resource"
	SelectorLableKey        = "workloadRefLabel"
	AssociateWorklodRefType = "workloadRef"
	AssociateSelectorType   = "selector"
)

var (
	recWorloadTarget = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "recommendation_workload_target",
			Help:      "Recommendation of workload resource request.",
		}, []string{
			NamespaceKey, RecommendationNameKey, WorkloadKindKey, WorkloadNameKey, WorkloadAPIVersionKey,
			ContainerNameKey, ResourceTypeKey,
		},
	)

	recWorkloadTargetMetricEnable = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "recommendation_target_metric_enable",
			Help:      "Recommendation metric is enabled.",
		},
	)

	recPodTarget = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "recommendation_pod_target",
			Help:      "Recommendation of pod resource request",
		}, []string{
			NamespaceKey, RecommendationNameKey, SelectorLableKey, ResourceTypeKey, ContainerNameKey,
		},
	)
)

func RegisterRecDetail() {
	prometheus.MustRegister(recWorloadTarget, recWorkloadTargetMetricEnable, recPodTarget)
}

func genRecommendationLabelForWorkload(recommendation *recv1alpha1.Recommendation) prometheus.Labels {
	return prometheus.Labels{
		NamespaceKey:          recommendation.Namespace,
		RecommendationNameKey: recommendation.Name,
		WorkloadKindKey:       recommendation.Spec.WorkloadRef.Kind,
		WorkloadNameKey:       recommendation.Spec.WorkloadRef.Name,
		WorkloadAPIVersionKey: recommendation.Spec.WorkloadRef.APIVersion,
	}
}

func genRecommendationLabelForPod(recommendation *recv1alpha1.Recommendation) prometheus.Labels {
	set := labels.Set{}
	for k, v := range recommendation.Labels {
		newKey := strings.TrimPrefix(k, recv1alpha1.RecommendationPodLabelPrefixKey+".")
		if newKey != k {
			set[newKey] = v
		}
	}
	selector := set.String()
	return prometheus.Labels{
		NamespaceKey:          recommendation.Namespace,
		RecommendationNameKey: recommendation.Name,
		SelectorLableKey:      selector,
	}
}

func RecordRecommendationMetricEnabled() {
	metricConfig := conf.GetMetricConfig()
	if metricConfig.EnableRecommendationTargetMetric {
		recWorkloadTargetMetricEnable.Set(1.0)
	} else {
		recWorkloadTargetMetricEnable.Set(0.0)
	}
}

func ResetRecordRecommendation() {
	recWorloadTarget.Reset()
	recPodTarget.Reset()
}

func RecordRecommendationTarget(oldRec *recv1alpha1.Recommendation, newStatus *recv1alpha1.RecommendationStatus) {
	metricConfig := conf.GetMetricConfig()
	if !metricConfig.EnableRecommendationTargetMetric {
		return
	}

	for i := range newStatus.Recommendation.ContainerRecommendations {
		containerRec := &newStatus.Recommendation.ContainerRecommendations[i]
		if oldRec.Spec.WorkloadRef != nil {
			recommendationKeys := genRecommendationLabelForWorkload(oldRec)
			recordContainerRecommendation(recommendationKeys, containerRec, AssociateWorklodRefType)
		} else if oldRec.Spec.Selector != nil {
			recommendationKeys := genRecommendationLabelForPod(oldRec)
			recordContainerRecommendation(recommendationKeys, containerRec, AssociateSelectorType)
		}
	}
}

func recordContainerRecommendation(labels prometheus.Labels, containerRec *recv1alpha1.RecommendedContainerResources, kind string) {
	labels[ContainerNameKey] = containerRec.ContainerName
	for resourceName, resourceVal := range containerRec.Target {
		labels[ResourceTypeKey] = resourceName.String()
		var val float64
		switch resourceName {
		case corev1.ResourceCPU:
			val = float64(resourceVal.MilliValue()) / 1000
		default:
			val = float64(resourceVal.Value())
		}
		if kind == AssociateWorklodRefType {
			recWorloadTarget.With(labels).Set(val)
		} else if kind == AssociateSelectorType {
			recPodTarget.With(labels).Set(val)
		}
	}
}
