/*
Copyright 2021.

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

package recommender

import (
	"context"
	"flag"
	"os"
	"time"

	"k8s.io/utils/pointer"

	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conf "gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/config"

	v1 "k8s.io/api/core/v1"

	"k8s.io/client-go/rest"
	"k8s.io/klog"

	"gitlab.alibaba-inc.com/cos/recommender/pkg/common"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/metrics"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/clientset/checkpoint"
	cluster "gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/clientset/cluster"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/logic"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/model"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/util"
	recv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	unifiedclientset "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned"
)

var (
	checkpointsWriteTimeout = flag.Duration("checkpoints-timeout", time.Minute, `Timeout for writing checkpoints since the start of the recommender's main loop`)
	minCheckpointsPerRun    = flag.Int("min-checkpoints", 10, "Minimum number of checkpoints to write per recommender's main loop")
	memorySaver             = flag.Bool("memory-saver", true, `If true, only track pods which have an associated Recommendation`)
)

type Recommender interface {
	// RunOnce 在一个周期内执行以下6步：
	// 1）LoadRecommendation： 将 Recommendation 加载至 ClusterState
	// 2）LoadPods： 将 Pods 对象加载至 ClusterState
	// 3）LoadRealTimeMetrics: 将 metrics server 聚合后的容器资源使用情况加载至ClusterState
	// 4）UpdateRecommendations：以半衰指数直方图作为输入，计算 cpu/memory 资源推荐值，并更新到 RecommendationStatus 字段
	// 5）MaintainCheckpoints：将推荐模型使用的半指数直方图备份至 RecommendationCheckpoint
	// 6）GarbageCollect：回收内存中无用数据，保证Recommender不占用过多的内存和推荐值的新鲜度
	RunOnce()

	// GetClusterState returns ClusterState used by Recommender
	GetClusterState() *model.ClusterState
	// GetClusterStateFeeder returns ClusterStateFeeder used by Recommender
	GetClusterStateFeeder() cluster.ClusterStateFeeder
	// UpdateRecommendations  computes recommendations and sends VPAs status updates to API Server
	UpdateRecommendations()
	// MaintainCheckpoints stores current checkpoints in API Server and garbage collect old ones
	// MaintainCheckpoints writes at least minCheckpoints if there are more checkpoints to write.
	// Checkpoints are written until ctx permits or all checkpoints are written.
	MaintainCheckpoints(ctx context.Context, minCheckpoints int)
	// GarbageCollect removes old AggregateCollectionStates
	GarbageCollect()
}

// RecommendationReconciler reconciles a Recommendation object
type recommender struct {
	kubeClient                    client.Client
	scheme                        *runtime.Scheme
	clusterState                  *model.ClusterState
	clusterStateFeeder            cluster.ClusterStateFeeder
	checkpointWriter              checkpoint.CheckpointWriter
	checkpointsGCInterval         time.Duration
	lastCheckpointGC              time.Time
	podResourceRecommender        logic.PodResourceRecommender
	useCheckpoints                bool
	lastAggregateContainerStateGC time.Time
}

func (r *recommender) GetClusterState() *model.ClusterState {
	return r.clusterState
}

func (r *recommender) GetClusterStateFeeder() cluster.ClusterStateFeeder {
	return r.clusterStateFeeder
}

// GetContainerNameToAggregateStateMap returns ContainerNameToAggregateStateMap for pods.
func GetContainerNameToAggregateStateMap(rec *model.Recommendation) model.ContainerNameToAggregateStateMap {
	containerNameToAggregateStateMap := rec.AggregateStateByContainerName()
	filteredContainerNameToAggregateStateMap := make(model.ContainerNameToAggregateStateMap)

	for containerName, aggregatedContainerState := range containerNameToAggregateStateMap {
		if aggregatedContainerState.TotalSamplesCount > 0 {
			filteredContainerNameToAggregateStateMap[containerName] = aggregatedContainerState
		}
	}
	return filteredContainerNameToAggregateStateMap
}

// UpdateRecommendations Updates Recommendation CRD objects' statuses.
func (r *recommender) UpdateRecommendations() {
	cnt := metrics.NewObjectCounter()
	defer cnt.Observe()

	metrics.ResetRecordRecommendation()
	metrics.RecordRecommendationMetricEnabled()
	for _, observedRec := range r.clusterState.ObservedRecommendations {
		key := model.RecommendationID{
			Namespace:          observedRec.Namespace,
			RecommendationName: observedRec.Name,
		}

		rec, found := r.clusterState.Recommendations[key]
		if !found {
			continue
		}
		// 按照 container name 对 metrics 进行聚合， 得到同一 workload 下相同 container name 的直方图聚合
		resources := r.podResourceRecommender.GetRecommendedPodResources(GetContainerNameToAggregateStateMap(rec))
		had := rec.HasRecommendation()
		rec.UpdateRecommendation(getUncappedRecommendation(rec.ID, resources))

		if rec.HasRecommendation() && !had {
			metrics.ObserveRecommendationLatency(rec.Created)
		}
		hasMatchingPods := rec.PodCount > 0

		rec.UpdateConditions(hasMatchingPods)
		if err := r.clusterState.RecordRecommendation(rec, time.Now()); err != nil {
			klog.Warningf("%v", err)
			klog.V(4).Infof("Recommendation dump")
			klog.V(4).Infof("%+v", rec)
			klog.V(4).Infof("HasMatchingPods: %v", hasMatchingPods)
			klog.V(4).Infof("PodCount: %v", rec.PodCount)
			pods := r.clusterState.GetMatchingPods(rec)
			klog.V(4).Infof("MatchingPods: %+v", pods)
			if len(pods) != rec.PodCount {
				klog.Errorf("ClusterState pod count and matching pods disagree for Recommendation %v/%v", rec.ID.Namespace, rec.ID.RecommendationName)
			}
		}
		cnt.Add(rec.HasRecommendation(), rec.HasMatchedPods(), rec.Conditions.ConditionActive(recv1alpha1.ConfigUnsupported))
		newStatus := rec.AsStatus()
		metrics.RecordRecommendationTarget(observedRec, newStatus)

		newWorkloadStatus := r.generateWorkloadStatus(key, observedRec, newStatus)
		if err := util.UpdateWorkloadStatusIfNeeded(r.kubeClient, observedRec, newWorkloadStatus); err != nil {
			klog.Errorf("Update Recommendation %v failed, Reason: %+v", rec.ID.RecommendationName, err)
		} else {
			klog.V(5).Infof("Update Recommendation %v workload annotation success, detail %+v", rec.ID.RecommendationName, newWorkloadStatus)
		}
		if err := util.UpdateStatusRecommendationIfNeeded(r.kubeClient, observedRec, newStatus); err != nil {
			klog.Errorf("Cannot update Recommendation Status %v. Reason: %+v", rec.ID.RecommendationName, err)
		} else {
			klog.V(5).Infof("Update Recommendation Status %v success, detail %+v", rec.ID.RecommendationName, newStatus)
		}
	}
}

func (r *recommender) generateWorkloadStatus(key model.RecommendationID,
	observedRec *recv1alpha1.Recommendation,
	newStatus *recv1alpha1.RecommendationStatus) *common.RecommenderWorkloadStatus {
	workloadInfo, workloadExist := r.clusterState.OriginWorkloads[key]
	if !workloadExist {
		return &common.RecommenderWorkloadStatus{
			Status:             common.RecommendationStatusNotExist,
			CPUAdjustDegree:    pointer.Float64Ptr(0),
			MemoryAdjustDegree: pointer.Float64Ptr(0),
		}
	}
	workloadReq := workloadInfo.GetRequestResources()
	marginRatio, err := common.GetRecommederSafetyMarginRatio(observedRec.Annotations)
	if err != nil {
		klog.Warningf("get safety margin from %v failed, reason %v", key.RecommendationName, err)
	}

	cpuDegree, memoryDegree := common.CalcWorkloadAdjustDegree(workloadInfo.ContainerRequests,
		newStatus.Recommendation, marginRatio)

	status := &common.RecommenderWorkloadStatus{
		Status:             common.GenerateRecommendationStatus(observedRec),
		OriginRequest:      *workloadReq,
		CPUAdjustDegree:    &cpuDegree,
		MemoryAdjustDegree: &memoryDegree,
	}
	return status
}

// getUncappedRecommendation creates a kubeClient based on recommended pod
// resources, setting the UncappedTarget to the calculated recommended target
// and if necessary, capping the Target(p90), p60, p95, p99, max , avg  according
// to the ResourcePolicy.
func getUncappedRecommendation(recID model.RecommendationID, resources logic.RecommendedPodResources) *recv1alpha1.RecommendedPodResources {
	containerResources := make([]recv1alpha1.RecommendedContainerResources, 0, len(resources))
	for containerName, res := range resources {
		containerResources = append(containerResources, recv1alpha1.RecommendedContainerResources{
			ContainerName: containerName,
			Target:        model.ResourceAsResourceList(res.Target),
			OriginalTarget: map[string]v1.ResourceList{
				common.StatisticsKeyAvg: model.ResourceAsResourceList(res.Avg),
				common.StatisticsKeyP60: model.ResourceAsResourceList(res.P60),
				common.StatisticsKeyP95: model.ResourceAsResourceList(res.P95),
				common.StatisticsKeyP99: model.ResourceAsResourceList(res.P99),
				common.StatisticsKeyMax: model.ResourceAsResourceList(res.Max),
			},
		})
	}
	recommendation := &recv1alpha1.RecommendedPodResources{ContainerRecommendations: containerResources}

	return recommendation
}

func (r *recommender) MaintainCheckpoints(ctx context.Context, minCheckpointsPerRun int) {
	now := time.Now()
	if r.useCheckpoints {
		if err := r.checkpointWriter.StoreCheckpoints(ctx, now, minCheckpointsPerRun); err != nil {
			klog.Warningf("Failed to store checkpoints. Reason: %+v", err)
		}
		if time.Since(r.lastCheckpointGC) > r.checkpointsGCInterval {
			r.lastCheckpointGC = now
			r.clusterStateFeeder.GarbageCollectCheckpoints()
		}
	}
}

// GarbageCollect per hour by default
func (r *recommender) GarbageCollect() {
	gcTime := time.Now()
	aggregateContainerStateGCInterval := conf.GetGCConfig().AggregateContainerStateGCInterval
	gcInterval := aggregateContainerStateGCInterval
	if gcTime.Sub(r.lastAggregateContainerStateGC) > gcInterval {
		r.clusterState.GarbageRecommendation(gcTime, r.kubeClient)
		r.clusterState.DeleteRecommendationPodNotExist(r.kubeClient, r.scheme)
		r.lastAggregateContainerStateGC = gcTime
	} else {
		klog.V(5).Infof("Garbage collect skipped because time is not ready, last %v, interval %v",
			r.lastAggregateContainerStateGC.String(), gcInterval.String())
	}
}

func (r *recommender) RunOnce() {
	timer := metrics.NewExecTimer()
	defer timer.ObserveTotal()

	ctx, cancelFunc := context.WithDeadline(context.Background(), time.Now().Add(*checkpointsWriteTimeout))
	defer cancelFunc()

	klog.Infof("Recommender Run")

	r.clusterStateFeeder.LoadRecommendations()
	timer.ObserveStep("LoadRecommendations")

	r.clusterStateFeeder.LoadPods()
	timer.ObserveStep("LoadPods")

	r.clusterStateFeeder.LoadRealTimeMetrics()
	timer.ObserveStep("LoadMetrics")
	klog.Infof("ClusterState is tracking %v PodStates and %v Recommendations", len(r.clusterState.Pods), len(r.clusterState.Recommendations))

	r.UpdateRecommendations()
	timer.ObserveStep("UpdateRecommendations")

	r.MaintainCheckpoints(ctx, *minCheckpointsPerRun)
	timer.ObserveStep("MaintainCheckpoints")

	r.GarbageCollect()
	timer.ObserveStep("GarbageCollect")
	klog.Infof("ClusterState is tracking %d aggregated container states", r.clusterState.StateMapSize())
}

// RecommenderFactory makes instances of Recommender.
type RecommenderFactory struct {
	Client       client.Client
	Scheme       *runtime.Scheme
	ClusterState *model.ClusterState

	ClusterStateFeeder     cluster.ClusterStateFeeder
	CheckpointWriter       checkpoint.CheckpointWriter
	PodResourceRecommender logic.PodResourceRecommender

	CheckpointsGCInterval time.Duration
	UseCheckpoints        bool
}

// Make creates a new recommender instance,
// which can be run in order to provide continuous resource recommendations for containers.
func (c RecommenderFactory) Make() Recommender {
	recommender := &recommender{
		kubeClient:                    c.Client,
		scheme:                        c.Scheme,
		clusterState:                  c.ClusterState,
		clusterStateFeeder:            c.ClusterStateFeeder,
		checkpointWriter:              c.CheckpointWriter,
		checkpointsGCInterval:         c.CheckpointsGCInterval,
		useCheckpoints:                c.UseCheckpoints,
		podResourceRecommender:        c.PodResourceRecommender,
		lastAggregateContainerStateGC: time.Now(),
		lastCheckpointGC:              time.Now(),
	}
	klog.V(3).Infof("New Recommender created %+v", recommender)
	return recommender
}

// NewRecommender creates a new recommender instance.
// Dependencies are created automatically.
// Deprecated; use RecommenderFactory instead.
func NewRecommender(config *rest.Config, checkpointsGCInterval time.Duration, useCheckpoints bool, namespace string,
	supportKruise bool, scheme *runtime.Scheme) Recommender {
	clusterState := model.NewClusterState()
	opt := client.Options{
		Scheme: scheme,
	}
	kubeClient, err := client.New(config, opt)
	if err != nil {
		klog.Errorf("unable to create generate kubeClient, err : %v", err)
		os.Exit(1)
	}
	return RecommenderFactory{
		Client:                 kubeClient,
		Scheme:                 scheme,
		ClusterState:           clusterState,
		ClusterStateFeeder:     cluster.NewClusterStateFeeder(config, clusterState, *memorySaver, namespace, supportKruise),
		CheckpointWriter:       checkpoint.NewCheckpointWriter(clusterState, unifiedclientset.NewForConfigOrDie(config).AutoscalingV1alpha1()),
		PodResourceRecommender: logic.CreatePodResourceRecommender(),
		CheckpointsGCInterval:  checkpointsGCInterval,
		UseCheckpoints:         useCheckpoints,
	}.Make()
}
