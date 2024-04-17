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
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	mockmetriccache "github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache/mockmetriccache"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	mockstatesinformer "github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer/mockstatesinformer"
)

func Test_getCPUSharePoolSize(t *testing.T) {
	type args struct {
		statesInformer func(ctrl *gomock.Controller) statesinformer.StatesInformer
	}
	tests := []struct {
		name    string
		args    args
		want    int
		want1   int
		wantErr bool
	}{
		{
			name: "failed to get node topology",
			args: args{
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					s := mockstatesinformer.NewMockStatesInformer(ctrl)
					s.EXPECT().GetNodeTopo().Return(nil)
					return s
				},
			},
			want:    -1,
			want1:   -1,
			wantErr: true,
		},
		{
			name: "failed to parse ls sharepool",
			args: args{
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					s := mockstatesinformer.NewMockStatesInformer(ctrl)
					s.EXPECT().GetNodeTopo().Return(&topologyv1alpha1.NodeResourceTopology{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-node",
							Annotations: map[string]string{
								extension.AnnotationNodeCPUSharedPools:   "invalidContent",
								extension.AnnotationNodeBECPUSharedPools: `[{"node": 0, "socket": 0, "cpuset": "0-15"}]`,
							},
						},
					})
					return s
				},
			},
			want:    -1,
			want1:   -1,
			wantErr: true,
		},
		{
			name: "failed to parse be sharepool",
			args: args{
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					s := mockstatesinformer.NewMockStatesInformer(ctrl)
					s.EXPECT().GetNodeTopo().Return(&topologyv1alpha1.NodeResourceTopology{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-node",
							Annotations: map[string]string{
								extension.AnnotationNodeCPUSharedPools:   `[{"node": 0, "socket": 0, "cpuset": "0-15"}]`,
								extension.AnnotationNodeBECPUSharedPools: "invalidContent",
							},
						},
					})
					return s
				},
			},
			want:    -1,
			want1:   -1,
			wantErr: true,
		},
		{
			name: "parse sharepool correctly",
			args: args{
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					s := mockstatesinformer.NewMockStatesInformer(ctrl)
					s.EXPECT().GetNodeTopo().Return(&topologyv1alpha1.NodeResourceTopology{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-node",
							Annotations: map[string]string{
								extension.AnnotationNodeCPUSharedPools:   `[{"node": 0, "socket": 0, "cpuset": "0-15"}]`,
								extension.AnnotationNodeBECPUSharedPools: `[{"node": 0, "socket": 0, "cpuset": "0-15"}]`,
							},
						},
					})
					return s
				},
			},
			want:    16,
			want1:   16,
			wantErr: false,
		},
		{
			name: "parse sharepool correctly 1",
			args: args{
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					s := mockstatesinformer.NewMockStatesInformer(ctrl)
					s.EXPECT().GetNodeTopo().Return(&topologyv1alpha1.NodeResourceTopology{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-node",
							Annotations: map[string]string{
								extension.AnnotationNodeCPUSharedPools:   `[{"node": 0, "socket": 0, "cpuset": "2-31"}, {"node": 1, "socket": 0, "cpuset": "34-63"}]`,
								extension.AnnotationNodeBECPUSharedPools: `[{"node": 0, "socket": 0, "cpuset": "0-31"}, {"node": 1, "socket": 0, "cpuset": "32-63"}]`,
							},
						},
					})
					return s
				},
			},
			want:    60,
			want1:   64,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			got, got1, gotErr := getCPUSharePoolSize(tt.args.statesInformer(ctrl))
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want1, got1)
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
		})
	}
}

