package orchestratingslo

import (
	"reflect"
	"testing"
	"time"

	sev1 "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

func fromString(val string) *intstr.IntOrString {
	intOrPercent := intstr.FromString(val)
	return &intOrPercent
}

func fromInt(val int) *intstr.IntOrString {
	intOrPercent := intstr.FromInt(val)
	return &intOrPercent
}

func TestGetMaxUnavailable(t *testing.T) {
	tests := []struct {
		name         string
		replicas     int
		intOrPercent *intstr.IntOrString
		want         int
		wantErr      bool
	}{
		{
			name:         "has replicas and has percent",
			replicas:     100,
			intOrPercent: fromString("10%"),
			want:         10,
			wantErr:      false,
		},
		{
			name:         "has replicas and has int value",
			replicas:     100,
			intOrPercent: fromString("10"),
			want:         10,
			wantErr:      false,
		},
		{
			name:     "has 100 replicas but missing percent",
			replicas: 100,
			want:     10,
			wantErr:  false,
		},
		{
			name:     "has 6 replicas but missing percent",
			replicas: 6,
			want:     2,
			wantErr:  false,
		},
		{
			name:     "has 3 replicas but missing percent",
			replicas: 3,
			want:     1,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetMaxUnavailable(tt.replicas, tt.intOrPercent)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMaxUnavailable() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetMaxUnavailable() got = %v, wantUnhealthySloes %v", got, tt.want)
			}
		})
	}
}

func TestGetMaxMigrating(t *testing.T) {
	tests := []struct {
		name         string
		replicas     int
		intOrPercent *intstr.IntOrString
		want         int
		wantErr      bool
	}{
		{
			name:         "has replicas and has percent",
			replicas:     100,
			intOrPercent: fromString("10%"),
			want:         10,
			wantErr:      false,
		},
		{
			name:         "has replicas and has int value",
			replicas:     100,
			intOrPercent: fromString("10"),
			want:         10,
			wantErr:      false,
		},
		{
			name:     "has 100 replicas but missing percent",
			replicas: 100,
			want:     10,
			wantErr:  false,
		},
		{
			name:     "has 6 replicas but missing percent",
			replicas: 6,
			want:     2,
			wantErr:  false,
		},
		{
			name:     "has 3 replicas but missing percent",
			replicas: 3,
			want:     1,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetMaxMigrating(tt.replicas, tt.intOrPercent)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMaxMigrating() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetMaxMigrating() got = %v, wantUnhealthySloes %v", got, tt.want)
			}
		})
	}
}

func TestNewRebindCPUParam(t *testing.T) {
	dummyRateLimiter := &RateLimiter{}
	tests := []struct {
		name             string
		orchestratingSLO *sev1.OrchestratingSLO
		want             *RebindCPUParam
	}{
		{
			name: "custom full params",
			orchestratingSLO: &sev1.OrchestratingSLO{
				Spec: sev1.OrchestratingSLOSpec{
					Operation: &sev1.OrchestratingSLOOperation{
						RebindCPU: &sev1.OrchestratingSLORebindCPUParams{
							DryRun: false,
							RetryPolicy: &sev1.OrchestratingSLORetryPolicy{
								MaxRetryCount:                pointer.Int32Ptr(6),
								RetryPeriodInSeconds:         pointer.Int32Ptr(666 * 60),
								StabilizationWindowInSeconds: pointer.Int32Ptr(777 * 60),
							},
						},
					},
				},
			},
			want: &RebindCPUParam{
				DryRun:              false,
				RateLimiter:         dummyRateLimiter,
				MaxRetryCount:       6,
				RetryPeriod:         666 * 60 * time.Second,
				StabilizationWindow: 777 * 60 * time.Second,
			},
		},
		{
			name: "missing full params",
			orchestratingSLO: &sev1.OrchestratingSLO{
				Spec: sev1.OrchestratingSLOSpec{
					Operation: &sev1.OrchestratingSLOOperation{
						RebindCPU: &sev1.OrchestratingSLORebindCPUParams{
							DryRun: false,
						},
					},
				},
			},
			want: &RebindCPUParam{
				DryRun:              false,
				RateLimiter:         dummyRateLimiter,
				MaxRetryCount:       defaultRebindCPUMaxRetryCount,
				RetryPeriod:         defaultRebindCPURetryPeriod,
				StabilizationWindow: defaultRebindCPUStabilizationWindow,
			},
		},
		{
			name: "set dryrun",
			orchestratingSLO: &sev1.OrchestratingSLO{
				Spec: sev1.OrchestratingSLOSpec{
					Operation: &sev1.OrchestratingSLOOperation{
						RebindCPU: &sev1.OrchestratingSLORebindCPUParams{
							DryRun: true,
							RetryPolicy: &sev1.OrchestratingSLORetryPolicy{
								MaxRetryCount:                pointer.Int32Ptr(6),
								RetryPeriodInSeconds:         pointer.Int32Ptr(666 * 60),
								StabilizationWindowInSeconds: pointer.Int32Ptr(777 * 60),
							},
						},
					},
				},
			},
			want: &RebindCPUParam{
				DryRun:              true,
				RateLimiter:         dummyRateLimiter,
				MaxRetryCount:       6,
				RetryPeriod:         666 * 60 * time.Second,
				StabilizationWindow: 777 * 60 * time.Second,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewRebindCPUParam(tt.orchestratingSLO, dummyRateLimiter); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewRebindCPUParam() = %v, wantUnhealthySloes %v", got, tt.want)
			}
		})
	}
}

