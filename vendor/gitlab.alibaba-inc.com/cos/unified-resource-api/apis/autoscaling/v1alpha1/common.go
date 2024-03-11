package v1alpha1

import "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/common"

const (
	// labels for Recommendation CRD, patched to Recommendation during handle RecommendationProfile for workload
	// e.g. alpha.alibabacloud.com/recommendation-workload-name=cpu-load-gen
	RecommendationWorkloadNameKey = common.AlphaDomainPrefix + "recommendation-workload-name"

	// labels for Recommendation CRD, patched to Recommendation during handle RecommendationProfile for workload
	// e.g. alpha.alibabacloud.com/recommendation-workload-kind=Deployment
	RecommendationWorkloadKindKey = common.AlphaDomainPrefix + "recommendation-workload-kind"

	// labels for Recommendation CRD, patched to Recommendation during handle RecommendationProfile for workload
	// e.g. alpha.alibabacloud.com/recommendation-workload-kind=apps.kruise.io
	RecommendationWorkloadApiVersionKey = common.AlphaDomainPrefix + "recommendation-workload-apiVersion"

	// Deprecated, will be remove later
	// labels for Recommendation CRD, pathed to Recommendation during handle RecommendationProfile for pod
	// origin pod label will be replaced by dot(".") for standard reasons
	// e.g.
	// for Pod with label: flink.alibabacloud.com/job-id="counterjob"
	// corresponding Recommendation will got label:
	// alpha.alibabacloud.com/recommendation-pod-prefix.flink.alibabacloud.com.job-id="counterjob"
	// NOTICE: k8s requires the total length of label <= 255, length without domain prefix <= 63, length of value <= 63
	RecommendationPodLabelPrefixKey = common.AlphaDomainPrefix + "recommendation-pod-prefix"

	// generating recommendation for group of pod, patched by user, handled by RecommendationProfile Controller
	// e.g. alpha.alibabacloud.com/recommendation-profile-name=profile-demo
	RecommendationProfileNameKey = common.AlphaDomainPrefix + "recommendation-profile-name"
)