func Test_calculateCPUSharePoolUsage(t *testing.T) {
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
		prepareFn func() func()
	}
	type args struct {
		statesInformer func(ctrl *gomock.Controller) statesinformer.StatesInformer
		metricCache    func(ctrl *gomock.Controller) metriccache.MetricCache
		podMetas       []*statesinformer.PodMeta
		queryWindow    time.Duration
		beScaleRatio   float64
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    float64
		wantErr bool
	}{
		{
			name: "calculate cpu sharepool usage correctly",
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
			},
			args: args{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					queryMetaForNodeUsage, err := metriccache.NodeCPUUsageMetric.BuildQueryMeta(nil)
					assert.NoError(t, err)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-30*time.Second), fakeNow).Return(mockQuerier, nil)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateNodeUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateNodeUsageResult.EXPECT().Count().Return(1)
					mockAggregateNodeUsageResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(10.0, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForNodeUsage).Return(mockAggregateNodeUsageResult)
					mockQuerier.EXPECT().QueryAndClose(queryMetaForNodeUsage, nil, mockAggregateNodeUsageResult).Return(nil)
					return mockMetricCache
				},
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					mockStatesInformer := mockstatesinformer.NewMockStatesInformer(ctrl)
					mockStatesInformer.EXPECT().GetAllPods().Return(nil)
					return mockStatesInformer
				},
				podMetas:     nil,
				queryWindow:  30 * time.Second,
				beScaleRatio: 1.0,
			},
			want:    10.0,
			wantErr: false,
		},
		{
			name: "calculate cpu sharepool usage correctly with multiple pods",
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
			},
			args: args{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					// node usage
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					queryMetaForNodeUsage, err := metriccache.NodeCPUUsageMetric.BuildQueryMeta(nil)
					assert.NoError(t, err)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-30*time.Second), fakeNow).Return(mockQuerier, nil).Times(4)
					mockAggregateNodeUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateNodeUsageResult.EXPECT().Count().Return(3)
					mockAggregateNodeUsageResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(10.0, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForNodeUsage).Return(mockAggregateNodeUsageResult)
					mockQuerier.EXPECT().QueryAndClose(queryMetaForNodeUsage, nil, mockAggregateNodeUsageResult).Return(nil)
					// ls pod, usage = 2.0
					queryMetaForLSPod, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
					assert.NoError(t, err)
					mockAggregateResultForLSPod := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateResultForLSPod.EXPECT().Count().Return(3)
					mockAggregateResultForLSPod.EXPECT().Value(metriccache.AggregationTypeAVG).Return(2.0, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForLSPod).Return(mockAggregateResultForLSPod)
					mockQuerier.EXPECT().QueryAndClose(queryMetaForLSPod, nil, mockAggregateResultForLSPod).Return(nil)
					// lse pod, usage is missing
					queryMetaForLSEPod, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("yyyyyy"))
					assert.NoError(t, err)
					mockAggregateResultForLSEPod := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateResultForLSEPod.EXPECT().Count().Return(0)
					mockAggregateResultFactory.EXPECT().New(queryMetaForLSEPod).Return(mockAggregateResultForLSEPod)
					mockQuerier.EXPECT().QueryAndClose(queryMetaForLSEPod, nil, mockAggregateResultForLSEPod).Return(nil)
					// be pod, usage = 6.0
					queryMetaForBEPod, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("zzzzzz"))
					assert.NoError(t, err)
					mockAggregateResultForBEPod := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateResultForBEPod.EXPECT().Count().Return(2)
					mockAggregateResultForBEPod.EXPECT().Value(metriccache.AggregationTypeAVG).Return(5.0, nil)
					mockAggregateResultFactory.EXPECT().New(queryMetaForBEPod).Return(mockAggregateResultForBEPod)
					mockQuerier.EXPECT().QueryAndClose(queryMetaForBEPod, nil, mockAggregateResultForBEPod).Return(nil)
					return mockMetricCache
				},
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					mockStatesInformer := mockstatesinformer.NewMockStatesInformer(ctrl)
					mockStatesInformer.EXPECT().GetAllPods().Return(testPodMetas)
					return mockStatesInformer
				},
				podMetas:     testPodMetas,
				queryWindow:  30 * time.Second,
				beScaleRatio: 0.8,
			},
			want:    6.0,
			wantErr: false,
		},
		{
			name: "failed to build querier for node usage",
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
			},
			args: args{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-30*time.Second), fakeNow).Return(nil, fmt.Errorf("expected error"))
					return mockMetricCache
				},
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					mockStatesInformer := mockstatesinformer.NewMockStatesInformer(ctrl)
					return mockStatesInformer
				},
				podMetas:     nil,
				queryWindow:  30 * time.Second,
				beScaleRatio: 1.0,
			},
			want:    -1,
			wantErr: true,
		},
		{
			name: "failed to query node usage",
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
			},
			args: args{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					queryMetaForNodeUsage, err := metriccache.NodeCPUUsageMetric.BuildQueryMeta(nil)
					assert.NoError(t, err)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-30*time.Second), fakeNow).Return(mockQuerier, nil)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateNodeUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateResultFactory.EXPECT().New(queryMetaForNodeUsage).Return(mockAggregateNodeUsageResult)
					mockQuerier.EXPECT().QueryAndClose(queryMetaForNodeUsage, nil, mockAggregateNodeUsageResult).Return(fmt.Errorf("expected error"))
					return mockMetricCache
				},
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					mockStatesInformer := mockstatesinformer.NewMockStatesInformer(ctrl)
					return mockStatesInformer
				},
				podMetas:     nil,
				queryWindow:  30 * time.Second,
				beScaleRatio: 1.0,
			},
			want:    -1,
			wantErr: true,
		},
		{
			name: "failed to find node usage metrics",
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
			},
			args: args{
				metricCache: func(ctrl *gomock.Controller) metriccache.MetricCache {
					mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
					mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
					queryMetaForNodeUsage, err := metriccache.NodeCPUUsageMetric.BuildQueryMeta(nil)
					assert.NoError(t, err)
					mockMetricCache.EXPECT().Querier(fakeNow.Add(-30*time.Second), fakeNow).Return(mockQuerier, nil)
					mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
					metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
					mockAggregateNodeUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
					mockAggregateNodeUsageResult.EXPECT().Count().Return(0)
					mockAggregateResultFactory.EXPECT().New(queryMetaForNodeUsage).Return(mockAggregateNodeUsageResult)
					mockQuerier.EXPECT().QueryAndClose(queryMetaForNodeUsage, nil, mockAggregateNodeUsageResult).Return(nil)
					return mockMetricCache
				},
				statesInformer: func(ctrl *gomock.Controller) statesinformer.StatesInformer {
					mockStatesInformer := mockstatesinformer.NewMockStatesInformer(ctrl)
					return mockStatesInformer
				},
				podMetas:     nil,
				queryWindow:  30 * time.Second,
				beScaleRatio: 1.0,
			},
			want:    -1,
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
			got, gotErr := calculateCPUSharePoolUsage(tt.args.statesInformer(ctrl), tt.args.metricCache(ctrl),
				tt.args.podMetas, tt.args.queryWindow, tt.args.beScaleRatio)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
		})
	}
}

