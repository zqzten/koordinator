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

package cpustable

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	mockmetriccache "github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache/mockmetriccache"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	mockstatesinformer "github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer/mockstatesinformer"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
	"github.com/koordinator-sh/koordinator/pkg/util"
	"github.com/koordinator-sh/koordinator/pkg/util/sloconfig"
)

func Test_htRatioPIDController_updateController(t *testing.T) {
	fakeNow := time.Now().Add(-10 * time.Second)
	testTime := fakeNow.Add(1 * time.Second)
	testInterval := 1 * time.Second
	type fields struct {
		podControllerCache func(t *testing.T) *gocache.Cache
	}
	type args struct {
		uid         string
		actualState float64
		refState    float64
		updateTime  time.Time
		cfg         unified.CPUStablePIDConfig
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   PodHTRatioPIDSignal
	}{
		{
			name: "P controller, first time",
			fields: fields{
				podControllerCache: func(t *testing.T) *gocache.Cache {
					c := gocache.New(10*time.Second, 10*time.Second)
					return c
				},
			},
			args: args{
				uid:         "xxxxxx",
				actualState: 1.0,
				refState:    0.75,
				updateTime:  testTime,
				cfg: unified.CPUStablePIDConfig{
					ProportionalPermill:     pointer.Int64(1000),
					IntegralPermill:         pointer.Int64(0),
					DerivativePermill:       pointer.Int64(0),
					SignalNormalizedPercent: pointer.Int64(-10000),
				},
			},
			want: PodHTRatioPIDSignal(25.0), // 25 * 1.0
		},
		{
			name: "PI controller, first time",
			fields: fields{
				podControllerCache: func(t *testing.T) *gocache.Cache {
					c := gocache.New(10*time.Second, 10*time.Second)
					return c
				},
			},
			args: args{
				uid:         "xxxxxx",
				actualState: 1.0,
				refState:    0.75,
				updateTime:  testTime,
				cfg: unified.CPUStablePIDConfig{
					ProportionalPermill:     pointer.Int64(800),
					IntegralPermill:         pointer.Int64(100),
					DerivativePermill:       pointer.Int64(0),
					SignalNormalizedPercent: pointer.Int64(-10000),
				},
			},
			want: PodHTRatioPIDSignal(22.5), // 25 * 0.8 + 25 * 0.1
		},
		{
			name: "PID controller, first time",
			fields: fields{
				podControllerCache: func(t *testing.T) *gocache.Cache {
					c := gocache.New(10*time.Second, 10*time.Second)
					return c
				},
			},
			args: args{
				uid:         "xxxxxx",
				actualState: 1.0,
				refState:    0.75,
				updateTime:  testTime,
				cfg: unified.CPUStablePIDConfig{
					ProportionalPermill:     pointer.Int64(800),
					IntegralPermill:         pointer.Int64(100),
					DerivativePermill:       pointer.Int64(100),
					SignalNormalizedPercent: pointer.Int64(-10000),
				},
			},
			want: PodHTRatioPIDSignal(22.5), // 25 * 0.8 + 25 * 0.1
		},
		{
			name: "PID controller, first time, different normalized ratio",
			fields: fields{
				podControllerCache: func(t *testing.T) *gocache.Cache {
					c := gocache.New(10*time.Second, 10*time.Second)
					return c
				},
			},
			args: args{
				uid:         "xxxxxx",
				actualState: 1.0,
				refState:    0.75,
				updateTime:  testTime,
				cfg: unified.CPUStablePIDConfig{
					ProportionalPermill:     pointer.Int64(400),
					IntegralPermill:         pointer.Int64(50),
					DerivativePermill:       pointer.Int64(50),
					SignalNormalizedPercent: pointer.Int64(-20000),
				},
			},
			want: PodHTRatioPIDSignal(22.5),
		},
		{
			name: "P controller",
			fields: fields{
				podControllerCache: func(t *testing.T) *gocache.Cache {
					c := gocache.New(10*time.Second, 10*time.Second)
					c.SetDefault("xxxxxx", newTestPIDController(1.0, 0, 0, func(controller *pidController) {
						controller.ErrIntegral = -0.25
						controller.PrevErr = -0.25
						controller.UpdateTime = &fakeNow
					}))
					return c
				},
			},
			args: args{
				uid:         "xxxxxx",
				actualState: 0.9,
				refState:    0.75,
				updateTime:  testTime,
				cfg: unified.CPUStablePIDConfig{
					SignalNormalizedPercent: pointer.Int64(-10000),
				},
			},
			want: PodHTRatioPIDSignal(15.0),
		},
		{
			name: "PI controller",
			fields: fields{
				podControllerCache: func(t *testing.T) *gocache.Cache {
					c := gocache.New(10*time.Second, 10*time.Second)
					c.SetDefault("xxxxxx", newTestPIDController(0.8, 0.1, 0, func(controller *pidController) {
						controller.ErrIntegral = -0.25
						controller.PrevErr = -0.25
						controller.UpdateTime = &fakeNow
					}))
					return c
				},
			},
			args: args{
				uid:         "xxxxxx",
				actualState: 0.9,
				refState:    0.75,
				updateTime:  testTime,
				cfg: unified.CPUStablePIDConfig{
					SignalNormalizedPercent: pointer.Int64(-10000),
				},
			},
			want: PodHTRatioPIDSignal(16.0), // 0.8 * 0.15 + 0.1 * (0.15 + 0.25)
		},
		{
			name: "PID controller",
			fields: fields{
				podControllerCache: func(t *testing.T) *gocache.Cache {
					c := gocache.New(10*time.Second, 10*time.Second)
					c.SetDefault("xxxxxx", newTestPIDController(0.8, 0.1, 0.1, func(controller *pidController) {
						controller.ErrIntegral = -0.25
						controller.PrevErr = -0.25
						controller.UpdateTime = &fakeNow
					}))
					return c
				},
			},
			args: args{
				uid:         "xxxxxx",
				actualState: 0.9,
				refState:    0.75,
				updateTime:  testTime,
				cfg: unified.CPUStablePIDConfig{
					SignalNormalizedPercent: pointer.Int64(-10000),
				},
			},
			want: PodHTRatioPIDSignal(15.0), // 0.8 * 0.15 + 0.1 * (0.15 + 0.25) + 0.1 * (0.15 - 0.25)
		},
		{
			name: "PID controller, different sample interval",
			fields: fields{
				podControllerCache: func(t *testing.T) *gocache.Cache {
					c := gocache.New(10*time.Second, 10*time.Second)
					c.SetDefault("xxxxxx", newTestPIDController(0.8, 0.05, 0.2, func(controller *pidController) {
						controller.ErrIntegral = -0.50
						controller.PrevErr = -0.25
						controller.UpdateTime = &fakeNow
					}))
					return c
				},
			},
			args: args{
				uid:         "xxxxxx",
				actualState: 0.9,
				refState:    0.75,
				updateTime:  fakeNow.Add(2 * time.Second),
				cfg: unified.CPUStablePIDConfig{
					SignalNormalizedPercent: pointer.Int64(-10000),
				},
			},
			want: PodHTRatioPIDSignal(15.0), // 0.8 * 0.15 + 0.05 * 2 * (0.15 + 0.25) + 0.2 / 2 * (0.15 - 0.25)
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldReconcileInterval := ReconcileInterval
			ReconcileInterval = testInterval
			defer func() {
				ReconcileInterval = oldReconcileInterval
			}()

			h := &htRatioPIDController{
				podControllerCache: tt.fields.podControllerCache(t),
			}
			defer h.podControllerCache.Flush()
			got := h.updateController(tt.args.uid, tt.args.actualState, tt.args.refState, tt.args.updateTime, tt.args.cfg)
			assert.LessOrEqual(t, tt.want-0.001, got)
			assert.GreaterOrEqual(t, tt.want+0.001, got)
		})
	}
}

