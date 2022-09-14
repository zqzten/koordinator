package orchestratingslo

import (
	"encoding/json"
	"time"

	sev1 "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/flowcontrol"
)

const (
	defaultRebindCPUQPS                 float32 = 50
	defaultRebindCPUBurst                       = 10
	defaultRebindCPURetryPeriod                 = 5 * time.Minute
	defaultRebindCPUStabilizationWindow         = 30 * time.Minute
	defaultRebindCPUMaxRetryCount               = 3
)

const (
	defaultMigrateQPS                 float32 = 1
	defaultMigrateBurst                       = 1
	defaultMigrateRetryPeriod                 = 5 * time.Minute
	defaultMigrateStabilizationWindow         = 30 * time.Minute
	defaultMigrateMaxRetryCounts              = 3
	defaultMigrateJobTimeout                  = 5 * time.Minute
	defaultMaxMigratingPerNode                = 1
)

const (
	LabelOrchestratingSLOHealthy               = "alibabacloud.com/orchestrating-healthy"
	AnnotationOrchestratingSLOProblems         = "alibabacloud.com/orchestrating-problems"
	LabelOrchestratingSLONamespace             = "alibabacloud.com/orchestrating-namespace"
	LabelOrchestratingSLOName                  = "alibabacloud.com/orchestrating-name"
	LabelOrchestratingSLOOperationStatus       = "alibabacloud.com/orchestrating-ops-status"
	AnnotationOrchestratingSLOOperationContext = "alibabacloud.com/orchestrating-ops-context"
)

var operationOrders = []sev1.OrchestratingSLOOperationType{
	sev1.RebindCPUOrchestratingSLOOperation,
	sev1.MigrateOrchestratingSLOOperation,
}

type ProblemContext struct {
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
	Problems       []Problem    `json:"problems,omitempty"`
}

type Problem struct {
	SLOType string      `json:"sloType,omitempty"`
	Reason  string      `json:"reason,omitempty"`
	Payload interface{} `json:"payload,omitempty"`
}

type SLOOperationStatus string

const (
	SLOOperationStatusPending SLOOperationStatus = "Pending"
	SLOOperationStatusRunning SLOOperationStatus = "Running"
	SLOOperationStatusUnknown SLOOperationStatus = "Unknown"
)

type OperationContext struct {
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
	Operation      string       `json:"operation,omitempty"`
	Count          int          `json:"count,omitempty"`
}

type RebindCPUParam struct {
	DryRun              bool
	RateLimiter         flowcontrol.RateLimiter
	MaxRetryCount       int
	RetryPeriod         time.Duration
	StabilizationWindow time.Duration
}

type MigrateParam struct {
	DryRun              bool
	RateLimiter         flowcontrol.RateLimiter
	MaxRetryCount       int
	RetryPeriod         time.Duration
	StabilizationWindow time.Duration
	MaxMigratingPerNode int
	MaxMigrating        *intstr.IntOrString
	MaxUnavailable      *intstr.IntOrString
	EvictType           sev1.PodMigrateEvictType
	NeedReserveResource bool
	Defragmentation     bool
	Timeout             time.Duration
}

type RateLimiter struct {
	flowcontrol.RateLimiter
	burst int
}

func ParseSLOOperationStatus(val string) SLOOperationStatus {
	switch SLOOperationStatus(val) {
	case SLOOperationStatusPending:
		return SLOOperationStatusPending
	case SLOOperationStatusRunning:
		return SLOOperationStatusRunning
	default:
		return SLOOperationStatusUnknown
	}
}

func GetOperationContext(pod *v1.Pod, operationType sev1.OrchestratingSLOOperationType) (*OperationContext, error) {
	val, ok := pod.Annotations[AnnotationOrchestratingSLOOperationContext]
	if ok {
		var contexts []*OperationContext
		err := json.Unmarshal([]byte(val), &contexts)
		if err != nil {
			return nil, err
		}
		for _, v := range contexts {
			if v.Operation == string(operationType) {
				return v, nil
			}
		}
	}
	return &OperationContext{}, nil
}

func UpdateOperationContexts(pod *v1.Pod, newCtx *OperationContext) ([]*OperationContext, error) {
	val, ok := pod.Annotations[AnnotationOrchestratingSLOOperationContext]
	if ok {
		var contexts []*OperationContext
		err := json.Unmarshal([]byte(val), &contexts)
		if err != nil {
			return nil, err
		}
		updated := false
		for _, v := range contexts {
			if v.Operation == newCtx.Operation {
				*v = *newCtx
				updated = true
				break
			}
		}
		if !updated {
			contexts = append(contexts, newCtx)
		}
		return contexts, nil
	}
	return []*OperationContext{newCtx}, nil
}

