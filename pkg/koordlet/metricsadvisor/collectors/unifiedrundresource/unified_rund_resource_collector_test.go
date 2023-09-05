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

package unifiedrundresource

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containerd/ttrpc"
	"github.com/golang/mock/gomock"
	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"gitlab.alibaba-inc.com/virtcontainers/agent-protocols/protos/extends"
	"gitlab.alibaba-inc.com/virtcontainers/agent-protocols/protos/grpc"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metricsadvisor/framework"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	mock_statesinformer "github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer/mockstatesinformer"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
)

func Test_rundResourceCollector_Run(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	metricCacheCfg := metriccache.NewDefaultConfig()
	metricCacheCfg.TSDBEnablePromMetrics = false
	metricCache, _ := metriccache.NewMetricCache(metricCacheCfg)
	mockStatesInformer := mock_statesinformer.NewMockStatesInformer(ctrl)
	mockStatesInformer.EXPECT().HasSynced().Return(true).AnyTimes()
	mockStatesInformer.EXPECT().GetAllPods().Return([]*statesinformer.PodMeta{}).AnyTimes()
	c := New(&framework.Options{
		Config:         framework.NewDefaultConfig(),
		StatesInformer: mockStatesInformer,
		MetricCache:    metricCache,
		CgroupReader:   resourceexecutor.NewCgroupReader(),
	})
	collector := c.(*rundResourceCollector)
	collector.started = atomic.NewBool(true)
	collector.Setup(&framework.Context{
		State: framework.NewSharedState(),
	})
	assert.True(t, collector.Enabled())
	assert.True(t, collector.Started())
	assert.NotPanics(t, func() {
		stopCh := make(chan struct{}, 1)
		collector.Run(stopCh)
		stopCh <- struct{}{}
	})
}