func Test_htRatioPIDController_GetWarmupState(t *testing.T) {
	fakeNow := time.Now()
	testPodMetas := []*statesinformer.PodMeta{
		{
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ls-pod",
					UID:  "xxxxxx",
					Labels: map[string]string{
						extension.LabelPodQoS: string(extension.QoSLS),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
						},
						{
							Name: "test-sidecar",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  unified.EnvSigmaIgnoreResource,
									Value: "true",
								},
							},
						},
					},
				},
			},
		},
		{
			Pod: &corev1.Pod{ // non-running
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-lse-pod",
					UID:  "yyyyyy",
					Labels: map[string]string{
						extension.LabelPodQoS: string(extension.QoSLSE),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
								},
							},
						},
					},
				},
			},
		},
		{
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-be-pod",
					UID:  "zzzzzz",
					Labels: map[string]string{
						extension.LabelPodQoS: string(extension.QoSBE),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									extension.BatchCPU:    resource.MustParse("8000"),
									extension.BatchMemory: resource.MustParse("8Gi"),
								},
								Requests: corev1.ResourceList{
									extension.BatchCPU:    resource.MustParse("8000"),
									extension.BatchMemory: resource.MustParse("8Gi"),
								},
							},
						},
					},
				},
			},
		},
	}
	type fields struct {
		statesInformer func(ctrl *gomock.Controller) statesinformer.StatesInformer
		metricCache    func(ctrl *gomock.Controller) metriccache.MetricCache
		prepareFn      func() func()
	}
	type args struct {
		podMetas []*statesinformer.PodMeta
		strategy *unified.CPUStableStrategy
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    WarmupState
		wantErr bool
	}{
		{
			name: "get node warmup state successfully",
			fields: fields{
				prepareFn: func() func() {
					oldTimeNow := timeNow
					timeNow = func() time.Time {
						return fakeNow
					}
					oldAggregateResultFactory := metriccache.DefaultAggregateResultFactory
					return func() {
						timeNow = oldTimeNow
						metriccache.DefaultAggregateResultFactory = oldAggregateResultFactory
					}
				},
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					// node usage = 10.0
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					queryMetaForNodeUsage, err := metriccache.NodeCPUUsageMetric.BuildQueryMeta(nil)
					assert.NoError(t, err)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-30*time.Second), fakeNow).Return(mockQuerier, nil).Times(4)
					mockAggregateNodeUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateNodeUsageResult.EXPECT().Count().Return(3)
					mockAggregateNodeUsageResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(10.0, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForNodeUsage).Return(mockAggregateNodeUsageResult)
					mockQuerier.EXPECT().Query(queryMetaForNodeUsage, nil, mockAggregateNodeUsageResult).Return(nil)
					// ls pod, usage = 2.0
					queryMetaForLSPod, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					mockAggregateResultForLSPod := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateResultForLSPod.EXPECT().Count().Return(3)
					mockAggregateResultForLSPod.EXPECT().Value(metriccache.AggregationTypeAVG).Return(2.0, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForLSPod).Return(mockAggregateResultForLSPod)
					mockQuerier.EXPECT().Query(queryMetaForLSPod, nil, mockAggregateResultForLSPod).Return(nil)
					// lse pod, usage is missing
					queryMetaForLSEPod, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("yyyyyy"))
					assert.NoError(t, err)
					mockAggregateResultForLSEPod := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateResultForLSEPod.EXPECT().Count().Return(0)
					mockAggregateResultFactory.EXPECT().New(queryMetaForLSEPod).Return(mockAggregateResultForLSEPod)
					mockQuerier.EXPECT().Query(queryMetaForLSEPod, nil, mockAggregateResultForLSEPod).Return(nil)
					// be pod, usage = 8.0, scaled usage = 4.0
					queryMetaForBEPod, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("zzzzzz"))
					assert.NoError(t, err)
					mockAggregateResultForBEPod := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateResultForBEPod.EXPECT().Count().Return(2)
					mockAggregateResultForBEPod.EXPECT().Value(metriccache.AggregationTypeAVG).Return(8.0, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForBEPod).Return(mockAggregateResultForBEPod)
					mockQuerier.EXPECT().Query(queryMetaForBEPod, nil, mockAggregateResultForBEPod).Return(nil)
					// ls share pool usage = 6.0, util = 37.5%
					return mockMetricCache
				},
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					mockStatesInformer := mockstatesinformer.NewMockStatesInformer(ctrl)
					mockStatesInformer.EXPECT().GetAllPods().Return(testPodMetas)
					// be scale ratio = 0.5
					mockStatesInformer.EXPECT().GetNodeTopo().Return(&topologyv1alpha1.NodeResourceTopology{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-node",
							Annotations: map[string]string{
								extension.AnnotationNodeCPUSharedPools:   `[{"node": 0, "socket": 0, "cpuset": "8-15"}, {"node": 1, "socket": 0, "cpuset": "24-31"}]`,
								extension.AnnotationNodeBECPUSharedPools: `[{"node": 0, "socket": 0, "cpuset": "0-15"}, {"node": 1, "socket": 0, "cpuset": "16-31"}]`,
							},
						},
					})
					return mockStatesInformer
				},
			},
			args: args{
				podMetas: testPodMetas,
				strategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.PodMetricWindowSeconds = pointer.Int64(30)
					strategy.WarmupConfig = unified.CPUStableWarmupModel{
						Entries: []unified.CPUStableWarmupEntry{
							{
								SharePoolUtilPercent: pointer.Int64(0),
								HTRatio:              pointer.Int64(150),
							},
							{
								SharePoolUtilPercent: pointer.Int64(20),
								HTRatio:              pointer.Int64(120),
							},
							{
								SharePoolUtilPercent: pointer.Int64(40),
								HTRatio:              pointer.Int64(100),
							},
						},
					}
				}),
			},
			want: &htRatioWarmupState{
				WarmupHTRatio: 120,
			},
			wantErr: false,
		},
		{
			name: "skipped for no warmup strategy",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					return mockmetriccache.NewMockMetricCache(ctrl)
				},
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					return mockstatesinformer.NewMockStatesInformer(ctrl)
				},
			},
			args: args{
				podMetas: testPodMetas,
				strategy: &unified.CPUStableStrategy{},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "failed to get valid sharepool size",
			fields: fields{
				prepareFn: func() func() {
					oldTimeNow := timeNow
					timeNow = func() time.Time {
						return fakeNow
					}
					oldAggregateResultFactory := metriccache.DefaultAggregateResultFactory
					return func() {
						timeNow = oldTimeNow
						metriccache.DefaultAggregateResultFactory = oldAggregateResultFactory
					}
				},
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					return mockMetricCache
				},
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					mockStatesInformer := mockstatesinformer.NewMockStatesInformer(ctrl)
					mockStatesInformer.EXPECT().GetNodeTopo().Return(&topologyv1alpha1.NodeResourceTopology{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-node",
							Annotations: map[string]string{
								extension.AnnotationNodeCPUSharedPools:   `[]`,
								extension.AnnotationNodeBECPUSharedPools: `[]`,
							},
						},
					})
					return mockStatesInformer
				},
			},
			args: args{
				podMetas: testPodMetas,
				strategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.PodMetricWindowSeconds = pointer.Int64(30)
					strategy.WarmupConfig = unified.CPUStableWarmupModel{
						Entries: []unified.CPUStableWarmupEntry{
							{
								SharePoolUtilPercent: pointer.Int64(0),
								HTRatio:              pointer.Int64(150),
							},
							{
								SharePoolUtilPercent: pointer.Int64(20),
								HTRatio:              pointer.Int64(120),
							},
							{
								SharePoolUtilPercent: pointer.Int64(40),
								HTRatio:              pointer.Int64(100),
							},
						},
					}
				}),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			if tt.fields.prepareFn != nil {
				cleanupFn := tt.fields.prepareFn()
				defer cleanupFn()
			}
			h := &htRatioPIDController{
				metricCache:    tt.fields.metricCache(ctrl),
				statesInformer: tt.fields.statesInformer(ctrl),
			}
			got, gotErr := h.GetWarmupState(nil, tt.args.podMetas, tt.args.strategy)
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_htRatioPIDController_GetSignal(t *testing.T) {
	fakeNow := time.Now()
	fakeStartTime := fakeNow.Add(-20 * time.Minute)
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			UID:  "xxxxxx",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test-container",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("8Gi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("8Gi"),
						},
					},
				},
				{
					Name: "test-sidecar",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  unified.EnvSigmaIgnoreResource,
							Value: "true",
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "test-container",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: metav1.Time{Time: fakeNow.Add(-10 * time.Minute)},
						},
					},
				},
				{
					Name: "test-sidecar",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: metav1.Time{Time: fakeNow.Add(-12 * time.Minute)},
						},
					},
				},
			},
		},
	}
	type fields struct {
		metricCache           func(ctrl *gomock.Controller) metriccache.MetricCache
		runInterval           time.Duration
		mutateControllerCache func(controllerCache *gocache.Cache)
	}
	type args struct {
		podStrategy *unified.CPUStableStrategy
		pod         *corev1.Pod
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    PodHTRatioPIDSignal
		wantErr bool
	}{
		{
			name: "reset if strategy disables",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					return mockmetriccache.NewMockMetricCache(ctrl)
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyNone),
				},
				pod: testPod,
			},
			want:    PodHTRatioPIDSpecialSignalReset,
			wantErr: false,
		},
		{
			name: "remain if strategy ignores",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					return mockmetriccache.NewMockMetricCache(ctrl)
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyIgnore),
				},
				pod: testPod,
			},
			want:    PodHTRatioPIDSpecialSignalRemain,
			wantErr: false,
		},
		{
			name: "failed to query pod cpu satisfaction metrics",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mc := mockmetriccache.NewMockMetricCache(ctrl)
					mc.EXPECT().Querier(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("expected error"))
					return mc
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
				}),
				pod: testPod,
			},
			want:    PodHTRatioPIDSpecialSignalRemain,
			wantErr: true,
		},
		{
			name: "failed to collect enough pod cpu satisfaction metrics",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(1)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(0)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
				}),
				pod: testPod,
			},
			want:    PodHTRatioPIDSpecialSignalRemain,
			wantErr: true,
		},
		{
			name: "failed to aggregate pod cpu satisfaction metrics",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(1)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(1)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult)
					mockAggregateSatisfactionResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(0.0, fmt.Errorf("expected error"))
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
				}),
				pod: testPod,
			},
			want:    PodHTRatioPIDSpecialSignalRemain,
			wantErr: true,
		},
		{
			name: "failed to collect pod cpu satisfaction metrics",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(2)
					queryMetaForUsage, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateUsageResult.EXPECT().Count().Return(0)
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(1)
					mockAggregateSatisfactionResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(0.83, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForUsage).Return(mockAggregateUsageResult).Times(1)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult).Times(1)
					mockQuerier.EXPECT().Query(queryMetaForUsage, nil, mockAggregateUsageResult).Return(nil).Times(1)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil).Times(1)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.CPUUtilMinPercent = pointer.Int64(50)
					strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent = pointer.Int64(80)
					strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill = pointer.Int64(50)
				}),
				pod: testPod,
			},
			want:    PodHTRatioPIDSpecialSignalRemain,
			wantErr: true,
		},
		{
			name: "aborted for unlimited pod",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.CPUStableScaleModel.PIDConfig.ProportionalPermill = pointer.Int64(800)
					strategy.CPUStableScaleModel.PIDConfig.IntegralPermill = pointer.Int64(100)
					strategy.CPUStableScaleModel.PIDConfig.DerivativePermill = pointer.Int64(50)
					strategy.CPUStableScaleModel.PIDConfig.SignalNormalizedPercent = pointer.Int64(-10000)
				}),
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod",
						UID:  "xxxxxx",
					},
				},
			},
			want:    PodHTRatioPIDSpecialSignalRemain,
			wantErr: false,
		},
		{
			name: "relax for pod whose cpu usage is too low",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(2)
					queryMetaForUsage, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateResult.EXPECT().Count().Return(1)
					mockAggregateResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(1.0, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForUsage).Return(mockAggregateResult).Times(1)
					mockQuerier.EXPECT().Query(queryMetaForUsage, nil, mockAggregateResult).Return(nil).Times(1)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(1)
					mockAggregateSatisfactionResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(0.83, nil)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult).Times(1)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil).Times(1)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
				mutateControllerCache: func(controllerCache *gocache.Cache) {
					controllerCache.SetDefault("xxxxxx", newPIDController(0.8, 0.1, 0.05))
				},
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.CPUUtilMinPercent = pointer.Int64(80)
					strategy.CPUStableScaleModel.PIDConfig.ProportionalPermill = pointer.Int64(800)
					strategy.CPUStableScaleModel.PIDConfig.IntegralPermill = pointer.Int64(100)
					strategy.CPUStableScaleModel.PIDConfig.DerivativePermill = pointer.Int64(50)
					strategy.CPUStableScaleModel.PIDConfig.SignalNormalizedPercent = pointer.Int64(-10000)
				}),
				pod: testPod,
			},
			want:    PodHTRatioPIDSpecialSignalRelax,
			wantErr: false,
		},
		{
			name: "pod cpu satisfaction is too low, need relax",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(2)
					queryMetaForUsage, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateUsageResult.EXPECT().Count().Return(1)
					mockAggregateUsageResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(2.0, nil)
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(1)
					mockAggregateSatisfactionResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(0.65, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForUsage).Return(mockAggregateUsageResult).Times(1)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult).Times(1)
					mockQuerier.EXPECT().Query(queryMetaForUsage, nil, mockAggregateUsageResult).Return(nil).Times(1)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil).Times(1)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.CPUUtilMinPercent = pointer.Int64(50)
					strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent = pointer.Int64(80)
					strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill = pointer.Int64(30)
					strategy.CPUStableScaleModel.PIDConfig.ProportionalPermill = pointer.Int64(800)
					strategy.CPUStableScaleModel.PIDConfig.IntegralPermill = pointer.Int64(100)
					strategy.CPUStableScaleModel.PIDConfig.DerivativePermill = pointer.Int64(50)
					strategy.CPUStableScaleModel.PIDConfig.SignalNormalizedPercent = pointer.Int64(-10000)
				}),
				pod: testPod,
			},
			want:    PodHTRatioPIDSignal(-19.5), // (0.8 - 0.65) * 0.8 + (0.8 - 0.65) * 5 * 0.1 = 19.5
			wantErr: false,
		},
		{
			name: "pod cpu satisfaction is too high, need throttle",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(2)
					queryMetaForUsage, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateUsageResult.EXPECT().Count().Return(1)
					mockAggregateUsageResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(2.0, nil)
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(1)
					mockAggregateSatisfactionResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(0.95, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForUsage).Return(mockAggregateUsageResult).Times(1)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult).Times(1)
					mockQuerier.EXPECT().Query(queryMetaForUsage, nil, mockAggregateUsageResult).Return(nil).Times(1)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil).Times(1)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.CPUUtilMinPercent = pointer.Int64(50)
					strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent = pointer.Int64(80)
					strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill = pointer.Int64(30)
					strategy.CPUStableScaleModel.PIDConfig.ProportionalPermill = pointer.Int64(800)
					strategy.CPUStableScaleModel.PIDConfig.IntegralPermill = pointer.Int64(100)
					strategy.CPUStableScaleModel.PIDConfig.DerivativePermill = pointer.Int64(50)
					strategy.CPUStableScaleModel.PIDConfig.SignalNormalizedPercent = pointer.Int64(-10000)
				}),
				pod: testPod,
			},
			want:    PodHTRatioPIDSignal(19.5), // (0.8 - 0.95) * 0.8 + (0.8 - 0.95) * 5 * 0.1 = 19.5
			wantErr: false,
		},
		{
			name: "pod cpu satisfaction approximates to the target",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(2)
					queryMetaForUsage, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateUsageResult.EXPECT().Count().Return(1)
					mockAggregateUsageResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(2.0, nil)
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(1)
					mockAggregateSatisfactionResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(0.83, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForUsage).Return(mockAggregateUsageResult).Times(1)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult).Times(1)
					mockQuerier.EXPECT().Query(queryMetaForUsage, nil, mockAggregateUsageResult).Return(nil).Times(1)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil).Times(1)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.CPUUtilMinPercent = pointer.Int64(50)
					strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent = pointer.Int64(80)
					strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill = pointer.Int64(50)
					strategy.CPUStableScaleModel.PIDConfig.ProportionalPermill = pointer.Int64(800)
					strategy.CPUStableScaleModel.PIDConfig.IntegralPermill = pointer.Int64(100)
					strategy.CPUStableScaleModel.PIDConfig.DerivativePermill = pointer.Int64(50)
					strategy.CPUStableScaleModel.PIDConfig.SignalNormalizedPercent = pointer.Int64(-10000)
				}),
				pod: testPod,
			},
			want:    PodHTRatioPIDSignal(3.9), // (0.8 - 0.83) * 0.8 + (0.8 - 0.83) * 0.1 * 5
			wantErr: false,
		},
		{
			name: "pod is initializing, use the warmup config",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodInitializeWindowSeconds = pointer.Int64(60)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(100)
					strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent = pointer.Int64(80)
					strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill = pointer.Int64(50)
					strategy.CPUStableScaleModel.PIDConfig.ProportionalPermill = pointer.Int64(800)
					strategy.CPUStableScaleModel.PIDConfig.IntegralPermill = pointer.Int64(100)
					strategy.CPUStableScaleModel.PIDConfig.DerivativePermill = pointer.Int64(50)
					strategy.CPUStableScaleModel.PIDConfig.SignalNormalizedPercent = pointer.Int64(-10000)
				}),
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod",
						UID:  "xxxxxx",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "test-container",
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("2"),
										corev1.ResourceMemory: resource.MustParse("8Gi"),
									},
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("2"),
										corev1.ResourceMemory: resource.MustParse("8Gi"),
									},
								},
							},
							{
								Name: "test-sidecar",
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("2Gi"),
									},
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("2Gi"),
									},
								},
								Env: []corev1.EnvVar{
									{
										Name:  unified.EnvSigmaIgnoreResource,
										Value: "true",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name: "test-container",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: fakeNow.Add(-10 * time.Second)},
									},
								},
							},
							{
								Name: "test-sidecar",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: fakeNow.Add(-30 * time.Minute)},
									},
								},
							},
						},
					},
				},
			},
			want:    PodHTRatioPIDSpecialSignalInit,
			wantErr: false,
		},
		{
			name: "pod is degrading, need reset",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(1)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(0)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult).Times(1)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil).Times(1)
					mockQuerierForDegrade := mockmetriccache.NewMockQuerier(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-300*time.Second), fakeNow).Return(mockQuerierForDegrade, nil).Times(1)
					mockAggregateSatisfactionResultForDegrade := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResultForDegrade.EXPECT().Count().Return(0)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResultForDegrade).Times(1)
					mockQuerierForDegrade.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResultForDegrade).Return(nil).Times(1)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
				mutateControllerCache: func(controllerCache *gocache.Cache) {
					controllerCache.SetDefault("xxxxxx", newPIDController(0.8, 0.1, 0.05))
				},
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.PodInitializeWindowSeconds = pointer.Int64(0)
					strategy.PodDegradeWindowSeconds = pointer.Int64(300)
					strategy.CPUStableScaleModel.PIDConfig.ProportionalPermill = pointer.Int64(800)
					strategy.CPUStableScaleModel.PIDConfig.IntegralPermill = pointer.Int64(100)
					strategy.CPUStableScaleModel.PIDConfig.DerivativePermill = pointer.Int64(50)
					strategy.CPUStableScaleModel.PIDConfig.SignalNormalizedPercent = pointer.Int64(-10000)
				}),
				pod: testPod,
			},
			want:    PodHTRatioPIDSpecialSignalReset,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := system.NewFileTestUtil(t)
			defer helper.Cleanup()
			oldTimeNow := timeNow
			timeNow = func() time.Time {
				return fakeNow
			}
			oldAggregateResultFactory := metriccache.DefaultAggregateResultFactory
			defer func() {
				timeNow = oldTimeNow
				metriccache.DefaultAggregateResultFactory = oldAggregateResultFactory
			}()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			h := &htRatioPIDController{
				metricCache:        tt.fields.metricCache(ctrl),
				runInterval:        tt.fields.runInterval,
				startTime:          &fakeStartTime,
				podControllerCache: gocache.New(120*time.Second, 120*time.Second),
			}
			if tt.fields.mutateControllerCache != nil {
				tt.fields.mutateControllerCache(h.podControllerCache)
			}
			got, gotErr := h.GetSignal(tt.args.pod, tt.args.podStrategy)
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
			assert.LessOrEqual(t, tt.want-0.001, got)
			assert.GreaterOrEqual(t, tt.want+0.001, got)
		})
	}
}

