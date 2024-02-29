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

package podcpusatisfaction

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metricsadvisor/framework"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	mock_statesinformer "github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer/mockstatesinformer"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
)

func Test_collector_collectPodCPUSatisfactionUsed(t *testing.T) {
	testNow := time.Now()
	timeNow = func() time.Time {
		return testNow
	}
	defer func() {
		timeNow = time.Now
	}()
	testContainerID := "containerd://123abc"
	testPodMetaDir := "kubepods.slice/kubepods-podxxxxxxxx.slice"
	kubepodsParentDir := "/kubepods.slice"
	testPodParentDir := "/kubepods.slice/kubepods-podxxxxxxxx.slice"
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test",
			UID:       "xxxxxxxx",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "test-container",
					ContainerID: testContainerID,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	testFailedPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-failed-pod",
			Namespace: "test",
			UID:       "yyyyyy",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "test-container",
					ContainerID: testContainerID,
				},
			},
		},
	}
	type fields struct {
		podFilterOption         framework.PodFilter
		getPodMetas             []*statesinformer.PodMeta
		initLastPodCPUSchedStat func(lastState *gocache.Cache)
		SetSysUtil              func(helper *system.FileTestUtil)
	}
	type wantFields struct {
		podMetricNum                     int
		cpuCfsSatisfactionValue          float64
		cpuSatisfactionValue             float64
		cpuSatisfactionWithThrottleValue float64
	}
	tests := []struct {
		name   string
		fields fields
		want   wantFields
	}{
		{
			name: "sysctl not enabled",
			fields: fields{
				podFilterOption: framework.DefaultPodFilter,
				getPodMetas: []*statesinformer.PodMeta{
					{
						CgroupDir: testPodMetaDir,
						Pod:       testPod,
					},
				},
				initLastPodCPUSchedStat: func(lastState *gocache.Cache) {
					lastState.Set(string(testPod.UID), CPUSchedStat{
						Serve:         0,
						OnCpu:         0,
						SibidleUSec:   0,
						ThrottledUSec: 0,
					}, gocache.DefaultExpiration)
				},
				SetSysUtil: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteCgroupFileContents(testPodParentDir, system.CPUStatV2, `
		usage_usec 1000
		user_usec 900
		system_usec 100
		sibidle_usec 500
		nr_periods 0
		nr_throttled 0
		throttled_usec 0
		`)
					helper.WriteCgroupFileContents(kubepodsParentDir, system.CPUSchedCfsStatisticsV2Anolis, `0 0 0 0 0 0`)
					helper.WriteCgroupFileContents(testPodParentDir, system.CPUSchedCfsStatisticsV2Anolis, `1200000 1000000 100000 100000 200000 0`)
				},
			},
			want: wantFields{
				podMetricNum: 0,
			},
		},
		{
			name: "anolis cgroups v2",
			fields: fields{
				podFilterOption: framework.DefaultPodFilter,
				getPodMetas: []*statesinformer.PodMeta{
					{
						CgroupDir: testPodMetaDir,
						Pod:       testPod,
					},
				},
				initLastPodCPUSchedStat: func(lastState *gocache.Cache) {
					lastState.Set(string(testPod.UID), CPUSchedStat{
						Serve:         0,
						OnCpu:         0,
						SibidleUSec:   0,
						ThrottledUSec: 0,
					}, gocache.DefaultExpiration)
				},
				SetSysUtil: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteFileContents(system.GetProcSysFilePath(system.KernelSchedSchedStats), `1`)
					helper.WriteFileContents(system.GetProcSysFilePath(system.KernelSchedAcpu), `1`)
					helper.WriteCgroupFileContents(testPodParentDir, system.CPUStatV2, `
		usage_usec 1000
		user_usec 900
		system_usec 100
		sibidle_usec 500
		nr_periods 0
		nr_throttled 0
		throttled_usec 0
		`)
					helper.WriteCgroupFileContents(kubepodsParentDir, system.CPUSchedCfsStatisticsV2Anolis, `0 0 0 0 0 0`)
					helper.WriteCgroupFileContents(testPodParentDir, system.CPUSchedCfsStatisticsV2Anolis, `1200000 1000000 100000 100000 200000 0`)
				},
			},
			want: wantFields{
				podMetricNum:                     1,
				cpuCfsSatisfactionValue:          float64(1000000) / float64(1200000),
				cpuSatisfactionValue:             (float64(1000000)*0.65 + float64(500)*1000*0.35) / float64(1200000),
				cpuSatisfactionWithThrottleValue: (float64(1000000)*0.65 + float64(500)*1000*0.35) / (float64(1200000) + float64(0)*1000),
			},
		},
		{
			name: "cgroups v1, filter non-running pods",
			fields: fields{
				podFilterOption: &framework.TerminatedPodFilter{},
				getPodMetas: []*statesinformer.PodMeta{
					{
						CgroupDir: testPodMetaDir,
						Pod:       testPod,
					},
					{
						Pod: testFailedPod,
					},
				},
				initLastPodCPUSchedStat: func(lastState *gocache.Cache) {
					lastState.Set(string(testPod.UID), CPUSchedStat{
						Serve:         0,
						OnCpu:         0,
						SibidleUSec:   0,
						ThrottledUSec: 0,
					}, gocache.DefaultExpiration)
				},
				SetSysUtil: func(helper *system.FileTestUtil) {
					helper.SetCgroupsV2(true)
					helper.WriteFileContents(system.GetProcSysFilePath(system.KernelSchedSchedStats), `1`)
					helper.WriteFileContents(system.GetProcSysFilePath(system.KernelSchedAcpu), `1`)
					helper.WriteCgroupFileContents(testPodParentDir, system.CPUStatV2, `
		usage_usec 1700
		user_usec 1200
		system_usec 500
		sibidle_usec 700
		nr_periods 0
		nr_throttled 0
		throttled_usec 300
		`)
					helper.WriteCgroupFileContents(kubepodsParentDir, system.CPUSchedCfsStatisticsV2Anolis, `0 0 0 0 0 0`)
					helper.WriteCgroupFileContents(testPodParentDir, system.CPUSchedCfsStatisticsV2Anolis, `2000000 1700000 200000 200000 300000 0`)
				},
			},
			want: wantFields{
				podMetricNum:                     1,
				cpuCfsSatisfactionValue:          float64(1700000) / float64(2000000),
				cpuSatisfactionValue:             (float64(1700000)*0.65 + float64(700)*1000*0.35) / float64(2000000),
				cpuSatisfactionWithThrottleValue: (float64(1700000)*0.65 + float64(700)*1000*0.35) / (float64(2000000) + float64(300)*1000),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := system.NewFileTestUtil(t)
			defer helper.Cleanup()
			if tt.fields.SetSysUtil != nil {
				tt.fields.SetSysUtil(helper)
			}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			metricCache, err := metriccache.NewMetricCache(&metriccache.Config{
				TSDBPath:              t.TempDir(),
				TSDBEnablePromMetrics: false,
			})
			assert.NoError(t, err)
			defer func() {
				metricCache.Close()
			}()
			statesInformer := mock_statesinformer.NewMockStatesInformer(ctrl)
			statesInformer.EXPECT().HasSynced().Return(true).AnyTimes()
			statesInformer.EXPECT().GetAllPods().Return(tt.fields.getPodMetas).MaxTimes(1)

			collector := New(&framework.Options{
				Config: &framework.Config{
					CollectResUsedInterval: 1 * time.Second,
				},
				StatesInformer: statesInformer,
				MetricCache:    metricCache,
				CgroupReader:   resourceexecutor.NewCgroupReader(),
				PodFilters: map[string]framework.PodFilter{
					CollectorName: tt.fields.podFilterOption,
				},
			})
			collector.Setup(&framework.Context{
				State: framework.NewSharedState(),
			})
			c := collector.(*podCPUSatisfactionCollector)
			tt.fields.initLastPodCPUSchedStat(c.lastPodCPUSchedStat)

			assert.NotPanics(t, func() {
				c.collectPodCPUSatisfactionUsed()
			})

			querier, err := metricCache.Querier(timeNow().Add(-time.Second), timeNow())
			assert.NoError(t, err)

			cpuCfsSatisfactionResult, err := testQuery(querier, metriccache.PodCPUCfsSatisfactionMetric, nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.want.podMetricNum, cpuCfsSatisfactionResult.Count())
			if tt.want.podMetricNum != 0 {
				cpuCfsSatisfactionValue, aggregateErr := cpuCfsSatisfactionResult.Value(metriccache.AggregationTypeLast)
				assert.NoError(t, aggregateErr)
				assert.Equal(t, tt.want.cpuCfsSatisfactionValue, cpuCfsSatisfactionValue)
			}
			cpuSatisfactionResult, err := testQuery(querier, metriccache.PodCPUSatisfactionMetric, nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.want.podMetricNum, cpuCfsSatisfactionResult.Count())
			if tt.want.cpuSatisfactionValue != 0 {
				cpuSatisfactionValue, aggregateErr := cpuSatisfactionResult.Value(metriccache.AggregationTypeLast)
				assert.NoError(t, aggregateErr)
				assert.Equal(t, tt.want.cpuSatisfactionValue, cpuSatisfactionValue)
			}
			cpuSatisfactionWithThrottleResult, err := testQuery(querier, metriccache.PodCPUSatisfactionWithThrottleMetric, nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.want.podMetricNum, cpuSatisfactionWithThrottleResult.Count())
			if tt.want.cpuSatisfactionValue != 0 {
				cpuSatisfactionWithThrottleValue, aggregateErr := cpuSatisfactionWithThrottleResult.Value(metriccache.AggregationTypeLast)
				assert.NoError(t, aggregateErr)
				assert.Equal(t, tt.want.cpuSatisfactionWithThrottleValue, cpuSatisfactionWithThrottleValue)
			}
		})
	}
}

