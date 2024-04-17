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
	"strconv"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/audit"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	mockmetriccache "github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache/mockmetriccache"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/qosmanager/framework"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	mockstatesinformer "github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer/mockstatesinformer"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
	"github.com/koordinator-sh/koordinator/pkg/util/sloconfig"
)

func Test_cpuStable(t *testing.T) {
	t.Run("test", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		statesInformer := mockstatesinformer.NewMockStatesInformer(ctrl)
		metricCache := mockmetriccache.NewMockMetricCache(ctrl)

		c := New(&framework.Options{
			StatesInformer: statesInformer,
			MetricCache:    metricCache,
		})
		c.Setup(&framework.Context{})

		stopCh := make(chan struct{})
		defer close(stopCh)
		c.Run(stopCh)
	})
}

func Test_cpuStable_runOnce(t *testing.T) {
	fakeNow := time.Now()
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("400Gi"),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100"),
				corev1.ResourceMemory: resource.MustParse("400Gi"),
			},
		},
	}
	testNodeSLO := &slov1alpha1.NodeSLO{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Spec: sloconfig.DefaultNodeSLOSpecConfig(),
	}
	testNodeSLO.Spec.Extensions.Object[unified.CPUStableExtKey] = getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
		strategy.WarmupConfig = unified.CPUStableWarmupModel{
			Entries: nil,
		}
	})
	testNodeSLO1 := testNodeSLO.DeepCopy()
	testNodeSLO1.Spec.Extensions.Object[unified.CPUStableExtKey] = getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
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
	})
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
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
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
				Status: corev1.PodStatus{
					Phase: corev1.PodFailed,
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
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
		},
	}
	t.Run("test", func(t *testing.T) {
		helper := system.NewFileTestUtil(t)
		defer helper.Cleanup()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		oldTimeNow := timeNow
		timeNow = func() time.Time {
			return fakeNow
		}
		oldAggregateResultFactory := metriccache.DefaultAggregateResultFactory
		defer func() {
			timeNow = oldTimeNow
			metriccache.DefaultAggregateResultFactory = oldAggregateResultFactory
		}()

		featuresPath := system.SchedFeatures.Path("")
		helper.WriteFileContents(featuresPath, `FEATURE_A FEATURE_B FEATURE_C SCHED_CORE_HT_AWARE_QUOTA`)
		mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
		mockStatesInformer := mockstatesinformer.NewMockStatesInformer(ctrl)

		c := New(&framework.Options{
			StatesInformer: mockStatesInformer,
			MetricCache:    mockMetricCache,
		})
		cc, ok := c.(*cpuStable)
		assert.True(t, ok)

		// nil node
		mockStatesInformer.EXPECT().GetNode().Return(nil)
		cc.runOnce()

		// nil nodeSLO
		mockStatesInformer.EXPECT().GetNode().Return(testNode)
		mockStatesInformer.EXPECT().GetNodeSLO().Return(nil)
		cc.runOnce()

		// no pod and no warmup config
		mockStatesInformer.EXPECT().GetNode().Return(testNode)
		mockStatesInformer.EXPECT().GetNodeSLO().Return(testNodeSLO)
		mockStatesInformer.EXPECT().GetAllPods().Return(nil)
		cc.runOnce()

		// some pods with valid strategy
		mockStatesInformer.EXPECT().GetNode().Return(testNode)
		mockStatesInformer.EXPECT().GetNodeSLO().Return(testNodeSLO1)
		mockStatesInformer.EXPECT().GetAllPods().Return(testPodMetas).Times(2)
		mockStatesInformer.EXPECT().GetNodeTopo().Return(&topologyv1alpha1.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
				Annotations: map[string]string{
					extension.AnnotationNodeCPUSharedPools:   `[{"node": 0, "socket": 0, "cpuset": "8-15"}, {"node": 1, "socket": 0, "cpuset": "24-31"}]`,
					extension.AnnotationNodeBECPUSharedPools: `[{"node": 0, "socket": 0, "cpuset": "0-15"}, {"node": 1, "socket": 0, "cpuset": "16-31"}]`,
				},
			},
		})
		mockAggregateResultFactory := mockmetriccache.NewMockAggregateResultFactory(ctrl)
		metriccache.DefaultAggregateResultFactory = mockAggregateResultFactory
		mockQuerier := mockmetriccache.NewMockQuerier(ctrl)
		queryMetaForNodeUsage, err := metriccache.NodeCPUUsageMetric.BuildQueryMeta(nil)
		assert.NoError(t, err)
		mockMetricCache.EXPECT().Querier(fakeNow.Add(-30*time.Second), fakeNow).Return(mockQuerier, nil).Times(4)
		mockAggregateNodeUsageResult := mockmetriccache.NewMockAggregateResult(ctrl)
		mockAggregateNodeUsageResult.EXPECT().Count().Return(3)
		mockAggregateNodeUsageResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(10.0, nil)
		mockAggregateResultFactory.EXPECT().New(queryMetaForNodeUsage).Return(mockAggregateNodeUsageResult)
		mockQuerier.EXPECT().QueryAndClose(queryMetaForNodeUsage, nil, mockAggregateNodeUsageResult).Return(nil)
		queryMetaForLSPod, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("xxxxxx"))
		assert.NoError(t, err)
		mockAggregateResultForLSPod := mockmetriccache.NewMockAggregateResult(ctrl)
		mockAggregateResultForLSPod.EXPECT().Count().Return(3)
		mockAggregateResultForLSPod.EXPECT().Value(metriccache.AggregationTypeAVG).Return(2.0, nil)
		mockAggregateResultFactory.EXPECT().New(queryMetaForLSPod).Return(mockAggregateResultForLSPod)
		mockQuerier.EXPECT().QueryAndClose(queryMetaForLSPod, nil, mockAggregateResultForLSPod).Return(nil)
		queryMetaForLSEPod, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("yyyyyy"))
		assert.NoError(t, err)
		mockAggregateResultForLSEPod := mockmetriccache.NewMockAggregateResult(ctrl)
		mockAggregateResultForLSEPod.EXPECT().Count().Return(0)
		mockAggregateResultFactory.EXPECT().New(queryMetaForLSEPod).Return(mockAggregateResultForLSEPod)
		mockQuerier.EXPECT().QueryAndClose(queryMetaForLSEPod, nil, mockAggregateResultForLSEPod).Return(nil)
		queryMetaForBEPod, err := metriccache.PodCPUUsageMetric.BuildQueryMeta(metriccache.MetricPropertiesFunc.Pod("zzzzzz"))
		assert.NoError(t, err)
		mockAggregateResultForBEPod := mockmetriccache.NewMockAggregateResult(ctrl)
		mockAggregateResultForBEPod.EXPECT().Count().Return(2)
		mockAggregateResultForBEPod.EXPECT().Value(metriccache.AggregationTypeAVG).Return(8.0, nil)
		mockAggregateResultFactory.EXPECT().New(queryMetaForBEPod).Return(mockAggregateResultForBEPod)
		mockQuerier.EXPECT().QueryAndClose(queryMetaForBEPod, nil, mockAggregateResultForBEPod).Return(nil)
		// TODO: complete the reconciliation
		cc.runOnce()

		// run with PID controller
		cc.controller = newPodStableController(HTRatioPIDController, &framework.Options{
			StatesInformer: mockStatesInformer,
			MetricCache:    mockMetricCache,
		})
		mockStatesInformer.EXPECT().GetNode().Return(testNode)
		mockStatesInformer.EXPECT().GetNodeSLO().Return(testNodeSLO1)
		mockStatesInformer.EXPECT().GetAllPods().Return(testPodMetas).Times(2)
		mockStatesInformer.EXPECT().GetNodeTopo().Return(&topologyv1alpha1.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
				Annotations: map[string]string{
					extension.AnnotationNodeCPUSharedPools:   `[{"node": 0, "socket": 0, "cpuset": "8-15"}, {"node": 1, "socket": 0, "cpuset": "24-31"}]`,
					extension.AnnotationNodeBECPUSharedPools: `[{"node": 0, "socket": 0, "cpuset": "0-15"}, {"node": 1, "socket": 0, "cpuset": "16-31"}]`,
				},
			},
		})
		mockMetricCache.EXPECT().Querier(fakeNow.Add(-30*time.Second), fakeNow).Return(mockQuerier, nil).Times(4)
		mockAggregateNodeUsageResult.EXPECT().Count().Return(3)
		mockAggregateNodeUsageResult.EXPECT().Value(metriccache.AggregationTypeAVG).Return(10.0, nil)
		mockAggregateResultFactory.EXPECT().New(queryMetaForNodeUsage).Return(mockAggregateNodeUsageResult)
		mockQuerier.EXPECT().QueryAndClose(queryMetaForNodeUsage, nil, mockAggregateNodeUsageResult).Return(nil)
		mockAggregateResultForLSPod.EXPECT().Count().Return(3)
		mockAggregateResultForLSPod.EXPECT().Value(metriccache.AggregationTypeAVG).Return(2.0, nil)
		mockAggregateResultFactory.EXPECT().New(queryMetaForLSPod).Return(mockAggregateResultForLSPod)
		mockQuerier.EXPECT().QueryAndClose(queryMetaForLSPod, nil, mockAggregateResultForLSPod).Return(nil)
		mockAggregateResultForLSEPod.EXPECT().Count().Return(0)
		mockAggregateResultFactory.EXPECT().New(queryMetaForLSEPod).Return(mockAggregateResultForLSEPod)
		mockQuerier.EXPECT().QueryAndClose(queryMetaForLSEPod, nil, mockAggregateResultForLSEPod).Return(nil)
		mockAggregateResultForBEPod.EXPECT().Count().Return(2)
		mockAggregateResultForBEPod.EXPECT().Value(metriccache.AggregationTypeAVG).Return(8.0, nil)
		mockAggregateResultFactory.EXPECT().New(queryMetaForBEPod).Return(mockAggregateResultForBEPod)
		mockQuerier.EXPECT().QueryAndClose(queryMetaForBEPod, nil, mockAggregateResultForBEPod).Return(nil)
		// TODO: complete the reconciliation
		cc.runOnce()

		// run with fake controller
		cc.controller = &fakePodStableController{
			ValidateStrategyErr: false,
			W:                   nil,
			GetWarmupStateErr:   false,
			PodSignals: map[string]PodControlSignal{
				"/test-ls-pod": &nopPodControlSignal{},
			},
			PodResourceUpdaters: map[string][]resourceexecutor.ResourceUpdater{
				"/test-ls-pod": nil,
			},
		}
		mockStatesInformer.EXPECT().GetNode().Return(testNode)
		mockStatesInformer.EXPECT().GetNodeSLO().Return(testNodeSLO1)
		mockStatesInformer.EXPECT().GetAllPods().Return(testPodMetas).Times(1)
		cc.runOnce()
	})
}