func Test_htRatioPIDController_GetResourceUpdaters(t *testing.T) {
	type fields struct {
		reader    resourceexecutor.CgroupReaderAnolis
		prepareFn func(helper *system.FileTestUtil)
	}
	type args struct {
		strategy    *unified.CPUStableStrategy
		podMeta     *statesinformer.PodMeta
		op          PodControlSignal
		warmupState WarmupState
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []resourceexecutor.ResourceUpdater
		wantErr bool
	}{
		{
			name: "do nothing for remain operation",
			args: args{
				op: PodHTRatioPIDSpecialSignalRemain,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "failed to get current ht_ratio on cgroup-v1",
			fields: fields{
				reader: &resourceexecutor.CgroupV1ReaderAnolis{},
			},
			args: args{
				podMeta: &statesinformer.PodMeta{},
				op:      PodHTRatioPIDSpecialSignalReset,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "throttle ht_ratio successfully",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "100")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioZoomOutPercent: pointer.Int64(90),
						HTRatioUpperBound:     pointer.Int64(200),
						HTRatioLowerBound:     pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioPIDSignal(10.0),
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 110, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 110, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 110, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "throttle ht_ratio and capped by upper bound",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "120")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "120")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "120")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioZoomOutPercent: pointer.Int64(90),
						HTRatioUpperBound:     pointer.Int64(120),
						HTRatioLowerBound:     pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioPIDSignal(10.0),
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 120, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 120, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 120, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "throttle ht_ratio successfully 1",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "195")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "195")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "195")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioPIDSignal(8),
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 200, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 200, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 200, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "relax ht_ratio successfully",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "120")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "120")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "120")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioZoomOutPercent: pointer.Int64(90),
						HTRatioUpperBound:     pointer.Int64(200),
						HTRatioLowerBound:     pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioPIDSignal(-10.0),
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 110, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 110, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 110, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "relax ht_ratio and capped by lower bound",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "120")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "120")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "120")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioZoomOutPercent: pointer.Int64(90),
						HTRatioUpperBound:     pointer.Int64(120),
						HTRatioLowerBound:     pointer.Int64(115),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioPIDSignal(-10.0),
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 115, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 115, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 115, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "relax ht_ratio successfully with fix ratio",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "200")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "200")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "200")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioPIDSpecialSignalRelax,
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 180, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 180, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 180, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "reset ht_ratio successfully",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "150")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "150")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "150")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyNone),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioZoomOutPercent: pointer.Int64(90),
						HTRatioUpperBound:     pointer.Int64(200),
						HTRatioLowerBound:     pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioPIDSpecialSignalReset,
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 100, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 100, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 100, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "warm up ht_ratio successfully",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "100")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyNone),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioPIDSpecialSignalInit,
				warmupState: &htRatioWarmupState{
					WarmupHTRatio: 120,
				},
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 120, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 120, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 120, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "skip since no warmupState",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "100")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioPIDSpecialSignalInit,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "unknown operation",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "100")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyIgnore),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: nil,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := system.NewFileTestUtil(t)
			defer helper.Cleanup()
			if tt.fields.prepareFn != nil {
				tt.fields.prepareFn(helper)
			}
			h := &htRatioPIDController{
				reader: tt.fields.reader,
			}
			got, gotErr := h.GetResourceUpdaters(tt.args.podMeta, tt.args.op, tt.args.strategy, tt.args.warmupState)
			assert.Equal(t, len(tt.want), len(got))
			for i := range tt.want {
				assertEqualCgroupUpdater(t, tt.want[i], got[i])
			}
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
		})
	}
}