func newTestPIDController(proportionalGain, integralGain, derivativeGain float64, prepareFn func(controller *pidController)) PIDController {
	c := newPIDController(proportionalGain, integralGain, derivativeGain)
	cc := c.(*pidController)
	if prepareFn != nil {
		prepareFn(cc)
	}
	return cc
}

func Test_pidController(t *testing.T) {
	fakeNow := time.Now().Add(-60 * time.Second)
	type fields struct {
		proportionalGain   float64
		integralGain       float64
		derivativeGain     float64
		mutateControllerFn func(c *pidController)
	}
	type args struct {
		actualState float64
		refState    float64
		updateTime  time.Time
		isSkip      bool
	}
	tests := []struct {
		name   string
		fields fields
		args   []args
		wants  []float64
	}{
		{
			name: "P controller, 1 turn",
			fields: fields{
				proportionalGain: 1.0,
				integralGain:     0,
				derivativeGain:   0,
				mutateControllerFn: func(c *pidController) {
					c.DefaultInterval = time.Second.Seconds()
				},
			},
			args: []args{
				{
					actualState: 1.0,
					refState:    0.8,
					updateTime:  fakeNow.Add(time.Second),
				},
			},
			wants: []float64{
				-0.2, // (0.8 - 1.0) * 1.0
			},
		},
		{
			name: "PI controller, 1 turn",
			fields: fields{
				proportionalGain: 0.8,
				integralGain:     0.1,
				derivativeGain:   0,
				mutateControllerFn: func(c *pidController) {
					c.DefaultInterval = time.Second.Seconds()
				},
			},
			args: []args{
				{
					actualState: 1.0,
					refState:    0.8,
					updateTime:  fakeNow.Add(time.Second),
				},
			},
			wants: []float64{
				-0.18, // (0.8 - 1.0) * (0.8 + 0.1)
			},
		},
		{
			name: "PID controller, 1 turn",
			fields: fields{
				proportionalGain: 0.8,
				integralGain:     0.1,
				derivativeGain:   0.05,
				mutateControllerFn: func(c *pidController) {
					c.DefaultInterval = time.Second.Seconds()
				},
			},
			args: []args{
				{
					actualState: 1.0,
					refState:    0.8,
					updateTime:  fakeNow.Add(time.Second),
				},
			},
			wants: []float64{
				-0.18, // (0.8 - 1.0) * (0.8 + 0.1)
			},
		},
		{
			name: "P controller, 5 turns",
			fields: fields{
				proportionalGain: 1.0,
				integralGain:     0,
				derivativeGain:   0,
				mutateControllerFn: func(c *pidController) {
					c.DefaultInterval = time.Second.Seconds()
				},
			},
			args: []args{
				{
					actualState: 1.0,
					refState:    0.8,
					updateTime:  fakeNow.Add(time.Second),
				},
				{
					actualState: 0.9,
					refState:    0.8,
					updateTime:  fakeNow.Add(2 * time.Second),
				},
				{
					actualState: 0.85,
					refState:    0.8,
					updateTime:  fakeNow.Add(3 * time.Second),
				},
				{
					actualState: 0.8,
					refState:    0.8,
					updateTime:  fakeNow.Add(4 * time.Second),
				},
				{
					actualState: 0.75,
					refState:    0.8,
					updateTime:  fakeNow.Add(5 * time.Second),
				},
			},
			wants: []float64{
				-0.2, // (0.8 - 1.0) * 1.0
				-0.1,
				-0.05,
				0.0,
				0.05,
			},
		},
		{
			name: "PI controller, 5 turns",
			fields: fields{
				proportionalGain: 0.8,
				integralGain:     0.1,
				derivativeGain:   0,
				mutateControllerFn: func(c *pidController) {
					c.DefaultInterval = time.Second.Seconds()
				},
			},
			args: []args{
				{
					actualState: 1.0,
					refState:    0.8,
					updateTime:  fakeNow.Add(time.Second),
				},
				{
					actualState: 0.9,
					refState:    0.8,
					updateTime:  fakeNow.Add(2 * time.Second),
				},
				{
					actualState: 0.85,
					refState:    0.8,
					updateTime:  fakeNow.Add(3 * time.Second),
				},
				{
					actualState: 0.8,
					refState:    0.8,
					updateTime:  fakeNow.Add(4 * time.Second),
				},
				{
					actualState: 0.75,
					refState:    0.8,
					updateTime:  fakeNow.Add(5 * time.Second),
				},
			},
			wants: []float64{
				-0.18,  // (0.8 - 1.0) * (0.8 + 0.1), ErrIntegral = -0.2
				-0.11,  // (0.8 - 0.9) * 0.8 + (0.8 - 0.9 - 0.2) * 0.1, ErrIntegral = -0.3
				-0.075, // (0.8 - 0.85) * 0.8 + (0.8 - 0.85 - 0.3) * 0.1, ErrIntegral = -0.35
				-0.035, // (0.8 - 0.8) * 0.8 + (0.8 - 0.8 - 0.35) * 0.1, ErrIntegral = -0.35
				0.01,   // (0.8 - 0.75) * 0.8 + (0.8 - 0.75 - 0.35) * 0.1, ErrIntegral = -0.3
			},
		},
		{
			name: "PID controller, 5 turns",
			fields: fields{
				proportionalGain: 0.8,
				integralGain:     0.1,
				derivativeGain:   0.05,
				mutateControllerFn: func(c *pidController) {
					c.DefaultInterval = time.Second.Seconds()
				},
			},
			args: []args{
				{
					actualState: 1.0,
					refState:    0.8,
					updateTime:  fakeNow.Add(time.Second),
				},
				{
					actualState: 0.9,
					refState:    0.8,
					updateTime:  fakeNow.Add(2 * time.Second),
				},
				{
					actualState: 0.85,
					refState:    0.8,
					updateTime:  fakeNow.Add(3 * time.Second),
				},
				{
					actualState: 0.8,
					refState:    0.8,
					updateTime:  fakeNow.Add(4 * time.Second),
				},
				{
					actualState: 0.75,
					refState:    0.8,
					updateTime:  fakeNow.Add(5 * time.Second),
				},
			},
			wants: []float64{
				-0.18,   // (0.8 - 1.0) * 0.8 + (0.8 - 1.0) * 0.1, ErrIntegral = -0.2, prevErr = -0.2
				-0.105,  // (0.8 - 0.9) * 0.8 + (0.8 - 0.9 - 0.2) * 0.1 + 0.1 * 0.05, ErrIntegral = -0.3, prevErr = -0.1
				-0.0725, // (0.8 - 0.85) * 0.8 + (0.8 - 0.85 - 0.3) * 0.1 + 0.05 * 0.05, ErrIntegral = -0.35, prevErr = -0.05
				-0.0325, // (0.8 - 0.8) * 0.8 + (0.8 - 0.8 - 0.35) * 0.1 + 0.05 * 0.05, ErrIntegral = -0.35, prevErr = 0
				0.0125,  // (0.8 - 0.75) * 0.8 + (0.8 - 0.75 - 0.35) * 0.1 + 0.05 * 0.05, ErrIntegral = -0.3, prevErr = 0.05
			},
		},
		{
			name: "PI controller, 1 turn, different interval",
			fields: fields{
				proportionalGain: 0.8,
				integralGain:     0.1,
				derivativeGain:   0,
				mutateControllerFn: func(c *pidController) {
					c.DefaultInterval = 2 * time.Second.Seconds() // use this at the first turn
				},
			},
			args: []args{
				{
					actualState: 1.0,
					refState:    0.8,
					updateTime:  fakeNow.Add(3 * time.Second),
				},
			},
			wants: []float64{
				-0.2, // (0.8 - 1.0) * 0.8 + (0.8 - 1.0) * 0.1 * 2, ErrIntegral = -0.4
			},
		},
		{
			name: "PID controller, 2 turns, different interval",
			fields: fields{
				proportionalGain: 0.8,
				integralGain:     0.1,
				derivativeGain:   0.06,
				mutateControllerFn: func(c *pidController) {
					c.DefaultInterval = 2 * time.Second.Seconds() // use this at the first turn
				},
			},
			args: []args{
				{
					actualState: 1.0,
					refState:    0.8,
					updateTime:  fakeNow.Add(3 * time.Second),
				},
				{
					actualState: 0.9,
					refState:    0.8,
					updateTime:  fakeNow.Add(6 * time.Second),
				},
			},
			wants: []float64{
				-0.2,   // (0.8 - 1.0) * 0.8 + (0.8 - 1.0) * 0.1 * 2, ErrIntegral = -0.4, prevErr = -0.2
				-0.148, // (0.8 - 0.9) * 0.8 + (-0.4 + (0.8 - 0.9) * 3) * 0.1 + (0.8 - 0.9 + 0.2) * 0.06 / 3, ErrIntegral = -0.7, prevErr = -0.1
			},
		},
		{
			name: "PI controller, 2 turns, capped by ErrIntegralLimit",
			fields: fields{
				proportionalGain: 0.8,
				integralGain:     0.1,
				derivativeGain:   0,
				mutateControllerFn: func(c *pidController) {
					c.DefaultInterval = 1 * time.Second.Seconds() // use this at the first turn
					c.ErrIntegralLimit = 0.2
				},
			},
			args: []args{
				{
					actualState: 1.0,
					refState:    0.8,
					updateTime:  fakeNow.Add(1 * time.Second),
				},
				{
					actualState: 0.9,
					refState:    0.8,
					updateTime:  fakeNow.Add(2 * time.Second),
				},
			},
			wants: []float64{
				-0.18, // (0.8 - 1.0) * 0.8 + (0.8 - 1.0) * 0.1 * 1, ErrIntegral = -0.2
				-0.1,  // (0.8 - 0.9) * 0.8 + min((0.8 - 0.9 - 0.2) * 1, 0.2) * 0.1), ErrIntegral = -0.2
			},
		},
		{
			name: "PI controller, 2 turns, capped by minus ErrIntegralLimit",
			fields: fields{
				proportionalGain: 0.8,
				integralGain:     0.1,
				derivativeGain:   0,
				mutateControllerFn: func(c *pidController) {
					c.DefaultInterval = 1 * time.Second.Seconds() // use this at the first turn
					c.ErrIntegralLimit = 0.2
				},
			},
			args: []args{
				{
					actualState: 0.6,
					refState:    0.8,
					updateTime:  fakeNow.Add(1 * time.Second),
				},
				{
					actualState: 0.7,
					refState:    0.8,
					updateTime:  fakeNow.Add(2 * time.Second),
				},
			},
			wants: []float64{
				0.18, // (0.8 - 0.6) * 0.8 + (0.8 - 0.6) * 0.1 * 1, ErrIntegral = 0.2
				0.1,  // (0.8 - 0.7) * 0.8 + min((0.8 - 0.7 - 0.2) * 1, 0.2) * 0.1), ErrIntegral = 0.2
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldTimeNow := timeNow
			timeNow = func() time.Time {
				return fakeNow
			}
			defer func() {
				timeNow = oldTimeNow
			}()
			c := newPIDController(tt.fields.proportionalGain, tt.fields.integralGain, tt.fields.derivativeGain)
			if tt.fields.mutateControllerFn != nil {
				cc, ok := c.(*pidController)
				assert.True(t, ok)
				tt.fields.mutateControllerFn(cc)
			}
			for i := range tt.args {
				arg := tt.args[i]
				if arg.isSkip {
					c.Skip(arg.updateTime)
				} else {
					got := c.Update(arg.actualState, arg.refState, arg.updateTime)
					assert.LessOrEqual(t, tt.wants[i]-0.001, got)
					assert.GreaterOrEqual(t, tt.wants[i]+0.001, got)
				}
				t.Logf("Test[%s]: turn %d, state %+v", tt.name, i, c)
			}
		})
	}
}