func Test_collectRundPodsResUsed(t *testing.T) {
	testNow := time.Now()
	timeNow = func() time.Time {
		return testNow
	}
	testContainerID := "containerd://123abc"
	testContainerIDParsed := "123abc"
	testPodMetaDir := "kubepods.slice/kubepods-podxxxxxxxx.slice"
	testPodParentDir := "/kubepods.slice/kubepods-podxxxxxxxx.slice"
	sandboxID := "8c000081"
	testRundPodMemoryParentDir := "/kata/8c000081"
	testRundSandboxContainerParentDir := "/kubepods.slice/kubepods-podxxxxxxxx.slice/cri-containerd-8c000081.scope"
	testRundPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rund-pod",
			Namespace: "test",
			UID:       "xxxxxxxx",
		},
		Spec: corev1.PodSpec{
			RuntimeClassName: pointer.String(extunified.PodRuntimeTypeRund),
			Containers: []corev1.Container{
				{
					Name: "test-rund-container",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "test-rund-container",
					ContainerID: testContainerID,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	testRuncPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-runc-pod",
			Namespace: "test",
			UID:       "xxxxxxxx",
		},
		Spec: corev1.PodSpec{
			// consider default RuntimeClass as "runc"
			Containers: []corev1.Container{
				{
					Name: "test-runc-container",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "test-runc-container",
					ContainerID: "test-runc-container-id",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	type fields struct {
		podFilterOption       framework.PodFilter
		getPodMetas           []*statesinformer.PodMeta
		initPodLastStat       func(lastState *gocache.Cache)
		initContainerLastTick func(lastState *gocache.Cache)
		SetSysUtil            func(helper *system.FileTestUtil)
		NewRundExtendedStats  func(helper *system.FileTestUtil) *ttrpc.Server
	}
	type wantFields struct {
		checkPodResourceMetric       func(t *testing.T, metricCache metriccache.MetricCache)
		checkContainerResourceMetric func(t *testing.T, metricCache metriccache.MetricCache)
		allCPU                       float64
		allMemory                    float64
	}
	tests := []struct {
		name   string
		fields fields
		want   wantFields
	}{
		{
			name: "cgroups v1",
			fields: fields{
				podFilterOption: framework.RundPodFilter,
				getPodMetas: []*statesinformer.PodMeta{
					{
						CgroupDir: testPodMetaDir,
						Pod:       testRundPod,
					},
					{
						Pod: testRuncPod,
					},
				},
				initPodLastStat: func(lastState *gocache.Cache) {
					lastState.Set(string(testRundPod.UID), framework.CPUStat{
						CPUUsage:  0,
						Timestamp: testNow.Add(-time.Second),
					}, gocache.DefaultExpiration)
				},
				initContainerLastTick: func(lastState *gocache.Cache) {
					lastState.Set(testContainerID, framework.CPUStat{
						CPUTick:   0,
						Timestamp: testNow.Add(-time.Second),
					}, gocache.DefaultExpiration)
				},
				SetSysUtil: func(helper *system.FileTestUtil) {
					helper.WriteCgroupFileContents(testPodParentDir, system.CPUAcctUsage, `
1000000000
`)
					helper.WriteCgroupFileContents(testRundSandboxContainerParentDir, system.CPUSet, `0-31`)
					helper.WriteCgroupFileContents(testRundPodMemoryParentDir, system.MemoryStat, `
total_cache 104857600
total_rss 104857600
total_inactive_anon 104857600
total_active_anon 0
total_inactive_file 104857600
total_active_file 0
total_unevictable 0
`)
				},
				NewRundExtendedStats: func(helper *system.FileTestUtil) *ttrpc.Server {
					expected := &grpc.ExtendedStatsResponse{
						PodStats: &grpc.PodStats{},
						ConStats: []*grpc.ContainerStats{
							{
								BaseStats: &grpc.ContainerBaseStats{
									ContainerId: testContainerIDParsed,
									CgroupCpu: &grpc.ContainerCgroupCpu{
										User: 90,
										Sys:  10,
										Nice: 0,
										Sirq: 0,
										Hirq: 0,
									},
									CgroupMem: &grpc.ContainerCgroupMem{
										Rss:   104857600,
										Cache: 52428800,
									},
									CgroupMemx: &grpc.ContainerCgroupMemx{
										Aanon: 52428800,
										Ianon: 52428800,
										Afile: 26214400,
										Ifile: 26214400,
									},
								},
							},
						},
					}

					s, err := ttrpc.NewServer()
					assert.NoError(t, err)
					svc := &system.FakeExtendedStatusService{}
					svc.FakeExtendedStats = func(ctx context.Context, req *grpc.ExtendedStatsRequest) (*grpc.ExtendedStatsResponse, error) {
						return expected, nil
					}
					extends.RegisterExtendedStatusService(s, svc)
					return s
				},
			},
			want: wantFields{
				checkPodResourceMetric: func(t *testing.T, metricCache metriccache.MetricCache) {
					testGetPodMetric(t, metricCache, "xxxxxxxx", testNow.Add(-time.Minute), testNow.Add(time.Minute))
				},
				checkContainerResourceMetric: func(t *testing.T, metricCache metriccache.MetricCache) {
					testGetContainerMetric(t, metricCache, testContainerID, testNow.Add(-time.Minute), testNow.Add(time.Minute))
				},
				allCPU:    1,
				allMemory: 104857600,
			},
		},
		// FIXME: currently rund pod collection does not support cgroups-v2
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := system.NewFileTestUtil(t)
			defer helper.Cleanup()
			system.SetupCgroupPathFormatter(system.Systemd)
			if tt.fields.SetSysUtil != nil {
				tt.fields.SetSysUtil(helper)
			}
			oldHostRunRootDir := system.Conf.RunRootDir
			system.Conf.RunRootDir = helper.TempDir
			s := tt.fields.NewRundExtendedStats(helper)
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			socketPath := system.GenRundShimSocketPath(sandboxID)
			err := os.MkdirAll(filepath.Dir(socketPath), 0766)
			assert.NoError(t, err)
			l, err := net.Listen("unix", socketPath)
			assert.NoError(t, err, "listen error")
			go s.Serve(ctx, l)
			defer func() {
				err := s.Close()
				assert.NoError(t, err)
				system.Conf.RunRootDir = oldHostRunRootDir
			}()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			statesInformer := mock_statesinformer.NewMockStatesInformer(ctrl)
			metricCacheCfg := metriccache.NewDefaultConfig()
			metricCacheCfg.TSDBEnablePromMetrics = false
			metricCacheCfg.TSDBPath = helper.TempDir
			metricCache, err := metriccache.NewMetricCache(metricCacheCfg)
			assert.NoError(t, err)
			statesInformer.EXPECT().HasSynced().Return(true).AnyTimes()
			statesInformer.EXPECT().GetAllPods().Return(tt.fields.getPodMetas).Times(1)

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
			c := collector.(*rundResourceCollector)
			tt.fields.initPodLastStat(c.lastPodCPUStat)
			tt.fields.initContainerLastTick(c.lastContainerCPUStat)

			assert.NotPanics(t, func() {
				c.collectRundPodsResUsed()
			})

			if tt.want.checkPodResourceMetric != nil {
				tt.want.checkPodResourceMetric(t, metricCache)
			}
			if tt.want.checkContainerResourceMetric != nil {
				tt.want.checkContainerResourceMetric(t, metricCache)
			}

			// check share state
			podsCPUUsageByCollector, podsMemoryUsageByCollector := c.sharedState.GetPodsUsageByCollector()
			podsCPUUsage, cpuExist := podsCPUUsageByCollector[CollectorName]
			assert.True(t, cpuExist)
			assert.Equal(t, podsCPUUsage.Value, tt.want.allCPU)
			podsMemoryUsage, memoryExist := podsMemoryUsageByCollector[CollectorName]
			assert.True(t, memoryExist)
			assert.Equal(t, podsMemoryUsage.Value, tt.want.allMemory)
		})
	}
}

func testGetPodMetric(t *testing.T, metricCache metriccache.MetricCache, podUID string, start, end time.Time) {
	queryParam := metriccache.QueryParam{
		Start:     &start,
		End:       &end,
		Aggregate: metriccache.AggregationTypeAVG,
	}

	querier, err := metricCache.Querier(*queryParam.Start, *queryParam.End)
	assert.NoError(t, err)

	cpuAggregateResult, err := doQuery(querier, metriccache.PodCPUUsageMetric, metriccache.MetricPropertiesFunc.Pod(podUID))
	assert.NoError(t, err)
	cpuV, err := cpuAggregateResult.Value(queryParam.Aggregate)
	assert.NoError(t, err)
	assert.NotNil(t, cpuV, cpuV)

	memAggregateResult, err := doQuery(querier, metriccache.PodMemUsageMetric, metriccache.MetricPropertiesFunc.Pod(podUID))
	assert.NoError(t, err)

	memV, err := memAggregateResult.Value(queryParam.Aggregate)
	assert.NoError(t, err)
	assert.NotNil(t, memV, memV)
}

func testGetContainerMetric(t *testing.T, metricCache metriccache.MetricCache, containerID string, start, end time.Time) {
	queryParam := metriccache.QueryParam{
		Start:     &start,
		End:       &end,
		Aggregate: metriccache.AggregationTypeAVG,
	}
	querier, err := metricCache.Querier(*queryParam.Start, *queryParam.End)
	assert.NoError(t, err)

	cpuAggregateResult, err := doQuery(querier, metriccache.ContainerCPUUsageMetric, metriccache.MetricPropertiesFunc.Container(containerID))
	assert.NoError(t, err)
	_, err = cpuAggregateResult.Value(queryParam.Aggregate)
	assert.NoError(t, err)

	memAggregateResult, err := doQuery(querier, metriccache.ContainerMemUsageMetric, metriccache.MetricPropertiesFunc.Container(containerID))
	assert.NoError(t, err)

	_, err = memAggregateResult.Value(queryParam.Aggregate)
	assert.NoError(t, err)
}

func doQuery(querier metriccache.Querier, resource metriccache.MetricResource, properties map[metriccache.MetricProperty]string) (metriccache.AggregateResult, error) {
	queryMeta, err := resource.BuildQueryMeta(properties)
	if err != nil {
		return nil, err
	}

	aggregateResult := metriccache.DefaultAggregateResultFactory.New(queryMeta)
	if err := querier.Query(queryMeta, nil, aggregateResult); err != nil {
		return nil, err
	}

	return aggregateResult, nil
}