func Test_htRatioAIMDController_GetSignal(t *testing.T) {
	fakeNow := time.Now()
	fakeStartTime := fakeNow.Add(-20 * time.Minute)
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			UID:  "xxxxxx",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test-container",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("8Gi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("8Gi"),
						},
					},
				},
				{
					Name: "test-sidecar",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  unified.EnvSigmaIgnoreResource,
							Value: "true",
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "test-container",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: metav1.Time{Time: fakeNow.Add(-10 * time.Minute)},
						},
					},
				},
				{
					Name: "test-sidecar",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: metav1.Time{Time: fakeNow.Add(-12 * time.Minute)},
						},
					},
				},
			},
		},
	}
	type fields struct {
		metricCache func(ctrl *gomock.Controller) metriccache.MetricCache
		runInterval time.Duration
	}
	type args struct {
		podStrategy *unified.CPUStableStrategy
		pod         *corev1.Pod
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    PodControlSignal
		wantErr bool
	}{
		{
			name: "reset if strategy disables",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					return mockmetriccache.NewMockMetricCache(ctrl)
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyNone),
				},
				pod: testPod,
			},
			want:    PodHTRatioAIMDSignalReset,
			wantErr: false,
		},
		{
			name: "remain if strategy ignores",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					return mockmetriccache.NewMockMetricCache(ctrl)
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyIgnore),
				},
				pod: testPod,
			},
			want:    PodHTRatioAIMDSignalRemain,
			wantErr: false,
		},
		{
			name: "failed to query pod cpu satisfaction metrics",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mc := mockmetriccache.NewMockMetricCache(ctrl)
					mc.EXPECT().Querier(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("expected error"))
					return mc
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
				}),
				pod: testPod,
			},
			want:    PodHTRatioAIMDSignalRemain,
			wantErr: true,
		},
		{
			name: "failed to collect enough pod cpu satisfaction metrics",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(1)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(0)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
				}),
				pod: testPod,
			},
			want:    PodHTRatioAIMDSignalRemain,
			wantErr: true,
		},
		{
			name: "failed to aggregate pod cpu satisfaction metrics",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(1)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(1)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult)
					mockAggregateSatisfactionResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(0.0, fmt.Errorf("expected error"))
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
				}),
				pod: testPod,
			},
			want:    PodHTRatioAIMDSignalRemain,
			wantErr: true,
		},
		{
			name: "failed to collect pod cpu satisfaction metrics",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(2)
					queryMetaForUsage, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateUsageResult.EXPECT().Count().Return(0)
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(1)
					mockAggregateSatisfactionResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(0.83, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForUsage).Return(mockAggregateUsageResult).Times(1)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult).Times(1)
					mockQuerier.EXPECT().Query(queryMetaForUsage, nil, mockAggregateUsageResult).Return(nil).Times(1)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil).Times(1)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.CPUUtilMinPercent = pointer.Int64(50)
					strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent = pointer.Int64(80)
					strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill = pointer.Int64(50)
				}),
				pod: testPod,
			},
			want:    PodHTRatioAIMDSignalRemain,
			wantErr: true,
		},
		{
			name: "aborted for unlimited pod",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
				}),
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod",
						UID:  "xxxxxx",
					},
				},
			},
			want:    PodHTRatioAIMDSignalRemain,
			wantErr: false,
		},
		{
			name: "relax for pod whose cpu usage is too low",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(2)
					queryMetaForUsage, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateResult.EXPECT().Count().Return(1)
					mockAggregateResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(1.0, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForUsage).Return(mockAggregateResult).Times(1)
					mockQuerier.EXPECT().Query(queryMetaForUsage, nil, mockAggregateResult).Return(nil).Times(1)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(1)
					mockAggregateSatisfactionResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(0.83, nil)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult).Times(1)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil).Times(1)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.CPUUtilMinPercent = pointer.Int64(80)
				}),
				pod: testPod,
			},
			want:    PodHTRatioAIMDSignalRelax,
			wantErr: false,
		},
		{
			name: "pod cpu satisfaction is too low, need relax",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(2)
					queryMetaForUsage, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateUsageResult.EXPECT().Count().Return(1)
					mockAggregateUsageResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(2.0, nil)
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(1)
					mockAggregateSatisfactionResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(0.65, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForUsage).Return(mockAggregateUsageResult).Times(1)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult).Times(1)
					mockQuerier.EXPECT().Query(queryMetaForUsage, nil, mockAggregateUsageResult).Return(nil).Times(1)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil).Times(1)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.CPUUtilMinPercent = pointer.Int64(50)
					strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent = pointer.Int64(80)
					strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill = pointer.Int64(30)
				}),
				pod: testPod,
			},
			want:    PodHTRatioAIMDSignalRelax,
			wantErr: false,
		},
		{
			name: "pod cpu satisfaction is too high, need throttle",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(2)
					queryMetaForUsage, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateUsageResult.EXPECT().Count().Return(1)
					mockAggregateUsageResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(2.0, nil)
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(1)
					mockAggregateSatisfactionResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(0.95, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForUsage).Return(mockAggregateUsageResult).Times(1)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult).Times(1)
					mockQuerier.EXPECT().Query(queryMetaForUsage, nil, mockAggregateUsageResult).Return(nil).Times(1)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil).Times(1)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.CPUUtilMinPercent = pointer.Int64(50)
					strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent = pointer.Int64(80)
					strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill = pointer.Int64(30)
				}),
				pod: testPod,
			},
			want:    PodHTRatioAIMDSignalThrottle,
			wantErr: false,
		},
		{
			name: "pod cpu satisfaction is just right, keep remain",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(2)
					queryMetaForUsage, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateUsageResult.EXPECT().Count().Return(1)
					mockAggregateUsageResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(2.0, nil)
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(1)
					mockAggregateSatisfactionResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(0.83, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForUsage).Return(mockAggregateUsageResult).Times(1)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult).Times(1)
					mockQuerier.EXPECT().Query(queryMetaForUsage, nil, mockAggregateUsageResult).Return(nil).Times(1)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil).Times(1)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.CPUUtilMinPercent = pointer.Int64(50)
					strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent = pointer.Int64(80)
					strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill = pointer.Int64(50)
				}),
				pod: testPod,
			},
			want:    PodHTRatioAIMDSignalRemain,
			wantErr: false,
		},
		{
			name: "pod is initializing, use the warmup config",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodInitializeWindowSeconds = pointer.Int64(60)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(100)
					strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent = pointer.Int64(80)
					strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill = pointer.Int64(50)
				}),
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod",
						UID:  "xxxxxx",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "test-container",
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("2"),
										corev1.ResourceMemory: resource.MustParse("8Gi"),
									},
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("2"),
										corev1.ResourceMemory: resource.MustParse("8Gi"),
									},
								},
							},
							{
								Name: "test-sidecar",
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("2Gi"),
									},
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("2Gi"),
									},
								},
								Env: []corev1.EnvVar{
									{
										Name:  unified.EnvSigmaIgnoreResource,
										Value: "true",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name: "test-container",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: fakeNow.Add(-10 * time.Second)},
									},
								},
							},
							{
								Name: "test-sidecar",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: fakeNow.Add(-30 * time.Minute)},
									},
								},
							},
						},
					},
				},
			},
			want:    PodHTRatioAIMDSignalInit,
			wantErr: false,
		},
		{
			name: "pod is degrading, need reset",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-15*time.Second), fakeNow).Return(mockQuerier, nil).Times(1)
					queryMeta, err := metriccache.PodCPUSatisfactionWithThrottleMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod(string("xxxxxx")))
					assert.NoError(t, err)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateSatisfactionResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResult.EXPECT().Count().Return(0)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResult).Times(1)
					mockQuerier.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResult).Return(nil).Times(1)
					mockQuerierForDegrade := mockmetriccache.NewMockQuerier(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-300*time.Second), fakeNow).Return(mockQuerierForDegrade, nil).Times(1)
					mockAggregateSatisfactionResultForDegrade := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateSatisfactionResultForDegrade.EXPECT().Count().Return(0)
					mockAggregateResultFactory.EXPECT().New(queryMeta).Return(mockAggregateSatisfactionResultForDegrade).Times(1)
					mockQuerierForDegrade.EXPECT().Query(queryMeta, nil, mockAggregateSatisfactionResultForDegrade).Return(nil).Times(1)
					return mockMetricCache
				},
				runInterval: 5 * time.Second,
			},
			args: args{
				podStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
					strategy.PodMetricWindowSeconds = pointer.Int64(15)
					strategy.ValidMetricPointsPercent = pointer.Int64(0)
					strategy.PodInitializeWindowSeconds = pointer.Int64(0)
					strategy.PodDegradeWindowSeconds = pointer.Int64(300)
				}),
				pod: testPod,
			},
			want:    PodHTRatioAIMDSignalReset,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := system.NewFileTestUtil(t)
			defer helper.Cleanup()
			oldTimeNow := timeNow
			timeNow = func() time.Time {
				return fakeNow
			}
			oldAggregateResultFactory := metriccache.DefaultAggregateResultFactory
			defer func() {
				timeNow = oldTimeNow
				metriccache.DefaultAggregateResultFactory = oldAggregateResultFactory
			}()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			h := &htRatioAIMDController{
				metricCache: tt.fields.metricCache(ctrl),
				runInterval: tt.fields.runInterval,
				startTime:   &fakeStartTime,
			}
			got, gotErr := h.GetSignal(tt.args.pod, tt.args.podStrategy)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
		})
	}
}