func Test_isPodShouldHaveMetricInWindow(t *testing.T) {
	fakeNow := time.Now()
	testTime := fakeNow.Add(-10 * time.Minute)
	type args struct {
		pod                   *corev1.Pod
		degradedWindowSeconds int64
		startTime             *time.Time
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "has no valid run before",
			args: args{
				startTime: nil,
			},
			want: false,
		},
		{
			name: "lack of enough metrics to judge",
			args: args{
				degradedWindowSeconds: 1800,
				startTime:             &testTime,
			},
			want: false,
		},
		{
			name: "pod has not been running for more than the degraded window",
			args: args{
				pod: &corev1.Pod{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "test",
							},
							{
								Name: "test-1",
							},
						},
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name: "test",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: fakeNow.Add(-30 * time.Second)},
									},
								},
							},
							{
								Name: "test-1",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: fakeNow.Add(-20 * time.Second)},
									},
								},
							},
						},
					},
				},
				degradedWindowSeconds: 60,
				startTime:             &testTime,
			},
			want: false,
		},
		{
			name: "pod has been running for more than the degraded window",
			args: args{
				pod: &corev1.Pod{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "test",
							},
							{
								Name: "test-1",
							},
						},
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name: "test",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: fakeNow.Add(-1200 * time.Second)},
									},
								},
							},
							{
								Name: "test-1",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: fakeNow.Add(-1200 * time.Second)},
									},
								},
							},
						},
					},
				},
				degradedWindowSeconds: 300,
				startTime:             &testTime,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldTimeNow := timeNow
			timeNow = func() time.Time {
				return fakeNow
			}
			defer func() {
				timeNow = oldTimeNow
			}()

			got := isPodShouldHaveMetricInWindow(tt.args.pod, tt.args.degradedWindowSeconds, tt.args.startTime)
			assert.Equal(t, tt.want, got)
		})
	}
}