func testQuery(querier metriccache.Querier, resource metriccache.MetricResource, properties map[metriccache.MetricProperty]string) (metriccache.AggregateResult, error) {
	queryMeta, err := resource.BuildQueryMeta(properties)
	if err != nil {
		return nil, err
	}
	aggregateResult := metriccache.DefaultAggregateResultFactory.New(queryMeta)
	if err = querier.Query(queryMeta, nil, aggregateResult); err != nil {
		return nil, err
	}
	return aggregateResult, nil
}

func Test_podCPUSatisfactionCollector_Run(t *testing.T) {
	helper := system.NewFileTestUtil(t)
	defer helper.Cleanup()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	metricCacheCfg := metriccache.NewDefaultConfig()
	metricCacheCfg.TSDBEnablePromMetrics = false
	metricCacheCfg.TSDBPath = helper.TempDir
	metricCache, err := metriccache.NewMetricCache(metricCacheCfg)
	assert.NoError(t, err)
	mockStatesInformer := mock_statesinformer.NewMockStatesInformer(ctrl)
	mockStatesInformer.EXPECT().HasSynced().Return(true).AnyTimes()
	mockStatesInformer.EXPECT().GetAllPods().Return([]*statesinformer.PodMeta{}).AnyTimes()
	c := New(&framework.Options{
		Config:         framework.NewDefaultConfig(),
		StatesInformer: mockStatesInformer,
		MetricCache:    metricCache,
		CgroupReader:   resourceexecutor.NewCgroupReader(),
	})
	collector := c.(*podCPUSatisfactionCollector)
	collector.started = atomic.NewBool(true)
	collector.Setup(&framework.Context{
		State: framework.NewSharedState(),
	})
	assert.False(t, collector.Enabled())
	assert.True(t, collector.Started())
	assert.NotPanics(t, func() {
		stopCh := make(chan struct{}, 1)
		collector.Run(stopCh)
		close(stopCh)
	})
}