func Test_htRatioAIMDController_GetResourceUpdaters(t *testing.T) {
	type fields struct {
		reader    resourceexecutor.CgroupReaderAnolis
		prepareFn func(helper *system.FileTestUtil)
	}
	type args struct {
		strategy    *unified.CPUStableStrategy
		podMeta     *statesinformer.PodMeta
		op          PodHTRatioAIMDSignal
		warmupState WarmupState
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []resourceexecutor.ResourceUpdater
		wantErr bool
	}{
		{
			name: "do nothing for remain operation",
			args: args{
				op: PodHTRatioAIMDSignalRemain,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "failed to get current ht_ratio on cgroup-v1",
			fields: fields{
				reader: &resourceexecutor.CgroupV1ReaderAnolis{},
			},
			args: args{
				podMeta: &statesinformer.PodMeta{},
				op:      PodHTRatioAIMDSignalThrottle,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "throttle ht_ratio successfully",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "100")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioAIMDSignalThrottle,
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 110, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 110, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 110, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "throttle ht_ratio and capped by upper bound",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "120")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "120")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "120")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(120),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioAIMDSignalThrottle,
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 120, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 120, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 120, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "throttle ht_ratio successfully 1",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "195")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "195")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "195")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioAIMDSignalThrottle,
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 200, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 200, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 200, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "relax ht_ratio successfully",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "150")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "150")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "150")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioAIMDSignalRelax,
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 135, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 135, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 135, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "relax ht_ratio successfully 1",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "110")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "110")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "110")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioAIMDSignalRelax,
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 100, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 100, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 100, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "relax ht_ratio and capped by lower bound",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "120")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "120")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "120")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(110),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioAIMDSignalRelax,
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 110, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 110, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 110, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "reset ht_ratio successfully",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "150")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "150")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "150")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyNone),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioAIMDSignalReset,
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 100, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 100, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 100, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "warm up ht_ratio successfully",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "100")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyNone),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioAIMDSignalInit,
				warmupState: &htRatioWarmupState{
					WarmupHTRatio: 120,
				},
			},
			want: []resourceexecutor.ResourceUpdater{
				getCPUHTRatioUpdater(t, "kubepods/podxxx", 120, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/abc123", 120, "", "test-pod"),
				getCPUHTRatioUpdater(t, "kubepods/podxxx/efg456", 120, "", "test-pod"),
			},
			wantErr: false,
		},
		{
			name: "skip since no warmupState",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "100")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: PodHTRatioAIMDSignalInit,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "unknown operation",
			fields: fields{
				reader: &resourceexecutor.CgroupV2ReaderAnolis{},
				prepareFn: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents("kubepods/podxxx", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/abc123", system.CPUHTRatioV2Anolis, "100")
					helper.WriteCgroupFileContents("kubepods/podxxx/efg456", system.CPUHTRatioV2Anolis, "100")
				},
			},
			args: args{
				strategy: &unified.CPUStableStrategy{
					Policy: sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyIgnore),
					CPUStableScaleModel: unified.CPUStableScaleModel{
						HTRatioIncreasePercent: pointer.Int64(10),
						HTRatioZoomOutPercent:  pointer.Int64(90),
						HTRatioUpperBound:      pointer.Int64(200),
						HTRatioLowerBound:      pointer.Int64(100),
					},
				},
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-pod",
						},
					},
					CgroupDir: "kubepods/podxxx",
				},
				op: -1,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := system.NewFileTestUtil(t)
			defer helper.Cleanup()
			if tt.fields.prepareFn != nil {
				tt.fields.prepareFn(helper)
			}
			h := &htRatioAIMDController{
				reader: tt.fields.reader,
			}
			got, gotErr := h.GetResourceUpdaters(tt.args.podMeta, tt.args.op, tt.args.strategy, tt.args.warmupState)
			assert.Equal(t, len(tt.want), len(got))
			for i := range tt.want {
				assertEqualCgroupUpdater(t, tt.want[i], got[i])
			}
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
		})
	}
}