func Test_getNodeCPUStableStrategy(t *testing.T) {
	tests := []struct {
		name    string
		arg     *slov1alpha1.NodeSLO
		want    *unified.CPUStableStrategy
		wantErr bool
	}{
		{
			name:    "invalid nodeSLO",
			arg:     nil,
			want:    nil,
			wantErr: true,
		},
		{
			name: "no CPUStableExtKey key in nodeSLO",
			arg: &slov1alpha1.NodeSLO{
				Spec: slov1alpha1.NodeSLOSpec{
					Extensions: &slov1alpha1.ExtensionsMap{},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "parse node strategy correctly",
			arg: &slov1alpha1.NodeSLO{
				Spec: slov1alpha1.NodeSLOSpec{
					Extensions: &slov1alpha1.ExtensionsMap{
						Object: map[string]interface{}{
							unified.CPUStableExtKey: map[string]interface{}{
								"cpuSatisfactionEpsilonPermill": 30,
								"cpuUtilMinPercent":             40,
								"htRatioIncreasePercent":        10,
								"htRatioLowerBound":             100,
								"htRatioUpperBound":             160,
								"htRatioZoomOutPercent":         90,
								"podMetricWindowSeconds":        30,
								"podDegradeWindowSeconds":       1800,
								"policy":                        "auto",
								"targetCPUSatisfactionPercent":  80,
								"validMetricPointsPercent":      80,
								"warmupConfig":                  map[string]interface{}{},
							},
						},
					},
				},
			},
			want: &unified.CPUStableStrategy{
				Policy:                   sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto),
				PodMetricWindowSeconds:   pointer.Int64(30),
				PodDegradeWindowSeconds:  pointer.Int64(1800),
				ValidMetricPointsPercent: pointer.Int64(80),
				CPUStableScaleModel: unified.CPUStableScaleModel{
					TargetCPUSatisfactionPercent:  pointer.Int64(80),
					CPUSatisfactionEpsilonPermill: pointer.Int64(30),
					HTRatioIncreasePercent:        pointer.Int64(10),
					HTRatioLowerBound:             pointer.Int64(100),
					HTRatioUpperBound:             pointer.Int64(160),
					HTRatioZoomOutPercent:         pointer.Int64(90),
					CPUUtilMinPercent:             pointer.Int64(40),
				},
				WarmupConfig: unified.CPUStableWarmupModel{},
			},
			wantErr: false,
		},
		{
			name: "failed to parse node strategy",
			arg: &slov1alpha1.NodeSLO{
				Spec: slov1alpha1.NodeSLOSpec{
					Extensions: &slov1alpha1.ExtensionsMap{
						Object: map[string]interface{}{
							unified.CPUStableExtKey: "invalidFields",
						},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "failed to parse node strategy 1",
			arg: &slov1alpha1.NodeSLO{
				Spec: slov1alpha1.NodeSLOSpec{
					Extensions: &slov1alpha1.ExtensionsMap{
						Object: map[string]interface{}{
							unified.CPUStableExtKey: []map[string]interface{}{
								{
									"cpuSatisfactionEpsilonPermill": 30,
								},
								{
									"policy": "none",
								},
							},
						},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := getNodeCPUStableStrategy(tt.arg)
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_getPodCPUStableStrategy(t *testing.T) {
	type args struct {
		pod          *corev1.Pod
		nodeStrategy *unified.CPUStableStrategy
	}
	tests := []struct {
		name    string
		args    args
		want    *unified.CPUStableStrategy
		wantErr bool
	}{
		{
			name: "parse pod strategy correctly",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod",
						Annotations: map[string]string{
							unified.AnnotationPodCPUStable: `
{
  "policy": "none"
}
`,
						},
					},
				},
				nodeStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyAuto)
				}),
			},
			want: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
				strategy.Policy = sloconfig.CPUStablePolicyPtr(unified.CPUStablePolicyNone)
			}),
			wantErr: false,
		},
		{
			name: "failed to parse pod strategy",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod",
						Annotations: map[string]string{
							unified.AnnotationPodCPUStable: `invalidContent`,
						},
					},
				},
				nodeStrategy: sloconfig.DefaultCPUStableStrategy(),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "failed to parse pod strategy 1",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod",
						Annotations: map[string]string{
							unified.AnnotationPodCPUStable: `
{
  "validMetricPointsPercent": 200,
}
`,
						},
					},
				},
				nodeStrategy: getMutatedDefaultStrategy(func(strategy *unified.CPUStableStrategy) {
					strategy.ValidMetricPointsPercent = pointer.Int64(80)
				}),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := getPodCPUStableStrategy(tt.args.pod, tt.args.nodeStrategy)
			assert.Equal(t, tt.wantErr, gotErr != nil, gotErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_isPodInitializing(t *testing.T) {
	type args struct {
		pod                     *corev1.Pod
		initializeWindowSeconds *int64
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "no need to check initialize window",
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
							{
								Name: "test-sidecar",
								Env: []corev1.EnvVar{
									{
										Name:  unified.EnvACSIgnoreResource,
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
								Name: "test",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: timeNow().Add(-30 * time.Second)},
									},
								},
							},
							{
								Name: "test-1",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: timeNow().Add(-20 * time.Second)},
									},
								},
							},
							{
								Name: "test-sidecar",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: timeNow().Add(-10 * time.Second)},
									},
								},
							},
						},
					},
				},
				initializeWindowSeconds: nil,
			},
			want: false,
		},
		{
			name: "pod is not running",
			args: args{
				pod: &corev1.Pod{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "test",
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodFailed,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name: "test",
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										FinishedAt: metav1.Time{Time: timeNow().Add(-30 * time.Second)},
									},
								},
							},
						},
					},
				},
				initializeWindowSeconds: pointer.Int64(60),
			},
			want: false,
		},
		{
			name: "running pod is not in the window",
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
							{
								Name: "test-sidecar",
								Env: []corev1.EnvVar{
									{
										Name:  unified.EnvACSIgnoreResource,
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
								Name: "test",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: timeNow().Add(-30 * time.Second)},
									},
								},
							},
							{
								Name: "test-1",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: timeNow().Add(-20 * time.Second)},
									},
								},
							},
							{
								Name: "test-sidecar",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: timeNow().Add(-10 * time.Second)},
									},
								},
							},
						},
					},
				},
				initializeWindowSeconds: pointer.Int64(10),
			},
			want: false,
		},
		{
			name: "running pod is in the window",
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
							{
								Name: "test-sidecar",
								Env: []corev1.EnvVar{
									{
										Name:  unified.EnvACSIgnoreResource,
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
								Name: "test",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: timeNow().Add(-30 * time.Second)},
									},
								},
							},
							{
								Name: "test-1",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: timeNow().Add(-20 * time.Second)},
									},
								},
							},
							{
								Name: "test-sidecar",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Time{Time: timeNow().Add(-10 * time.Second)},
									},
								},
							},
						},
					},
				},
				initializeWindowSeconds: pointer.Int64(120),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPodInitializing(tt.args.pod, tt.args.initializeWindowSeconds)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_getPodStartedDuration(t *testing.T) {
	now := time.Now()
	testNow := func() time.Time {
		return now
	}
	tests := []struct {
		name    string
		fakeNow func() time.Time
		arg     *corev1.Pod
		want    time.Duration
	}{
		{
			name: "pod not started",
			arg: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test",
						},
					},
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "test",
							State: corev1.ContainerState{
								Running: nil,
							},
						},
					},
				},
			},
			want: 0,
		},
		{
			name: "pod terminated",
			arg: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test",
						},
					},
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "test",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									FinishedAt: metav1.Now(),
								},
							},
						},
					},
				},
			},
			want: 0,
		},
		{
			name:    "get pod started duration correctly",
			fakeNow: testNow,
			arg: &corev1.Pod{
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
									StartedAt: metav1.Time{Time: testNow().Add(-30 * time.Second)},
								},
							},
						},
						{
							Name: "test-1",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{
									StartedAt: metav1.Time{Time: testNow().Add(-20 * time.Second)},
								},
							},
						},
					},
				},
			},
			want: 20 * time.Second,
		},
		{
			name:    "get pod started duration ignoring sidecar containers",
			fakeNow: testNow,
			arg: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test",
						},
						{
							Name: "test-1",
						},
						{
							Name: "test-sidecar",
							Env: []corev1.EnvVar{
								{
									Name:  unified.EnvACSIgnoreResource,
									Value: "true",
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "test",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{
									StartedAt: metav1.Time{Time: testNow().Add(-30 * time.Second)},
								},
							},
						},
						{
							Name: "test-1",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{
									StartedAt: metav1.Time{Time: testNow().Add(-20 * time.Second)},
								},
							},
						},
						{
							Name: "test-sidecar",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{
									StartedAt: metav1.Time{Time: testNow().Add(-10 * time.Second)},
								},
							},
						},
					},
				},
			},
			want: 20 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.fakeNow != nil {
				oldTimeNow := timeNow
				defer func() {
					timeNow = oldTimeNow
				}()
				timeNow = tt.fakeNow
			}
			got := getPodStartedDuration(tt.arg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func getMutatedDefaultStrategy(mutateFn func(*unified.CPUStableStrategy)) *unified.CPUStableStrategy {
	s := sloconfig.DefaultCPUStableStrategy()
	mutateFn(s)
	return s
}

func assertEqualCgroupUpdater(t *testing.T, want, got resourceexecutor.ResourceUpdater) {
	if want == nil {
		assert.Nil(t, got)
		return
	}
	cg, ok := got.(*resourceexecutor.CgroupResourceUpdater)
	assert.True(t, ok)
	cw := want.(*resourceexecutor.CgroupResourceUpdater)
	// NOTE: these reset updateFn and event helper
	cw.WithUpdateFunc(nil).WithMergeUpdateFunc(nil)
	cg.WithUpdateFunc(nil).WithMergeUpdateFunc(nil)
	cw.SetEventHelper(nil)
	cg.SetEventHelper(nil)
	assert.Equal(t, cw, cg)
}

func getCPUHTRatioUpdater(t *testing.T, cgroupDir string, value int64, namespace, pod string) resourceexecutor.ResourceUpdater {
	e := audit.V(3).Pod(namespace, pod).Reason("CPUStable").Message("update pod ht_ratio: %v", value)
	u, err := resourceexecutor.NewDetailCgroupUpdater(system.CPUHTRatioV2Anolis, cgroupDir, strconv.FormatInt(value, 10), resourceexecutor.CommonCgroupUpdateFunc, e)
	assert.NoError(t, err)
	return u
}