func TestNewMigrateParam(t *testing.T) {
	dummyRateLimiter := &RateLimiter{}
	tests := []struct {
		name             string
		orchestratingSLO *sev1.OrchestratingSLO
		want             *MigrateParam
	}{
		{
			name: "custom full params",
			orchestratingSLO: &sev1.OrchestratingSLO{
				Spec: sev1.OrchestratingSLOSpec{
					Operation: &sev1.OrchestratingSLOOperation{
						Migrate: &sev1.OrchestratingSLOMigrateParams{
							DryRun: false,
							RetryPolicy: &sev1.OrchestratingSLORetryPolicy{
								MaxRetryCount:                pointer.Int32Ptr(6),
								RetryPeriodInSeconds:         pointer.Int32Ptr(666 * 60),
								StabilizationWindowInSeconds: pointer.Int32Ptr(777 * 60),
							},
							MaxMigratingPerNode: pointer.Int32Ptr(3),
							MaxMigrating:        fromString("10%"),
							MaxUnavailable:      fromString("10%"),
							Defragmentation:     pointer.BoolPtr(true),
							NeedReserveResource: pointer.BoolPtr(false),
							EvictType:           "NativeEvict",
							TimeoutInSeconds:    pointer.Int64Ptr(666 * 60),
						},
					},
				},
			},
			want: &MigrateParam{
				DryRun:              false,
				RateLimiter:         dummyRateLimiter,
				MaxRetryCount:       6,
				RetryPeriod:         666 * 60 * time.Second,
				StabilizationWindow: 777 * 60 * time.Second,
				MaxMigratingPerNode: 3,
				MaxMigrating:        fromString("10%"),
				MaxUnavailable:      fromString("10%"),
				Defragmentation:     true,
				NeedReserveResource: false,
				EvictType:           sev1.PodMigrateNativeEvict,
				Timeout:             666 * 60 * time.Second,
			},
		},
		{
			name: "missing full params",
			orchestratingSLO: &sev1.OrchestratingSLO{
				Spec: sev1.OrchestratingSLOSpec{
					Operation: &sev1.OrchestratingSLOOperation{
						Migrate: &sev1.OrchestratingSLOMigrateParams{
							DryRun: false,
						},
					},
				},
			},
			want: &MigrateParam{
				DryRun:              false,
				RateLimiter:         dummyRateLimiter,
				MaxRetryCount:       defaultRebindCPUMaxRetryCount,
				RetryPeriod:         defaultRebindCPURetryPeriod,
				StabilizationWindow: defaultRebindCPUStabilizationWindow,
				MaxMigratingPerNode: defaultMaxMigratingPerNode,
				MaxMigrating:        nil,
				MaxUnavailable:      nil,
				Defragmentation:     false,
				NeedReserveResource: true,
				EvictType:           sev1.PodMigrateEvictByLabel,
				Timeout:             defaultMigrateJobTimeout,
			},
		},
		{
			name: "set dryrun",
			orchestratingSLO: &sev1.OrchestratingSLO{
				Spec: sev1.OrchestratingSLOSpec{
					Operation: &sev1.OrchestratingSLOOperation{
						Migrate: &sev1.OrchestratingSLOMigrateParams{
							DryRun: true,
							RetryPolicy: &sev1.OrchestratingSLORetryPolicy{
								MaxRetryCount:                pointer.Int32Ptr(6),
								RetryPeriodInSeconds:         pointer.Int32Ptr(666 * 60),
								StabilizationWindowInSeconds: pointer.Int32Ptr(777 * 60),
							},
						},
					},
				},
			},
			want: &MigrateParam{
				DryRun:              true,
				RateLimiter:         dummyRateLimiter,
				MaxRetryCount:       6,
				RetryPeriod:         666 * 60 * time.Second,
				StabilizationWindow: 777 * 60 * time.Second,
				MaxMigratingPerNode: defaultMaxMigratingPerNode,
				MaxMigrating:        nil,
				MaxUnavailable:      nil,
				Defragmentation:     false,
				NeedReserveResource: true,
				EvictType:           sev1.PodMigrateEvictByLabel,
				Timeout:             defaultMigrateJobTimeout,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewMigrateParam(tt.orchestratingSLO, dummyRateLimiter); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewMigrateParam() = %v, wantUnhealthySloes %v", got, tt.want)
			}
		})
	}
}
