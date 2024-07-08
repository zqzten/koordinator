package common

import (
	"encoding/json"
	"math"
	"strconv"
	"time"

	recv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	unicommon "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// https://yuque.antfin-inc.com/docs/share/b3a6bfb3-a8ff-4421-a0eb-530ef3761623

	// unnecessary for Recommender, only filled by ack console
	LabelKeyRecommenderManagedBy = unicommon.AlphaDomainPrefix + "recommendation-profile-managed-by"

	// unnecessary for Recommender, only filled by ack console
	LabelKeyRecommenderProfileAlias = unicommon.AlphaDomainPrefix + "recommendation-profile-alias"

	LabelKeyRecommenderCPUOperation = unicommon.AlphaDomainPrefix + "recommendation-cpu-operation"

	LabelKeyRecommenderMemoryOperation = unicommon.AlphaDomainPrefix + "recommendation-memory-operation"

	AnnotationKeyRecommenderSaftyMargin = unicommon.AlphaDomainPrefix + "recommendation-safety-margin-ratio"

	AnnotationRecommenderWorkloadNamespace = unicommon.AlphaDomainPrefix + "recommendation-workload-namespace"

	AnnotationRecommenderWorkloadStatus = unicommon.AlphaDomainPrefix + "recommendation-workload-status"

	AnnotationRecommenderWarmUpHours = unicommon.AlphaDomainPrefix + "recommendation-warm-up-hours"
)

const (
	LabelValRecommenderOperationIncrease = "Increase"
	LabelValRecommenderOperationDecrease = "Decrease"
	LabelValRecommenderOperationMaintain = "Maintain"
)

type RecommendationStatus string

const (
	RecommendationStatusCollecting   RecommendationStatus = "Collecting"
	RecommendationStatusWorking      RecommendationStatus = "Working"
	RecommendationStatusRecorrecting RecommendationStatus = "Recorrecting"
	RecommendationStatusNotExist     RecommendationStatus = "WorkloadNotExist"
	RecommendationStatusError        RecommendationStatus = "Error"

	DefaultRecommendationWarmUp = time.Hour * 24
	MinRecommendationWarmUp     = time.Hour
)

type RecommenderWorkloadStatus struct {
	Status             RecommendationStatus `json:"status,omitempty"`
	OriginRequest      corev1.ResourceList  `json:"originRequest,omitempty"`
	CPUAdjustDegree    *float64             `json:"cpuAdjustDegree,omitempty"`
	MemoryAdjustDegree *float64             `json:"memoryAdjustDegree,omitempty"`
}

func GetRecommenderWorkloadStatus(annotations map[string]string) (*RecommenderWorkloadStatus, error) {
	value, exist := annotations[AnnotationRecommenderWorkloadStatus]
	if !exist {
		return nil, nil
	}
	workloadStatus := RecommenderWorkloadStatus{}
	err := json.Unmarshal([]byte(value), &workloadStatus)
	if err != nil {
		return nil, err
	}
	return &workloadStatus, nil
}

func GetRecommederSafetyMarginRatio(annotations map[string]string) (float64, error) {
	value, exist := annotations[AnnotationKeyRecommenderSaftyMargin]
	if !exist {
		return 0, nil
	}
	ratio, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	return ratio, nil
}

func GenerateRecommenderOperation(adjustDegree float64) string {
	if adjustDegree > 0 {
		return LabelValRecommenderOperationIncrease
	} else if adjustDegree < 0 {
		return LabelValRecommenderOperationDecrease
	} else {
		return LabelValRecommenderOperationMaintain
	}
}

func GenerateRecommendationStatus(rec *recv1alpha1.Recommendation) RecommendationStatus {
	now := time.Now()
	warmUpDuration := getWarmUpDuration(rec.Annotations)
	if now.Sub(rec.CreationTimestamp.Time) < warmUpDuration {
		return RecommendationStatusCollecting
	}
	return RecommendationStatusWorking
}

func CalcWorkloadAdjustDegree(containerReqs map[string]corev1.ResourceList,
	podRecommends *recv1alpha1.RecommendedPodResources, marginRatio float64) (cpuDegree float64, memoryDegree float64) {
	cpuDegree, memoryDegree = 0, 0

	containerRecs := make(map[string]*recv1alpha1.RecommendedContainerResources, len(containerReqs))
	for i, _ := range podRecommends.ContainerRecommendations {
		containerRec := podRecommends.ContainerRecommendations[i]
		containerRecs[containerRec.ContainerName] = &containerRec
	}

	// should follow the origin container request in workload, since some sidecar container maybe injected like edas
	for containerName, containerReq := range containerReqs {
		containerRec, exist := containerRecs[containerName]
		if !exist {
			return
		}
		curCPUDegree := calcAdjustDegree(containerRec.Target.Cpu(), containerReq.Cpu(), marginRatio)
		if math.Abs(curCPUDegree) > math.Abs(cpuDegree) {
			cpuDegree = curCPUDegree
		}
		curMemoryDegree := calcAdjustDegree(containerRec.Target.Memory(), containerReq.Memory(), marginRatio)
		if math.Abs(curMemoryDegree) > math.Abs(memoryDegree) {
			memoryDegree = curMemoryDegree
		}
	}
	return cpuDegree, memoryDegree
}

// 0 <= marginRatio <= 1
// recommend > request: min(1.0, recommend / request - 1)
// requestWithoutMargin * <= recommend <= request: 0
// recommend < requestWithoutMargin: recommend / requestWithoutMargin - 1
func calcAdjustDegree(recommend, request *resource.Quantity, marginRatio float64) float64 {
	marginRatio = math.Max(0, math.Min(1.0, marginRatio))
	recommendVal := float64(recommend.MilliValue())
	requestVal := float64(request.MilliValue())
	recommendWithMargin := recommendVal * (1 + marginRatio)

	// handle corner exception
	if recommendVal == 0 {
		return 0.0
	} else if requestVal == 0 {
		return 1.0
	}

	if requestVal < recommendVal {
		// request needs to be increased
		return math.Min(1.0, 1-requestVal/recommendVal)
	} else if recommendWithMargin < requestVal {
		// request needs to be decreased
		degree := (recommendWithMargin - requestVal) / recommendWithMargin
		if math.Abs(degree) <= 0.1 {
			// gap is limited, can be maintained
			return 0
		}
		return degree
	} else {
		return 0
	}
}

func getWarmUpDuration(annotations map[string]string) time.Duration {
	if annotations == nil {
		return DefaultRecommendationWarmUp
	}
	recommendWarmUpHoursStr, exist := annotations[AnnotationRecommenderWarmUpHours]
	if !exist {
		return DefaultRecommendationWarmUp
	}
	recommendWarmUpHours, err := strconv.ParseInt(recommendWarmUpHoursStr, 10, 64)
	if err != nil {
		return DefaultRecommendationWarmUp
	}
	recommendWarmUpDuration := time.Duration(recommendWarmUpHours) * time.Hour
	if recommendWarmUpDuration < MinRecommendationWarmUp {
		return MinRecommendationWarmUp
	}
	return recommendWarmUpDuration
}