func Test_htRatioAIMDController_GetWarmupState(t *testing.T) {
	fakeNow := time.Now()
	testPodMetas := []*statesinformer.PodMeta{
		{
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ls-pod",
					UID:  "xxxxxx",
					Labels: map[string]string{
						extension.LabelPodQoS: string(extension.QoSLS),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
						},
						{
							Name: "test-sidecar",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  unified.EnvSigmaIgnoreResource,
									Value: "true",
								},
							},
						},
					},
				},
			},
		},
		{
			Pod: &corev1.Pod{ // non-running
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-lse-pod",
					UID:  "yyyyyy",
					Labels: map[string]string{
						extension.LabelPodQoS: string(extension.QoSLSE),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
								},
							},
						},
					},
				},
			},
		},
		{
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-be-pod",
					UID:  "zzzzzz",
					Labels: map[string]string{
						extension.LabelPodQoS: string(extension.QoSBE),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									extension.BatchCPU:    resource.MustParse("8000"),
									extension.BatchMemory: resource.MustParse("8Gi"),
								},
								Requests: corev1.ResourceList{
									extension.BatchCPU:    resource.MustParse("8000"),
									extension.BatchMemory: resource.MustParse("8Gi"),
								},
							},
						},
					},
				},
			},
		},
	}
	type fields struct {
		statesInformer func(ctrl *gomock.Controller) statesinformer.StatesInformer
		metricCache    func(ctrl *gomock.Controller) metriccache.MetricCache
		prepareFn      func() func()
	}
	type args struct {
		podMetas []*statesinformer.PodMeta
		strategy *unified.CPUStableStrategy
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    WarmupState
		wantErr bool
	}{
		{
			name: "get node warmup state successfully",
			fields: fields{
				prepareFn: func() func() {
					oldTimeNow := timeNow
					timeNow = func() time.Time {
						return fakeNow
					}
					oldAggregateResultFactory := metriccache.DefaultAggregateResultFactory
					return func() {
						timeNow = oldTimeNow
						metriccache.DefaultAggregateResultFactory = oldAggregateResultFactory
					}
				},
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					// node usage = 10.0
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					queryMetaForNodeUsage, err := metriccache.NodeCPUUsageMetric.BuildQueryMeta(nil)
					assert.NoError(t, err)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-30*time.Second), fakeNow).Return(mockQuerier, nil).Times(4)
					mockAggregateNodeUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateNodeUsageResult.EXPECT().Count().Return(3)
					mockAggregateNodeUsageResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(10.0, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForNodeUsage).Return(mockAggregateNodeUsageResult)
					mockQuerier.EXPECT().Query(queryMetaForNodeUsage, nil, mockAggregateNodeUsageResult).Return(nil)
					// ls pod, usage = 2.0
					queryMetaForLSPod, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					mockAggregateResultForLSPod := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateResultForLSPod.EXPECT().Count().Return(3)
					mockAggregateResultForLSPod.EXPECT().Value(metriccache.AggregationTypeAVG).Return(2.0, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForLSPod).Return(mockAggregateResultForLSPod)
					mockQuerier.EXPECT().Query(queryMetaForLSPod, nil, mockAggregateResultForLSPod).Return(nil)
					// lse pod, usage is missing
					queryMetaForLSEPod, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("yyyyyy"))
					assert.NoError(t, err)
					mockAggregateResultForLSEPod := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateResultForLSEPod.EXPECT().Count().Return(0)
					mockAggregateResultFactory.EXPECT().New(queryMetaForLSEPod).Return(mockAggregateResultForLSEPod)
					mockQuerier.EXPECT().Query(queryMetaForLSEPod, nil, mockAggregateResultForLSEPod).Return(nil)
					// be pod, usage = 8.0, scaled usage = 4.0
					queryMetaForBEPod, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("zzzzzz"))
					assert.NoError(t, err)
					mockAggregateResultForBEPod := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateResultForBEPod.EXPECT().Count().Return(2)
					mockAggregateResultForBEPod.EXPECT().Value(metriccache.AggregationTypeAVG).Return(8.0, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForBEPod).Return(mockAggregateResultForBEPod)
					mockQuerier.EXPECT().Query(queryMetaForBEPod, nil, mockAggregateResultForBEPod).Return(nil)
					// ls share pool usage = 6.0, util = 37.5%
					return mockMetricCache
				},
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					mockStatesInformer := mockstatesinformer.NewMockStatesInformer(ctrl)
					mockStatesInformer.EXPECT().GetAllPods().Return(testPodMetas)
					// be scale ratio = 0.5
					mockStatesInformer.EXPECT().GetNodeTopo().Return(&topologyv1alpha1.NodeResourceTopology{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-node",
							Annotations: map[string]string{
								extension.AnnotationNodeCPUSharedPools:   `[{"node": 0, "socket": 0, "cpuset": "8-15"}, {"node": 1, "socket": 0, "cpuset": "24-31"}]`,
								extension.AnnotationNodeBECPUSharedPools: `[{"node": 0, "socket": 0, "cpuset": "0-15"}, {"node": 1, "socket": 0, "cpuset": "16-31"}]`,
							},
						},
					})
					return mockStatesInformer
				},
			},
			args: args{
				podMetas: testPodMetas,
				strategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.PodMetricWindowSeconds = pointer.Int64(30)
					strategy.WarmupConfig = unified.CPUStableWarmupModel{
						Entries: []unified.CPUStableWarmupEntry{
							{
								SharePoolUtilPercent: pointer.Int64(0),
								HTRatio:              pointer.Int64(150),
							},
							{
								SharePoolUtilPercent: pointer.Int64(20),
								HTRatio:              pointer.Int64(120),
							},
							{
								SharePoolUtilPercent: pointer.Int64(40),
								HTRatio:              pointer.Int64(100),
							},
						},
					}
				}),
			},
			want: &htRatioWarmupState{
				WarmupHTRatio: 120,
			},
			wantErr: false,
		},
		{
			name: "skipped for no warmup strategy",
			fields: fields{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					return mockmetriccache.NewMockMetricCache(ctrl)
				},
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					return mockstatesinformer.NewMockStatesInformer(ctrl)
				},
			},
			args: args{
				podMetas: testPodMetas,
				strategy: &unified.CPUStableStrategy{},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "failed to get valid sharepool size",
			fields: fields{
				prepareFn: func() func() {
					oldTimeNow := timeNow
					timeNow = func() time.Time {
						return fakeNow
					}
					oldAggregateResultFactory := metriccache.DefaultAggregateResultFactory
					return func() {
						timeNow = oldTimeNow
						metriccache.DefaultAggregateResultFactory = oldAggregateResultFactory
					}
				},
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					return mockMetricCache
				},
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					mockStatesInformer := mockstatesinformer.NewMockStatesInformer(ctrl)
					mockStatesInformer.EXPECT().GetNodeTopo().Return(&topologyv1alpha1.NodeResourceTopology{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-node",
							Annotations: map[string]string{
								extension.AnnotationNodeCPUSharedPools:   `[]`,
								extension.AnnotationNodeBECPUSharedPools: `[]`,
							},
						},
					})
					return mockStatesInformer
				},
			},
			args: args{
				podMetas: testPodMetas,
				strategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.PodMetricWindowSeconds = pointer.Int64(30)
					strategy.WarmupConfig = unified.CPUStableWarmupModel{
						Entries: []unified.CPUStableWarmupEntry{
							{
								SharePoolUtilPercent: pointer.Int64(0),
								HTRatio:              pointer.Int64(150),
							},
							{
								SharePoolUtilPercent: pointer.Int64(20),
								HTRatio:              pointer.Int64(120),
							},
							{
								SharePoolUtilPercent: pointer.Int64(40),
								HTRatio:              pointer.Int64(100),
							},
						},
					}
				}),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			if tt.fields.prepareFn != nil {
				cleanupFn := tt.fields.prepareFn()
				defer cleanupFn()
			}
			h := &htRatioAIMDController{
				metricCache:    tt.fields.metricCache(ctrl),
				statesInformer: tt.fields.statesInformer(ctrl),
			}
			got, gotErr := h.GetWarmupState(nil, tt.args.podMetas, tt.args.strategy)
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_htRatioAIMDController_ValidateStrategy(t *testing.T) {
	tests := []struct {
		name    string
		arg     *unified.CPUStableStrategy
		wantErr error
	}{
		{
			name:    "default strategy is valid",
			arg:     sloconfig.DefaultCPUStableStrategy(),
			wantErr: nil,
		},
		{
			name:    "unexpected nil strategy",
			arg:     nil,
			wantErr: fmt.Errorf("strategy is nil"),
		},
		{
			name: "no warmup config is valid",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.WarmupConfig = unified.CPUStableWarmupModel{}
			}),
			wantErr: nil,
		},
		{
			name: "missing strategy policy",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.Policy = nil
			}),
			wantErr: fmt.Errorf("strategy policy is nil"),
		},
		{
			name: "missing PodMetricWindowSeconds",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.PodMetricWindowSeconds = nil
			}),
			wantErr: fmt.Errorf("PodMetricWindowSeconds is nil"),
		},
		{
			name: "illegal PodMetricWindowSeconds",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.PodMetricWindowSeconds = pointer.Int64(-10)
			}),
			wantErr: fmt.Errorf("PodMetricWindowSeconds is invalid, -10"),
		},
		{
			name: "missing ValidMetricPointsPercent",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.ValidMetricPointsPercent = nil
			}),
			wantErr: fmt.Errorf("ValidMetricPointsPercent is nil"),
		},
		{
			name: "illegal ValidMetricPointsPercent",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.ValidMetricPointsPercent = pointer.Int64(200)
			}),
			wantErr: fmt.Errorf("ValidMetricPointsPercent is invalid, 200"),
		},
		{
			name: "missing TargetCPUSatisfactionPercent",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent = nil
			}),
			wantErr: fmt.Errorf("TargetCPUSatisfactionPercent is nil"),
		},
		{
			name: "illegal TargetCPUSatisfactionPercent",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.CPUStableScaleModel.TargetCPUSatisfactionPercent = pointer.Int64(150)
			}),
			wantErr: fmt.Errorf("TargetCPUSatisfactionPercent is invalid, 150"),
		},
		{
			name: "missing CPUSatisfactionEpsilonPermill",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill = nil
			}),
			wantErr: fmt.Errorf("CPUSatisfactionEpsilonPermill is nil"),
		},
		{
			name: "illegal CPUSatisfactionEpsilonPermill",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.CPUStableScaleModel.CPUSatisfactionEpsilonPermill = pointer.Int64(-10)
			}),
			wantErr: fmt.Errorf("CPUSatisfactionEpsilonPermill is invalid, -10"),
		},
		{
			name: "missing HTRatioIncreasePercent",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.CPUStableScaleModel.HTRatioIncreasePercent = nil
			}),
			wantErr: fmt.Errorf("HTRatioIncreasePercent is nil"),
		},
		{
			name: "illegal HTRatioIncreasePercent",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.CPUStableScaleModel.HTRatioIncreasePercent = pointer.Int64(-10)
			}),
			wantErr: fmt.Errorf("HTRatioIncreasePercent is invalid, -10"),
		},
		{
			name: "missing HTRatioZoomOutPercent",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.CPUStableScaleModel.HTRatioZoomOutPercent = nil
			}),
			wantErr: fmt.Errorf("HTRatioZoomOutPercent is nil"),
		},
		{
			name: "illegal HTRatioZoomOutPercent",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.CPUStableScaleModel.HTRatioZoomOutPercent = pointer.Int64(200)
			}),
			wantErr: fmt.Errorf("HTRatioZoomOutPercent is invalid, 200"),
		},
		{
			name: "invalid warmup config since missing sharepool util",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.WarmupConfig.Entries = []unified.CPUStableWarmupEntry{
					{
						HTRatio: pointer.Int64(100),
					},
					{
						HTRatio: pointer.Int64(150),
					},
				}
			}),
			wantErr: fmt.Errorf("warmup config entry 0 is invalid, SharePoolUtilPercent is nil"),
		},
		{
			name: "invalid warmup config since missing ht ratio",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.WarmupConfig.Entries = []unified.CPUStableWarmupEntry{
					{
						SharePoolUtilPercent: pointer.Int64(0),
					},
					{
						SharePoolUtilPercent: pointer.Int64(40),
					},
				}
			}),
			wantErr: fmt.Errorf("warmup config entry 0 is invalid, HTRatio is nil"),
		},
		{
			name: "invalid warmup config since sharepool utils out of order",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.WarmupConfig.Entries = []unified.CPUStableWarmupEntry{
					{
						SharePoolUtilPercent: pointer.Int64(40),
						HTRatio:              pointer.Int64(100),
					},
					{
						SharePoolUtilPercent: pointer.Int64(0),
						HTRatio:              pointer.Int64(150),
					},
				}
			}),
			wantErr: fmt.Errorf("warmup config entry 1 is invalid, illegal SharePoolUtilPercent 0"),
		},
		{
			name: "invalid warmup config since ht ratio is illegal",
			arg: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.WarmupConfig.Entries = []unified.CPUStableWarmupEntry{
					{
						SharePoolUtilPercent: pointer.Int64(0),
						HTRatio:              pointer.Int64(100),
					},
					{
						SharePoolUtilPercent: pointer.Int64(40),
						HTRatio:              pointer.Int64(50),
					},
				}
			}),
			wantErr: fmt.Errorf("warmup config entry 1 is invalid, illegal HTRatio 50"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &htRatioAIMDController{}
			gotErr := h.ValidateStrategy(tt.arg)
			assert.Equal(t, tt.wantErr, gotErr)
		})
	}
}