func GetMaxUnavailable(replicas int, intOrPercent *intstr.IntOrString) (int, error) {
	if intOrPercent == nil {
		if replicas > 10 {
			s := intstr.FromString("10%")
			intOrPercent = &s
		} else if replicas >= 4 && replicas <= 10 {
			s := intstr.FromInt(2)
			intOrPercent = &s
		} else {
			s := intstr.FromInt(1)
			intOrPercent = &s
		}
	}
	return intstr.GetValueFromIntOrPercent(intOrPercent, replicas, true)
}

func GetMaxMigrating(replicas int, intOrPercent *intstr.IntOrString) (int, error) {
	return GetMaxUnavailable(replicas, intOrPercent)
}

func NewRebindCPUParam(orchestratingSLO *sev1.OrchestratingSLO, rateLimiter *RateLimiter) *RebindCPUParam {
	param := &RebindCPUParam{
		DryRun:              false,
		RateLimiter:         rateLimiter,
		MaxRetryCount:       defaultRebindCPUMaxRetryCount,
		RetryPeriod:         defaultRebindCPURetryPeriod,
		StabilizationWindow: defaultRebindCPUStabilizationWindow,
	}
	if orchestratingSLO.Spec.Operation != nil && orchestratingSLO.Spec.Operation.RebindCPU != nil {
		rebindCPUParams := orchestratingSLO.Spec.Operation.RebindCPU

		param.DryRun = rebindCPUParams.DryRun

		retryPolicy := rebindCPUParams.RetryPolicy
		if retryPolicy != nil {
			if retryPolicy.MaxRetryCount != nil && *retryPolicy.MaxRetryCount > 0 {
				param.MaxRetryCount = int(*retryPolicy.MaxRetryCount)
			}
			if retryPolicy.RetryPeriodInSeconds != nil && *retryPolicy.RetryPeriodInSeconds > 0 {
				param.RetryPeriod = time.Duration(*retryPolicy.RetryPeriodInSeconds) * time.Second
			}
			if retryPolicy.StabilizationWindowInSeconds != nil && *retryPolicy.StabilizationWindowInSeconds > 0 {
				param.StabilizationWindow = time.Duration(*retryPolicy.StabilizationWindowInSeconds) * time.Second
			}
		}
	}
	return param
}

func NewMigrateParam(orchestratingSLO *sev1.OrchestratingSLO, rateLimiter *RateLimiter) *MigrateParam {
	param := &MigrateParam{
		DryRun:              false,
		RateLimiter:         rateLimiter,
		MaxRetryCount:       defaultMigrateMaxRetryCounts,
		RetryPeriod:         defaultMigrateRetryPeriod,
		StabilizationWindow: defaultMigrateStabilizationWindow,
		MaxMigratingPerNode: defaultMaxMigratingPerNode,
		EvictType:           sev1.PodMigrateEvictByLabel,
		NeedReserveResource: true,
		Defragmentation:     false,
		Timeout:             defaultMigrateJobTimeout,
	}
	if orchestratingSLO.Spec.Operation != nil && orchestratingSLO.Spec.Operation.Migrate != nil {
		migrateParams := orchestratingSLO.Spec.Operation.Migrate

		param.DryRun = migrateParams.DryRun
		param.MaxMigrating = migrateParams.MaxMigrating
		param.MaxUnavailable = migrateParams.MaxUnavailable

		if migrateParams.MaxMigratingPerNode != nil {
			param.MaxMigratingPerNode = int(*migrateParams.MaxMigratingPerNode)
		}

		if migrateParams.EvictType != "" {
			param.EvictType = sev1.PodMigrateEvictType(migrateParams.EvictType)
		}

		retryPolicy := migrateParams.RetryPolicy
		if retryPolicy != nil {
			if retryPolicy.MaxRetryCount != nil && *retryPolicy.MaxRetryCount > 0 {
				param.MaxRetryCount = int(*retryPolicy.MaxRetryCount)
			}
			if retryPolicy.RetryPeriodInSeconds != nil && *retryPolicy.RetryPeriodInSeconds > 0 {
				param.RetryPeriod = time.Duration(*retryPolicy.RetryPeriodInSeconds) * time.Second
			}
			if retryPolicy.StabilizationWindowInSeconds != nil && *retryPolicy.StabilizationWindowInSeconds > 0 {
				param.StabilizationWindow = time.Duration(*retryPolicy.StabilizationWindowInSeconds) * time.Second
			}
		}

		if migrateParams.TimeoutInSeconds != nil && *migrateParams.TimeoutInSeconds > 0 {
			param.Timeout = time.Duration(*migrateParams.TimeoutInSeconds) * time.Second
		}
		if migrateParams.Defragmentation != nil {
			param.Defragmentation = *migrateParams.Defragmentation
		}
		if migrateParams.NeedReserveResource != nil {
			param.NeedReserveResource = *migrateParams.NeedReserveResource
		}
	}
	return param
}