type nopPodControlSignal struct{}

func (n *nopPodControlSignal) String() string {
	return ""
}

type fakePodStableController struct {
	ValidateStrategyErr bool

	W                 WarmupState
	GetWarmupStateErr bool

	PodSignals   map[string]PodControlSignal
	PodSignalErr map[string]bool

	PodResourceUpdaters    map[string][]resourceexecutor.ResourceUpdater
	PodResourceUpdatersErr map[string]bool
}

func (f *fakePodStableController) Name() string {
	return "fakePodStableController"
}

func (f *fakePodStableController) ValidateStrategy(strategy *unified.CPUStableStrategy) error {
	if f.ValidateStrategyErr {
		return fmt.Errorf("expected error")
	}
	return nil
}

func (f *fakePodStableController) GetWarmupState(node *corev1.Node, podMetas []*statesinformer.PodMeta, strategy *unified.CPUStableStrategy) (WarmupState, error) {
	if f.GetWarmupStateErr {
		return nil, fmt.Errorf("expected error")
	}
	return f.W, nil
}

func (f *fakePodStableController) GetSignal(pod *corev1.Pod, strategy *unified.CPUStableStrategy) (PodControlSignal, error) {
	if f.PodSignalErr != nil && f.PodSignalErr[util.GetPodKey(pod)] {
		return nil, fmt.Errorf("expected error")
	}
	if f.PodSignals != nil && f.PodSignals[util.GetPodKey(pod)] != nil {
		return f.PodSignals[util.GetPodKey(pod)], nil
	}
	return &nopPodControlSignal{}, nil
}

func (f *fakePodStableController) GetResourceUpdaters(podMeta *statesinformer.PodMeta, signal PodControlSignal, strategy *unified.CPUStableStrategy, warmupState WarmupState) ([]resourceexecutor.ResourceUpdater, error) {
	if f.PodResourceUpdatersErr != nil && f.PodResourceUpdatersErr[podMeta.Key()] {
		return nil, fmt.Errorf("expected error")
	}
	if f.PodResourceUpdaters != nil && f.PodResourceUpdaters[podMeta.Key()] != nil {
		return f.PodResourceUpdaters[podMeta.Key()], nil
	}
	return nil, nil
}
